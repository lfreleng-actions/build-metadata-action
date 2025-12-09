// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package javascript

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lfreleng-actions/build-metadata-action/internal/extractor"
	"github.com/lfreleng-actions/build-metadata-action/internal/jsonutil"
)

// Extractor extracts metadata from JavaScript/Node.js projects
type Extractor struct {
	extractor.BaseExtractor
}

// NewExtractor creates a new JavaScript extractor
func NewExtractor() *Extractor {
	return &Extractor{
		BaseExtractor: extractor.NewBaseExtractor("javascript", 2),
	}
}

// PackageJSON represents the structure of a package.json file
type PackageJSON struct {
	Name                 string            `json:"name"`
	Version              string            `json:"version"`
	Description          string            `json:"description"`
	License              interface{}       `json:"license"` // Can be string or object
	Author               interface{}       `json:"author"`  // Can be string or object
	Contributors         []interface{}     `json:"contributors"`
	Homepage             string            `json:"homepage"`
	Repository           interface{}       `json:"repository"` // Can be string or object
	Bugs                 interface{}       `json:"bugs"`
	Keywords             []string          `json:"keywords"`
	Main                 string            `json:"main"`
	Module               string            `json:"module"`
	Types                string            `json:"types"`
	Bin                  interface{}       `json:"bin"`
	Scripts              map[string]string `json:"scripts"`
	Dependencies         map[string]string `json:"dependencies"`
	DevDependencies      map[string]string `json:"devDependencies"`
	PeerDependencies     map[string]string `json:"peerDependencies"`
	OptionalDependencies map[string]string `json:"optionalDependencies"`
	Engines              map[string]string `json:"engines"`
	OS                   []string          `json:"os"`
	CPU                  []string          `json:"cpu"`
	Private              bool              `json:"private"`
	Workspaces           interface{}       `json:"workspaces"` // Can be array or object
	Type                 string            `json:"type"`       // "module" or "commonjs"

	// Package manager specific
	PackageManager string                 `json:"packageManager"` // e.g., "pnpm@8.0.0"
	Volta          map[string]interface{} `json:"volta"`

	// Build tool specific
	Config map[string]interface{} `json:"config"`
}

// Author represents a package author
type Author struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	URL   string `json:"url"`
}

// Repository represents repository information
type Repository struct {
	Type      string `json:"type"`
	URL       string `json:"url"`
	Directory string `json:"directory"`
}

// Extract retrieves metadata from a JavaScript/Node.js project
func (e *Extractor) Extract(projectPath string) (*extractor.ProjectMetadata, error) {
	metadata := &extractor.ProjectMetadata{
		LanguageSpecific: make(map[string]interface{}),
	}

	// Parse package.json
	packageJSONPath := filepath.Join(projectPath, "package.json")
	if _, err := os.Stat(packageJSONPath); err != nil {
		return nil, fmt.Errorf("package.json not found in %s", projectPath)
	}

	if err := e.extractFromPackageJSON(packageJSONPath, projectPath, metadata); err != nil {
		return nil, err
	}

	return metadata, nil
}

