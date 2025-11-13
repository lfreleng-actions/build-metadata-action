// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package ruby

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

// Extractor extracts metadata from Ruby projects
type Extractor struct {
	extractor.BaseExtractor
}

// NewExtractor creates a new Ruby extractor
func NewExtractor() *Extractor {
	return &Extractor{
		BaseExtractor: extractor.NewBaseExtractor("ruby", 1),
	}
}

func init() {
	extractor.RegisterExtractor(NewExtractor())
}

// GemspecMetadata represents parsed gemspec metadata
type GemspecMetadata struct {
	Name                    string
	Version                 string
	Authors                 []string
	Email                   []string
	Summary                 string
	Description             string
	Homepage                string
	License                 string
	RequiredRubyVersion     string
	RuntimeDependencies     []Dependency
	DevelopmentDependencies []Dependency
	Platform                string
}

// Dependency represents a gem dependency
type Dependency struct {
	Name        string
	Requirement string
	Type        string // "runtime" or "development"
}

// GemfileMetadata represents parsed Gemfile metadata
type GemfileMetadata struct {
	RubyVersion  string
	Source       string
	Dependencies []Dependency
	Groups       map[string][]Dependency
	HasBundler   bool
	Platforms    []string
}

// Detect checks if this is a Ruby project
func (e *Extractor) Detect(projectPath string) bool {
	// Check for common Ruby project files
	indicators := []string{
		"*.gemspec",
		"Gemfile",
		"Rakefile",
		"config.ru",
		".ruby-version",
	}

	for _, pattern := range indicators {
		matches, err := filepath.Glob(filepath.Join(projectPath, pattern))
		if err == nil && len(matches) > 0 {
			return true
		}
	}

	// Check for Ruby files in lib/ directory
	libPath := filepath.Join(projectPath, "lib")
	if info, err := os.Stat(libPath); err == nil && info.IsDir() {
		entries, err := os.ReadDir(libPath)
		if err == nil {
			for _, entry := range entries {
				if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".rb") {
					return true
				}
			}
		}
	}

	return false
}

// Extract retrieves metadata from a Ruby project
func (e *Extractor) Extract(projectPath string) (*extractor.ProjectMetadata, error) {
	metadata := &extractor.ProjectMetadata{
		LanguageSpecific: make(map[string]interface{}),
	}

	// Try to extract from gemspec first
	gemspecPath, err := e.findGemspec(projectPath)
	if err == nil && gemspecPath != "" {
		if err := e.extractFromGemspec(gemspecPath, metadata); err != nil {
			// Continue with Gemfile if gemspec fails
		}
	}

	// Extract from Gemfile
	gemfilePath := filepath.Join(projectPath, "Gemfile")
	if _, err := os.Stat(gemfilePath); err == nil {
		if err := e.extractFromGemfile(gemfilePath, metadata); err != nil {
			// Non-fatal error, continue
		}
	}

	// Extract Ruby version from .ruby-version
	rubyVersionPath := filepath.Join(projectPath, ".ruby-version")
	if _, err := os.Stat(rubyVersionPath); err == nil {
		if version, err := e.extractRubyVersion(rubyVersionPath); err == nil {
			metadata.LanguageSpecific["ruby_version"] = version
		}
	}

	// Detect frameworks
	frameworks := e.detectFrameworks(projectPath)
	if len(frameworks) > 0 {
		metadata.LanguageSpecific["ruby_frameworks"] = frameworks
	}

	// Ensure we have at least some metadata
	if metadata.Name == "" && metadata.Version == "" && len(metadata.LanguageSpecific) == 0 {
		return nil, fmt.Errorf("no Ruby metadata found in project")
	}

	return metadata, nil
}

// findGemspec locates the gemspec file in the project
func (e *Extractor) findGemspec(projectPath string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(projectPath, "*.gemspec"))
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no gemspec file found")
	}
	return matches[0], nil
}

