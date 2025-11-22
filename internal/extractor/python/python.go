// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package python

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/lfreleng-actions/build-metadata-action/internal/extractor"
)

// Extractor extracts metadata from Python projects
type Extractor struct {
	extractor.BaseExtractor
}

// NewExtractor creates a new Python extractor
func NewExtractor() *Extractor {
	return &Extractor{
		BaseExtractor: extractor.NewBaseExtractor("python", 1),
	}
}

// PyProjectTOML represents the structure of a pyproject.toml file
type PyProjectTOML struct {
	Project struct {
		Name           string                       `toml:"name"`
		Version        string                       `toml:"version"`
		Description    string                       `toml:"description"`
		License        interface{}                  `toml:"license"` // Can be string or {text = "..."} or {file = "..."}
		Authors        []Author                     `toml:"authors"`
		Maintainers    []Author                     `toml:"maintainers"`
		Keywords       []string                     `toml:"keywords"`
		Classifiers    []string                     `toml:"classifiers"`
		RequiresPython string                       `toml:"requires-python"`
		Dependencies   []string                     `toml:"dependencies"`
		URLs           map[string]string            `toml:"urls"`
		Scripts        map[string]string            `toml:"scripts"`
		EntryPoints    map[string]map[string]string `toml:"entry-points"`
		Dynamic        []string                     `toml:"dynamic"`
		Readme         interface{}                  `toml:"readme"`
	} `toml:"project"`

	BuildSystem struct {
		Requires     []string `toml:"requires"`
		BuildBackend string   `toml:"build-backend"`
	} `toml:"build-system"`

	Tool map[string]interface{} `toml:"tool"`
}

// Author represents a project author or maintainer
type Author struct {
	Name  string `toml:"name"`
	Email string `toml:"email"`
}

// SetupCfg represents the structure of a setup.cfg file
type SetupCfg struct {
	Metadata struct {
		Name            string   `ini:"name"`
		Version         string   `ini:"version"`
		Author          string   `ini:"author"`
		AuthorEmail     string   `ini:"author_email"`
		Maintainer      string   `ini:"maintainer"`
		MaintainerEmail string   `ini:"maintainer_email"`
		Description     string   `ini:"description"`
		LongDescription string   `ini:"long_description"`
		License         string   `ini:"license"`
		Keywords        string   `ini:"keywords"`
		Classifiers     []string `ini:"classifiers"`
		URL             string   `ini:"url"`
		ProjectURLs     string   `ini:"project_urls"`
		PythonRequires  string   `ini:"python_requires"`
	}
	Options struct {
		Packages           []string `ini:"packages"`
		InstallRequires    []string `ini:"install_requires"`
		PythonRequires     string   `ini:"python_requires"`
		IncludePackageData bool     `ini:"include_package_data"`
		ZipSafe            bool     `ini:"zip_safe"`
	}
}

