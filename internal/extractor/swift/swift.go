// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package swift

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/lfreleng-actions/build-metadata-action/internal/extractor"
)

// Extractor extracts metadata from Swift projects
type Extractor struct {
	extractor.BaseExtractor
}

// NewExtractor creates a new Swift extractor
func NewExtractor() *Extractor {
	return &Extractor{
		BaseExtractor: extractor.NewBaseExtractor("swift", 1),
	}
}

// PackageManifest represents parsed Package.swift metadata
type PackageManifest struct {
	Name           string
	Platforms      []Platform
	Products       []Product
	Dependencies   []Dependency
	Targets        []Target
	SwiftVersion   string
	CLanguageStd   string
	CXXLanguageStd string
}

// Platform represents a platform requirement
type Platform struct {
	Name    string
	Version string
}

// Product represents a Swift package product
type Product struct {
	Name    string
	Type    string // library, executable, plugin
	Targets []string
}

// Dependency represents a package dependency
type Dependency struct {
	Name    string
	URL     string
	Version string
	Branch  string
	Commit  string
}

// Target represents a build target
type Target struct {
	Name         string
	Type         string // target, testTarget, systemLibrary, binaryTarget
	Dependencies []string
	Path         string
}

// Extract retrieves metadata from a Swift project
func (e *Extractor) Extract(projectPath string) (*extractor.ProjectMetadata, error) {
	metadata := &extractor.ProjectMetadata{
		LanguageSpecific: make(map[string]interface{}),
	}

	// Look for Package.swift
	packagePath := filepath.Join(projectPath, "Package.swift")
	if _, err := os.Stat(packagePath); err != nil {
		return nil, fmt.Errorf("Package.swift not found in %s", projectPath)
	}

	manifest, err := e.parsePackageSwift(packagePath)
	if err != nil {
		return nil, err
	}

	e.populateMetadata(manifest, metadata, projectPath)

	return metadata, nil
}

// parsePackageSwift parses Package.swift using regex patterns
func (e *Extractor) parsePackageSwift(path string) (*PackageManifest, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read Package.swift: %w", err)
	}

	text := string(content)
	manifest := &PackageManifest{
		Platforms:    make([]Platform, 0),
		Products:     make([]Product, 0),
		Dependencies: make([]Dependency, 0),
		Targets:      make([]Target, 0),
	}

	// Extract package name
	manifest.Name = e.extractPackageName(text)

	// Extract Swift tools version
	manifest.SwiftVersion = e.extractSwiftToolsVersion(text)

	// Extract platforms
	manifest.Platforms = e.extractPlatforms(text)

	// Extract products
	manifest.Products = e.extractProducts(text)

	// Extract dependencies
	manifest.Dependencies = e.extractDependencies(text)

	// Extract targets
	manifest.Targets = e.extractTargets(text)

	// Extract language standards
	manifest.CLanguageStd = e.extractFieldValue(text, "cLanguageStandard")
	manifest.CXXLanguageStd = e.extractFieldValue(text, "cxxLanguageStandard")

	return manifest, nil
}