// extractFromGemspec parses a gemspec file
func (e *Extractor) extractFromGemspec(gemspecPath string, metadata *extractor.ProjectMetadata) error {
	file, err := os.Open(gemspecPath)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var (
		name                string
		version             string
		authors             []string
		email               []string
		summary             string
		description         string
		homepage            string
		license             string
		requiredRubyVersion string
		runtimeDeps         []Dependency
		devDeps             []Dependency
		platform            string
	)

	// Regular expressions for parsing gemspec
	nameRe := regexp.MustCompile(`(?:spec|s)\.name\s*=\s*["']([^"']+)["']`)
	versionRe := regexp.MustCompile(`(?:spec|s)\.version\s*=\s*["']([^"']+)["']`)
	authorsRe := regexp.MustCompile(`(?:spec|s)\.authors?\s*=\s*(?:\[["']([^"']+)["']\]|["']([^"']+)["'])`)
	emailRe := regexp.MustCompile(`(?:spec|s)\.email\s*=\s*(?:\[["']([^"']+)["']\]|["']([^"']+)["'])`)
	summaryRe := regexp.MustCompile(`(?:spec|s)\.summary\s*=\s*["']([^"']+)["']`)
	descriptionRe := regexp.MustCompile(`(?:spec|s)\.description\s*=\s*["']([^"']+)["']`)
	homepageRe := regexp.MustCompile(`(?:spec|s)\.homepage\s*=\s*["']([^"']+)["']`)
	licenseRe := regexp.MustCompile(`(?:spec|s)\.licen[cs]e\s*=\s*["']([^"']+)["']`)
	rubyVersionRe := regexp.MustCompile(`(?:spec|s)\.required_ruby_version\s*=\s*["']([^"']+)["']`)
	platformRe := regexp.MustCompile(`(?:spec|s)\.platform\s*=\s*["']?([^"'\s]+)["']?`)

	// Dependency regexes
	runtimeDepRe := regexp.MustCompile(`(?:spec|s)\.add_(?:runtime_)?dependency\s*["']([^"']+)["'](?:\s*,\s*["']([^"']+)["'])?`)
	devDepRe := regexp.MustCompile(`(?:spec|s)\.add_development_dependency\s*["']([^"']+)["'](?:\s*,\s*["']([^"']+)["'])?`)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip comments and empty lines
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}

		// Extract name
		if matches := nameRe.FindStringSubmatch(line); len(matches) > 1 {
			name = matches[1]
		}

		// Extract version
		if matches := versionRe.FindStringSubmatch(line); len(matches) > 1 {
			version = matches[1]
		}

		// Extract authors
		if matches := authorsRe.FindStringSubmatch(line); len(matches) > 1 {
			if matches[1] != "" {
				authors = append(authors, matches[1])
			} else if matches[2] != "" {
				authors = append(authors, matches[2])
			}
		}

		// Extract email
		if matches := emailRe.FindStringSubmatch(line); len(matches) > 1 {
			if matches[1] != "" {
				email = append(email, matches[1])
			} else if matches[2] != "" {
				email = append(email, matches[2])
			}
		}

		// Extract summary
		if matches := summaryRe.FindStringSubmatch(line); len(matches) > 1 {
			summary = matches[1]
		}

		// Extract description
		if matches := descriptionRe.FindStringSubmatch(line); len(matches) > 1 {
			description = matches[1]
		}

		// Extract homepage
		if matches := homepageRe.FindStringSubmatch(line); len(matches) > 1 {
			homepage = matches[1]
		}

		// Extract license
		if matches := licenseRe.FindStringSubmatch(line); len(matches) > 1 {
			license = matches[1]
		}

		// Extract required Ruby version
		if matches := rubyVersionRe.FindStringSubmatch(line); len(matches) > 1 {
			requiredRubyVersion = matches[1]
		}

		// Extract platform
		if matches := platformRe.FindStringSubmatch(line); len(matches) > 1 {
			platform = matches[1]
		}

		// Extract runtime dependencies
		if matches := runtimeDepRe.FindStringSubmatch(line); len(matches) > 1 {
			dep := Dependency{
				Name: matches[1],
				Type: "runtime",
			}
			if len(matches) > 2 && matches[2] != "" {
				dep.Requirement = matches[2]
			}
			runtimeDeps = append(runtimeDeps, dep)
		}

		// Extract development dependencies
		if matches := devDepRe.FindStringSubmatch(line); len(matches) > 1 {
			dep := Dependency{
				Name: matches[1],
				Type: "development",
			}
			if len(matches) > 2 && matches[2] != "" {
				dep.Requirement = matches[2]
			}
			devDeps = append(devDeps, dep)
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	// Populate metadata
	metadata.Name = name
	metadata.Version = version
	metadata.Description = description
	metadata.Homepage = homepage
	metadata.License = license
	metadata.Authors = authors

	if summary != "" {
		metadata.LanguageSpecific["ruby_summary"] = summary
	}
	if len(email) > 0 {
		metadata.LanguageSpecific["ruby_email"] = email
	}
	if requiredRubyVersion != "" {
		metadata.LanguageSpecific["ruby_required_ruby_version"] = requiredRubyVersion
	}
	if platform != "" {
		metadata.LanguageSpecific["ruby_platform"] = platform
	}
	if len(runtimeDeps) > 0 {
		metadata.LanguageSpecific["ruby_runtime_dependencies"] = runtimeDeps
	}
	if len(devDeps) > 0 {
		metadata.LanguageSpecific["ruby_development_dependencies"] = devDeps
	}

	return nil
}

