// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package julia

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/lfreleng-actions/build-metadata-action/internal/extractor"
)

// Extractor extracts metadata from Julia projects
type Extractor struct {
	extractor.BaseExtractor
}

// NewExtractor creates a new Julia extractor
func NewExtractor() *Extractor {
	return &Extractor{
		BaseExtractor: extractor.NewBaseExtractor("julia", 1),
	}
}

func init() {
	extractor.RegisterExtractor(NewExtractor())
}

// ProjectToml represents the structure of a Project.toml file
type ProjectToml struct {
	Name    string            `toml:"name"`
	UUID    string            `toml:"uuid"`
	Version string            `toml:"version"`
	Authors []string          `toml:"authors"`
	Deps    map[string]string `toml:"deps"`
	Compat  map[string]string `toml:"compat"`
}

// Detect checks if this is a Julia project
func (e *Extractor) Detect(projectPath string) bool {
	// Check for Project.toml
	if _, err := os.Stat(filepath.Join(projectPath, "Project.toml")); err == nil {
		return true
	}

	// Check for JuliaProject.toml (alternative name)
	if _, err := os.Stat(filepath.Join(projectPath, "JuliaProject.toml")); err == nil {
		return true
	}

	// Check for Manifest.toml (usually alongside Project.toml)
	if _, err := os.Stat(filepath.Join(projectPath, "Manifest.toml")); err == nil {
		return true
	}

	// Check for src/ directory with .jl files
	srcDir := filepath.Join(projectPath, "src")
	if info, err := os.Stat(srcDir); err == nil && info.IsDir() {
		matches, err := filepath.Glob(filepath.Join(srcDir, "*.jl"))
		if err == nil && len(matches) > 0 {
			return true
		}
	}

	// Check for .jl files in root
	matches, err := filepath.Glob(filepath.Join(projectPath, "*.jl"))
	if err == nil && len(matches) > 0 {
		return true
	}

	return false
}

// Extract retrieves metadata from a Julia project
func (e *Extractor) Extract(projectPath string) (*extractor.ProjectMetadata, error) {
	metadata := &extractor.ProjectMetadata{
		LanguageSpecific: make(map[string]interface{}),
	}

	// Try Project.toml first
	projectTomlPath := filepath.Join(projectPath, "Project.toml")
	if _, err := os.Stat(projectTomlPath); err == nil {
		if err := e.extractFromProjectToml(projectTomlPath, metadata); err != nil {
			return nil, err
		}
	} else {
		// Try JuliaProject.toml as fallback
		juliaProjectPath := filepath.Join(projectPath, "JuliaProject.toml")
		if _, err := os.Stat(juliaProjectPath); err == nil {
			if err := e.extractFromProjectToml(juliaProjectPath, metadata); err != nil {
				return nil, err
			}
		}
	}

	// Check for Manifest.toml
	manifestPath := filepath.Join(projectPath, "Manifest.toml")
	if _, err := os.Stat(manifestPath); err == nil {
		metadata.LanguageSpecific["has_manifest"] = true
	}

	metadata.LanguageSpecific["build_tool"] = "Pkg"

	return metadata, nil
}

// extractFromProjectToml parses Project.toml
func (e *Extractor) extractFromProjectToml(path string, metadata *extractor.ProjectMetadata) error {
	var project ProjectToml
	if _, err := toml.DecodeFile(path, &project); err != nil {
		return err
	}

	// Extract basic metadata
	if project.Name != "" {
		metadata.Name = project.Name
	}

	if project.Version != "" {
		metadata.Version = project.Version
		metadata.VersionSource = filepath.Base(path)
	}

	if len(project.Authors) > 0 {
		metadata.Authors = project.Authors
	}

	if project.UUID != "" {
		metadata.LanguageSpecific["uuid"] = project.UUID
	}

	// Extract dependencies
	if len(project.Deps) > 0 {
		var dependencies []string
		for dep := range project.Deps {
			dependencies = append(dependencies, dep)
		}
		metadata.LanguageSpecific["dependencies"] = dependencies
		metadata.LanguageSpecific["dependency_count"] = len(dependencies)
	}

	// Extract Julia version compatibility
	if juliaCompat, ok := project.Compat["julia"]; ok {
		metadata.LanguageSpecific["julia_version"] = juliaCompat

		// Generate version matrix
		matrix := generateJuliaVersionMatrix(juliaCompat)
		if len(matrix) > 0 {
			metadata.LanguageSpecific["julia_version_matrix"] = matrix
		}
	}

	// Extract other compatibility info
	if len(project.Compat) > 0 {
		compat := make(map[string]string)
		for pkg, version := range project.Compat {
			if pkg != "julia" {
				compat[pkg] = version
			}
		}
		if len(compat) > 0 {
			metadata.LanguageSpecific["compat"] = compat
		}
	}

	// Detect if this is a registered package
	e.detectPackageType(filepath.Dir(path), metadata)

	return nil
}

