// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package rust

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/lfreleng-actions/build-metadata-action/internal/extractor"
)

// rustVersionCache holds cached Rust version information
var rustVersionCache struct {
	sync.RWMutex
	versions  []string
	fetchedAt time.Time
	cacheTTL  time.Duration
}

func init() {
	// Set cache TTL to 72 hours
	// Rust releases every 6 weeks, so 72 hours is conservative
	rustVersionCache.cacheTTL = 72 * time.Hour
}

// Extractor extracts metadata from Rust projects
type Extractor struct {
	extractor.BaseExtractor
}

// NewExtractor creates a new Rust extractor
func NewExtractor() *Extractor {
	return &Extractor{
		BaseExtractor: extractor.NewBaseExtractor("rust-cargo", 1),
	}
}

// CargoToml represents the structure of a Cargo.toml file
type CargoToml struct {
	Package           Package                           `toml:"package"`
	Dependencies      map[string]interface{}            `toml:"dependencies"`
	DevDependencies   map[string]interface{}            `toml:"dev-dependencies"`
	BuildDependencies map[string]interface{}            `toml:"build-dependencies"`
	Features          map[string][]string               `toml:"features"`
	Workspace         Workspace                         `toml:"workspace"`
	Bin               []Bin                             `toml:"bin"`
	Lib               Lib                               `toml:"lib"`
	Profile           map[string]map[string]interface{} `toml:"profile"`
}

// Package represents the [package] section of Cargo.toml
type Package struct {
	Name          string                 `toml:"name"`
	Version       interface{}            `toml:"version"`      // Can be string or map (workspace inheritance)
	Authors       interface{}            `toml:"authors"`      // Can be []string or map (workspace inheritance)
	Edition       interface{}            `toml:"edition"`      // Can be string or map (workspace inheritance)
	RustVersion   interface{}            `toml:"rust-version"` // Can be string or map (workspace inheritance)
	Description   interface{}            `toml:"description"`  // Can be string or map (workspace inheritance)
	Documentation string                 `toml:"documentation"`
	Homepage      interface{}            `toml:"homepage"`   // Can be string or map (workspace inheritance)
	Repository    interface{}            `toml:"repository"` // Can be string or map (workspace inheritance)
	License       interface{}            `toml:"license"`    // Can be string or map (workspace inheritance)
	LicenseFile   string                 `toml:"license-file"`
	Keywords      interface{}            `toml:"keywords"`   // Can be []string or map (workspace inheritance)
	Categories    interface{}            `toml:"categories"` // Can be []string or map (workspace inheritance)
	Readme        interface{}            `toml:"readme"`     // Can be string or map (workspace inheritance)
	Publish       interface{}            `toml:"publish"`
	Metadata      map[string]interface{} `toml:"metadata"`
	DefaultRun    string                 `toml:"default-run"`
	AutoBenches   bool                   `toml:"autobins"`
	AutoExamples  bool                   `toml:"autoexamples"`
	AutoTests     bool                   `toml:"autotests"`
	Build         string                 `toml:"build"`
}

// Workspace represents the [workspace] section of Cargo.toml
type Workspace struct {
	Members  []string         `toml:"members"`
	Exclude  []string         `toml:"exclude"`
	Resolver string           `toml:"resolver"`
	Package  WorkspacePackage `toml:"package"`
}

// WorkspacePackage represents workspace-level package metadata
type WorkspacePackage struct {
	Version     string   `toml:"version"`
	Authors     []string `toml:"authors"`
	Edition     string   `toml:"edition"`
	RustVersion string   `toml:"rust-version"`
	Description string   `toml:"description"`
	Homepage    string   `toml:"homepage"`
	Repository  string   `toml:"repository"`
	License     string   `toml:"license"`
	Keywords    []string `toml:"keywords"`
	Categories  []string `toml:"categories"`
}

// Bin represents a [[bin]] section
type Bin struct {
	Name string `toml:"name"`
	Path string `toml:"path"`
}