// Extract retrieves metadata from a Python project
func (e *Extractor) Extract(projectPath string) (*extractor.ProjectMetadata, error) {
	metadata := &extractor.ProjectMetadata{
		LanguageSpecific: make(map[string]interface{}),
	}

	// Track which files exist
	pyprojectPath := filepath.Join(projectPath, "pyproject.toml")
	setupCfgPath := filepath.Join(projectPath, "setup.cfg")
	setupPyPath := filepath.Join(projectPath, "setup.py")

	pyprojectExists := false
	setupCfgExists := false
	setupPyExists := false

	if _, err := os.Stat(pyprojectPath); err == nil {
		pyprojectExists = true
	}
	if _, err := os.Stat(setupCfgPath); err == nil {
		setupCfgExists = true
	}
	if _, err := os.Stat(setupPyPath); err == nil {
		setupPyExists = true
	}

	// Build diagnostic message about which files were found
	filesFound := []string{}
	filesNotFound := []string{}

	if pyprojectExists {
		filesFound = append(filesFound, "pyproject.toml")
	} else {
		filesNotFound = append(filesNotFound, "pyproject.toml")
	}

	if setupCfgExists {
		filesFound = append(filesFound, "setup.cfg")
	} else {
		filesNotFound = append(filesNotFound, "setup.cfg")
	}

	if setupPyExists {
		filesFound = append(filesFound, "setup.py")
	} else {
		filesNotFound = append(filesNotFound, "setup.py")
	}

	// Try pyproject.toml first (modern Python)
	if pyprojectExists {
		if err := e.extractFromPyProject(pyprojectPath, metadata); err != nil {
			// Provide detailed error about pyproject.toml parsing failure
			return nil, fmt.Errorf("found pyproject.toml but failed to parse it: %w\n\nFiles found: %s\nFiles not found: %s\n\nThis error often occurs due to:\n- Invalid TOML syntax (check for merge conflict markers like <<<<<<<, =======, >>>>>>>)\n- Malformed data structures\n- Encoding issues",
				err, strings.Join(filesFound, ", "), strings.Join(filesNotFound, ", "))
		}
		// Check if we got meaningful metadata from pyproject.toml
		// Consider it valid if we have a [project] section OR tool-specific configs
		hasProjectSection := metadata.Name != ""
		hasToolConfig := metadata.LanguageSpecific["poetry_config"] == true ||
			metadata.LanguageSpecific["pdm_config"] == true ||
			metadata.LanguageSpecific["hatch_config"] == true ||
			metadata.LanguageSpecific["setuptools_config"] == true

		if hasProjectSection || hasToolConfig {
			// We have a proper [project] section, but might need requires_python from setup.py
			if metadata.LanguageSpecific["requires_python"] == nil || metadata.LanguageSpecific["requires_python"] == "" {
				// Try setup.py for requires_python
				if setupPyExists {
					fallbackMetadata := &extractor.ProjectMetadata{
						LanguageSpecific: make(map[string]interface{}),
					}
					if err := e.extractFromSetupPy(setupPyPath, fallbackMetadata); err == nil {
						if requiresPython, ok := fallbackMetadata.LanguageSpecific["requires_python"].(string); ok && requiresPython != "" {
							metadata.LanguageSpecific["requires_python"] = requiresPython
							if matrix, ok := fallbackMetadata.LanguageSpecific["version_matrix"].([]string); ok {
								metadata.LanguageSpecific["version_matrix"] = matrix
							}
							if matrixJSON, ok := fallbackMetadata.LanguageSpecific["matrix_json"].(string); ok {
								metadata.LanguageSpecific["matrix_json"] = matrixJSON
							}
							if buildVersion, ok := fallbackMetadata.LanguageSpecific["build_version"].(string); ok {
								metadata.LanguageSpecific["build_version"] = buildVersion
							}
						}
					}
				}
				// Try setup.cfg if we still don't have it
				if (metadata.LanguageSpecific["requires_python"] == nil || metadata.LanguageSpecific["requires_python"] == "") && setupCfgExists {
					fallbackMetadata := &extractor.ProjectMetadata{
						LanguageSpecific: make(map[string]interface{}),
					}
					if err := e.extractFromSetupCfg(setupCfgPath, fallbackMetadata); err == nil {
						if requiresPython, ok := fallbackMetadata.LanguageSpecific["requires_python"].(string); ok && requiresPython != "" {
							metadata.LanguageSpecific["requires_python"] = requiresPython
							if matrix, ok := fallbackMetadata.LanguageSpecific["version_matrix"].([]string); ok {
								metadata.LanguageSpecific["version_matrix"] = matrix
							}
							if matrixJSON, ok := fallbackMetadata.LanguageSpecific["matrix_json"].(string); ok {
								metadata.LanguageSpecific["matrix_json"] = matrixJSON
							}
							if buildVersion, ok := fallbackMetadata.LanguageSpecific["build_version"].(string); ok {
								metadata.LanguageSpecific["build_version"] = buildVersion
							}
						}
					}
				}
			}
			return metadata, nil
		}
		// pyproject.toml exists but has no [project] section
		// Fall through to try setup.cfg or setup.py
	}

	// Try setup.cfg (intermediate format)
	if setupCfgExists {
		if err := e.extractFromSetupCfg(setupCfgPath, metadata); err != nil {
			return nil, fmt.Errorf("found setup.cfg but failed to parse it: %w\n\nFiles found: %s\nFiles not found: %s",
				err, strings.Join(filesFound, ", "), strings.Join(filesNotFound, ", "))
		}
		return metadata, nil
	}

	// Try setup.py (legacy format)
	if setupPyExists {
		if err := e.extractFromSetupPy(setupPyPath, metadata); err != nil {
			return nil, fmt.Errorf("found setup.py but failed to parse it: %w\n\nFiles found: %s\nFiles not found: %s",
				err, strings.Join(filesFound, ", "), strings.Join(filesNotFound, ", "))
		}
		return metadata, nil
	}

	return nil, fmt.Errorf("no Python project files found in %s\n\nSearched for: pyproject.toml, setup.cfg, setup.py\nFiles found: %s\nFiles not found: %s",
		projectPath, strings.Join(filesFound, ", "), strings.Join(filesNotFound, ", "))
}

