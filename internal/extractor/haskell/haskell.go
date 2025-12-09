// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package haskell

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/lfreleng-actions/build-metadata-action/internal/extractor"
)

// Extractor extracts metadata from Haskell projects
type Extractor struct {
	extractor.BaseExtractor
}

// NewExtractor creates a new Haskell extractor
func NewExtractor() *Extractor {
	return &Extractor{
		BaseExtractor: extractor.NewBaseExtractor("haskell", 1),
	}
}

func init() {
	extractor.RegisterExtractor(NewExtractor())
}

// Detect checks if this is a Haskell project
func (e *Extractor) Detect(projectPath string) bool {
	// Check for .cabal file
	matches, err := filepath.Glob(filepath.Join(projectPath, "*.cabal"))
	if err == nil && len(matches) > 0 {
		return true
	}

	// Check for stack.yaml
	if _, err := os.Stat(filepath.Join(projectPath, "stack.yaml")); err == nil {
		return true
	}

	// Check for package.yaml (hpack)
	if _, err := os.Stat(filepath.Join(projectPath, "package.yaml")); err == nil {
		return true
	}

	// Check for cabal.project
	if _, err := os.Stat(filepath.Join(projectPath, "cabal.project")); err == nil {
		return true
	}

	// Check for Haskell source files
	srcDir := filepath.Join(projectPath, "src")
	if info, err := os.Stat(srcDir); err == nil && info.IsDir() {
		matches, err := filepath.Glob(filepath.Join(srcDir, "*.hs"))
		if err == nil && len(matches) > 0 {
			return true
		}
	}

	return false
}

// Extract retrieves metadata from a Haskell project
func (e *Extractor) Extract(projectPath string) (*extractor.ProjectMetadata, error) {
	metadata := &extractor.ProjectMetadata{
		LanguageSpecific: make(map[string]interface{}),
	}

	// Try to find .cabal file
	cabalFiles, err := filepath.Glob(filepath.Join(projectPath, "*.cabal"))
	if err == nil && len(cabalFiles) > 0 {
		if err := e.extractFromCabal(cabalFiles[0], metadata); err == nil {
			metadata.LanguageSpecific["build_tool"] = "Cabal"
		}
	}

	// Check for Stack
	stackPath := filepath.Join(projectPath, "stack.yaml")
	if _, err := os.Stat(stackPath); err == nil {
		e.extractFromStack(stackPath, metadata)
		if metadata.LanguageSpecific["build_tool"] == "" {
			metadata.LanguageSpecific["build_tool"] = "Stack"
		} else {
			metadata.LanguageSpecific["build_tool"] = "Stack + Cabal"
		}
	}

	// Check for package.yaml (hpack)
	packageYamlPath := filepath.Join(projectPath, "package.yaml")
	if _, err := os.Stat(packageYamlPath); err == nil {
		e.extractFromPackageYaml(packageYamlPath, metadata)
	}

	return metadata, nil
}

// extractFromCabal parses a .cabal file
func (e *Extractor) extractFromCabal(path string, metadata *extractor.ProjectMetadata) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	nameRegex := regexp.MustCompile(`(?i)^name:\s*(.+)$`)
	versionRegex := regexp.MustCompile(`(?i)^version:\s*(.+)$`)
	synopsisRegex := regexp.MustCompile(`(?i)^synopsis:\s*(.+)$`)
	descriptionRegex := regexp.MustCompile(`(?i)^description:\s*(.+)$`)
	homepageRegex := regexp.MustCompile(`(?i)^homepage:\s*(.+)$`)
	licenseRegex := regexp.MustCompile(`(?i)^license:\s*(.+)$`)
	authorRegex := regexp.MustCompile(`(?i)^author:\s*(.+)$`)
	maintainerRegex := regexp.MustCompile(`(?i)^maintainer:\s*(.+)$`)
	buildDependsRegex := regexp.MustCompile(`(?i)^build-depends:\s*(.+)$`)
	categoryRegex := regexp.MustCompile(`(?i)^category:\s*(.+)$`)
	testedWithRegex := regexp.MustCompile(`(?i)^tested-with:\s*(.+)$`)

	var dependencies []string
	var authors []string
	inBuildDepends := false

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Skip comments
		if strings.HasPrefix(trimmed, "--") {
			continue
		}

		if matches := nameRegex.FindStringSubmatch(trimmed); matches != nil {
			metadata.Name = strings.TrimSpace(matches[1])
		}

		if matches := versionRegex.FindStringSubmatch(trimmed); matches != nil {
			metadata.Version = strings.TrimSpace(matches[1])
			metadata.VersionSource = filepath.Base(path)
		}

		if matches := synopsisRegex.FindStringSubmatch(trimmed); matches != nil {
			metadata.Description = strings.TrimSpace(matches[1])
		}

		if matches := descriptionRegex.FindStringSubmatch(trimmed); matches != nil && metadata.Description == "" {
			metadata.Description = strings.TrimSpace(matches[1])
		}

		if matches := homepageRegex.FindStringSubmatch(trimmed); matches != nil {
			metadata.Homepage = strings.TrimSpace(matches[1])
		}

		if matches := licenseRegex.FindStringSubmatch(trimmed); matches != nil {
			metadata.License = strings.TrimSpace(matches[1])
		}

		if matches := authorRegex.FindStringSubmatch(trimmed); matches != nil {
			author := strings.TrimSpace(matches[1])
			if author != "" {
				authors = append(authors, author)
			}
		}

		if matches := maintainerRegex.FindStringSubmatch(trimmed); matches != nil {
			maintainer := strings.TrimSpace(matches[1])
			if maintainer != "" {
				metadata.LanguageSpecific["maintainer"] = maintainer
			}
		}

		if matches := categoryRegex.FindStringSubmatch(trimmed); matches != nil {
			metadata.LanguageSpecific["category"] = strings.TrimSpace(matches[1])
		}

		if matches := testedWithRegex.FindStringSubmatch(trimmed); matches != nil {
			metadata.LanguageSpecific["tested_with"] = strings.TrimSpace(matches[1])
		}

		if matches := buildDependsRegex.FindStringSubmatch(trimmed); matches != nil {
			depLine := strings.TrimSpace(matches[1])
			deps := parseDependencies(depLine)
			dependencies = append(dependencies, deps...)
			inBuildDepends = true
		} else if inBuildDepends && strings.HasPrefix(line, " ") {
			// Continuation of build-depends
			deps := parseDependencies(trimmed)
			dependencies = append(dependencies, deps...)
		} else if inBuildDepends && !strings.HasPrefix(line, " ") {
			inBuildDepends = false
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	if len(authors) > 0 {
		metadata.Authors = authors
	}

	if len(dependencies) > 0 {
		metadata.LanguageSpecific["dependencies"] = dependencies
		metadata.LanguageSpecific["dependency_count"] = len(dependencies)
	}

	return nil
}