// Lib represents the [lib] section
type Lib struct {
	Name      string   `toml:"name"`
	Path      string   `toml:"path"`
	CrateType []string `toml:"crate-type"`
}

// Dependency represents a parsed dependency
type Dependency struct {
	Name     string
	Version  string
	Optional bool
	Features []string
	Source   string // "crates.io", "git", "path", etc.
}

// Extract retrieves metadata from a Rust project
func (e *Extractor) Extract(projectPath string) (*extractor.ProjectMetadata, error) {
	metadata := &extractor.ProjectMetadata{
		LanguageSpecific: make(map[string]interface{}),
	}

	// Try Cargo.toml file
	cargoTomlPath := filepath.Join(projectPath, "Cargo.toml")
	if _, err := os.Stat(cargoTomlPath); err == nil {
		if err := e.extractFromCargoToml(cargoTomlPath, metadata); err != nil {
			return nil, err
		}
		return metadata, nil
	}

	return nil, fmt.Errorf("no Cargo.toml file found in %s", projectPath)
}

// extractFromCargoToml extracts metadata from Cargo.toml file
func (e *Extractor) extractFromCargoToml(path string, metadata *extractor.ProjectMetadata) error {
	var cargo CargoToml

	if _, err := toml.DecodeFile(path, &cargo); err != nil {
		return fmt.Errorf("failed to parse Cargo.toml: %w", err)
	}

	// Extract common metadata with workspace inheritance support
	metadata.Name = cargo.Package.Name

	// Debug logging
	if os.Getenv("INPUT_VERBOSE") == "true" {
		log.Printf("[DEBUG] Rust: Package.Version type=%T, value=%#v", cargo.Package.Version, cargo.Package.Version)
		log.Printf("[DEBUG] Rust: Workspace.Package.Version=%s", cargo.Workspace.Package.Version)
	}

	version := getStringValue(cargo.Package.Version, cargo.Workspace.Package.Version)

	if os.Getenv("INPUT_VERBOSE") == "true" {
		log.Printf("[DEBUG] Rust: getStringValue returned: '%s'", version)
	}

	// Validate version string - reject invalid versions like "true", empty strings, or non-semantic versions
	if version != "" && version != "true" && version != "false" {
		metadata.Version = version
		if os.Getenv("INPUT_VERBOSE") == "true" {
			log.Printf("[DEBUG] Rust: Setting metadata.Version to: '%s'", version)
		}
	} else {
		// Version is invalid or missing, will be extracted from git tags as fallback
		metadata.Version = ""
		if os.Getenv("INPUT_VERBOSE") == "true" {
			log.Printf("[DEBUG] Rust: Version invalid ('%s'), clearing for git tag fallback", version)
		}
	}
	metadata.Description = getStringValue(cargo.Package.Description, cargo.Workspace.Package.Description)
	metadata.License = getStringValue(cargo.Package.License, cargo.Workspace.Package.License)
	metadata.Homepage = getStringValue(cargo.Package.Homepage, cargo.Workspace.Package.Homepage)
	metadata.Repository = getStringValue(cargo.Package.Repository, cargo.Workspace.Package.Repository)
	metadata.Authors = getStringSliceValue(cargo.Package.Authors, cargo.Workspace.Package.Authors)
	metadata.VersionSource = "Cargo.toml"

	// Rust-specific metadata
	metadata.LanguageSpecific["package_name"] = cargo.Package.Name
	metadata.LanguageSpecific["metadata_source"] = "Cargo.toml"

	edition := getStringValue(cargo.Package.Edition, cargo.Workspace.Package.Edition)
	if edition != "" {
		metadata.LanguageSpecific["edition"] = edition
	}

	rustVersion := getStringValue(cargo.Package.RustVersion, cargo.Workspace.Package.RustVersion)
	if rustVersion != "" {
		metadata.LanguageSpecific["rust_version"] = rustVersion
		metadata.LanguageSpecific["msrv"] = rustVersion // Minimum Supported Rust Version
	}

	// Documentation and repository URLs
	if cargo.Package.Documentation != "" {
		metadata.LanguageSpecific["documentation"] = cargo.Package.Documentation
	}

	// Keywords and categories
	keywords := getStringSliceValue(cargo.Package.Keywords, cargo.Workspace.Package.Keywords)
	if len(keywords) > 0 {
		metadata.LanguageSpecific["keywords"] = keywords
	}

	categories := getStringSliceValue(cargo.Package.Categories, cargo.Workspace.Package.Categories)
	if len(categories) > 0 {
		metadata.LanguageSpecific["categories"] = categories
	}

	// Publish settings
	if cargo.Package.Publish != nil {
		metadata.LanguageSpecific["publish"] = cargo.Package.Publish
	}

	// License file
	if cargo.Package.LicenseFile != "" {
		metadata.LanguageSpecific["license_file"] = cargo.Package.LicenseFile
	}

	// README
	readme := getStringValue(cargo.Package.Readme, "")
	if readme != "" {
		metadata.LanguageSpecific["readme"] = readme
	}

	// Parse dependencies
	if len(cargo.Dependencies) > 0 {
		deps := parseDependencies(cargo.Dependencies, "normal")
		metadata.LanguageSpecific["dependencies"] = formatDependencies(deps)
		metadata.LanguageSpecific["dependency_count"] = len(deps)

		// Extract optional dependencies
		optionalDeps := []string{}
		for _, dep := range deps {
			if dep.Optional {
				optionalDeps = append(optionalDeps, dep.Name)
			}
		}
		if len(optionalDeps) > 0 {
			metadata.LanguageSpecific["optional_dependencies"] = optionalDeps
		}
	}

	// Parse dev dependencies
	if len(cargo.DevDependencies) > 0 {
		devDeps := parseDependencies(cargo.DevDependencies, "dev")
		metadata.LanguageSpecific["dev_dependencies"] = formatDependencies(devDeps)
		metadata.LanguageSpecific["dev_dependency_count"] = len(devDeps)
	}

	// Parse build dependencies
	if len(cargo.BuildDependencies) > 0 {
		buildDeps := parseDependencies(cargo.BuildDependencies, "build")
		metadata.LanguageSpecific["build_dependencies"] = formatDependencies(buildDeps)
		metadata.LanguageSpecific["build_dependency_count"] = len(buildDeps)
	}

	// Total dependencies
	totalDeps := len(cargo.Dependencies) + len(cargo.DevDependencies) + len(cargo.BuildDependencies)
	if totalDeps > 0 {
		metadata.LanguageSpecific["total_dependency_count"] = totalDeps
	}

	// Features
	if len(cargo.Features) > 0 {
		metadata.LanguageSpecific["features"] = cargo.Features
		metadata.LanguageSpecific["feature_count"] = len(cargo.Features)

		// Extract feature names
		featureNames := make([]string, 0, len(cargo.Features))
		for name := range cargo.Features {
			featureNames = append(featureNames, name)
		}
		metadata.LanguageSpecific["feature_names"] = featureNames
	}

	// Workspace information
	if len(cargo.Workspace.Members) > 0 {
		metadata.LanguageSpecific["is_workspace"] = true
		metadata.LanguageSpecific["workspace_members"] = cargo.Workspace.Members
		metadata.LanguageSpecific["workspace_member_count"] = len(cargo.Workspace.Members)

		if cargo.Workspace.Resolver != "" {
			metadata.LanguageSpecific["workspace_resolver"] = cargo.Workspace.Resolver
		}
	}

	// Binary targets
	if len(cargo.Bin) > 0 {
		binNames := make([]string, 0, len(cargo.Bin))
		for _, bin := range cargo.Bin {
			binNames = append(binNames, bin.Name)
		}
		metadata.LanguageSpecific["binary_targets"] = binNames
		metadata.LanguageSpecific["binary_count"] = len(cargo.Bin)
	}

	// Library information
	if cargo.Lib.Name != "" {
		metadata.LanguageSpecific["lib_name"] = cargo.Lib.Name
		if len(cargo.Lib.CrateType) > 0 {
			metadata.LanguageSpecific["crate_types"] = cargo.Lib.CrateType
		}
	}

	// Build script
	if cargo.Package.Build != "" {
		metadata.LanguageSpecific["has_build_script"] = true
		metadata.LanguageSpecific["build_script"] = cargo.Package.Build
	}

	// Detect common Rust frameworks and tools
	frameworks := detectRustFrameworks(cargo.Dependencies)
	if len(frameworks) > 0 {
		metadata.LanguageSpecific["frameworks"] = frameworks
	}

	// Generate Rust version matrix
	if rustVersion != "" {
		matrix := generateRustVersionMatrix(rustVersion)
		if len(matrix) > 0 {
			metadata.LanguageSpecific["rust_version_matrix"] = matrix

			// Convert to JSON for easy use in GitHub Actions
			matrixJSON := fmt.Sprintf(`{"rust-version": [%s]}`,
				strings.Join(quoteStrings(matrix), ", "))
			metadata.LanguageSpecific["matrix_json"] = matrixJSON
		}
	} else if edition != "" {
		// Use edition as fallback
		matrix := generateRustVersionMatrixFromEdition(edition)
		if len(matrix) > 0 {
			metadata.LanguageSpecific["rust_version_matrix"] = matrix
			matrixJSON := fmt.Sprintf(`{"rust-version": [%s]}`,
				strings.Join(quoteStrings(matrix), ", "))
			metadata.LanguageSpecific["matrix_json"] = matrixJSON
		}
	}

	return nil
}