// extractFromPyProject extracts metadata from pyproject.toml
func (e *Extractor) extractFromPyProject(path string, metadata *extractor.ProjectMetadata) error {
	var pyproject PyProjectTOML

	// Read file content for debugging and validation
	fileContent, readErr := os.ReadFile(path)
	if readErr != nil {
		return fmt.Errorf("failed to read pyproject.toml: %w", readErr)
	}

	// Check for common corruption patterns BEFORE parsing
	fileContentStr := string(fileContent)

	// Detect unquoted version value (invalid TOML syntax)
	// Valid:   version = "1.0.0"
	// Invalid: version = 1.0.0  or  version = v1.0.0
	unquotedVersionPattern := regexp.MustCompile(`(?m)^\s*version\s*=\s*([^"\s][^\s]*)\s*$`)
	if matches := unquotedVersionPattern.FindStringSubmatch(fileContentStr); len(matches) > 1 {
		fmt.Fprintf(os.Stderr, "[ERROR] Corrupted pyproject.toml detected!\n")
		fmt.Fprintf(os.Stderr, "[ERROR] Version field has invalid TOML syntax (missing quotes): version = %s\n", matches[1])
		fmt.Fprintf(os.Stderr, "[ERROR] Should be: version = \"%s\"\n", matches[1])
		fmt.Fprintf(os.Stderr, "[ERROR] This is likely caused by a buggy version patching tool.\n")
		return fmt.Errorf("pyproject.toml contains invalid TOML syntax: unquoted version value")
	}

	if _, err := toml.DecodeFile(path, &pyproject); err != nil {
		// Check if the error message indicates common issues
		errMsg := err.Error()
		if strings.Contains(errMsg, "expected") || strings.Contains(errMsg, "invalid") {
			// Log problematic content around the error
			fmt.Fprintf(os.Stderr, "[ERROR] TOML parsing failed for %s\n", path)
			fmt.Fprintf(os.Stderr, "[ERROR] Error: %v\n", err)
			// Show first 500 chars of file for debugging
			preview := string(fileContent)
			if len(preview) > 500 {
				preview = preview[:500] + "..."
			}
			fmt.Fprintf(os.Stderr, "[ERROR] File preview:\n%s\n", preview)
			return fmt.Errorf("TOML parsing failed - file contains invalid TOML syntax: %w\n\nCommon causes:\n- Git merge conflict markers (<<<<<<<, =======, >>>>>>>)\n- Unclosed strings or brackets\n- Invalid escape sequences\n- Incorrect indentation or structure", err)
		}
		return fmt.Errorf("TOML parsing failed: %w", err)
	}

	// Validate parsed data
	if pyproject.Project.Name == "" {
		fmt.Fprintf(os.Stderr, "[WARNING] pyproject.toml parsed successfully but [project].name is empty\n")
	}
	if pyproject.Project.Version == "" {
		fmt.Fprintf(os.Stderr, "[WARNING] pyproject.toml parsed successfully but [project].version is empty\n")
	}
	if pyproject.Project.RequiresPython == "" {
		fmt.Fprintf(os.Stderr, "[WARNING] pyproject.toml parsed successfully but [project].requires-python is empty or missing\n")
		// Check if it exists in the raw file
		if strings.Contains(string(fileContent), "requires-python") {
			fmt.Fprintf(os.Stderr, "[WARNING] requires-python field EXISTS in file but was not parsed into struct\n")
			// Try to extract it manually
			re := regexp.MustCompile(`requires-python\s*=\s*"([^"]+)"`)
			if matches := re.FindStringSubmatch(string(fileContent)); len(matches) > 1 {
				fmt.Fprintf(os.Stderr, "[INFO] Manual extraction found: requires-python = %q\n", matches[1])
			}
		}
	}

	// Extract common metadata
	metadata.Name = pyproject.Project.Name
	metadata.Version = pyproject.Project.Version
	metadata.Description = pyproject.Project.Description

	// Handle license - can be string or table format per PEP 621
	if pyproject.Project.License != nil {
		switch license := pyproject.Project.License.(type) {
		case string:
			metadata.License = license
		case map[string]interface{}:
			// Handle {text = "..."} or {file = "..."}
			if text, ok := license["text"].(string); ok {
				metadata.License = text
			} else if file, ok := license["file"].(string); ok {
				metadata.License = fmt.Sprintf("file:%s", file)
			}
		}
	}

	metadata.VersionSource = "pyproject.toml"

	// Extract authors
	authors := make([]string, 0, len(pyproject.Project.Authors))
	for _, author := range pyproject.Project.Authors {
		if author.Name != "" {
			if author.Email != "" {
				authors = append(authors, fmt.Sprintf("%s <%s>", author.Name, author.Email))
			} else {
				authors = append(authors, author.Name)
			}
		}
	}
	metadata.Authors = authors

	// Extract URLs
	if len(pyproject.Project.URLs) > 0 {
		for key, value := range pyproject.Project.URLs {
			lowerKey := strings.ToLower(key)
			if lowerKey == "homepage" || lowerKey == "home" {
				metadata.Homepage = value
			} else if lowerKey == "repository" || lowerKey == "source" {
				metadata.Repository = value
			}
		}
	}

	// Python-specific metadata
	metadata.LanguageSpecific["package_name"] = pyproject.Project.Name
	// Store requires_python even if empty (for diagnostics)
	metadata.LanguageSpecific["requires_python"] = pyproject.Project.RequiresPython
	metadata.LanguageSpecific["build_backend"] = pyproject.BuildSystem.BuildBackend
	metadata.LanguageSpecific["build_requires"] = pyproject.BuildSystem.Requires

	// Debug: Log requires_python value and provide detailed diagnostic
	requiresPythonValue := pyproject.Project.RequiresPython
	fmt.Fprintf(os.Stderr, "[DEBUG] pyproject.Project.RequiresPython = %q (len=%d, empty=%v)\n",
		requiresPythonValue, len(requiresPythonValue), requiresPythonValue == "")
	if requiresPythonValue == "" {
		fmt.Fprintf(os.Stderr, "[DEBUG] RequiresPython is EMPTY - matrix generation will be skipped\n")
	}
	metadata.LanguageSpecific["metadata_source"] = "pyproject.toml"
	metadata.LanguageSpecific["keywords"] = pyproject.Project.Keywords
	metadata.LanguageSpecific["classifiers"] = pyproject.Project.Classifiers

	// Check if version is dynamic
	isDynamic := false
	for _, field := range pyproject.Project.Dynamic {
		if field == "version" {
			metadata.LanguageSpecific["versioning_type"] = "dynamic"
			isDynamic = true
			break
		}
	}
	if !isDynamic {
		metadata.LanguageSpecific["versioning_type"] = "static"
	}

	// Extract dependencies
	if len(pyproject.Project.Dependencies) > 0 {
		metadata.LanguageSpecific["dependencies"] = pyproject.Project.Dependencies
		metadata.LanguageSpecific["dependency_count"] = len(pyproject.Project.Dependencies)
	}

	// Extract tool-specific configurations
	if pyproject.Tool != nil {
		// Poetry
		if poetry, ok := pyproject.Tool["poetry"].(map[string]interface{}); ok {
			metadata.LanguageSpecific["poetry_config"] = true
			if version, ok := poetry["version"].(string); ok && metadata.Version == "" {
				metadata.Version = version
				metadata.VersionSource = "pyproject.toml (poetry)"
			}
		}

		// PDM
		if pdm, ok := pyproject.Tool["pdm"].(map[string]interface{}); ok {
			metadata.LanguageSpecific["pdm_config"] = true
			if version, ok := pdm["version"].(map[string]interface{}); ok {
				metadata.LanguageSpecific["pdm_version_source"] = version["source"]
			}
		}

		// Hatch
		if hatch, ok := pyproject.Tool["hatch"].(map[string]interface{}); ok {
			metadata.LanguageSpecific["hatch_config"] = true
			if version, ok := hatch["version"].(map[string]interface{}); ok {
				metadata.LanguageSpecific["hatch_version_source"] = version["source"]
			}
		}

		// Setuptools
		if _, ok := pyproject.Tool["setuptools"].(map[string]interface{}); ok {
			metadata.LanguageSpecific["setuptools_config"] = true
		}
	}

	// Generate Python version matrix
	if pyproject.Project.RequiresPython != "" {
		fmt.Fprintf(os.Stderr, "[DEBUG] Generating matrix for requires_python: %q\n", pyproject.Project.RequiresPython)
		matrix := generatePythonVersionMatrix(pyproject.Project.RequiresPython)
		fmt.Fprintf(os.Stderr, "[DEBUG] Generated matrix: %v (len=%d)\n", matrix, len(matrix))
		if len(matrix) > 0 {
			metadata.LanguageSpecific["version_matrix"] = matrix

			// Convert to JSON for easy use in GitHub Actions
			matrixJSON := fmt.Sprintf(`{"python-version": [%s]}`,
				strings.Join(quoteStrings(matrix), ", "))
			metadata.LanguageSpecific["matrix_json"] = matrixJSON

			// Set recommended build version (latest from matrix)
			if len(matrix) > 0 {
				metadata.LanguageSpecific["build_version"] = matrix[len(matrix)-1]
				fmt.Fprintf(os.Stderr, "[DEBUG] Set build_version to: %s\n", matrix[len(matrix)-1])
			}
		} else {
			fmt.Fprintf(os.Stderr, "[DEBUG] Matrix generation returned empty slice\n")
		}
	} else {
		fmt.Fprintf(os.Stderr, "[DEBUG] RequiresPython is empty, skipping matrix generation\n")
	}

	// Compare project name and package name
	if metadata.Name != "" && pyproject.Project.Name != "" {
		// Package name is project name with dashes replaced by underscores
		packageName := strings.ReplaceAll(pyproject.Project.Name, "-", "_")
		projectMatchPackage := metadata.Name == packageName
		metadata.LanguageSpecific["project_match_package"] = projectMatchPackage
	}

	return nil
}

