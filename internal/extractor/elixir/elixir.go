// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package elixir

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/lfreleng-actions/build-metadata-action/internal/extractor"
)

// Extractor extracts metadata from Elixir projects
type Extractor struct {
	extractor.BaseExtractor
}

// NewExtractor creates a new Elixir extractor
func NewExtractor() *Extractor {
	return &Extractor{
		BaseExtractor: extractor.NewBaseExtractor("elixir", 1),
	}
}

func init() {
	extractor.RegisterExtractor(NewExtractor())
}

// Detect checks if this is an Elixir project
func (e *Extractor) Detect(projectPath string) bool {
	// Check for mix.exs
	if _, err := os.Stat(filepath.Join(projectPath, "mix.exs")); err == nil {
		return true
	}

	// Check for lib/ directory with .ex files
	libDir := filepath.Join(projectPath, "lib")
	if info, err := os.Stat(libDir); err == nil && info.IsDir() {
		matches, err := filepath.Glob(filepath.Join(libDir, "*.ex"))
		if err == nil && len(matches) > 0 {
			return true
		}
	}

	// Check for .ex or .exs files in root
	patterns := []string{"*.ex", "*.exs"}
	for _, pattern := range patterns {
		matches, err := filepath.Glob(filepath.Join(projectPath, pattern))
		if err == nil && len(matches) > 0 {
			return true
		}
	}

	return false
}

// Extract retrieves metadata from an Elixir project
func (e *Extractor) Extract(projectPath string) (*extractor.ProjectMetadata, error) {
	metadata := &extractor.ProjectMetadata{
		LanguageSpecific: make(map[string]interface{}),
	}

	mixExsPath := filepath.Join(projectPath, "mix.exs")
	if _, err := os.Stat(mixExsPath); err == nil {
		if err := e.extractFromMixExs(mixExsPath, metadata); err != nil {
			return nil, err
		}
	}

	metadata.LanguageSpecific["build_tool"] = "Mix"
	return metadata, nil
}

// extractFromMixExs parses mix.exs
func (e *Extractor) extractFromMixExs(path string, metadata *extractor.ProjectMetadata) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	// Regex patterns
	appRegex := regexp.MustCompile(`app:\s*:(\w+)`)
	versionRegex := regexp.MustCompile(`version:\s*"([^"]+)"`)
	elixirRegex := regexp.MustCompile(`elixir:\s*"([^"]+)"`)
	descriptionRegex := regexp.MustCompile(`description:\s*"([^"]+)"`)
	packageRegex := regexp.MustCompile(`package:\s*\[`)
	licenseRegex := regexp.MustCompile(`licenses:\s*\["([^"]+)"`)
	linksRegex := regexp.MustCompile(`links:\s*%\{`)
	homepageRegex := regexp.MustCompile(`"([^"]+)"\s*=>\s*"([^"]+)"`)
	depRegex := regexp.MustCompile(`\{:(\w+),\s*"([^"]+)"`)

	var dependencies []string
	var inPackageBlock bool
	var inLinksBlock bool
	var elixirVersion string

	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)

		// Skip comments
		if strings.HasPrefix(line, "#") {
			continue
		}

		// Extract app name
		if matches := appRegex.FindStringSubmatch(line); matches != nil {
			metadata.Name = matches[1]
		}

		// Extract version
		if matches := versionRegex.FindStringSubmatch(line); matches != nil {
			metadata.Version = matches[1]
			metadata.VersionSource = "mix.exs"
		}

		// Extract Elixir version requirement
		if matches := elixirRegex.FindStringSubmatch(line); matches != nil {
			elixirVersion = matches[1]
		}

		// Extract description
		if matches := descriptionRegex.FindStringSubmatch(line); matches != nil {
			metadata.Description = matches[1]
		}

		// Track package block
		if packageRegex.MatchString(line) {
			inPackageBlock = true
		}

		// Extract licenses in package block
		if inPackageBlock {
			if matches := licenseRegex.FindStringSubmatch(line); matches != nil {
				metadata.License = matches[1]
			}
		}

		// Track links block
		if linksRegex.MatchString(line) {
			inLinksBlock = true
		}

		// Extract homepage from links
		if inLinksBlock {
			if matches := homepageRegex.FindStringSubmatch(line); matches != nil {
				if matches[1] == "GitHub" || matches[1] == "Homepage" {
					metadata.Homepage = matches[2]
				}
			}
		}

		// End blocks
		if strings.Contains(line, "]") {
			if inPackageBlock && !strings.Contains(line, "[") {
				inPackageBlock = false
			}
		}
		if strings.Contains(line, "}") {
			if inLinksBlock && !strings.Contains(line, "%{") {
				inLinksBlock = false
			}
		}

		// Extract dependencies
		if matches := depRegex.FindStringSubmatch(line); matches != nil {
			dep := fmt.Sprintf("%s:%s", matches[1], matches[2])
			dependencies = append(dependencies, dep)
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	// Store Elixir version
	if elixirVersion != "" {
		metadata.LanguageSpecific["elixir_version"] = elixirVersion

		// Generate version matrix
		matrix := generateElixirVersionMatrix(elixirVersion)
		if len(matrix) > 0 {
			metadata.LanguageSpecific["elixir_version_matrix"] = matrix
		}
	}

	// Store dependencies
	if len(dependencies) > 0 {
		metadata.LanguageSpecific["dependencies"] = dependencies
		metadata.LanguageSpecific["dependency_count"] = len(dependencies)
	}

	// Detect frameworks
	framework := detectFramework(dependencies)
	if framework != "" {
		metadata.LanguageSpecific["framework"] = framework
	}

	return nil
}

// generateElixirVersionMatrix generates a matrix of Elixir versions
func generateElixirVersionMatrix(requirement string) []string {
	// Remove constraint operators
	version := strings.TrimPrefix(requirement, "~> ")
	version = strings.TrimPrefix(version, ">= ")
	version = strings.TrimPrefix(version, "== ")

	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return []string{"1.14", "1.15", "1.16"}
	}

	major := parts[0]
	minor := parts[1]

	if major == "1" {
		switch minor {
		case "16":
			return []string{"1.16", "1.17"}
		case "15":
			return []string{"1.15", "1.16", "1.17"}
		case "14":
			return []string{"1.14", "1.15", "1.16"}
		case "13":
			return []string{"1.13", "1.14", "1.15"}
		case "12":
			return []string{"1.12", "1.13", "1.14"}
		default:
			return []string{"1.14", "1.15", "1.16"}
		}
	}

	return []string{"1.14", "1.15", "1.16"}
}

// detectFramework detects if the project uses a framework
func detectFramework(dependencies []string) string {
	for _, dep := range dependencies {
		if strings.Contains(dep, "phoenix:") {
			return "Phoenix"
		}
		if strings.Contains(dep, "nerves:") {
			return "Nerves"
		}
		if strings.Contains(dep, "plug:") {
			return "Plug"
		}
	}
	return ""
}
