// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package detector

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// ProjectType represents a detected project type
type ProjectType struct {
	Type     string
	Subtype  string
	File     string
	Priority int
}

// String returns the full project type identifier
func (pt *ProjectType) String() string {
	if pt.Subtype != "" {
		return fmt.Sprintf("%s-%s", pt.Type, pt.Subtype)
	}
	return pt.Type
}

// DetectionRule defines a rule for detecting a project type
type DetectionRule struct {
	Type     string
	Subtype  string
	Files    []string // Files that must exist
	Priority int      // Higher priority types are checked first
}

// Common detection rules based on file presence
var detectionRules = []DetectionRule{
	// Python - Modern
	{Type: "python", Subtype: "modern", Files: []string{"pyproject.toml"}, Priority: 2},
	{Type: "python", Subtype: "legacy", Files: []string{"setup.py"}, Priority: 9},
	{Type: "python", Subtype: "setup-cfg", Files: []string{"setup.cfg"}, Priority: 9},

	// JavaScript/Node.js
	{Type: "javascript", Subtype: "npm", Files: []string{"package.json"}, Priority: 1},

	// Java
	{Type: "java", Subtype: "maven", Files: []string{"pom.xml"}, Priority: 3},
	{Type: "java", Subtype: "gradle", Files: []string{"build.gradle"}, Priority: 4},
	{Type: "java", Subtype: "gradle-kts", Files: []string{"build.gradle.kts"}, Priority: 4},

	// .NET/C#
	{Type: "csharp", Subtype: "project", Files: []string{"*.csproj"}, Priority: 5},
	{Type: "csharp", Subtype: "solution", Files: []string{"*.sln"}, Priority: 5},
	{Type: "csharp", Subtype: "props", Files: []string{"*.props"}, Priority: 6},

	// Go
	{Type: "go", Subtype: "module", Files: []string{"go.mod"}, Priority: 6},

	// Rust
	{Type: "rust", Subtype: "cargo", Files: []string{"Cargo.toml"}, Priority: 11},

	// Ruby
	{Type: "ruby", Subtype: "gemspec", Files: []string{"*.gemspec"}, Priority: 8},
	{Type: "ruby", Subtype: "bundler", Files: []string{"Gemfile"}, Priority: 8},

	// PHP
	{Type: "php", Subtype: "composer", Files: []string{"composer.json"}, Priority: 7},

	// Swift
	{Type: "swift", Subtype: "package", Files: []string{"Package.swift"}, Priority: 12},

	// Dart/Flutter
	{Type: "dart", Subtype: "flutter", Files: []string{"pubspec.yaml"}, Priority: 13},

	// Elixir
	{Type: "elixir", Subtype: "mix", Files: []string{"mix.exs"}, Priority: 15},

	// Scala
	{Type: "scala", Subtype: "sbt", Files: []string{"build.sbt"}, Priority: 16},

	// Haskell
	{Type: "haskell", Subtype: "cabal", Files: []string{"*.cabal"}, Priority: 17},

	// Julia
	{Type: "julia", Subtype: "project", Files: []string{"Project.toml"}, Priority: 18},

	// C/C++
	{Type: "c", Subtype: "cmake", Files: []string{"CMakeLists.txt"}, Priority: 14},
	{Type: "c", Subtype: "qmake", Files: []string{".qmake.conf"}, Priority: 14},
	{Type: "c", Subtype: "autoconf", Files: []string{"configure.ac"}, Priority: 8},
	{Type: "c", Subtype: "autoconf-legacy", Files: []string{"configure.in"}, Priority: 9},
	{Type: "c", Subtype: "meson", Files: []string{"meson.build"}, Priority: 14},

	// Kotlin (check before java-gradle-kts since build.gradle.kts could be either)
	{Type: "kotlin", Subtype: "gradle", Files: []string{"build.gradle.kts"}, Priority: 3},

	// TypeScript
	{Type: "typescript", Subtype: "npm", Files: []string{"package.json", "tsconfig.json"}, Priority: 1},

	// Clojure
	{Type: "clojure", Subtype: "leiningen", Files: []string{"project.clj"}, Priority: 19},
	{Type: "clojure", Subtype: "deps", Files: []string{"deps.edn"}, Priority: 19},

	// Erlang
	{Type: "erlang", Subtype: "rebar", Files: []string{"rebar.config"}, Priority: 20},

	// Perl
	{Type: "perl", Subtype: "cpan", Files: []string{"Makefile.PL"}, Priority: 21},
	{Type: "perl", Subtype: "module-build", Files: []string{"Build.PL"}, Priority: 21},

	// R
	{Type: "r", Subtype: "package", Files: []string{"DESCRIPTION"}, Priority: 22},

	// Docker
	{Type: "docker", Subtype: "", Files: []string{"Dockerfile"}, Priority: 23},

	// Helm
	{Type: "helm", Subtype: "chart", Files: []string{"Chart.yaml"}, Priority: 24},

	// Terraform/OpenTofu
	{Type: "terraform", Subtype: "module", Files: []string{"main.tf"}, Priority: 25},
	{Type: "terraform", Subtype: "module", Files: []string{"variables.tf"}, Priority: 25},
	{Type: "terraform", Subtype: "module", Files: []string{"*.tf"}, Priority: 26},
}