// extractFromSetupCfg extracts metadata from setup.cfg
func (e *Extractor) extractFromSetupCfg(path string, metadata *extractor.ProjectMetadata) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read setup.cfg: %w", err)
	}

	// Parse INI-style configuration
	cfg := parseINI(string(content))

	// Extract metadata section
	if metadataSection, ok := cfg["metadata"]; ok {
		metadata.Name = metadataSection["name"]
		metadata.Version = metadataSection["version"]
		metadata.Description = metadataSection["description"]
		metadata.License = metadataSection["license"]
		metadata.Homepage = metadataSection["url"]
		metadata.VersionSource = "setup.cfg"

		// Authors
		if author := metadataSection["author"]; author != "" {
			email := metadataSection["author_email"]
			if email != "" {
				metadata.Authors = []string{fmt.Sprintf("%s <%s>", author, email)}
			} else {
				metadata.Authors = []string{author}
			}
		}

		// Python-specific
		metadata.LanguageSpecific["package_name"] = metadataSection["name"]
		metadata.LanguageSpecific["metadata_source"] = "setup.cfg"

		if pythonRequires := metadataSection["python_requires"]; pythonRequires != "" {
			metadata.LanguageSpecific["requires_python"] = pythonRequires

			// Generate matrix
			matrix := generatePythonVersionMatrix(pythonRequires)
			if len(matrix) > 0 {
				metadata.LanguageSpecific["version_matrix"] = matrix
				matrixJSON := fmt.Sprintf(`{"python-version": [%s]}`,
					strings.Join(quoteStrings(matrix), ", "))
				metadata.LanguageSpecific["matrix_json"] = matrixJSON

				// Set recommended build version (latest from matrix)
				metadata.LanguageSpecific["build_version"] = matrix[len(matrix)-1]
			}
		}
	}

	// Extract options section
	if optionsSection, ok := cfg["options"]; ok {
		if installRequires := optionsSection["install_requires"]; installRequires != "" {
			deps := strings.Split(installRequires, "\n")
			cleanDeps := make([]string, 0, len(deps))
			for _, dep := range deps {
				dep = strings.TrimSpace(dep)
				if dep != "" {
					cleanDeps = append(cleanDeps, dep)
				}
			}
			if len(cleanDeps) > 0 {
				metadata.LanguageSpecific["dependencies"] = cleanDeps
				metadata.LanguageSpecific["dependency_count"] = len(cleanDeps)
			}
		}
	}

	// Compare project name and package name
	if metadata.Name != "" {
		// Package name is project name with dashes replaced by underscores
		packageName := strings.ReplaceAll(metadata.Name, "-", "_")
		projectMatchPackage := metadata.Name == packageName
		metadata.LanguageSpecific["project_match_package"] = projectMatchPackage
	}

	return nil
}

