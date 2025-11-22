// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package dart

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/lfreleng-actions/build-metadata-action/internal/extractor"
)

// Extractor extracts metadata from Dart/Flutter projects
type Extractor struct {
	extractor.BaseExtractor
}

// NewExtractor creates a new Dart extractor
func NewExtractor() *Extractor {
	return &Extractor{
		BaseExtractor: extractor.NewBaseExtractor("dart", 1),
	}
}

// PubspecYAML represents the structure of a pubspec.yaml file
type PubspecYAML struct {
	Name                string                 `yaml:"name"`
	Description         string                 `yaml:"description"`
	Version             string                 `yaml:"version"`
	Homepage            string                 `yaml:"homepage"`
	Repository          string                 `yaml:"repository"`
	IssueTracker        string                 `yaml:"issue_tracker"`
	Documentation       string                 `yaml:"documentation"`
	Publish             interface{}            `yaml:"publish_to"`
	Environment         Environment            `yaml:"environment"`
	Dependencies        map[string]interface{} `yaml:"dependencies"`
	DevDependencies     map[string]interface{} `yaml:"dev_dependencies"`
	DependencyOverrides map[string]interface{} `yaml:"dependency_overrides"`
	Flutter             FlutterConfig          `yaml:"flutter"`
	Executables         map[string]string      `yaml:"executables"`
	Funding             []string               `yaml:"funding"`
	Screenshots         []Screenshot           `yaml:"screenshots"`
	Topics              []string               `yaml:"topics"`
	FalseSecrets        []string               `yaml:"false_secrets"`
	Platforms           map[string]interface{} `yaml:"platforms"`
}

// Environment represents SDK constraints
type Environment struct {
	SDK     string `yaml:"sdk"`
	Flutter string `yaml:"flutter"`
}

// FlutterConfig represents Flutter-specific configuration
type FlutterConfig struct {
	UseMaterialDesign bool         `yaml:"uses-material-design"`
	Generate          bool         `yaml:"generate"`
	Assets            []string     `yaml:"assets"`
	Fonts             []FontConfig `yaml:"fonts"`
	Plugin            PluginConfig `yaml:"plugin"`
	Module            ModuleConfig `yaml:"module"`
}

// FontConfig represents a font configuration
type FontConfig struct {
	Family string      `yaml:"family"`
	Fonts  []FontAsset `yaml:"fonts"`
}

// FontAsset represents a font asset
type FontAsset struct {
	Asset  string `yaml:"asset"`
	Weight int    `yaml:"weight"`
	Style  string `yaml:"style"`
}

// PluginConfig represents Flutter plugin configuration
type PluginConfig struct {
	Platforms map[string]interface{} `yaml:"platforms"`
}

// ModuleConfig represents Flutter module configuration
type ModuleConfig struct {
	AndroidX       bool   `yaml:"androidX"`
	AndroidPackage string `yaml:"androidPackage"`
	IOSBundleID    string `yaml:"iosBundleIdentifier"`
}

// Screenshot represents a screenshot entry
type Screenshot struct {
	Description string `yaml:"description"`
	Path        string `yaml:"path"`
}

// Extract retrieves metadata from a Dart/Flutter project
func (e *Extractor) Extract(projectPath string) (*extractor.ProjectMetadata, error) {
	metadata := &extractor.ProjectMetadata{
		LanguageSpecific: make(map[string]interface{}),
	}

	// Look for pubspec.yaml
	pubspecPath := filepath.Join(projectPath, "pubspec.yaml")
	if _, err := os.Stat(pubspecPath); err != nil {
		return nil, fmt.Errorf("pubspec.yaml not found in %s", projectPath)
	}

	if err := e.extractFromPubspec(pubspecPath, metadata); err != nil {
		return nil, err
	}

	return metadata, nil
}