// extractFromPackageJSON extracts metadata from package.json
func (e *Extractor) extractFromPackageJSON(path, projectPath string, metadata *extractor.ProjectMetadata) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read package.json: %w", err)
	}

	var pkg PackageJSON
	if err := json.Unmarshal(content, &pkg); err != nil {
		return fmt.Errorf("failed to parse package.json: %w", err)
	}

	// Extract common metadata
	metadata.Name = pkg.Name
	metadata.Version = pkg.Version
	metadata.Description = pkg.Description
	metadata.Homepage = pkg.Homepage
	metadata.VersionSource = "package.json"

	// Extract license
	metadata.License = extractLicense(pkg.License)

	// Extract authors
	metadata.Authors = extractAuthors(pkg.Author, pkg.Contributors)

	// Extract repository
	metadata.Repository = extractRepository(pkg.Repository)

	// JavaScript-specific metadata
	metadata.LanguageSpecific["package_name"] = pkg.Name
	metadata.LanguageSpecific["metadata_source"] = "package.json"
	metadata.LanguageSpecific["is_private"] = pkg.Private

	// Module type
	if pkg.Type != "" {
		metadata.LanguageSpecific["module_type"] = pkg.Type
	} else {
		metadata.LanguageSpecific["module_type"] = "commonjs" // default
	}

	// Entry points
	if pkg.Main != "" {
		metadata.LanguageSpecific["main_entry"] = pkg.Main
	}
	if pkg.Module != "" {
		metadata.LanguageSpecific["module_entry"] = pkg.Module
	}
	if pkg.Types != "" {
		metadata.LanguageSpecific["types_entry"] = pkg.Types
	}

	// Engines (Node.js version requirements)
	if len(pkg.Engines) > 0 {
		metadata.LanguageSpecific["engines"] = pkg.Engines
		if nodeVersion, ok := pkg.Engines["node"]; ok {
			metadata.LanguageSpecific["requires_node"] = nodeVersion
		}
		if npmVersion, ok := pkg.Engines["npm"]; ok {
			metadata.LanguageSpecific["requires_npm"] = npmVersion
		}
	}

	// Detect package manager
	packageManager := detectPackageManager(projectPath, pkg.PackageManager)
	metadata.LanguageSpecific["package_manager"] = packageManager

	// Lock file information
	lockFile, lockFileExists := detectLockFile(projectPath, packageManager)
	if lockFileExists {
		metadata.LanguageSpecific["lock_file"] = lockFile
		metadata.LanguageSpecific["has_lock_file"] = true
	} else {
		metadata.LanguageSpecific["has_lock_file"] = false
	}

	// Workspace/monorepo detection
	if pkg.Workspaces != nil {
		workspaces := extractWorkspaces(pkg.Workspaces)
		if len(workspaces) > 0 {
			metadata.LanguageSpecific["is_workspace"] = true
			metadata.LanguageSpecific["workspaces"] = workspaces
			metadata.LanguageSpecific["workspace_count"] = len(workspaces)
		}
	}

	// Dependencies
	totalDeps := len(pkg.Dependencies) + len(pkg.DevDependencies) +
		len(pkg.PeerDependencies) + len(pkg.OptionalDependencies)

	if totalDeps > 0 {
		metadata.LanguageSpecific["dependency_count"] = len(pkg.Dependencies)
		metadata.LanguageSpecific["dev_dependency_count"] = len(pkg.DevDependencies)
		metadata.LanguageSpecific["peer_dependency_count"] = len(pkg.PeerDependencies)
		metadata.LanguageSpecific["optional_dependency_count"] = len(pkg.OptionalDependencies)
		metadata.LanguageSpecific["total_dependency_count"] = totalDeps

		// List top-level dependencies
		if len(pkg.Dependencies) > 0 {
			metadata.LanguageSpecific["dependencies"] = pkg.Dependencies
		}
	}

	// Scripts
	if len(pkg.Scripts) > 0 {
		metadata.LanguageSpecific["has_scripts"] = true
		metadata.LanguageSpecific["script_count"] = len(pkg.Scripts)

		// Detect common script patterns
		scriptPatterns := detectScriptPatterns(pkg.Scripts)
		if len(scriptPatterns) > 0 {
			metadata.LanguageSpecific["detected_scripts"] = scriptPatterns
		}
	}

	// Detect framework/tooling
	frameworks := detectFrameworks(pkg.Dependencies, pkg.DevDependencies)
	if len(frameworks) > 0 {
		metadata.LanguageSpecific["frameworks"] = frameworks
	}

	// Build tools
	buildTools := detectBuildTools(pkg.Dependencies, pkg.DevDependencies)
	if len(buildTools) > 0 {
		metadata.LanguageSpecific["build_tools"] = buildTools
	}

	// Testing frameworks
	testingFrameworks := detectTestingFrameworks(pkg.Dependencies, pkg.DevDependencies)
	if len(testingFrameworks) > 0 {
		metadata.LanguageSpecific["testing_frameworks"] = testingFrameworks
	}

	// Keywords
	if len(pkg.Keywords) > 0 {
		metadata.LanguageSpecific["keywords"] = pkg.Keywords
	}

	// Check if version is dynamic (e.g., 0.0.0-development)
	if pkg.Version == "0.0.0-development" ||
		pkg.Version == "0.0.0-semantic-release" ||
		strings.Contains(pkg.Version, "workspace:") {
		metadata.LanguageSpecific["versioning_type"] = "dynamic"
	} else {
		metadata.LanguageSpecific["versioning_type"] = "static"
	}

	// TypeScript detection
	isTypeScript := detectTypeScript(projectPath, pkg.Dependencies, pkg.DevDependencies)
	if isTypeScript {
		metadata.LanguageSpecific["has_typescript"] = true

		// Read tsconfig.json if exists
		tsconfigPath := filepath.Join(projectPath, "tsconfig.json")
		if tsconfig, err := readTSConfig(tsconfigPath); err == nil {
			metadata.LanguageSpecific["typescript_config"] = tsconfig
		}
	}

	return nil
}

