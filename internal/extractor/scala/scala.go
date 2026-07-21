// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package scala

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/lfreleng-actions/build-metadata-action/internal/extractor"
)

// Extractor extracts metadata from Scala projects
type Extractor struct {
	extractor.BaseExtractor
}

// NewExtractor creates a new Scala extractor
func NewExtractor() *Extractor {
	return &Extractor{
		BaseExtractor: extractor.NewBaseExtractor("scala", 1),
	}
}

func init() {
	extractor.RegisterExtractor(NewExtractor())
}

// Detect checks if this is a Scala project
func (e *Extractor) Detect(projectPath string) bool {
	if _, err := os.Stat(filepath.Join(projectPath, "build.sbt")); err == nil {
		return true
	}

	// Check for project/build.properties (SBT)
	if _, err := os.Stat(filepath.Join(projectPath, "project", "build.properties")); err == nil {
		return true
	}

	// Check for build.sc (Mill)
	if _, err := os.Stat(filepath.Join(projectPath, "build.sc")); err == nil {
		return true
	}

	// Check for pom.xml with Scala (Maven)
	pomPath := filepath.Join(projectPath, "pom.xml")
	if content, err := os.ReadFile(pomPath); err == nil {
		if strings.Contains(string(content), "scala") {
			return true
		}
	}

	srcMain := filepath.Join(projectPath, "src", "main", "scala")
	if info, err := os.Stat(srcMain); err == nil && info.IsDir() {
		return true
	}

	patterns := []string{
		filepath.Join(projectPath, "*.scala"),
		filepath.Join(projectPath, "src", "*.scala"),
	}
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err == nil && len(matches) > 0 {
			return true
		}
	}

	return false
}

// Extract retrieves metadata from a Scala project
func (e *Extractor) Extract(projectPath string) (*extractor.ProjectMetadata, error) {
	metadata := &extractor.ProjectMetadata{
		LanguageSpecific: make(map[string]interface{}),
	}

	// Try build.sbt first (most common)
	buildSbtPath := filepath.Join(projectPath, "build.sbt")
	if _, err := os.Stat(buildSbtPath); err == nil {
		if err := e.extractFromBuildSbt(buildSbtPath, metadata); err == nil {
			metadata.LanguageSpecific["build_tool"] = "SBT"
			e.extractSbtVersion(projectPath, metadata)
			return metadata, nil
		}
	}

	// Try build.sc (Mill)
	buildScPath := filepath.Join(projectPath, "build.sc")
	if _, err := os.Stat(buildScPath); err == nil {
		if err := e.extractFromMill(buildScPath, metadata); err == nil {
			metadata.LanguageSpecific["build_tool"] = "Mill"
			return metadata, nil
		}
	}

	// Fallback
	metadata.LanguageSpecific["build_tool"] = "unknown"
	return metadata, nil
}

// sbtMatcher pairs a single-value build.sbt regex with its assignment.
type sbtMatcher struct {
	re     *regexp.Regexp
	assign func(value string)
}

// sbtFieldMatchers builds the ordered single-value field matchers for build.sbt.
func sbtFieldMatchers(metadata *extractor.ProjectMetadata, scalaVersion *string) []sbtMatcher {
	return []sbtMatcher{
		{regexp.MustCompile(`name\s*:=\s*"([^"]+)"`), func(v string) { metadata.Name = v }},
		{regexp.MustCompile(`version\s*:=\s*"([^"]+)"`), func(v string) {
			metadata.Version = v
			metadata.VersionSource = "build.sbt"
		}},
		{regexp.MustCompile(`scalaVersion\s*:=\s*"([^"]+)"`), func(v string) { *scalaVersion = v }},
		{regexp.MustCompile(`organization\s*:=\s*"([^"]+)"`), func(v string) {
			metadata.LanguageSpecific["organization"] = v
		}},
		{regexp.MustCompile(`description\s*:=\s*"([^"]+)"`), func(v string) { metadata.Description = v }},
		{regexp.MustCompile(`homepage\s*:=\s*Some\(url\("([^"]+)"\)\)`), func(v string) { metadata.Homepage = v }},
		// License name is the first quoted string in: licenses := Seq("Apache-2.0" -> url("..."))
		{regexp.MustCompile(`licenses\s*:=\s*Seq\(\s*"([^"]+)"`), func(v string) { metadata.License = v }},
	}
}

// formatSbtDependency renders an "org:name:version" dependency from a regex match.
func formatSbtDependency(matches []string) string {
	return fmt.Sprintf("%s:%s:%s", matches[1], matches[2], matches[3])
}

// trackSbtDependencyBlock advances the multi-line libraryDependencies Seq(...)
// state machine, collecting standalone dependency lines while the block is open.
// It returns the updated open flag and parenthesis depth.
func trackSbtDependencyBlock(line string, standaloneRe *regexp.Regexp, inBlock bool, parenDepth int, dependencies *[]string) (bool, int) {
	if strings.Contains(line, "libraryDependencies") && strings.Contains(line, "Seq(") {
		depth := strings.Count(line, "(") - strings.Count(line, ")")
		if depth <= 0 {
			return false, depth
		}
		return true, depth
	}

	if !inBlock {
		return inBlock, parenDepth
	}

	if matches := standaloneRe.FindStringSubmatch(line); matches != nil {
		*dependencies = append(*dependencies, formatSbtDependency(matches))
	}
	parenDepth += strings.Count(line, "(") - strings.Count(line, ")")
	if parenDepth <= 0 {
		return false, 0
	}
	return true, parenDepth
}

