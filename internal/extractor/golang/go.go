// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package golang

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/lfreleng-actions/build-metadata-action/internal/extractor"
)

// Extractor extracts metadata from Go projects
type Extractor struct {
	extractor.BaseExtractor
}

// NewExtractor creates a new Go extractor
func NewExtractor() *Extractor {
	return &Extractor{
		BaseExtractor: extractor.NewBaseExtractor("go-module", 1),
	}
}

// GoMod represents the structure of a go.mod file
type GoMod struct {
	Module       string
	GoVersion    string
	Require      []Dependency
	Replace      []Replace
	Exclude      []string
	Retract      []string
	Toolchain    string
	Dependencies map[string]string // module -> version
}

// Dependency represents a Go module dependency
type Dependency struct {
	Module   string
	Version  string
	Indirect bool
}

// Replace represents a module replacement directive
type Replace struct {
	Old string
	New string
}

// Extract retrieves metadata from a Go project
func (e *Extractor) Extract(projectPath string) (*extractor.ProjectMetadata, error) {
	metadata := &extractor.ProjectMetadata{
		LanguageSpecific: make(map[string]interface{}),
	}

	// Try go.mod file
	goModPath := filepath.Join(projectPath, "go.mod")
	if _, err := os.Stat(goModPath); err == nil {
		if err := e.extractFromGoMod(goModPath, metadata); err != nil {
			return nil, err
		}
		return metadata, nil
	}

	return nil, fmt.Errorf("no go.mod file found in %s", projectPath)
}

// extractFromGoMod extracts metadata from go.mod file
func (e *Extractor) extractFromGoMod(path string, metadata *extractor.ProjectMetadata) error {
	goMod, err := parseGoMod(path)
	if err != nil {
		return fmt.Errorf("failed to parse go.mod: %w", err)
	}

	// Extract module path (this is the project name/import path)
	metadata.Name = goMod.Module
	metadata.VersionSource = "go.mod"

	// Extract the base name from the module path for a friendlier name
	parts := strings.Split(goMod.Module, "/")
	if baseName := extractBaseNameFromModulePath(parts); baseName != "" {
		metadata.LanguageSpecific["base_name"] = baseName
	}

	// Extract repository URL from module path
	if strings.HasPrefix(goMod.Module, "github.com/") ||
		strings.HasPrefix(goMod.Module, "gitlab.com/") ||
		strings.HasPrefix(goMod.Module, "bitbucket.org/") {
		metadata.Repository = "https://" + goMod.Module
		metadata.Homepage = "https://" + goMod.Module
	}

	// Go-specific metadata
	metadata.LanguageSpecific["module_path"] = goMod.Module
	metadata.LanguageSpecific["go_version"] = goMod.GoVersion
	metadata.LanguageSpecific["metadata_source"] = "go.mod"

	if goMod.Toolchain != "" {
		metadata.LanguageSpecific["toolchain"] = goMod.Toolchain
	}

	// Extract dependencies
	if len(goMod.Require) > 0 {
		directDeps := []string{}
		indirectDeps := []string{}
		depMap := make(map[string]string)

		for _, dep := range goMod.Require {
			depMap[dep.Module] = dep.Version
			if dep.Indirect {
				indirectDeps = append(indirectDeps, fmt.Sprintf("%s@%s", dep.Module, dep.Version))
			} else {
				directDeps = append(directDeps, fmt.Sprintf("%s@%s", dep.Module, dep.Version))
			}
		}

		metadata.LanguageSpecific["dependencies"] = directDeps
		metadata.LanguageSpecific["indirect_dependencies"] = indirectDeps
		metadata.LanguageSpecific["dependency_count"] = len(directDeps)
		metadata.LanguageSpecific["total_dependency_count"] = len(goMod.Require)
		metadata.LanguageSpecific["dependency_map"] = depMap
	}

	// Extract replace directives
	if len(goMod.Replace) > 0 {
		replaces := make([]map[string]string, 0, len(goMod.Replace))
		for _, r := range goMod.Replace {
			replaces = append(replaces, map[string]string{
				"old": r.Old,
				"new": r.New,
			})
		}
		metadata.LanguageSpecific["replace_directives"] = replaces
		metadata.LanguageSpecific["replace_count"] = len(goMod.Replace)
	}

	// Extract exclude directives
	if len(goMod.Exclude) > 0 {
		metadata.LanguageSpecific["exclude_directives"] = goMod.Exclude
		metadata.LanguageSpecific["exclude_count"] = len(goMod.Exclude)
	}

	// Extract retract directives
	if len(goMod.Retract) > 0 {
		metadata.LanguageSpecific["retract_directives"] = goMod.Retract
		metadata.LanguageSpecific["retract_count"] = len(goMod.Retract)
	}

	// Detect common Go frameworks and tools from dependencies
	frameworks := detectGoFrameworks(goMod.Require)
	if len(frameworks) > 0 {
		metadata.LanguageSpecific["frameworks"] = frameworks
	}

	// Generate Go version matrix
	if goMod.GoVersion != "" {
		matrix := generateGoVersionMatrix(goMod.GoVersion)
		if len(matrix) > 0 {
			metadata.LanguageSpecific["go_version_matrix"] = matrix

			// Convert to JSON for easy use in GitHub Actions
			matrixJSON := fmt.Sprintf(`{"go-version": [%s]}`,
				strings.Join(quoteStrings(matrix), ", "))
			metadata.LanguageSpecific["matrix_json"] = matrixJSON
		}
	}

	// Try to extract version from common patterns
	version := extractVersionFromProject(filepath.Dir(path))
	if version != "" {
		// Validate that the version is not just a module path component
		// Reject versions that are just "v" followed by only digits (e.g., "v2", "v10", "v100")
		// Real semantic versions have dots: v1.2.3, v2.0.0, etc.
		majorVerOnly := regexp.MustCompile(`^v\d+$`)
		if majorVerOnly.MatchString(version) {
			// This is just a major version indicator, not a real version
			// Don't use it - will fall back to git tags
			version = ""
		}

		if version != "" {
			metadata.Version = version
			metadata.VersionSource = "version file or git tag"
		}
	}

	return nil
}

