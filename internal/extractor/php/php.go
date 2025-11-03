// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package php

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/lfreleng-actions/build-metadata-action/internal/extractor"
)

// Extractor extracts metadata from PHP projects
type Extractor struct {
	extractor.BaseExtractor
}

// NewExtractor creates a new PHP extractor
func NewExtractor() *Extractor {
	return &Extractor{
		BaseExtractor: extractor.NewBaseExtractor("php", 1),
	}
}

// ComposerJSON represents the structure of a composer.json file
type ComposerJSON struct {
	Name             string                 `json:"name"`
	Description      string                 `json:"description"`
	Version          string                 `json:"version"`
	Type             string                 `json:"type"`
	Keywords         []string               `json:"keywords"`
	Homepage         string                 `json:"homepage"`
	License          interface{}            `json:"license"` // Can be string or array
	Authors          []Author               `json:"authors"`
	Support          Support                `json:"support"`
	Require          map[string]string      `json:"require"`
	RequireDev       map[string]string      `json:"require-dev"`
	Conflict         map[string]string      `json:"conflict"`
	Replace          map[string]string      `json:"replace"`
	Provide          map[string]string      `json:"provide"`
	Suggest          map[string]string      `json:"suggest"`
	Autoload         Autoload               `json:"autoload"`
	AutoloadDev      Autoload               `json:"autoload-dev"`
	MinimumStability string                 `json:"minimum-stability"`
	PreferStable     bool                   `json:"prefer-stable"`
	Repositories     []interface{}          `json:"repositories"`
	Config           map[string]interface{} `json:"config"`
	Scripts          map[string]interface{} `json:"scripts"`
	Extra            map[string]interface{} `json:"extra"`
	Bin              []string               `json:"bin"`
	Archive          Archive                `json:"archive"`
}

// Author represents a composer author
type Author struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Homepage string `json:"homepage"`
	Role     string `json:"role"`
}

// Support contains support information
type Support struct {
	Email  string `json:"email"`
	Issues string `json:"issues"`
	Forum  string `json:"forum"`
	Source string `json:"source"`
	Docs   string `json:"docs"`
	Wiki   string `json:"wiki"`
	IRC    string `json:"irc"`
	Chat   string `json:"chat"`
}

// Autoload represents autoload configuration
type Autoload struct {
	PSR0                map[string]interface{} `json:"psr-0"`
	PSR4                map[string]interface{} `json:"psr-4"`
	Classmap            []string               `json:"classmap"`
	Files               []string               `json:"files"`
	ExcludeFromClassmap []string               `json:"exclude-from-classmap"`
}

// Archive represents archive configuration
type Archive struct {
	Name    string   `json:"name"`
	Exclude []string `json:"exclude"`
}

// Extract retrieves metadata from a PHP project
func (e *Extractor) Extract(projectPath string) (*extractor.ProjectMetadata, error) {
	metadata := &extractor.ProjectMetadata{
		LanguageSpecific: make(map[string]interface{}),
	}

	// Look for composer.json
	composerPath := filepath.Join(projectPath, "composer.json")
	if _, err := os.Stat(composerPath); err != nil {
		return nil, fmt.Errorf("composer.json not found in %s", projectPath)
	}

	if err := e.extractFromComposerJSON(composerPath, metadata); err != nil {
		return nil, err
	}

	return metadata, nil
}