// Detect checks if this extractor can handle the project
func (e *Extractor) Detect(projectPath string) bool {
	packageJSONPath := filepath.Join(projectPath, "package.json")
	_, err := os.Stat(packageJSONPath)
	return err == nil
}

// Helper functions

// extractLicense extracts license information
func extractLicense(license interface{}) string {
	if license == nil {
		return ""
	}

	switch v := license.(type) {
	case string:
		return v
	case map[string]interface{}:
		if licenseType, ok := v["type"].(string); ok {
			return licenseType
		}
	}

	return ""
}

// extractAuthors extracts author and contributor information
func extractAuthors(author interface{}, contributors []interface{}) []string {
	authors := make([]string, 0)

	// Extract main author
	if author != nil {
		authorStr := formatAuthor(author)
		if authorStr != "" {
			authors = append(authors, authorStr)
		}
	}

	// Extract contributors
	for _, contributor := range contributors {
		contributorStr := formatAuthor(contributor)
		if contributorStr != "" {
			authors = append(authors, contributorStr)
		}
	}

	return authors
}

// formatAuthor formats an author object or string
func formatAuthor(author interface{}) string {
	switch v := author.(type) {
	case string:
		return v
	case map[string]interface{}:
		name, _ := v["name"].(string)
		email, _ := v["email"].(string)

		if name != "" {
			if email != "" {
				return fmt.Sprintf("%s <%s>", name, email)
			}
			return name
		}
	}

	return ""
}

// extractRepository extracts repository information
func extractRepository(repo interface{}) string {
	if repo == nil {
		return ""
	}

	switch v := repo.(type) {
	case string:
		return v
	case map[string]interface{}:
		if url, ok := v["url"].(string); ok {
			return url
		}
	}

	return ""
}

// extractWorkspaces extracts workspace patterns
func extractWorkspaces(workspaces interface{}) []string {
	if workspaces == nil {
		return nil
	}

	switch v := workspaces.(type) {
	case []interface{}:
		patterns := make([]string, 0, len(v))
		for _, item := range v {
			if pattern, ok := item.(string); ok {
				patterns = append(patterns, pattern)
			}
		}
		return patterns
	case map[string]interface{}:
		if packages, ok := v["packages"].([]interface{}); ok {
			patterns := make([]string, 0, len(packages))
			for _, item := range packages {
				if pattern, ok := item.(string); ok {
					patterns = append(patterns, pattern)
				}
			}
			return patterns
		}
	}

	return nil
}

// detectPackageManager detects which package manager is being used
func detectPackageManager(projectPath, packageManagerField string) string {
	// Check packageManager field first
	if packageManagerField != "" {
		// Format: "pnpm@8.0.0" or "yarn@3.0.0"
		if strings.Contains(packageManagerField, "@") {
			parts := strings.Split(packageManagerField, "@")
			return parts[0]
		}
		return packageManagerField
	}

	// Check for lock files
	if _, err := os.Stat(filepath.Join(projectPath, "pnpm-lock.yaml")); err == nil {
		return "pnpm"
	}

	if _, err := os.Stat(filepath.Join(projectPath, "yarn.lock")); err == nil {
		// Check if it's Yarn 2+ (berry)
		yarnrcPath := filepath.Join(projectPath, ".yarnrc.yml")
		if _, err := os.Stat(yarnrcPath); err == nil {
			return "yarn-berry"
		}
		return "yarn"
	}

	if _, err := os.Stat(filepath.Join(projectPath, "package-lock.json")); err == nil {
		return "npm"
	}

	if _, err := os.Stat(filepath.Join(projectPath, "bun.lockb")); err == nil {
		return "bun"
	}

	// Default to npm
	return "npm"
}

// detectLockFile returns the lock file name and whether it exists
func detectLockFile(projectPath, packageManager string) (string, bool) {
	lockFiles := map[string]string{
		"npm":        "package-lock.json",
		"yarn":       "yarn.lock",
		"yarn-berry": "yarn.lock",
		"pnpm":       "pnpm-lock.yaml",
		"bun":        "bun.lockb",
	}

	lockFile, ok := lockFiles[packageManager]
	if !ok {
		return "", false
	}

	lockFilePath := filepath.Join(projectPath, lockFile)
	if _, err := os.Stat(lockFilePath); err == nil {
		return lockFile, true
	}

	return lockFile, false
}