// getStringValue extracts a string from an interface{} that could be a string or workspace reference
func getStringValue(value interface{}, workspaceDefault string) string {
	if value == nil {
		return workspaceDefault
	}

	switch v := value.(type) {
	case string:
		return v
	case map[string]interface{}:
		// Check if it's a workspace reference
		if workspace, ok := v["workspace"].(bool); ok && workspace {
			return workspaceDefault
		}
	case bool:
		// If we get a boolean value directly (shouldn't happen but be defensive)
		// This means the TOML structure is malformed, return empty string
		return ""
	}

	return ""
}

// getStringSliceValue extracts a []string from an interface{} that could be []string or workspace reference
func getStringSliceValue(value interface{}, workspaceDefault []string) []string {
	if value == nil {
		return workspaceDefault
	}

	switch v := value.(type) {
	case []interface{}:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if str, ok := item.(string); ok {
				result = append(result, str)
			}
		}
		return result
	case []string:
		return v
	case map[string]interface{}:
		// Check if it's a workspace reference
		if workspace, ok := v["workspace"].(bool); ok && workspace {
			return workspaceDefault
		}
	}

	return nil
}

// parseDependencies parses the dependencies from Cargo.toml
func parseDependencies(deps map[string]interface{}, depType string) []Dependency {
	result := []Dependency{}

	for name, spec := range deps {
		dep := Dependency{
			Name: name,
		}

		switch v := spec.(type) {
		case string:
			// Simple version string: package = "1.0.0"
			dep.Version = v
			dep.Source = "crates.io"

		case map[string]interface{}:
			// Detailed dependency specification
			if version, ok := v["version"].(string); ok {
				dep.Version = version
			}
			if optional, ok := v["optional"].(bool); ok {
				dep.Optional = optional
			}
			if features, ok := v["features"].([]interface{}); ok {
				dep.Features = make([]string, 0, len(features))
				for _, f := range features {
					if fs, ok := f.(string); ok {
						dep.Features = append(dep.Features, fs)
					}
				}
			}

			// Determine source
			if _, ok := v["git"]; ok {
				dep.Source = "git"
			} else if _, ok := v["path"]; ok {
				dep.Source = "path"
			} else if registry, ok := v["registry"].(string); ok {
				dep.Source = registry
			} else {
				dep.Source = "crates.io"
			}
		}

		result = append(result, dep)
	}

	return result
}