// extractFromPubspec extracts metadata from pubspec.yaml
func (e *Extractor) extractFromPubspec(path string, metadata *extractor.ProjectMetadata) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read pubspec.yaml: %w", err)
	}

	var pubspec PubspecYAML
	if err := yaml.Unmarshal(content, &pubspec); err != nil {
		return fmt.Errorf("failed to parse pubspec.yaml: %w", err)
	}

	// Extract common metadata
	metadata.Name = pubspec.Name
	metadata.Version = pubspec.Version
	metadata.Description = pubspec.Description
	metadata.Homepage = pubspec.Homepage
	metadata.Repository = pubspec.Repository
	metadata.VersionSource = "pubspec.yaml"

	// Dart/Flutter-specific metadata
	metadata.LanguageSpecific["package_name"] = pubspec.Name
	metadata.LanguageSpecific["metadata_source"] = "pubspec.yaml"

	// Detect if this is a Flutter project
	isFlutter := false
	if _, hasFlutterDep := pubspec.Dependencies["flutter"]; hasFlutterDep {
		isFlutter = true
		metadata.LanguageSpecific["is_flutter"] = true
		metadata.LanguageSpecific["framework"] = "Flutter"
	} else {
		metadata.LanguageSpecific["is_flutter"] = false
		metadata.LanguageSpecific["framework"] = "Dart"
	}

	// Extract SDK constraints
	if pubspec.Environment.SDK != "" {
		metadata.LanguageSpecific["dart_sdk"] = pubspec.Environment.SDK

		// Generate Dart version matrix
		matrix := generateDartVersionMatrix(pubspec.Environment.SDK)
		if len(matrix) > 0 {
			metadata.LanguageSpecific["dart_version_matrix"] = matrix
			matrixJSON := fmt.Sprintf(`{"dart-version": [%s]}`,
				strings.Join(quoteStrings(matrix), ", "))
			metadata.LanguageSpecific["matrix_json"] = matrixJSON
		}
	}

	if pubspec.Environment.Flutter != "" {
		metadata.LanguageSpecific["flutter_sdk"] = pubspec.Environment.Flutter
	}

	// Extract dependencies
	if len(pubspec.Dependencies) > 0 {
		deps := make(map[string]string)
		for name, constraint := range pubspec.Dependencies {
			// Skip SDK dependencies
			if name == "flutter" || name == "flutter_test" {
				continue
			}

			// Convert constraint to string
			constraintStr := ""
			switch v := constraint.(type) {
			case string:
				constraintStr = v
			case map[string]interface{}:
				// Handle complex dependency specifications
				if version, ok := v["version"].(string); ok {
					constraintStr = version
				} else if path, ok := v["path"].(string); ok {
					constraintStr = "path: " + path
				} else if git, ok := v["git"].(map[string]interface{}); ok {
					if url, ok := git["url"].(string); ok {
						constraintStr = "git: " + url
					}
				}
			}

			if constraintStr != "" {
				deps[name] = constraintStr
			}
		}

		if len(deps) > 0 {
			metadata.LanguageSpecific["dependencies"] = deps
			metadata.LanguageSpecific["dependency_count"] = len(deps)
		}
	}

	// Extract dev dependencies
	if len(pubspec.DevDependencies) > 0 {
		devDeps := make(map[string]string)
		for name, constraint := range pubspec.DevDependencies {
			if name == "flutter_test" {
				continue
			}

			constraintStr := ""
			switch v := constraint.(type) {
			case string:
				constraintStr = v
			case map[string]interface{}:
				if version, ok := v["version"].(string); ok {
					constraintStr = version
				}
			}

			if constraintStr != "" {
				devDeps[name] = constraintStr
			}
		}

		if len(devDeps) > 0 {
			metadata.LanguageSpecific["dev_dependencies"] = devDeps
			metadata.LanguageSpecific["dev_dependency_count"] = len(devDeps)
		}
	}

	// Publishing information
	if pubspec.Publish != nil {
		switch v := pubspec.Publish.(type) {
		case string:
			metadata.LanguageSpecific["publish_to"] = v
			if v == "none" {
				metadata.LanguageSpecific["is_publishable"] = false
			} else {
				metadata.LanguageSpecific["is_publishable"] = true
			}
		case bool:
			metadata.LanguageSpecific["is_publishable"] = v
		}
	} else {
		metadata.LanguageSpecific["is_publishable"] = true
	}

	// URLs
	if pubspec.IssueTracker != "" {
		metadata.LanguageSpecific["issue_tracker"] = pubspec.IssueTracker
	}
	if pubspec.Documentation != "" {
		metadata.LanguageSpecific["documentation"] = pubspec.Documentation
	}

	// Topics
	if len(pubspec.Topics) > 0 {
		metadata.LanguageSpecific["topics"] = pubspec.Topics
	}

	// Funding
	if len(pubspec.Funding) > 0 {
		metadata.LanguageSpecific["funding"] = pubspec.Funding
	}

	// Executables
	if len(pubspec.Executables) > 0 {
		metadata.LanguageSpecific["executables"] = pubspec.Executables
		metadata.LanguageSpecific["executable_count"] = len(pubspec.Executables)
	}

	// Flutter-specific metadata
	if isFlutter {
		e.extractFlutterMetadata(&pubspec, metadata)
	}

	// Determine package type
	packageType := "library" // Default
	if len(pubspec.Executables) > 0 {
		packageType = "application"
	}

	// Check if it's a Flutter plugin
	if len(pubspec.Flutter.Plugin.Platforms) > 0 {
		packageType = "plugin"
		metadata.LanguageSpecific["is_flutter_plugin"] = true
	} else {
		metadata.LanguageSpecific["is_flutter_plugin"] = false
	}

	metadata.LanguageSpecific["package_type"] = packageType

	return nil
}