// parseGoMod parses a go.mod file and returns its structure
func parseGoMod(path string) (*GoMod, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	goMod := &GoMod{
		Dependencies: make(map[string]string),
	}

	scanner := bufio.NewScanner(file)
	var inBlock string
	var blockLines []string

	moduleRe := regexp.MustCompile(`^module\s+(.+)$`)
	goVersionRe := regexp.MustCompile(`^go\s+(\d+\.\d+(?:\.\d+)?)$`)
	toolchainRe := regexp.MustCompile(`^toolchain\s+(.+)$`)
	requireRe := regexp.MustCompile(`^require\s+(.+)$`)
	replaceRe := regexp.MustCompile(`^replace\s+(.+)$`)
	excludeRe := regexp.MustCompile(`^exclude\s+(.+)$`)
	retractRe := regexp.MustCompile(`^retract\s+(.+)$`)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}

		// Handle block closing
		if inBlock != "" && line == ")" {
			// Process accumulated block lines
			switch inBlock {
			case "require":
				goMod.Require = append(goMod.Require, parseRequireBlock(blockLines)...)
			case "replace":
				goMod.Replace = append(goMod.Replace, parseReplaceBlock(blockLines)...)
			case "exclude":
				goMod.Exclude = append(goMod.Exclude, parseExcludeBlock(blockLines)...)
			case "retract":
				goMod.Retract = append(goMod.Retract, parseRetractBlock(blockLines)...)
			}
			inBlock = ""
			blockLines = nil
			continue
		}

		// If in a block, accumulate lines
		if inBlock != "" {
			blockLines = append(blockLines, line)
			continue
		}

		// Parse single-line directives
		if matches := moduleRe.FindStringSubmatch(line); len(matches) > 1 {
			goMod.Module = strings.TrimSpace(matches[1])
			continue
		}

		if matches := goVersionRe.FindStringSubmatch(line); len(matches) > 1 {
			goMod.GoVersion = strings.TrimSpace(matches[1])
			continue
		}

		if matches := toolchainRe.FindStringSubmatch(line); len(matches) > 1 {
			goMod.Toolchain = strings.TrimSpace(matches[1])
			continue
		}

		// Check for block start
		if matches := requireRe.FindStringSubmatch(line); len(matches) > 1 {
			rest := strings.TrimSpace(matches[1])
			if rest == "(" {
				inBlock = "require"
				blockLines = []string{}
			} else {
				// Single-line require
				goMod.Require = append(goMod.Require, parseRequireLine(rest))
			}
			continue
		}

		if matches := replaceRe.FindStringSubmatch(line); len(matches) > 1 {
			rest := strings.TrimSpace(matches[1])
			if rest == "(" {
				inBlock = "replace"
				blockLines = []string{}
			} else {
				// Single-line replace
				goMod.Replace = append(goMod.Replace, parseReplaceLine(rest))
			}
			continue
		}

		if matches := excludeRe.FindStringSubmatch(line); len(matches) > 1 {
			rest := strings.TrimSpace(matches[1])
			if rest == "(" {
				inBlock = "exclude"
				blockLines = []string{}
			} else {
				// Single-line exclude
				goMod.Exclude = append(goMod.Exclude, rest)
			}
			continue
		}

		if matches := retractRe.FindStringSubmatch(line); len(matches) > 1 {
			rest := strings.TrimSpace(matches[1])
			if rest == "(" {
				inBlock = "retract"
				blockLines = []string{}
			} else {
				// Single-line retract
				goMod.Retract = append(goMod.Retract, rest)
			}
			continue
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// Build dependency map
	for _, dep := range goMod.Require {
		goMod.Dependencies[dep.Module] = dep.Version
	}

	return goMod, nil
}