// extractFromStack parses stack.yaml
func (e *Extractor) extractFromStack(path string, metadata *extractor.ProjectMetadata) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	resolverRegex := regexp.MustCompile(`^resolver:\s*(.+)$`)

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "#") {
			continue
		}

		if matches := resolverRegex.FindStringSubmatch(trimmed); matches != nil {
			resolver := strings.TrimSpace(matches[1])
			metadata.LanguageSpecific["stack_resolver"] = resolver

			// Extract GHC version from resolver (e.g., lts-21.22 -> GHC 9.4.8)
			if ghcVersion := extractGHCVersionFromResolver(resolver); ghcVersion != "" {
				metadata.LanguageSpecific["ghc_version"] = ghcVersion
			}
		}
	}

	return scanner.Err()
}

// extractFromPackageYaml parses package.yaml (hpack format)
func (e *Extractor) extractFromPackageYaml(path string, metadata *extractor.ProjectMetadata) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	nameRegex := regexp.MustCompile(`^name:\s*(.+)$`)
	versionRegex := regexp.MustCompile(`^version:\s*(.+)$`)

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "#") {
			continue
		}

		if matches := nameRegex.FindStringSubmatch(trimmed); matches != nil && metadata.Name == "" {
			metadata.Name = strings.TrimSpace(matches[1])
		}

		if matches := versionRegex.FindStringSubmatch(trimmed); matches != nil && metadata.Version == "" {
			metadata.Version = strings.TrimSpace(matches[1])
			metadata.VersionSource = "package.yaml"
		}
	}

	metadata.LanguageSpecific["uses_hpack"] = true

	return scanner.Err()
}

// parseDependencies parses a dependency line from cabal file
func parseDependencies(line string) []string {
	var deps []string

	// Remove trailing comma
	line = strings.TrimSuffix(line, ",")

	// Split by comma
	parts := strings.Split(line, ",")

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Extract package name (before version constraint)
		fields := strings.Fields(part)
		if len(fields) > 0 {
			pkgName := fields[0]
			// Exclude "base" package itself
			if pkgName == "base" {
				continue
			}
			// Keep other package names as-is, including those with "base-" prefix
			if pkgName != "" {
				deps = append(deps, pkgName)
			}
		}
	}

	return deps
}

// extractGHCVersionFromResolver attempts to extract GHC version from Stack resolver
func extractGHCVersionFromResolver(resolver string) string {
	// Map common LTS versions to GHC versions
	ltsToGHC := map[string]string{
		"lts-22": "9.6.4",
		"lts-21": "9.4.8",
		"lts-20": "9.2.8",
		"lts-19": "9.0.2",
		"lts-18": "8.10.7",
	}

	for lts, ghc := range ltsToGHC {
		if strings.HasPrefix(resolver, lts) {
			return ghc
		}
	}

	// Try to extract from nightly
	if strings.HasPrefix(resolver, "nightly-") {
		return "9.6" // Latest stable
	}

	return ""
}

// generateGHCVersionMatrix generates a matrix of GHC versions
func generateGHCVersionMatrix(ghcVersion string) []string {
	// Return common GHC versions for testing
	return []string{"9.4", "9.6", "9.8"}
}