// extractFromSetupPy extracts metadata from setup.py using regex patterns
func (e *Extractor) extractFromSetupPy(path string, metadata *extractor.ProjectMetadata) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read setup.py: %w", err)
	}

	text := string(content)

	// Extract common fields using regex
	metadata.Name = extractSetupPyField(text, "name")
	metadata.Version = extractSetupPyField(text, "version")
	metadata.Description = extractSetupPyField(text, "description")
	metadata.License = extractSetupPyField(text, "license")
	metadata.Homepage = extractSetupPyField(text, "url")
	metadata.VersionSource = "setup.py"

	// Extract author
	if author := extractSetupPyField(text, "author"); author != "" {
		email := extractSetupPyField(text, "author_email")
		if email != "" {
			metadata.Authors = []string{fmt.Sprintf("%s <%s>", author, email)}
		} else {
			metadata.Authors = []string{author}
		}
	}

	// Python-specific
	metadata.LanguageSpecific["package_name"] = metadata.Name
	metadata.LanguageSpecific["metadata_source"] = "setup.py"

	if pythonRequires := extractSetupPyField(text, "python_requires"); pythonRequires != "" {
		metadata.LanguageSpecific["requires_python"] = pythonRequires

		// Generate matrix
		matrix := generatePythonVersionMatrix(pythonRequires)
		if len(matrix) > 0 {
			metadata.LanguageSpecific["version_matrix"] = matrix
			matrixJSON := fmt.Sprintf(`{"python-version": [%s]}`,
				strings.Join(quoteStrings(matrix), ", "))
			metadata.LanguageSpecific["matrix_json"] = matrixJSON

			// Set recommended build version (latest from matrix)
			metadata.LanguageSpecific["build_version"] = matrix[len(matrix)-1]
		}
	}

	// Check for dynamic versioning patterns
	if strings.Contains(text, "__version__") ||
		strings.Contains(text, "version=get_version") ||
		strings.Contains(text, "version=read_version") {
		metadata.LanguageSpecific["versioning_type"] = "dynamic"
	} else {
		metadata.LanguageSpecific["versioning_type"] = "static"
	}

	// Compare project name and package name
	if metadata.Name != "" {
		// Package name is project name with dashes replaced by underscores
		packageName := strings.ReplaceAll(metadata.Name, "-", "_")
		projectMatchPackage := metadata.Name == packageName
		metadata.LanguageSpecific["project_match_package"] = projectMatchPackage
	}

	return nil
}