// parseRequireBlock parses a block of require statements
func parseRequireBlock(lines []string) []Dependency {
	deps := []Dependency{}
	for _, line := range lines {
		if dep := parseRequireLine(line); dep.Module != "" {
			deps = append(deps, dep)
		}
	}
	return deps
}

// parseRequireLine parses a single require line
func parseRequireLine(line string) Dependency {
	// Remove inline comments
	if idx := strings.Index(line, "//"); idx != -1 {
		comment := strings.TrimSpace(line[idx+2:])
		line = strings.TrimSpace(line[:idx])

		// Check if it's an indirect dependency
		indirect := strings.Contains(comment, "indirect")

		parts := strings.Fields(line)
		if len(parts) >= 2 {
			return Dependency{
				Module:   parts[0],
				Version:  parts[1],
				Indirect: indirect,
			}
		}
	} else {
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			return Dependency{
				Module:  parts[0],
				Version: parts[1],
			}
		}
	}
	return Dependency{}
}

// parseReplaceBlock parses a block of replace statements
func parseReplaceBlock(lines []string) []Replace {
	replaces := []Replace{}
	for _, line := range lines {
		if r := parseReplaceLine(line); r.Old != "" {
			replaces = append(replaces, r)
		}
	}
	return replaces
}

// parseReplaceLine parses a single replace line
func parseReplaceLine(line string) Replace {
	// Format: old => new or old version => new version
	parts := strings.Split(line, "=>")
	if len(parts) != 2 {
		return Replace{}
	}

	return Replace{
		Old: strings.TrimSpace(parts[0]),
		New: strings.TrimSpace(parts[1]),
	}
}

// parseExcludeBlock parses a block of exclude statements
func parseExcludeBlock(lines []string) []string {
	excludes := []string{}
	for _, line := range lines {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			excludes = append(excludes, trimmed)
		}
	}
	return excludes
}

// parseRetractBlock parses a block of retract statements
func parseRetractBlock(lines []string) []string {
	retracts := []string{}
	for _, line := range lines {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			retracts = append(retracts, trimmed)
		}
	}
	return retracts
}

// detectGoFrameworks detects common Go frameworks from dependencies
func detectGoFrameworks(deps []Dependency) []string {
	frameworks := []string{}
	frameworkMap := map[string]string{
		"github.com/gin-gonic/gin":            "Gin (Web Framework)",
		"github.com/labstack/echo":            "Echo (Web Framework)",
		"github.com/gofiber/fiber":            "Fiber (Web Framework)",
		"github.com/gorilla/mux":              "Gorilla Mux (Router)",
		"github.com/go-chi/chi":               "Chi (Router)",
		"gorm.io/gorm":                        "GORM (ORM)",
		"github.com/jmoiron/sqlx":             "sqlx (SQL Extensions)",
		"github.com/spf13/cobra":              "Cobra (CLI)",
		"github.com/urfave/cli":               "CLI (CLI Framework)",
		"github.com/stretchr/testify":         "Testify (Testing)",
		"github.com/sirupsen/logrus":          "Logrus (Logging)",
		"go.uber.org/zap":                     "Zap (Logging)",
		"github.com/grpc/grpc-go":             "gRPC",
		"google.golang.org/grpc":              "gRPC",
		"k8s.io/client-go":                    "Kubernetes Client",
		"github.com/prometheus/client_golang": "Prometheus Client",
	}

	seen := make(map[string]bool)
	for _, dep := range deps {
		for prefix, name := range frameworkMap {
			if strings.HasPrefix(dep.Module, prefix) && !seen[name] {
				frameworks = append(frameworks, name)
				seen[name] = true
			}
		}
	}

	return frameworks
}