// extractPackageName extracts the package name
func (e *Extractor) extractPackageName(text string) string {
	// Pattern: name: "PackageName"
	re := regexp.MustCompile(`name:\s*"([^"]+)"`)
	if matches := re.FindStringSubmatch(text); len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// extractSwiftToolsVersion extracts the Swift tools version from the header comment
func (e *Extractor) extractSwiftToolsVersion(text string) string {
	// Pattern: // swift-tools-version:5.7
	re := regexp.MustCompile(`//\s*swift-tools-version:\s*(\d+\.\d+(?:\.\d+)?)`)
	if matches := re.FindStringSubmatch(text); len(matches) > 1 {
		return matches[1]
	}

	// Also try older format
	re = regexp.MustCompile(`//\s*swift-tools-version\s+(\d+\.\d+(?:\.\d+)?)`)
	if matches := re.FindStringSubmatch(text); len(matches) > 1 {
		return matches[1]
	}

	return ""
}

// extractPlatforms extracts platform requirements
func (e *Extractor) extractPlatforms(text string) []Platform {
	platforms := make([]Platform, 0)

	// Pattern: platforms: [.macOS(.v10_15), .iOS(.v13)]
	re := regexp.MustCompile(`platforms:\s*\[(.*?)\]`)
	if matches := re.FindStringSubmatch(text); len(matches) > 1 {
		platformsText := matches[1]

		// Extract individual platforms
		platformRe := regexp.MustCompile(`\.(\w+)\(\.(v\d+(?:_\d+)?)\)`)
		for _, match := range platformRe.FindAllStringSubmatch(platformsText, -1) {
			if len(match) > 2 {
				platforms = append(platforms, Platform{
					Name:    match[1],
					Version: strings.ReplaceAll(match[2], "v", ""),
				})
			}
		}
	}

	return platforms
}

// extractProducts extracts package products
func (e *Extractor) extractProducts(text string) []Product {
	products := make([]Product, 0)

	// Find products array
	re := regexp.MustCompile(`products:\s*\[(.*?)\]`)
	if matches := re.FindStringSubmatch(text); len(matches) > 1 {
		productsText := matches[1]

		// Extract library products
		libRe := regexp.MustCompile(`\.library\(\s*name:\s*"([^"]+)".*?targets:\s*\[([^\]]+)\]`)
		for _, match := range libRe.FindAllStringSubmatch(productsText, -1) {
			if len(match) > 2 {
				targets := e.parseStringArray(match[2])
				products = append(products, Product{
					Name:    match[1],
					Type:    "library",
					Targets: targets,
				})
			}
		}

		// Extract executable products
		execRe := regexp.MustCompile(`\.executable\(\s*name:\s*"([^"]+)".*?targets:\s*\[([^\]]+)\]`)
		for _, match := range execRe.FindAllStringSubmatch(productsText, -1) {
			if len(match) > 2 {
				targets := e.parseStringArray(match[2])
				products = append(products, Product{
					Name:    match[1],
					Type:    "executable",
					Targets: targets,
				})
			}
		}

		// Extract plugin products
		pluginRe := regexp.MustCompile(`\.plugin\(\s*name:\s*"([^"]+)".*?targets:\s*\[([^\]]+)\]`)
		for _, match := range pluginRe.FindAllStringSubmatch(productsText, -1) {
			if len(match) > 2 {
				targets := e.parseStringArray(match[2])
				products = append(products, Product{
					Name:    match[1],
					Type:    "plugin",
					Targets: targets,
				})
			}
		}
	}

	return products
}

// extractDependencies extracts package dependencies
func (e *Extractor) extractDependencies(text string) []Dependency {
	dependencies := make([]Dependency, 0)

	// Find dependencies array
	re := regexp.MustCompile(`dependencies:\s*\[(.*?)\]`)
	if matches := re.FindStringSubmatch(text); len(matches) > 1 {
		depsText := matches[1]

		// Extract package dependencies with URL
		urlRe := regexp.MustCompile(`\.package\(\s*url:\s*"([^"]+)"`)
		urls := urlRe.FindAllStringSubmatch(depsText, -1)

		// For each URL, try to extract version info
		for _, urlMatch := range urls {
			if len(urlMatch) > 1 {
				dep := Dependency{
					URL: urlMatch[1],
				}

				// Try to extract name from URL
				dep.Name = e.extractNameFromURL(dep.URL)

				// Try to extract version constraint near this URL
				// This is a simplified approach - actual parsing would be more complex
				versionRe := regexp.MustCompile(`from:\s*"([^"]+)"`)
				if versionMatch := versionRe.FindStringSubmatch(depsText); len(versionMatch) > 1 {
					dep.Version = ">=" + versionMatch[1]
				}

				// Check for exact version
				exactRe := regexp.MustCompile(`exact:\s*"([^"]+)"`)
				if exactMatch := exactRe.FindStringSubmatch(depsText); len(exactMatch) > 1 {
					dep.Version = exactMatch[1]
				}

				// Check for branch
				branchRe := regexp.MustCompile(`branch:\s*"([^"]+)"`)
				if branchMatch := branchRe.FindStringSubmatch(depsText); len(branchMatch) > 1 {
					dep.Branch = branchMatch[1]
				}

				// Check for revision
				revisionRe := regexp.MustCompile(`revision:\s*"([^"]+)"`)
				if revisionMatch := revisionRe.FindStringSubmatch(depsText); len(revisionMatch) > 1 {
					dep.Commit = revisionMatch[1]
				}

				dependencies = append(dependencies, dep)
			}
		}
	}

	return dependencies
}

// extractTargets extracts build targets
func (e *Extractor) extractTargets(text string) []Target {
	targets := make([]Target, 0)

	// Find targets array
	re := regexp.MustCompile(`targets:\s*\[(.*)\]`)
	if matches := re.FindStringSubmatch(text); len(matches) > 1 {
		targetsText := matches[1]

		// Extract regular targets
		targetRe := regexp.MustCompile(`\.target\(\s*name:\s*"([^"]+)"`)
		for _, match := range targetRe.FindAllStringSubmatch(targetsText, -1) {
			if len(match) > 1 {
				targets = append(targets, Target{
					Name: match[1],
					Type: "target",
				})
			}
		}

		// Extract test targets
		testRe := regexp.MustCompile(`\.testTarget\(\s*name:\s*"([^"]+)"`)
		for _, match := range testRe.FindAllStringSubmatch(targetsText, -1) {
			if len(match) > 1 {
				targets = append(targets, Target{
					Name: match[1],
					Type: "testTarget",
				})
			}
		}

		// Extract binary targets
		binaryRe := regexp.MustCompile(`\.binaryTarget\(\s*name:\s*"([^"]+)"`)
		for _, match := range binaryRe.FindAllStringSubmatch(targetsText, -1) {
			if len(match) > 1 {
				targets = append(targets, Target{
					Name: match[1],
					Type: "binaryTarget",
				})
			}
		}
	}

	return targets
}

// extractFieldValue extracts a simple field value
func (e *Extractor) extractFieldValue(text, field string) string {
	re := regexp.MustCompile(field + `:\s*\.(\w+)`)
	if matches := re.FindStringSubmatch(text); len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// parseStringArray parses a comma-separated list of quoted strings
func (e *Extractor) parseStringArray(text string) []string {
	result := make([]string, 0)
	re := regexp.MustCompile(`"([^"]+)"`)
	for _, match := range re.FindAllStringSubmatch(text, -1) {
		if len(match) > 1 {
			result = append(result, match[1])
		}
	}
	return result
}

// extractNameFromURL extracts a package name from a Git URL
func (e *Extractor) extractNameFromURL(url string) string {
	// Remove .git suffix
	url = strings.TrimSuffix(url, ".git")

	// Get the last path component
	parts := strings.Split(url, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}

	return ""
}

// populateMetadata converts PackageManifest to ProjectMetadata
func (e *Extractor) populateMetadata(manifest *PackageManifest, metadata *extractor.ProjectMetadata, projectPath string) {
	// Extract common metadata
	metadata.Name = manifest.Name
	metadata.VersionSource = "Package.swift"

	// Try to extract version from git tags or other sources
	// Note: Swift packages typically use git tags for versioning
	metadata.Version = "" // Swift packages don't have version in Package.swift

	// Swift-specific metadata
	metadata.LanguageSpecific["package_name"] = manifest.Name
	metadata.LanguageSpecific["swift_tools_version"] = manifest.SwiftVersion
	metadata.LanguageSpecific["metadata_source"] = "Package.swift"

	// Platforms
	if len(manifest.Platforms) > 0 {
		platforms := make([]map[string]string, 0, len(manifest.Platforms))
		for _, p := range manifest.Platforms {
			platforms = append(platforms, map[string]string{
				"name":    p.Name,
				"version": p.Version,
			})
		}
		metadata.LanguageSpecific["platforms"] = platforms
		metadata.LanguageSpecific["platform_count"] = len(platforms)
	}

	// Products
	if len(manifest.Products) > 0 {
		products := make([]map[string]interface{}, 0, len(manifest.Products))
		libraryCount := 0
		executableCount := 0

		for _, p := range manifest.Products {
			product := map[string]interface{}{
				"name":    p.Name,
				"type":    p.Type,
				"targets": p.Targets,
			}
			products = append(products, product)

			if p.Type == "library" {
				libraryCount++
			} else if p.Type == "executable" {
				executableCount++
			}
		}

		metadata.LanguageSpecific["products"] = products
		metadata.LanguageSpecific["product_count"] = len(products)
		metadata.LanguageSpecific["library_count"] = libraryCount
		metadata.LanguageSpecific["executable_count"] = executableCount
	}

	// Dependencies
	if len(manifest.Dependencies) > 0 {
		deps := make([]map[string]string, 0, len(manifest.Dependencies))
		for _, d := range manifest.Dependencies {
			dep := map[string]string{
				"name": d.Name,
				"url":  d.URL,
			}
			if d.Version != "" {
				dep["version"] = d.Version
			}
			if d.Branch != "" {
				dep["branch"] = d.Branch
			}
			if d.Commit != "" {
				dep["commit"] = d.Commit
			}
			deps = append(deps, dep)
		}
		metadata.LanguageSpecific["dependencies"] = deps
		metadata.LanguageSpecific["dependency_count"] = len(deps)
	}

	// Targets
	if len(manifest.Targets) > 0 {
		targets := make([]map[string]string, 0, len(manifest.Targets))
		testTargetCount := 0

		for _, t := range manifest.Targets {
			targets = append(targets, map[string]string{
				"name": t.Name,
				"type": t.Type,
			})

			if t.Type == "testTarget" {
				testTargetCount++
			}
		}

		metadata.LanguageSpecific["targets"] = targets
		metadata.LanguageSpecific["target_count"] = len(targets)
		metadata.LanguageSpecific["test_target_count"] = testTargetCount
	}

	// Language standards
	if manifest.CLanguageStd != "" {
		metadata.LanguageSpecific["c_language_standard"] = manifest.CLanguageStd
	}
	if manifest.CXXLanguageStd != "" {
		metadata.LanguageSpecific["cxx_language_standard"] = manifest.CXXLanguageStd
	}

	// Generate Swift version matrix
	if manifest.SwiftVersion != "" {
		matrix := generateSwiftVersionMatrix(manifest.SwiftVersion)
		if len(matrix) > 0 {
			metadata.LanguageSpecific["swift_version_matrix"] = matrix
			matrixJSON := fmt.Sprintf(`{"swift-version": [%s]}`,
				strings.Join(quoteStrings(matrix), ", "))
			metadata.LanguageSpecific["matrix_json"] = matrixJSON
		}
	}

	// Determine package type
	hasExecutable := false
	hasLibrary := false
	for _, p := range manifest.Products {
		if p.Type == "executable" {
			hasExecutable = true
		} else if p.Type == "library" {
			hasLibrary = true
		}
	}

	if hasLibrary {
		metadata.LanguageSpecific["is_library"] = true
	}
	if hasExecutable {
		metadata.LanguageSpecific["is_executable"] = true
	}
}

// Detect checks if this extractor can handle the project
func (e *Extractor) Detect(projectPath string) bool {
	// Check for Package.swift
	packagePath := filepath.Join(projectPath, "Package.swift")
	if _, err := os.Stat(packagePath); err == nil {
		return true
	}

	return false
}

// Helper functions

// generateSwiftVersionMatrix generates a list of Swift versions from a tools version
func generateSwiftVersionMatrix(toolsVersion string) []string {
	versions := []string{}

	// Parse the tools version
	parts := strings.Split(toolsVersion, ".")
	if len(parts) < 2 {
		return []string{"5.9", "5.10"}
	}

	major := parts[0]
	minor := parts[1]
	minVersion := major + "." + minor

	// Map minimum version to supported versions
	// Only includes actively supported Swift versions (5.9+)
	// Swift 5.8 and earlier are no longer actively supported
	supportedVersions := map[string][]string{
		"5.9":  {"5.9", "5.10", "5.11", "6.0", "6.1"},
		"5.10": {"5.10", "5.11", "6.0", "6.1"},
		"5.11": {"5.11", "6.0", "6.1"},
		"6.0":  {"6.0", "6.1"},
		"6.1":  {"6.1"},
	}

	if versionList, ok := supportedVersions[minVersion]; ok {
		versions = versionList
	}

	// Default to recent versions if not found or for legacy versions
	if len(versions) == 0 {
		// For legacy versions (< 5.9), map to supported versions
		if minVersion < "5.9" {
			versions = []string{"5.9", "5.10", "5.11", "6.0", "6.1"}
		} else {
			versions = []string{"5.10", "5.11", "6.0", "6.1"}
		}
	}

	return versions
}

// quoteStrings adds quotes around each string
func quoteStrings(strs []string) []string {
	quoted := make([]string, len(strs))
	for i, s := range strs {
		quoted[i] = fmt.Sprintf(`"%s"`, s)
	}
	return quoted
}

// init registers the Swift extractor
func init() {
	extractor.RegisterExtractor(NewExtractor())
}