// applySbtScalaVersion records the detected Scala version and its matrix.
func applySbtScalaVersion(scalaVersion string, metadata *extractor.ProjectMetadata) {
	if scalaVersion == "" {
		return
	}
	metadata.LanguageSpecific["scala_version"] = scalaVersion
	if matrix := generateScalaVersionMatrix(scalaVersion); len(matrix) > 0 {
		metadata.LanguageSpecific["scala_version_matrix"] = matrix
	}
}

// extractFromBuildSbt parses build.sbt
func (e *Extractor) extractFromBuildSbt(path string, metadata *extractor.ProjectMetadata) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	var dependencies []string
	var scalaVersion string
	matchers := sbtFieldMatchers(metadata, &scalaVersion)
	// Match dependencies on same line as libraryDependencies
	inlineDepRegex := regexp.MustCompile(`libraryDependencies\s*\+\+?=\s*(?:Seq\()?\s*"([^"]+)"\s*%+\s*"([^"]+)"\s*%\s*"([^"]+)"`)
	// Match standalone dependency lines within Seq block: "org" %% "name" % "version"
	standaloneDepRegex := regexp.MustCompile(`^\s*"([^"]+)"\s*%%?\s*"([^"]+)"\s*%\s*"([^"]+)"`)

	inLibraryDependencies := false
	parenDepth := 0

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip comments
		if strings.HasPrefix(line, "//") {
			continue
		}

		for _, matcher := range matchers {
			if matches := matcher.re.FindStringSubmatch(line); matches != nil {
				matcher.assign(matches[1])
			}
		}

		if matches := inlineDepRegex.FindStringSubmatch(line); matches != nil {
			dependencies = append(dependencies, formatSbtDependency(matches))
		}

		inLibraryDependencies, parenDepth = trackSbtDependencyBlock(
			line, standaloneDepRegex, inLibraryDependencies, parenDepth, &dependencies)
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	applySbtScalaVersion(scalaVersion, metadata)

	if len(dependencies) > 0 {
		metadata.LanguageSpecific["dependencies"] = dependencies
		metadata.LanguageSpecific["dependency_count"] = len(dependencies)
	}

	return nil
}

// extractSbtVersion extracts SBT version from project/build.properties
func (e *Extractor) extractSbtVersion(projectPath string, metadata *extractor.ProjectMetadata) {
	buildPropsPath := filepath.Join(projectPath, "project", "build.properties")
	content, err := os.ReadFile(buildPropsPath)
	if err != nil {
		return
	}

	sbtVersionRegex := regexp.MustCompile(`sbt\.version\s*=\s*([0-9.]+)`)
	if matches := sbtVersionRegex.FindStringSubmatch(string(content)); matches != nil {
		metadata.LanguageSpecific["sbt_version"] = matches[1]
	}
}

// extractFromMill parses build.sc (Mill build tool)
func (e *Extractor) extractFromMill(path string, metadata *extractor.ProjectMetadata) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	objectRegex := regexp.MustCompile(`object\s+(\w+)\s+extends`)
	scalaVersionRegex := regexp.MustCompile(`def\s+scalaVersion\s*=\s*"([^"]+)"`)
	// Match ivy dependencies with both : and :: (Scala cross-version) syntax
	// e.g., ivy"com.lihaoyi::upickle:3.1.3" or ivy"org.example:artifact:1.0"
	ivyDepRegex := regexp.MustCompile(`ivy"([^:]+)::?([^:]+):([^"]+)"`)

	var dependencies []string
	var scalaVersion string

	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "//") {
			continue
		}

		if matches := objectRegex.FindStringSubmatch(line); matches != nil && metadata.Name == "" {
			metadata.Name = matches[1]
		}

		if matches := scalaVersionRegex.FindStringSubmatch(line); matches != nil {
			scalaVersion = matches[1]
		}

		if matches := ivyDepRegex.FindStringSubmatch(line); matches != nil {
			dep := fmt.Sprintf("%s:%s:%s", matches[1], matches[2], matches[3])
			dependencies = append(dependencies, dep)
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	if scalaVersion != "" {
		metadata.LanguageSpecific["scala_version"] = scalaVersion
	}

	if len(dependencies) > 0 {
		metadata.LanguageSpecific["dependencies"] = dependencies
		metadata.LanguageSpecific["dependency_count"] = len(dependencies)
	}

	return nil
}

// generateScalaVersionMatrix generates a matrix of compatible Scala versions
func generateScalaVersionMatrix(version string) []string {
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return []string{version}
	}

	major := parts[0]
	minor := parts[1]

	// Scala 3.x
	if major == "3" {
		return []string{"3.3", "3.4"}
	}

	// Scala 2.13.x
	if major == "2" && minor == "13" {
		return []string{"2.13"}
	}

	// Scala 2.12.x
	if major == "2" && minor == "12" {
		return []string{"2.12", "2.13"}
	}

	// Scala 2.11.x (legacy)
	if major == "2" && minor == "11" {
		return []string{"2.11", "2.12"}
	}

	return []string{version}
}