// Detect checks if this extractor can handle the project
func (e *Extractor) Detect(projectPath string) bool {
	// Check for pyproject.toml
	if _, err := os.Stat(filepath.Join(projectPath, "pyproject.toml")); err == nil {
		return true
	}

	// Check for setup.cfg
	if _, err := os.Stat(filepath.Join(projectPath, "setup.cfg")); err == nil {
		return true
	}

	// Check for setup.py
	if _, err := os.Stat(filepath.Join(projectPath, "setup.py")); err == nil {
		return true
	}

	return false
}

// Helper functions

// parseINI parses a simple INI file into a map of sections
func parseINI(content string) map[string]map[string]string {
	result := make(map[string]map[string]string)
	var currentSection string

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		// Section header
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentSection = strings.ToLower(strings.Trim(line, "[]"))
			result[currentSection] = make(map[string]string)
			continue
		}

		// Key-value pair
		if currentSection != "" && strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				result[currentSection][key] = value
			}
		}
	}

	return result
}

// extractSetupPyField extracts a field value from setup.py using regex
func extractSetupPyField(content, field string) string {
	// Pattern: field='value' or field="value" or field='''value''' or field="""value"""
	patterns := []string{
		fmt.Sprintf(`%s\s*=\s*['"]([^'"]+)['"]`, field),
		fmt.Sprintf(`%s\s*=\s*'''([^']+)'''`, field),
		fmt.Sprintf(`%s\s*=\s*"""([^"]+)"""`, field),
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(content); len(matches) > 1 {
			return strings.TrimSpace(matches[1])
		}
	}

	return ""
}