// formatDependencies formats dependencies for output
func formatDependencies(deps []Dependency) []string {
	formatted := make([]string, 0, len(deps))
	for _, dep := range deps {
		depStr := dep.Name
		if dep.Version != "" {
			depStr += "@" + dep.Version
		}
		if dep.Optional {
			depStr += " (optional)"
		}
		if len(dep.Features) > 0 {
			depStr += " [" + strings.Join(dep.Features, ", ") + "]"
		}
		formatted = append(formatted, depStr)
	}
	return formatted
}

// detectRustFrameworks detects common Rust frameworks from dependencies
func detectRustFrameworks(deps map[string]interface{}) []string {
	frameworks := []string{}
	frameworkMap := map[string]string{
		"tokio":     "Tokio (Async Runtime)",
		"async-std": "async-std (Async Runtime)",
		"actix-web": "Actix Web (Web Framework)",
		"rocket":    "Rocket (Web Framework)",
		"axum":      "Axum (Web Framework)",
		"warp":      "Warp (Web Framework)",
		"tide":      "Tide (Web Framework)",
		"serde":     "Serde (Serialization)",
		"clap":      "Clap (CLI Parser)",
		"diesel":    "Diesel (ORM)",
		"sqlx":      "SQLx (SQL Toolkit)",
		"reqwest":   "Reqwest (HTTP Client)",
		"hyper":     "Hyper (HTTP Library)",
		"tonic":     "Tonic (gRPC)",
		"tracing":   "Tracing (Logging/Diagnostics)",
		"log":       "Log (Logging Facade)",
		"anyhow":    "Anyhow (Error Handling)",
		"thiserror": "thiserror (Error Derive)",
		"rayon":     "Rayon (Parallelism)",
		"crossbeam": "Crossbeam (Concurrency)",
		"gtk":       "GTK (GUI)",
		"bevy":      "Bevy (Game Engine)",
		"yew":       "Yew (Web Framework)",
		"tauri":     "Tauri (Desktop Apps)",
	}

	seen := make(map[string]bool)
	for depName := range deps {
		if name, ok := frameworkMap[depName]; ok && !seen[name] {
			frameworks = append(frameworks, name)
			seen[name] = true
		}
	}

	return frameworks
}