// generateGoVersionMatrix generates a list of Go versions from a go version requirement
func generateGoVersionMatrix(goVersion string) []string {
	// Map Go version to supported versions for testing
	supportedVersions := map[string][]string{
		"1.22": {"1.22", "1.23"},
		"1.21": {"1.21", "1.22", "1.23"},
		"1.20": {"1.20", "1.21", "1.22", "1.23"},
		"1.19": {"1.19", "1.20", "1.21", "1.22"},
		"1.18": {"1.18", "1.19", "1.20", "1.21"},
		"1.17": {"1.17", "1.18", "1.19", "1.20"},
	}

	if versionList, ok := supportedVersions[goVersion]; ok {
		return versionList
	}

	// Default to recent versions if not found
	return []string{"1.21", "1.22", "1.23"}
}

// extractVersionFromProject tries to extract version from common patterns
func extractVersionFromProject(projectPath string) string {
	// Try VERSION file
	versionPath := filepath.Join(projectPath, "VERSION")
	if content, err := os.ReadFile(versionPath); err == nil {
		version := strings.TrimSpace(string(content))
		if version != "" {
			return version
		}
	}

	// Try version.go or main.go for version constants
	patterns := []string{
		filepath.Join(projectPath, "version.go"),
		filepath.Join(projectPath, "main.go"),
		filepath.Join(projectPath, "cmd", "*", "main.go"),
	}

	versionRe := regexp.MustCompile(`(?:Version|version)\s*=\s*"([^"]+)"`)

	for _, pattern := range patterns {
		matches, _ := filepath.Glob(pattern)
		for _, match := range matches {
			file, err := os.Open(match)
			if err != nil {
				continue
			}

			scanner := bufio.NewScanner(file)
			found := false
			var version string

			for scanner.Scan() {
				line := scanner.Text()
				if m := versionRe.FindStringSubmatch(line); len(m) > 1 {
					version = m[1]
					found = true
					break
				}
			}

			file.Close()

			if found {
				return version
			}
		}
	}

	return ""
}

// Detect checks if this extractor can handle the project
func (e *Extractor) Detect(projectPath string) bool {
	// Check for go.mod
	if _, err := os.Stat(filepath.Join(projectPath, "go.mod")); err == nil {
		return true
	}

	return false
}

// Helper functions

// extractBaseNameFromModulePath extracts a friendly base name from a Go module path,
// handling semantic import versioning (v2+) where applicable.
//
// Examples:
//   - github.com/user/repo -> "repo"
//   - github.com/user/repo/v2 -> "repo" (strips v2+ suffix)
//   - github.com/user/v2 -> "v2" (preserves, as it's likely the repo name)
func extractBaseNameFromModulePath(parts []string) string {
	if len(parts) == 0 {
		return ""
	}

	baseName := parts[len(parts)-1]

	// Strip /vX suffix for semantic import versioning (Go modules v2+)
	// Only applies when path has 4+ components (domain/user/repo/vN)
	// to avoid incorrectly stripping repos literally named "v2", "v3", etc.
	if len(parts) >= 4 && strings.HasPrefix(baseName, "v") && len(baseName) > 1 {
		versionSuffix := baseName[1:]
		if versionNum, err := strconv.Atoi(versionSuffix); err == nil && versionNum >= 2 {
			// It's a semantic import version suffix, use the previous part
			return parts[len(parts)-2]
		}
	}

	return baseName
}

// quoteStrings adds quotes around each string
func quoteStrings(strs []string) []string {
	quoted := make([]string, len(strs))
	for i, s := range strs {
		quoted[i] = fmt.Sprintf(`"%s"`, s)
	}
	return quoted
}

// init registers the Go extractor
func init() {
	extractor.RegisterExtractor(NewExtractor())
}