// extractFromComposerJSON extracts metadata from composer.json
func (e *Extractor) extractFromComposerJSON(path string, metadata *extractor.ProjectMetadata) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read composer.json: %w", err)
	}

	var composer ComposerJSON
	if err := json.Unmarshal(content, &composer); err != nil {
		return fmt.Errorf("failed to parse composer.json: %w", err)
	}

	// Extract common metadata
	metadata.Name = composer.Name
	metadata.Version = composer.Version
	metadata.Description = composer.Description
	metadata.Homepage = composer.Homepage
	metadata.VersionSource = "composer.json"

	// Extract license (handle both string and array)
	if composer.License != nil {
		switch v := composer.License.(type) {
		case string:
			metadata.License = v
		case []interface{}:
			licenses := make([]string, 0, len(v))
			for _, lic := range v {
				if licStr, ok := lic.(string); ok {
					licenses = append(licenses, licStr)
				}
			}
			if len(licenses) > 0 {
				metadata.License = strings.Join(licenses, ", ")
			}
		}
	}

	// Extract authors
	authors := make([]string, 0, len(composer.Authors))
	for _, author := range composer.Authors {
		if author.Name != "" {
			if author.Email != "" {
				authors = append(authors, fmt.Sprintf("%s <%s>", author.Name, author.Email))
			} else {
				authors = append(authors, author.Name)
			}
		}
	}
	metadata.Authors = authors

	// Extract repository from support
	if composer.Support.Source != "" {
		metadata.Repository = composer.Support.Source
	}

	// PHP-specific metadata
	metadata.LanguageSpecific["package_name"] = composer.Name
	metadata.LanguageSpecific["package_type"] = composer.Type
	metadata.LanguageSpecific["metadata_source"] = "composer.json"

	if len(composer.Keywords) > 0 {
		metadata.LanguageSpecific["keywords"] = composer.Keywords
	}

	// Extract PHP version requirement
	if phpVersion, ok := composer.Require["php"]; ok {
		metadata.LanguageSpecific["requires_php"] = phpVersion

		// Generate PHP version matrix
		matrix := generatePHPVersionMatrix(phpVersion)
		if len(matrix) > 0 {
			metadata.LanguageSpecific["php_version_matrix"] = matrix
			matrixJSON := fmt.Sprintf(`{"php-version": [%s]}`,
				strings.Join(quoteStrings(matrix), ", "))
			metadata.LanguageSpecific["matrix_json"] = matrixJSON
		}
	}

	// Extract dependencies
	if len(composer.Require) > 0 {
		deps := make(map[string]string)
		for pkg, version := range composer.Require {
			if pkg != "php" && !strings.HasPrefix(pkg, "ext-") {
				deps[pkg] = version
			}
		}
		if len(deps) > 0 {
			metadata.LanguageSpecific["dependencies"] = deps
			metadata.LanguageSpecific["dependency_count"] = len(deps)
		}
	}

	// Extract dev dependencies
	if len(composer.RequireDev) > 0 {
		metadata.LanguageSpecific["dev_dependencies"] = composer.RequireDev
		metadata.LanguageSpecific["dev_dependency_count"] = len(composer.RequireDev)
	}

	// Extract PHP extensions
	extensions := make([]string, 0)
	for pkg := range composer.Require {
		if strings.HasPrefix(pkg, "ext-") {
			extensions = append(extensions, strings.TrimPrefix(pkg, "ext-"))
		}
	}
	if len(extensions) > 0 {
		metadata.LanguageSpecific["php_extensions"] = extensions
		metadata.LanguageSpecific["extension_count"] = len(extensions)
	}

	// Autoload information
	hasAutoload := false
	autoloadTypes := make([]string, 0)

	if len(composer.Autoload.PSR4) > 0 {
		hasAutoload = true
		autoloadTypes = append(autoloadTypes, "psr-4")
		metadata.LanguageSpecific["psr4_namespaces"] = composer.Autoload.PSR4
	}

	if len(composer.Autoload.PSR0) > 0 {
		hasAutoload = true
		autoloadTypes = append(autoloadTypes, "psr-0")
		metadata.LanguageSpecific["psr0_namespaces"] = composer.Autoload.PSR0
	}

	if len(composer.Autoload.Classmap) > 0 {
		hasAutoload = true
		autoloadTypes = append(autoloadTypes, "classmap")
		metadata.LanguageSpecific["classmap_paths"] = composer.Autoload.Classmap
	}

	if len(composer.Autoload.Files) > 0 {
		hasAutoload = true
		autoloadTypes = append(autoloadTypes, "files")
		metadata.LanguageSpecific["autoload_files"] = composer.Autoload.Files
	}

	if hasAutoload {
		metadata.LanguageSpecific["autoload_types"] = autoloadTypes
	}

	// Stability preferences
	if composer.MinimumStability != "" {
		metadata.LanguageSpecific["minimum_stability"] = composer.MinimumStability
	}

	metadata.LanguageSpecific["prefer_stable"] = composer.PreferStable

	// Scripts
	if len(composer.Scripts) > 0 {
		scriptNames := make([]string, 0, len(composer.Scripts))
		for name := range composer.Scripts {
			scriptNames = append(scriptNames, name)
		}
		metadata.LanguageSpecific["scripts"] = scriptNames
		metadata.LanguageSpecific["script_count"] = len(scriptNames)
	}

	// Binary files
	if len(composer.Bin) > 0 {
		metadata.LanguageSpecific["binaries"] = composer.Bin
	}

	// Support information
	if composer.Support.Issues != "" {
		metadata.LanguageSpecific["issues_url"] = composer.Support.Issues
	}
	if composer.Support.Docs != "" {
		metadata.LanguageSpecific["docs_url"] = composer.Support.Docs
	}

	// Detect framework
	framework := detectPHPFramework(composer.Require)
	if framework != "" {
		metadata.LanguageSpecific["framework"] = framework
	}

	// Detect if this is a library or application
	packageType := composer.Type
	if packageType == "" {
		packageType = "library" // Default type
	}
	metadata.LanguageSpecific["is_library"] = (packageType == "library")

	return nil
}