// fetchRustVersions fetches available Rust versions dynamically from rust-lang.org.
//
// This function queries the official Rust release channel to get the current stable
// version and generates a reasonable testing matrix. It has a 5-second timeout and
// will return an error if the fetch fails, allowing the caller to fall back to
// static version lists.
//
// The function:
// 1. Fetches channel-rust-stable.toml from static.rust-lang.org
// 2. Parses the current stable version (e.g., "1.84.0")
// 3. Generates a range of the last 6 minor versions (roughly 9 months)
// 4. Always includes "stable" for testing against the latest release
//
// This ensures version matrices stay current without manual updates, while the
// 5-second timeout and error handling prevent workflow failures if the API is
// unreachable. The caller should always have a fallback strategy.
func fetchRustVersions() ([]string, error) {
	// Check cache first
	rustVersionCache.RLock()
	if len(rustVersionCache.versions) > 0 && time.Since(rustVersionCache.fetchedAt) < rustVersionCache.cacheTTL {
		versions := rustVersionCache.versions
		rustVersionCache.RUnlock()
		return versions, nil
	}
	rustVersionCache.RUnlock()

	// Cache miss or expired - fetch from network
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	// Try to fetch from Rust releases API
	resp, err := client.Get("https://static.rust-lang.org/dist/channel-rust-stable.toml")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch Rust versions: status %d", resp.StatusCode)
	}

	// Parse the TOML to get the current stable version
	var data map[string]interface{}
	if _, err := toml.DecodeReader(resp.Body, &data); err != nil {
		return nil, err
	}

	// Extract version from pkg.rust.version
	if pkg, ok := data["pkg"].(map[string]interface{}); ok {
		if rust, ok := pkg["rust"].(map[string]interface{}); ok {
			if version, ok := rust["version"].(string); ok {
				// Parse the version (format: "1.XX.Y (hash date)")
				re := regexp.MustCompile(`^(\d+\.\d+)`)
				if matches := re.FindStringSubmatch(version); len(matches) > 1 {
					stableVersion := matches[1]
					// Generate a reasonable range
					versions := generateVersionRange(stableVersion)

					// Cache the result
					rustVersionCache.Lock()
					rustVersionCache.versions = versions
					rustVersionCache.fetchedAt = time.Now()
					rustVersionCache.Unlock()

					return versions, nil
				}
			}
		}
	}

	return nil, fmt.Errorf("could not parse version from response")
}