// extractFromGemfile parses a Gemfile
func (e *Extractor) extractFromGemfile(gemfilePath string, metadata *extractor.ProjectMetadata) error {
	file, err := os.Open(gemfilePath)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var (
		rubyVersion  string
		source       string
		dependencies []Dependency
		hasBundler   bool
		platforms    []string
	)

	// Regular expressions for Gemfile parsing
	rubyRe := regexp.MustCompile(`ruby\s+["']([^"']+)["']`)
	sourceRe := regexp.MustCompile(`source\s+["']([^"']+)["']`)
	gemRe := regexp.MustCompile(`gem\s+["']([^"']+)["'](?:\s*,\s*["']([^"']+)["'])?`)
	platformRe := regexp.MustCompile(`platform\s+:([a-z_]+)`)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip comments and empty lines
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}

		// Extract Ruby version
		if matches := rubyRe.FindStringSubmatch(line); len(matches) > 1 {
			rubyVersion = matches[1]
		}

		// Extract source
		if matches := sourceRe.FindStringSubmatch(line); len(matches) > 1 {
			source = matches[1]
		}

		// Extract gems
		if matches := gemRe.FindStringSubmatch(line); len(matches) > 1 {
			dep := Dependency{
				Name: matches[1],
				Type: "runtime",
			}
			if len(matches) > 2 && matches[2] != "" {
				dep.Requirement = matches[2]
			}
			dependencies = append(dependencies, dep)

			// Check if bundler is explicitly listed
			if matches[1] == "bundler" {
				hasBundler = true
			}
		}

		// Extract platform
		if matches := platformRe.FindStringSubmatch(line); len(matches) > 1 {
			if !contains(platforms, matches[1]) {
				platforms = append(platforms, matches[1])
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	// Store Gemfile metadata
	if rubyVersion != "" {
		// Don't override if already set from .ruby-version
		if _, exists := metadata.LanguageSpecific["ruby_version"]; !exists {
			metadata.LanguageSpecific["ruby_version"] = rubyVersion
		}
	}
	if source != "" {
		metadata.LanguageSpecific["ruby_gem_source"] = source
	}
	if len(dependencies) > 0 {
		metadata.LanguageSpecific["ruby_gemfile_dependencies"] = dependencies
	}
	if hasBundler {
		metadata.LanguageSpecific["ruby_has_bundler"] = true
	}
	if len(platforms) > 0 {
		metadata.LanguageSpecific["ruby_platforms"] = platforms
	}

	return nil
}

// extractRubyVersion reads the Ruby version from .ruby-version file
func (e *Extractor) extractRubyVersion(versionPath string) (string, error) {
	data, err := os.ReadFile(versionPath)
	if err != nil {
		return "", err
	}
	version := strings.TrimSpace(string(data))
	return version, nil
}

// detectFrameworks detects Ruby frameworks in use
func (e *Extractor) detectFrameworks(projectPath string) []string {
	var frameworks []string

	// Check for Rails
	if e.isRailsProject(projectPath) {
		frameworks = append(frameworks, "rails")

		// Check for specific Rails features
		if e.hasPath(projectPath, "app/javascript") {
			frameworks = append(frameworks, "webpacker")
		}
		if e.hasPath(projectPath, "app/views") {
			frameworks = append(frameworks, "action_view")
		}
		if e.hasPath(projectPath, "app/models") {
			frameworks = append(frameworks, "active_record")
		}
		if e.hasPath(projectPath, "app/controllers") {
			frameworks = append(frameworks, "action_controller")
		}
	}

	// Check for Sinatra
	if e.isSinatraProject(projectPath) {
		frameworks = append(frameworks, "sinatra")
	}

	// Check for Hanami
	if e.hasPath(projectPath, "config/hanami.rb") {
		frameworks = append(frameworks, "hanami")
	}

	// Check for Grape (API framework)
	if e.hasGemfileDependency(projectPath, "grape") {
		frameworks = append(frameworks, "grape")
	}

	// Check for RSpec
	if e.hasPath(projectPath, "spec") {
		frameworks = append(frameworks, "rspec")
	}

	// Check for Minitest
	if e.hasPath(projectPath, "test") {
		frameworks = append(frameworks, "minitest")
	}

	// Check for Cucumber
	if e.hasPath(projectPath, "features") {
		frameworks = append(frameworks, "cucumber")
	}

	return frameworks
}

// isRailsProject checks if the project is a Rails application
func (e *Extractor) isRailsProject(projectPath string) bool {
	// Check for config/application.rb (Rails signature file)
	if e.hasPath(projectPath, "config/application.rb") {
		return true
	}

	// Check for bin/rails
	if e.hasPath(projectPath, "bin/rails") {
		return true
	}

	// Check for Gemfile with rails dependency
	if e.hasGemfileDependency(projectPath, "rails") {
		return true
	}

	return false
}

// isSinatraProject checks if the project is a Sinatra application
func (e *Extractor) isSinatraProject(projectPath string) bool {
	// Check for config.ru (Rack config file)
	configRuPath := filepath.Join(projectPath, "config.ru")
	if content, err := os.ReadFile(configRuPath); err == nil {
		if strings.Contains(string(content), "Sinatra") {
			return true
		}
	}

	// Check for Gemfile with sinatra dependency
	if e.hasGemfileDependency(projectPath, "sinatra") {
		return true
	}

	return false
}

// hasPath checks if a path exists in the project
func (e *Extractor) hasPath(projectPath, subPath string) bool {
	fullPath := filepath.Join(projectPath, subPath)
	_, err := os.Stat(fullPath)
	return err == nil
}

// hasGemfileDependency checks if a gem is listed in the Gemfile
func (e *Extractor) hasGemfileDependency(projectPath, gemName string) bool {
	gemfilePath := filepath.Join(projectPath, "Gemfile")
	content, err := os.ReadFile(gemfilePath)
	if err != nil {
		return false
	}

	// Simple check for gem declaration
	gemPattern := regexp.MustCompile(fmt.Sprintf(`gem\s+["']%s["']`, regexp.QuoteMeta(gemName)))
	return gemPattern.Match(content)
}

// contains checks if a string slice contains a value
func contains(slice []string, value string) bool {
	for _, v := range slice {
		if v == value {
			return true
		}
	}
	return false
}

// GenerateVersionMatrix generates a version matrix for CI/CD
func (e *Extractor) GenerateVersionMatrix(metadata *extractor.ProjectMetadata) map[string]interface{} {
	matrix := make(map[string]interface{})

	// Extract Ruby version requirement
	var rubyVersions []string

	if requiredVersion, ok := metadata.LanguageSpecific["ruby_required_ruby_version"].(string); ok {
		rubyVersions = e.parseRubyVersionRequirement(requiredVersion)
	} else if version, ok := metadata.LanguageSpecific["ruby_version"].(string); ok {
		rubyVersions = []string{version}
	}

	// Default Ruby versions if none specified
	if len(rubyVersions) == 0 {
		rubyVersions = []string{"3.3", "3.2", "3.1"}
	}

	matrix["ruby-version"] = rubyVersions

	// Add OS matrix
	matrix["os"] = []string{"ubuntu-latest", "macos-latest"}

	return matrix
}

// parseRubyVersionRequirement parses Ruby version requirements into a list of versions
func (e *Extractor) parseRubyVersionRequirement(requirement string) []string {
	// Remove operators and get base versions
	requirement = strings.TrimSpace(requirement)

	// Handle >= requirements
	if strings.HasPrefix(requirement, ">=") {
		version := strings.TrimSpace(strings.TrimPrefix(requirement, ">="))
		return e.getCompatibleVersions(version)
	}

	// Handle ~> requirements (approximately greater than)
	if strings.HasPrefix(requirement, "~>") {
		version := strings.TrimSpace(strings.TrimPrefix(requirement, "~>"))
		return e.getCompatibleVersions(version)
	}

	// Handle exact version
	return []string{requirement}
}

// getCompatibleVersions returns compatible Ruby versions based on a base version
func (e *Extractor) getCompatibleVersions(baseVersion string) []string {
	// Common Ruby versions
	allVersions := []string{"3.3", "3.2", "3.1", "3.0", "2.7"}

	var compatible []string
	for _, v := range allVersions {
		if e.isVersionCompatible(baseVersion, v) {
			compatible = append(compatible, v)
		}
	}

	if len(compatible) == 0 {
		return []string{baseVersion}
	}

	return compatible
}

// isVersionCompatible checks if a version satisfies a requirement
// Uses numeric comparison to correctly handle versions like 3.0 vs 3.10
func (e *Extractor) isVersionCompatible(requirement, version string) bool {
	// Parse versions into major.minor components
	reqParts := strings.Split(requirement, ".")
	verParts := strings.Split(version, ".")

	// Compare each component numerically
	for i := 0; i < len(reqParts) && i < len(verParts); i++ {
		reqNum, reqErr := strconv.Atoi(reqParts[i])
		verNum, verErr := strconv.Atoi(verParts[i])

		// If parsing fails, fall back to string comparison for that component
		if reqErr != nil || verErr != nil {
			if reqParts[i] != verParts[i] {
				return reqParts[i] <= verParts[i]
			}
			continue
		}

		// Numeric comparison
		if verNum > reqNum {
			return true
		} else if verNum < reqNum {
			return false
		}
		// If equal, continue to next component
	}

	// If all compared components are equal, version satisfies requirement
	// This handles cases like requirement "3.0" matching version "3.0.5"
	return true
}