// detectPackageType determines if this is a registered package, script, or notebook
func (e *Extractor) detectPackageType(projectPath string, metadata *extractor.ProjectMetadata) {
	// Check for src/ directory (typical for packages)
	srcDir := filepath.Join(projectPath, "src")
	if info, err := os.Stat(srcDir); err == nil && info.IsDir() {
		metadata.LanguageSpecific["package_type"] = "package"

		// Look for main module file
		mainFile := filepath.Join(srcDir, metadata.Name+".jl")
		if _, err := os.Stat(mainFile); err == nil {
			metadata.LanguageSpecific["has_main_module"] = true
		}
	}

	// Check for test/ directory
	testDir := filepath.Join(projectPath, "test")
	if info, err := os.Stat(testDir); err == nil && info.IsDir() {
		metadata.LanguageSpecific["has_tests"] = true
	}

	// Check for docs/ directory
	docsDir := filepath.Join(projectPath, "docs")
	if info, err := os.Stat(docsDir); err == nil && info.IsDir() {
		metadata.LanguageSpecific["has_docs"] = true
	}

	// Check for notebooks
	notebooks, _ := filepath.Glob(filepath.Join(projectPath, "*.ipynb"))
	if len(notebooks) > 0 {
		metadata.LanguageSpecific["has_notebooks"] = true
		metadata.LanguageSpecific["notebook_count"] = len(notebooks)
	}
}

// generateJuliaVersionMatrix generates a matrix of Julia versions
func generateJuliaVersionMatrix(versionSpec string) []string {
	// Remove spaces
	versionSpec = strings.ReplaceAll(versionSpec, " ", "")

	// Parse version specification
	if strings.HasPrefix(versionSpec, "^") {
		// Caret notation: ^1.9 means >=1.9.0, <2.0.0
		version := strings.TrimPrefix(versionSpec, "^")
		return generateCaretVersions(version)
	} else if strings.HasPrefix(versionSpec, "~") {
		// Tilde notation: ~1.9 means >=1.9.0, <1.10.0
		version := strings.TrimPrefix(versionSpec, "~")
		return generateTildeVersions(version)
	} else if strings.Contains(versionSpec, "-") {
		// Range notation: 1.6-1.9
		parts := strings.Split(versionSpec, "-")
		if len(parts) == 2 {
			return generateRangeVersions(parts[0], parts[1])
		}
	} else if matched, _ := regexp.MatchString(`^[0-9.]+$`, versionSpec); matched {
		// Exact version
		return []string{normalizeVersion(versionSpec)}
	}

	// Default to recent stable versions
	return []string{"1.9", "1.10"}
}

// generateCaretVersions generates versions for caret notation
func generateCaretVersions(version string) []string {
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return []string{"1.9", "1.10"}
	}

	major := parts[0]
	if major == "1" {
		// Julia 1.x series
		return []string{"1.9", "1.10"}
	}

	return []string{version}
}

// generateTildeVersions generates versions for tilde notation
func generateTildeVersions(version string) []string {
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return []string{version}
	}

	// ~1.9 means 1.9.x
	major := parts[0]
	minor := parts[1]

	if major == "1" && minor == "9" {
		return []string{"1.9"}
	} else if major == "1" && minor == "10" {
		return []string{"1.10"}
	}

	return []string{major + "." + minor}
}

// generateRangeVersions generates versions for range notation
func generateRangeVersions(start, end string) []string {
	// Simplified: return start, middle, and end versions
	startNorm := normalizeVersion(start)
	endNorm := normalizeVersion(end)

	versions := []string{startNorm}

	// Add middle version if range is large enough
	if startNorm != endNorm {
		versions = append(versions, endNorm)
	}

	return versions
}

// normalizeVersion normalizes a version string to major.minor format
func normalizeVersion(version string) string {
	parts := strings.Split(version, ".")
	if len(parts) >= 2 {
		return parts[0] + "." + parts[1]
	}
	return version
}