// generateVersionRange generates a range of versions from a stable version.
//
// Given a stable version (e.g., "1.84"), this generates approximately the last
// 6 minor versions plus "stable". This provides ~9 months of version coverage,
// which is a reasonable testing range that balances thoroughness with CI time.
func generateVersionRange(stableVersion string) []string {
	// Parse major.minor
	parts := strings.Split(stableVersion, ".")
	if len(parts) < 2 {
		return []string{stableVersion, "stable"}
	}

	major, _ := strconv.Atoi(parts[0])
	minor, _ := strconv.Atoi(parts[1])

	// Generate last 6 minor versions (roughly 9 months of releases at 6-week cadence)
	versions := []string{}
	for i := 0; i < 6 && minor-i >= 0; i++ {
		versions = append(versions, fmt.Sprintf("%d.%d", major, minor-i))
	}

	// Always include "stable" as a special version marker
	// This allows testing against whatever is currently stable
	versions = append(versions, "stable")

	return versions
}

// generateRustVersionMatrix generates a list of Rust versions from MSRV.
//
// Strategy:
// 1. PRIMARY: Dynamically fetch current Rust versions from rust-lang.org
//   - Ensures we always test against the latest stable releases
//   - Rust's 6-week release cycle makes static lists outdated quickly
//   - Generates a range of ~6 recent versions (approximately 9 months)
//
// 2. FALLBACK: Use static version map if dynamic fetch fails
//   - Prevents workflow failures due to network issues or API downtime
//   - Static list is maintained with recent versions as of code update
//   - Ensures CI/CD pipelines remain reliable even with connectivity issues
//
// The fallback ensures that temporary network issues or API maintenance don't
// cause build failures, while the dynamic approach keeps testing current.
func generateRustVersionMatrix(msrv string) []string {
	// Try to fetch dynamic versions first (preferred method)
	dynamicVersions, err := fetchRustVersions()
	if err == nil && len(dynamicVersions) > 0 {
		// Filter to versions >= MSRV
		return filterVersionsFromMSRV(msrv, dynamicVersions)
	}

	// FALLBACK: Static version map (updated as of November 2025)
	// Only used if dynamic fetching fails (network issues, API down, timeout)
	// This prevents workflow failures while still providing reasonable version coverage
	versionMap := map[string][]string{
		"1.84": {"1.84", "stable"},
		"1.83": {"1.83", "1.84", "stable"},
		"1.82": {"1.82", "1.83", "1.84", "stable"},
		"1.81": {"1.81", "1.82", "1.83", "1.84", "stable"},
		"1.80": {"1.80", "1.81", "1.82", "1.83", "1.84", "stable"},
		"1.79": {"1.79", "1.80", "1.81", "1.82", "1.83", "1.84", "stable"},
		"1.78": {"1.78", "1.79", "1.80", "1.81", "1.82", "1.83", "1.84", "stable"},
		"1.77": {"1.77", "1.78", "1.79", "1.80", "1.81", "1.82", "1.83", "1.84", "stable"},
		"1.76": {"1.76", "1.77", "1.78", "1.79", "1.80", "1.81", "1.82", "1.83", "1.84", "stable"},
		"1.75": {"1.75", "1.76", "1.77", "1.78", "1.79", "1.80", "1.81", "1.82", "1.83", "1.84", "stable"},
	}

	// Try to find exact match
	if versions, ok := versionMap[msrv]; ok {
		return versions
	}

	// Try to find prefix match
	for version, testVersions := range versionMap {
		if strings.HasPrefix(msrv, version) {
			return testVersions
		}
	}

	// For older versions, use fallback
	if msrv < "1.75" {
		return []string{msrv, "1.75", "1.80", "1.84", "stable"}
	}

	// Default: test the MSRV and stable
	return []string{msrv, "stable"}
}