// extractFlutterMetadata extracts Flutter-specific metadata
func (e *Extractor) extractFlutterMetadata(pubspec *PubspecYAML, metadata *extractor.ProjectMetadata) {
	flutter := pubspec.Flutter

	if flutter.UseMaterialDesign {
		metadata.LanguageSpecific["uses_material_design"] = true
	}

	if flutter.Generate {
		metadata.LanguageSpecific["uses_code_generation"] = true
	}

	// Assets
	if len(flutter.Assets) > 0 {
		metadata.LanguageSpecific["assets"] = flutter.Assets
		metadata.LanguageSpecific["asset_count"] = len(flutter.Assets)
	}

	// Fonts
	if len(flutter.Fonts) > 0 {
		fontFamilies := make([]string, 0, len(flutter.Fonts))
		for _, font := range flutter.Fonts {
			fontFamilies = append(fontFamilies, font.Family)
		}
		metadata.LanguageSpecific["custom_fonts"] = fontFamilies
		metadata.LanguageSpecific["font_count"] = len(fontFamilies)
	}

	// Plugin platforms
	if len(flutter.Plugin.Platforms) > 0 {
		platforms := make([]string, 0, len(flutter.Plugin.Platforms))
		for platform := range flutter.Plugin.Platforms {
			platforms = append(platforms, platform)
		}
		metadata.LanguageSpecific["plugin_platforms"] = platforms
		metadata.LanguageSpecific["plugin_platform_count"] = len(platforms)
	}

	// Module configuration
	if flutter.Module.AndroidPackage != "" {
		metadata.LanguageSpecific["android_package"] = flutter.Module.AndroidPackage
		metadata.LanguageSpecific["uses_androidx"] = flutter.Module.AndroidX
	}

	if flutter.Module.IOSBundleID != "" {
		metadata.LanguageSpecific["ios_bundle_id"] = flutter.Module.IOSBundleID
	}
}

// Detect checks if this extractor can handle the project
func (e *Extractor) Detect(projectPath string) bool {
	// Check for pubspec.yaml
	pubspecPath := filepath.Join(projectPath, "pubspec.yaml")
	if _, err := os.Stat(pubspecPath); err == nil {
		return true
	}

	return false
}

// Helper functions

// generateDartVersionMatrix generates a list of Dart versions from a constraint
func generateDartVersionMatrix(sdkConstraint string) []string {
	versions := []string{}

	// Clean up the constraint
	sdkConstraint = strings.TrimSpace(sdkConstraint)

	// Extract minimum version
	minVersion := ""

	// Handle >= constraints
	if strings.Contains(sdkConstraint, ">=") {
		re := regexp.MustCompile(`>=\s*(\d+\.\d+)`)
		if matches := re.FindStringSubmatch(sdkConstraint); len(matches) > 1 {
			minVersion = matches[1]
		}
	} else if strings.HasPrefix(sdkConstraint, "^") {
		// Caret constraint
		version := strings.TrimPrefix(sdkConstraint, "^")
		parts := strings.Split(version, ".")
		if len(parts) >= 2 {
			minVersion = parts[0] + "." + parts[1]
		}
	}

	// Map minimum version to supported versions
	supportedVersions := map[string][]string{
		"2.12": {"2.12", "2.13", "2.14", "2.15", "2.16", "2.17", "2.18", "2.19", "3.0", "3.1", "3.2"},
		"2.13": {"2.13", "2.14", "2.15", "2.16", "2.17", "2.18", "2.19", "3.0", "3.1", "3.2"},
		"2.14": {"2.14", "2.15", "2.16", "2.17", "2.18", "2.19", "3.0", "3.1", "3.2"},
		"2.15": {"2.15", "2.16", "2.17", "2.18", "2.19", "3.0", "3.1", "3.2"},
		"2.16": {"2.16", "2.17", "2.18", "2.19", "3.0", "3.1", "3.2"},
		"2.17": {"2.17", "2.18", "2.19", "3.0", "3.1", "3.2"},
		"2.18": {"2.18", "2.19", "3.0", "3.1", "3.2"},
		"2.19": {"2.19", "3.0", "3.1", "3.2"},
		"3.0":  {"3.0", "3.1", "3.2", "3.3"},
		"3.1":  {"3.1", "3.2", "3.3"},
		"3.2":  {"3.2", "3.3"},
		"3.3":  {"3.3"},
	}

	if minVersion != "" {
		if versionList, ok := supportedVersions[minVersion]; ok {
			versions = versionList
		}
	}

	// Default to recent stable versions
	if len(versions) == 0 {
		versions = []string{"3.1", "3.2", "3.3"}
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

// init registers the Dart extractor
func init() {
	extractor.RegisterExtractor(NewExtractor())
}
