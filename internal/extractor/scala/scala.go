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
	// Check for build.sbt
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

	// Check for Scala source files
	srcMain := filepath.Join(projectPath, "src", "main", "scala")
	if info, err := os.Stat(srcMain); err == nil && info.IsDir() {
		return true
	}

	// Check for .scala files in root or src
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

// extractFromBuildSbt parses build.sbt
func (e *Extractor) extractFromBuildSbt(path string, metadata *extractor.ProjectMetadata) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	// Regex patterns for SBT
	nameRegex := regexp.MustCompile(`name\s*:=\s*"([^"]+)"`)
	versionRegex := regexp.MustCompile(`version\s*:=\s*"([^"]+)"`)
	scalaVersionRegex := regexp.MustCompile(`scalaVersion\s*:=\s*"([^"]+)"`)
	organizationRegex := regexp.MustCompile(`organization\s*:=\s*"([^"]+)"`)
	descriptionRegex := regexp.MustCompile(`description\s*:=\s*"([^"]+)"`)
	homepageRegex := regexp.MustCompile(`homepage\s*:=\s*Some\(url\("([^"]+)"\)\)`)
	licenseRegex := regexp.MustCompile(`licenses\s*:=\s*Seq\(.*"([^"]+)"`)
	libraryDependencyRegex := regexp.MustCompile(`libraryDependencies\s*\+\+?=\s*(?:Seq\()?\s*"([^"]+)"\s*%+\s*"([^"]+)"\s*%\s*"([^"]+)"`)

	var dependencies []string
	var scalaVersion string

	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)

		// Skip comments
		if strings.HasPrefix(line, "//") {
			continue
		}

		if matches := nameRegex.FindStringSubmatch(line); matches != nil {
			metadata.Name = matches[1]
		}

		if matches := versionRegex.FindStringSubmatch(line); matches != nil {
			metadata.Version = matches[1]
			metadata.VersionSource = "build.sbt"
		}

		if matches := scalaVersionRegex.FindStringSubmatch(line); matches != nil {
			scalaVersion = matches[1]
		}

		if matches := organizationRegex.FindStringSubmatch(line); matches != nil {
			metadata.LanguageSpecific["organization"] = matches[1]
		}

		if matches := descriptionRegex.FindStringSubmatch(line); matches != nil {
			metadata.Description = matches[1]
		}

		if matches := homepageRegex.FindStringSubmatch(line); matches != nil {
			metadata.Homepage = matches[1]
		}

		if matches := licenseRegex.FindStringSubmatch(line); matches != nil {
			metadata.License = matches[1]
		}

		if matches := libraryDependencyRegex.FindStringSubmatch(line); matches != nil {
			dep := fmt.Sprintf("%s:%s:%s", matches[1], matches[2], matches[3])
			dependencies = append(dependencies, dep)
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	if scalaVersion != "" {
		metadata.LanguageSpecific["scala_version"] = scalaVersion

		// Generate Scala version matrix
		matrix := generateScalaVersionMatrix(scalaVersion)
		if len(matrix) > 0 {
			metadata.LanguageSpecific["scala_version_matrix"] = matrix
		}
	}

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
	ivyDepRegex := regexp.MustCompile(`ivy"([^:]+):([^:]+):([^"]+)"`)

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
	// Parse major.minor from version
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