// generatePythonVersionMatrix generates a list of Python versions from a requires-python specifier
func generatePythonVersionMatrix(requiresPython string) []string {
	// Common patterns: ">=3.8", ">=3.8,<4.0", "~=3.8", "<3.13,>=3.11", etc.
	versions := []string{}

	// Extract minimum version
	minVersion := ""
	if strings.Contains(requiresPython, ">=") {
		re := regexp.MustCompile(`>=\s*(\d+\.\d+)`)
		if matches := re.FindStringSubmatch(requiresPython); len(matches) > 1 {
			minVersion = matches[1]
		}
	} else if strings.Contains(requiresPython, "~=") {
		re := regexp.MustCompile(`~=\s*(\d+\.\d+)`)
		if matches := re.FindStringSubmatch(requiresPython); len(matches) > 1 {
			minVersion = matches[1]
		}
	}

	// Extract maximum version (exclusive upper bound)
	maxVersion := ""
	if strings.Contains(requiresPython, "<") && !strings.Contains(requiresPython, "<=") {
		re := regexp.MustCompile(`<\s*(\d+\.\d+)`)
		if matches := re.FindStringSubmatch(requiresPython); len(matches) > 1 {
			maxVersion = matches[1]
		}
	}

	// Map minimum version to supported versions
	// Only includes actively supported Python versions (3.9+)
	// Python 3.6, 3.7, and 3.8 have reached end-of-life
	supportedVersions := map[string][]string{
		"3.9":  {"3.9", "3.10", "3.11", "3.12", "3.13", "3.14"},
		"3.10": {"3.10", "3.11", "3.12", "3.13", "3.14"},
		"3.11": {"3.11", "3.12", "3.13", "3.14"},
		"3.12": {"3.12", "3.13", "3.14"},
		"3.13": {"3.13", "3.14"},
		"3.14": {"3.14"},
	}

	if minVersion != "" {
		if versionList, ok := supportedVersions[minVersion]; ok {
			// Filter versions based on maximum constraint if present
			if maxVersion != "" {
				filteredVersions := []string{}
				for _, v := range versionList {
					// Compare versions numerically (e.g., "3.9" < "3.11")
					// Simple string comparison works for single-digit minor versions
					if compareVersions(v, maxVersion) < 0 {
						filteredVersions = append(filteredVersions, v)
					}
				}
				versions = filteredVersions
			} else {
				versions = versionList
			}
		} else {
			// Map legacy/unsupported versions to minimum supported version
			// This handles projects still requiring Python 3.6, 3.7, or 3.8
			if minVersion < "3.9" {
				versions = []string{"3.9", "3.10", "3.11", "3.12", "3.13", "3.14"}
			} else {
				versions = []string{"3.9", "3.10", "3.11", "3.12", "3.13", "3.14"}
			}
		}
	}

	// If we couldn't determine, use a reasonable default
	if len(versions) == 0 {
		versions = []string{"3.9", "3.10", "3.11", "3.12", "3.13", "3.14"}
	}

	return versions
}

// quoteStrings adds quotes around each string
// compareVersions compares two version strings (e.g., "3.9" vs "3.11")
// Returns: -1 if v1 < v2, 0 if v1 == v2, 1 if v1 > v2
func compareVersions(v1, v2 string) int {
	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")

	// Compare each part numerically
	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	for i := 0; i < maxLen; i++ {
		var p1, p2 int
		if i < len(parts1) {
			fmt.Sscanf(parts1[i], "%d", &p1)
		}
		if i < len(parts2) {
			fmt.Sscanf(parts2[i], "%d", &p2)
		}

		if p1 < p2 {
			return -1
		}
		if p1 > p2 {
			return 1
		}
	}
	return 0
}

func quoteStrings(strs []string) []string {
	quoted := make([]string, len(strs))
	for i, s := range strs {
		quoted[i] = fmt.Sprintf(`"%s"`, s)
	}
	return quoted
}

// init registers the Python extractor
func init() {
	extractor.RegisterExtractor(NewExtractor())
}