// detectScriptPatterns detects common script patterns
func detectScriptPatterns(scripts map[string]string) []string {
	patterns := make([]string, 0)

	scriptChecks := map[string]string{
		"build":          "build",
		"test":           "test",
		"start":          "start",
		"dev":            "dev",
		"lint":           "lint",
		"format":         "format",
		"prepare":        "prepare",
		"prepublishOnly": "prepublishOnly",
	}

	for pattern, scriptName := range scriptChecks {
		if _, exists := scripts[scriptName]; exists {
			patterns = append(patterns, pattern)
		}
	}

	return patterns
}

// detectFrameworks detects common JavaScript frameworks
func detectFrameworks(deps, devDeps map[string]string) []string {
	frameworks := make([]string, 0)

	frameworkChecks := map[string]string{
		"react":            "React",
		"vue":              "Vue.js",
		"@angular/core":    "Angular",
		"next":             "Next.js",
		"nuxt":             "Nuxt.js",
		"svelte":           "Svelte",
		"solid-js":         "Solid.js",
		"preact":           "Preact",
		"gatsby":           "Gatsby",
		"astro":            "Astro",
		"@remix-run/react": "Remix",
		"@builder.io/qwik": "Qwik",
	}

	for pkg, name := range frameworkChecks {
		if _, exists := deps[pkg]; exists {
			frameworks = append(frameworks, name)
		}
		if _, exists := devDeps[pkg]; exists {
			frameworks = append(frameworks, name)
		}
	}

	return frameworks
}

// detectBuildTools detects build tools
func detectBuildTools(deps, devDeps map[string]string) []string {
	tools := make([]string, 0)

	toolChecks := map[string]string{
		"webpack":     "Webpack",
		"vite":        "Vite",
		"rollup":      "Rollup",
		"parcel":      "Parcel",
		"esbuild":     "esbuild",
		"swc":         "SWC",
		"turbopack":   "Turbopack",
		"@babel/core": "Babel",
		"typescript":  "TypeScript",
	}

	for pkg, name := range toolChecks {
		if _, exists := deps[pkg]; exists {
			tools = append(tools, name)
		}
		if _, exists := devDeps[pkg]; exists {
			tools = append(tools, name)
		}
	}

	return tools
}

// detectTestingFrameworks detects testing frameworks
func detectTestingFrameworks(deps, devDeps map[string]string) []string {
	frameworks := make([]string, 0)

	testChecks := map[string]string{
		"jest":                   "Jest",
		"vitest":                 "Vitest",
		"mocha":                  "Mocha",
		"jasmine":                "Jasmine",
		"@playwright/test":       "Playwright",
		"cypress":                "Cypress",
		"@testing-library/react": "React Testing Library",
		"ava":                    "AVA",
	}

	for pkg, name := range testChecks {
		if _, exists := deps[pkg]; exists {
			frameworks = append(frameworks, name)
		}
		if _, exists := devDeps[pkg]; exists {
			frameworks = append(frameworks, name)
		}
	}

	return frameworks
}

// detectTypeScript checks if the project uses TypeScript
func detectTypeScript(projectPath string, deps, devDeps map[string]string) bool {
	// Check for typescript dependency
	if _, exists := deps["typescript"]; exists {
		return true
	}
	if _, exists := devDeps["typescript"]; exists {
		return true
	}

	// Check for tsconfig.json
	tsconfigPath := filepath.Join(projectPath, "tsconfig.json")
	if _, err := os.Stat(tsconfigPath); err == nil {
		return true
	}

	return false
}

// readTSConfig reads TypeScript configuration
func readTSConfig(path string) (map[string]interface{}, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Parse JSON (tsconfig.json allows comments, but we'll do basic parsing)
	var config map[string]interface{}

	// Remove comments using jsonutil package
	contentStr := string(content)
	contentStr = jsonutil.RemoveComments(contentStr)

	if err := json.Unmarshal([]byte(contentStr), &config); err != nil {
		return nil, err
	}

	return config, nil
}

// init registers the JavaScript extractor
func init() {
	extractor.RegisterExtractor(NewExtractor())
}