// filterVersionsFromMSRV filters versions to only include those >= MSRV.
//
// This ensures we don't test against versions older than the project's
// Minimum Supported Rust Version (MSRV), as those tests would fail.
//
// The MSRV is always included as the first version in the result, followed
// by newer versions in ascending order, with special versions (stable, beta,
// nightly) at the end.
func filterVersionsFromMSRV(msrv string, allVersions []string) []string {
	filtered := []string{}

	// Parse MSRV
	msvParts := strings.Split(msrv, ".")
	if len(msvParts) < 2 {
		// If we can't parse, return all versions
		return allVersions
	}

	msrvMajor, _ := strconv.Atoi(msvParts[0])
	msrvMinor, _ := strconv.Atoi(msvParts[1])

	for _, version := range allVersions {
		// Always include "stable", "beta", "nightly"
		if version == "stable" || version == "beta" || version == "nightly" {
			filtered = append(filtered, version)
			continue
		}

		// Parse version
		parts := strings.Split(version, ".")
		if len(parts) < 2 {
			continue
		}

		major, err1 := strconv.Atoi(parts[0])
		minor, err2 := strconv.Atoi(parts[1])
		if err1 != nil || err2 != nil {
			continue
		}

		// Include if version >= MSRV
		if major > msrvMajor || (major == msrvMajor && minor >= msrvMinor) {
			filtered = append(filtered, version)
		}
	}

	// Sort versions (excluding stable/beta/nightly)
	numericVersions := []string{}
	specialVersions := []string{}
	for _, v := range filtered {
		if v == "stable" || v == "beta" || v == "nightly" {
			specialVersions = append(specialVersions, v)
		} else {
			numericVersions = append(numericVersions, v)
		}
	}

	// Sort numeric versions
	sort.Slice(numericVersions, func(i, j int) bool {
		iParts := strings.Split(numericVersions[i], ".")
		jParts := strings.Split(numericVersions[j], ".")

		if len(iParts) < 2 || len(jParts) < 2 {
			return numericVersions[i] < numericVersions[j]
		}

		iMajor, _ := strconv.Atoi(iParts[0])
		jMajor, _ := strconv.Atoi(jParts[0])
		if iMajor != jMajor {
			return iMajor < jMajor
		}

		iMinor, _ := strconv.Atoi(iParts[1])
		jMinor, _ := strconv.Atoi(jParts[1])
		return iMinor < jMinor
	})

	// Combine: MSRV + numeric versions + special versions
	result := []string{msrv}
	for _, v := range numericVersions {
		if v != msrv {
			result = append(result, v)
		}
	}
	result = append(result, specialVersions...)

	return result
}

// generateRustVersionMatrixFromEdition generates versions based on Rust edition
func generateRustVersionMatrixFromEdition(edition string) []string {
	switch edition {
	case "2021":
		return []string{"1.56", "stable"}
	case "2018":
		return []string{"1.31", "stable"}
	case "2015":
		return []string{"1.0", "stable"}
	default:
		return []string{"stable"}
	}
}

// Detect checks if this extractor can handle the project
func (e *Extractor) Detect(projectPath string) bool {
	// Check for Cargo.toml
	if _, err := os.Stat(filepath.Join(projectPath, "Cargo.toml")); err == nil {
		return true
	}

	return false
}

// Helper functions

// quoteStrings adds quotes around each string
func quoteStrings(strs []string) []string {
	quoted := make([]string, len(strs))
	for i, s := range strs {
		quoted[i] = fmt.Sprintf(`"%s"`, s)
	}
	return quoted
}

// init registers the Rust extractor
func init() {
	extractor.RegisterExtractor(NewExtractor())
}