// Detect checks if this extractor can handle the project
func (e *Extractor) Detect(projectPath string) bool {
	// Check for composer.json
	composerPath := filepath.Join(projectPath, "composer.json")
	if _, err := os.Stat(composerPath); err == nil {
		return true
	}

	return false
}

// Helper functions

// generatePHPVersionMatrix generates a list of PHP versions from a constraint
func generatePHPVersionMatrix(phpVersion string) []string {
	versions := []string{}

	// Clean up the version string
	phpVersion = strings.TrimSpace(phpVersion)

	// Extract minimum version
	minVersion := ""

	// Handle >= constraints
	if strings.Contains(phpVersion, ">=") {
		re := regexp.MustCompile(`>=\s*(\d+\.\d+)`)
		if matches := re.FindStringSubmatch(phpVersion); len(matches) > 1 {
			minVersion = matches[1]
		}
	} else if strings.HasPrefix(phpVersion, "^") {
		// Caret constraint (e.g., ^7.4 or ^8.0)
		version := strings.TrimPrefix(phpVersion, "^")
		parts := strings.Split(version, ".")
		if len(parts) >= 2 {
			minVersion = parts[0] + "." + parts[1]
		}
	} else if strings.HasPrefix(phpVersion, "~") {
		// Tilde constraint (e.g., ~7.4 or ~8.0)
		version := strings.TrimPrefix(phpVersion, "~")
		parts := strings.Split(version, ".")
		if len(parts) >= 2 {
			minVersion = parts[0] + "." + parts[1]
		}
	}

	// Map minimum version to supported versions
	// Only includes actively supported PHP versions (8.1+)
	// PHP 7.2, 7.3, 7.4, and 8.0 have reached end-of-life
	supportedVersions := map[string][]string{
		"8.1": {"8.1", "8.2", "8.3"},
		"8.2": {"8.2", "8.3"},
		"8.3": {"8.3"},
		"8.4": {"8.4"}, // Future-proofing for PHP 8.4
	}

	if minVersion != "" {
		if versionList, ok := supportedVersions[minVersion]; ok {
			versions = versionList
		} else {
			// Map legacy/unsupported versions to minimum supported version
			// This handles projects still requiring PHP 7.x or 8.0
			switch {
			case minVersion < "8.1":
				versions = []string{"8.1", "8.2", "8.3"}
			default:
				versions = []string{"8.1", "8.2", "8.3"}
			}
		}
	}

	// If we couldn't determine, use reasonable defaults
	if len(versions) == 0 {
		versions = []string{"8.1", "8.2", "8.3"}
	}

	return versions
}

// detectPHPFramework attempts to detect which PHP framework is being used
func detectPHPFramework(requirements map[string]string) string {
	frameworkPatterns := map[string]string{
		"laravel/framework":           "Laravel",
		"symfony/symfony":             "Symfony",
		"symfony/framework-bundle":    "Symfony",
		"cakephp/cakephp":             "CakePHP",
		"yiisoft/yii2":                "Yii2",
		"codeigniter4/framework":      "CodeIgniter",
		"slim/slim":                   "Slim",
		"laminas/laminas-mvc":         "Laminas",
		"zendframework/zendframework": "Zend Framework",
		"phalcon/cphalcon":            "Phalcon",
		"drupal/core":                 "Drupal",
		"wordpress/wordpress":         "WordPress",
	}

	for pkg, framework := range frameworkPatterns {
		if _, exists := requirements[pkg]; exists {
			return framework
		}
	}

	return ""
}

// quoteStrings adds quotes around each string
func quoteStrings(strs []string) []string {
	quoted := make([]string, len(strs))
	for i, s := range strs {
		quoted[i] = fmt.Sprintf(`"%s"`, s)
	}
	return quoted
}

// init registers the PHP extractor
func init() {
	extractor.RegisterExtractor(NewExtractor())
}