// DetectProjectType attempts to detect the project type at the given path
func DetectProjectType(projectPath string) (string, error) {
	// Sort rules by priority (higher priority first)
	sortedRules := make([]DetectionRule, len(detectionRules))
	copy(sortedRules, detectionRules)
	sort.Slice(sortedRules, func(i, j int) bool {
		return sortedRules[i].Priority < sortedRules[j].Priority
	})

	// Check each rule
	for _, rule := range sortedRules {
		if matchesRule(projectPath, rule) {
			pt := &ProjectType{
				Type:     rule.Type,
				Subtype:  rule.Subtype,
				Priority: rule.Priority,
			}
			return pt.String(), nil
		}
	}

	return "", fmt.Errorf("could not detect project type in %s", projectPath)
}

// DetectAllProjectTypes returns all matching project types (useful for monorepos)
func DetectAllProjectTypes(projectPath string) ([]string, error) {
	var projectTypes []string
	var detected []*ProjectType

	// Sort rules by priority
	sortedRules := make([]DetectionRule, len(detectionRules))
	copy(sortedRules, detectionRules)
	sort.Slice(sortedRules, func(i, j int) bool {
		return sortedRules[i].Priority < sortedRules[j].Priority
	})

	// Check each rule
	for _, rule := range sortedRules {
		if matchesRule(projectPath, rule) {
			pt := &ProjectType{
				Type:     rule.Type,
				Subtype:  rule.Subtype,
				File:     rule.Files[0],
				Priority: rule.Priority,
			}
			detected = append(detected, pt)
		}
	}

	if len(detected) == 0 {
		return nil, fmt.Errorf("could not detect any project types in %s", projectPath)
	}

	// Convert to strings
	for _, pt := range detected {
		projectTypes = append(projectTypes, pt.String())
	}

	return projectTypes, nil
}

// matchesRule checks if the given path matches the detection rule
func matchesRule(projectPath string, rule DetectionRule) bool {
	// All files must exist for the rule to match
	for _, filePattern := range rule.Files {
		if !fileExists(projectPath, filePattern) {
			return false
		}
	}
	return true
}

// fileExists checks if a file or pattern exists in the given path
func fileExists(projectPath, pattern string) bool {
	// Check if pattern contains wildcards
	if containsWildcard(pattern) {
		matches, err := filepath.Glob(filepath.Join(projectPath, pattern))
		return err == nil && len(matches) > 0
	}

	// Direct file check
	fullPath := filepath.Join(projectPath, pattern)
	_, err := os.Stat(fullPath)
	return err == nil
}

// containsWildcard checks if a pattern contains wildcard characters
func containsWildcard(pattern string) bool {
	return filepath.Base(pattern) != pattern ||
		filepath.Dir(pattern) != "." ||
		(len(pattern) > 0 && (pattern[0] == '*' || pattern[0] == '?'))
}

// GetDetectionRules returns all detection rules (useful for testing/debugging)
func GetDetectionRules() []DetectionRule {
	return detectionRules
}

// AddDetectionRule adds a custom detection rule (useful for extensions)
func AddDetectionRule(rule DetectionRule) {
	detectionRules = append(detectionRules, rule)
}
