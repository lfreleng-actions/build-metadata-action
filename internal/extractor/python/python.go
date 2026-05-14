// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package python

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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
						propagateFallbackPythonMatrix(metadata, fallbackMetadata)
					}
				}
				// Try setup.cfg if we still don't have it
				if (metadata.LanguageSpecific["requires_python"] == nil || metadata.LanguageSpecific["requires_python"] == "") && setupCfgExists {
					fallbackMetadata := &extractor.ProjectMetadata{
						LanguageSpecific: make(map[string]interface{}),
					}
					if err := e.extractFromSetupCfg(setupCfgPath, fallbackMetadata); err == nil {
						propagateFallbackPythonMatrix(metadata, fallbackMetadata)
					}
				}
			}
			applyFallbackPythonMatrix(metadata, "pyproject.toml")
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
		// Canonical PBR layout pairs declarative setup.cfg with a tiny
		// setup.py shim such as `setup(setup_requires=['pbr'], pbr=True)`.
		// Cross-reference setup.py when the cfg-derived versioning_type is
		// still static, so PBR/setuptools-scm/versioneer projects that
		// only signal their dynamic provider from setup.py are not
		// misclassified.
		if setupPyExists {
			crossCheckDynamicFromSetupPy(setupPyPath, metadata)
		}
		// setup.cfg projects very commonly delegate the install_requires
		// list to a sibling requirements.txt; pull it in opportunistically
		// so downstream consumers have a non-empty dependency list.
		if _, hasDeps := metadata.LanguageSpecific["dependencies"]; !hasDeps {
			loadRequirementsTxt(projectPath, metadata)
		}
		applyFallbackPythonMatrix(metadata, "setup.cfg")
		return metadata, nil
	}

	// Try setup.py (legacy format)
	if setupPyExists {
		if err := e.extractFromSetupPy(setupPyPath, metadata); err != nil {
			return nil, fmt.Errorf("found setup.py but failed to parse it: %w\n\nFiles found: %s\nFiles not found: %s",
				err, strings.Join(filesFound, ", "), strings.Join(filesNotFound, ", "))
		}
		if _, hasDeps := metadata.LanguageSpecific["dependencies"]; !hasDeps {
			loadRequirementsTxt(projectPath, metadata)
		}
		applyFallbackPythonMatrix(metadata, "setup.py")
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
		metadata.LanguageSpecific["dependencies_source"] = "pyproject.toml"
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

// extractFromSetupCfg extracts metadata from setup.cfg using a
// continuation-aware INI parser. It handles classic declarative
// setuptools layouts, PBR-style configurations, and the older
// hyphen-separated key forms that pre-date PEP 8 alignment.
func (e *Extractor) extractFromSetupCfg(path string, metadata *extractor.ProjectMetadata) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read setup.cfg: %w", err)
	}

	cfg := parseSetupCfg(string(content))

	// Always mark this as the metadata source so downstream consumers
	// can see that setup.cfg was inspected even when individual fields
	// are absent.
	metadata.LanguageSpecific["metadata_source"] = "setup.cfg"
	metadata.VersionSource = "setup.cfg"

	getScalar := func(section, key string) string {
		if s, ok := cfg[section]; ok {
			if v, ok := s[key]; ok {
				return strings.TrimSpace(v.Raw)
			}
		}
		return ""
	}
	getLines := func(section, key string) []string {
		if s, ok := cfg[section]; ok {
			if v, ok := s[key]; ok {
				return v.Lines
			}
		}
		return nil
	}

	// Common metadata fields
	metadata.Name = getScalar("metadata", "name")
	metadata.Version = getScalar("metadata", "version")
	metadata.Description = getScalar("metadata", "description")
	metadata.License = getScalar("metadata", "license")
	metadata.Homepage = getScalar("metadata", "url")
	if metadata.Homepage == "" {
		metadata.Homepage = getScalar("metadata", "home_page")
	}

	if author := getScalar("metadata", "author"); author != "" {
		email := getScalar("metadata", "author_email")
		if email != "" {
			metadata.Authors = []string{fmt.Sprintf("%s <%s>", author, email)}
		} else {
			metadata.Authors = []string{author}
		}
	}

	metadata.LanguageSpecific["package_name"] = metadata.Name

	// Classifiers are multi-line by convention; the parser splits them
	// into one line per entry already.
	classifiers := getLines("metadata", "classifiers")
	if len(classifiers) == 0 {
		// Older PBR/setuptools spelt this as the singular form.
		classifiers = getLines("metadata", "classifier")
	}
	if len(classifiers) > 0 {
		metadata.LanguageSpecific["classifiers"] = classifiers
	}

	if kw := getScalar("metadata", "keywords"); kw != "" {
		metadata.LanguageSpecific["keywords"] = kw
	}

	// python_requires can appear in either [metadata] or [options]; the
	// parser has already normalised the hyphenated form (`python-requires`)
	// to underscores so we only need to check one spelling.
	pythonRequires := getScalar("metadata", "python_requires")
	if pythonRequires == "" {
		pythonRequires = getScalar("options", "python_requires")
	}
	if pythonRequires != "" {
		metadata.LanguageSpecific["requires_python"] = pythonRequires
		matrix := generatePythonVersionMatrix(pythonRequires)
		if len(matrix) > 0 {
			metadata.LanguageSpecific["version_matrix"] = matrix
			metadata.LanguageSpecific["matrix_json"] = fmt.Sprintf(`{"python-version": [%s]}`,
				strings.Join(quoteStrings(matrix), ", "))
			metadata.LanguageSpecific["build_version"] = matrix[len(matrix)-1]
		}
	} else if classifierVersions := derivePythonVersionsFromClassifiers(classifiers); len(classifierVersions) > 0 {
		// No python_requires declared but classifiers contain explicit
		// Python version markers. Treat those as the authoritative matrix.
		metadata.LanguageSpecific["version_matrix"] = classifierVersions
		metadata.LanguageSpecific["matrix_json"] = fmt.Sprintf(`{"python-version": [%s]}`,
			strings.Join(quoteStrings(classifierVersions), ", "))
		metadata.LanguageSpecific["build_version"] = classifierVersions[len(classifierVersions)-1]
		metadata.LanguageSpecific["requires_python_source"] = "classifiers"
	}

	// install_requires: multi-line list
	if deps := getLines("options", "install_requires"); len(deps) > 0 {
		metadata.LanguageSpecific["dependencies"] = deps
		metadata.LanguageSpecific["dependency_count"] = len(deps)
		metadata.LanguageSpecific["dependencies_source"] = "setup.cfg"
	}

	// Determine versioning type and (if dynamic) the provider responsible.
	provider := detectDynamicProviderFromSetupCfg(cfg)
	if provider != "" {
		metadata.LanguageSpecific["versioning_type"] = "dynamic"
		metadata.LanguageSpecific["dynamic_provider"] = provider
		// Any dynamic provider that hasn't already produced a concrete
		// version string is, by definition, unresolved at extraction time
		// (PBR/setuptools-scm/versioneer/runtime-attr all defer to build).
		if strings.TrimSpace(metadata.Version) == "" {
			metadata.LanguageSpecific["version_unresolved"] = true
		}
		// `version = attr:` / `file:` are indirections that only resolve
		// at build-time. Stash the raw expression for diagnostics and
		// clear the surface Version so it doesn't pollute outputs like
		// project_version with a non-resolvable literal.
		if provider == "setuptools-dynamic" {
			if rawVer := strings.TrimSpace(metadata.Version); rawVer != "" {
				if strings.HasPrefix(rawVer, "attr:") || strings.HasPrefix(rawVer, "file:") {
					metadata.LanguageSpecific["version_expression"] = rawVer
					metadata.Version = ""
					metadata.LanguageSpecific["version_unresolved"] = true
				}
			}
		}
	} else {
		metadata.LanguageSpecific["versioning_type"] = "static"
	}

	if metadata.Name != "" {
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

	// Surface install_requires when declared inline, so the Extract()
	// loop knows not to fall back to requirements.txt for projects that
	// already provide an explicit dependency list in setup.py.
	if deps := extractSetupPyInstallRequires(text); len(deps) > 0 {
		metadata.LanguageSpecific["dependencies"] = deps
		metadata.LanguageSpecific["dependency_count"] = len(deps)
		metadata.LanguageSpecific["dependencies_source"] = "setup.py"
	}

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
	} else if classifiers := extractSetupPyClassifiers(text); len(classifiers) > 0 {
		metadata.LanguageSpecific["classifiers"] = classifiers
		if classifierVersions := derivePythonVersionsFromClassifiers(classifiers); len(classifierVersions) > 0 {
			metadata.LanguageSpecific["version_matrix"] = classifierVersions
			metadata.LanguageSpecific["matrix_json"] = fmt.Sprintf(`{"python-version": [%s]}`,
				strings.Join(quoteStrings(classifierVersions), ", "))
			metadata.LanguageSpecific["build_version"] = classifierVersions[len(classifierVersions)-1]
			metadata.LanguageSpecific["requires_python_source"] = "classifiers"
		}
	}

	// Determine versioning type and (if dynamic) the provider responsible.
	provider := detectDynamicProviderFromSetupPy(text)
	if provider != "" {
		metadata.LanguageSpecific["versioning_type"] = "dynamic"
		metadata.LanguageSpecific["dynamic_provider"] = provider
		// All dynamic providers resolve the real version at build time;
		// surface that as `version_unresolved` whenever extraction did
		// not turn up a concrete value (matches setup.cfg behaviour).
		if strings.TrimSpace(metadata.Version) == "" {
			metadata.LanguageSpecific["version_unresolved"] = true
		}
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

// crossCheckDynamicFromSetupPy reads a sibling setup.py and, if it
// reveals a dynamic versioning provider that the setup.cfg analysis did
// not surface, upgrades the metadata accordingly. This is the canonical
// PBR layout (declarative setup.cfg + minimal setup.py shim).
func crossCheckDynamicFromSetupPy(setupPyPath string, metadata *extractor.ProjectMetadata) {
	if metadata == nil || metadata.LanguageSpecific == nil {
		return
	}
	if provider, _ := metadata.LanguageSpecific["dynamic_provider"].(string); provider != "" {
		return // already determined from setup.cfg
	}
	content, err := os.ReadFile(setupPyPath)
	if err != nil {
		return
	}
	provider := detectDynamicProviderFromSetupPy(string(content))
	if provider == "" {
		return
	}
	metadata.LanguageSpecific["versioning_type"] = "dynamic"
	metadata.LanguageSpecific["dynamic_provider"] = provider
	if strings.TrimSpace(metadata.Version) == "" {
		metadata.LanguageSpecific["version_unresolved"] = true
	}
}

// loadRequirementsTxt opportunistically loads `requirements.txt` from the
// project root and records it as the dependency list. PBR/OpenStack-style
// projects typically delegate runtime dependency declaration to this file
// rather than `install_requires` in setup.cfg/setup.py.
func loadRequirementsTxt(projectPath string, metadata *extractor.ProjectMetadata) {
	path := filepath.Join(projectPath, "requirements.txt")
	content, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var deps []string
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") {
			// Skip blanks, comments, and pip directives (-r, -c, -e ...).
			continue
		}
		// Strip inline comments. pip's parser only recognises `#` as a
		// comment when preceded by whitespace, so e.g. URL fragments like
		// `pkg @ https://example.com/x#egg=pkg` are preserved intact.
		if idx := indexInlineComment(line); idx >= 0 {
			line = strings.TrimSpace(line[:idx])
			if line == "" {
				continue
			}
		}
		deps = append(deps, line)
	}
	if len(deps) > 0 {
		metadata.LanguageSpecific["dependencies"] = deps
		metadata.LanguageSpecific["dependency_count"] = len(deps)
		metadata.LanguageSpecific["dependencies_source"] = "requirements.txt"
	}
}

// indexInlineComment returns the index of an inline `#` comment marker
// in a requirements.txt line, or -1 if none is present. pip treats `#`
// as the start of a comment only when preceded by ASCII whitespace, so
// URL fragments such as `pkg @ https://x.example/y#egg=pkg` survive.
func indexInlineComment(line string) int {
	for i := 1; i < len(line); i++ {
		if line[i] == '#' && (line[i-1] == ' ' || line[i-1] == '\t') {
			return i
		}
	}
	return -1
}

// Helper functions

// setupCfgValue represents a value parsed from setup.cfg. Python's
// RawConfigParser folds indented continuation lines onto the preceding
// key; multi-line values are typically intended as lists. We retain both
// the raw scalar (joined with newlines, trimmed) and the per-line split
// for convenience.
type setupCfgValue struct {
	Raw   string
	Lines []string
}

// parseSetupCfg parses a setup.cfg file using continuation-aware INI
// rules. Section and key names are lowercased and hyphens normalised to
// underscores so callers can look up canonical names regardless of
// whether the file used `author-email` (older setuptools / PBR) or
// `author_email` (canonical) styles. Both `=` and `:` separators are
// accepted, matching Python's configparser.
func parseSetupCfg(content string) map[string]map[string]setupCfgValue {
	result := make(map[string]map[string]setupCfgValue)
	var currentSection string
	var currentKey string
	var currentValue []string

	flush := func() {
		if currentSection == "" || currentKey == "" {
			return
		}
		raw := strings.TrimSpace(strings.Join(currentValue, "\n"))
		var lines []string
		if raw != "" {
			for _, l := range strings.Split(raw, "\n") {
				l = strings.TrimSpace(l)
				if l != "" {
					lines = append(lines, l)
				}
			}
		}
		result[currentSection][currentKey] = setupCfgValue{Raw: raw, Lines: lines}
	}

	for _, rawLine := range strings.Split(content, "\n") {
		line := strings.TrimRight(rawLine, "\r")
		trimmed := strings.TrimSpace(line)
		isIndented := len(line) > 0 && (line[0] == ' ' || line[0] == '\t')

		// Indented (continuation) whitespace-only lines are preserved as
		// empty entries rather than terminating the value, matching
		// Python's RawConfigParser semantics.
		if trimmed == "" && isIndented && currentKey != "" {
			currentValue = append(currentValue, "")
			continue
		}

		// A fully blank (unindented) line terminates the current value.
		if trimmed == "" {
			flush()
			currentKey = ""
			currentValue = nil
			continue
		}

		// Full-line comments (configparser treats inline `;` as part of
		// the value, so only handle leading-character comments here).
		if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
			continue
		}

		// Section header
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			flush()
			// Trim brackets first, then strip any internal whitespace so
			// inputs like `[ metadata ]` normalise to `metadata` (matching
			// Python's configparser behaviour).
			currentSection = strings.ToLower(strings.TrimSpace(strings.Trim(trimmed, "[]")))
			if _, ok := result[currentSection]; !ok {
				result[currentSection] = make(map[string]setupCfgValue)
			}
			currentKey = ""
			currentValue = nil
			continue
		}

		// Continuation line: starts with whitespace and we already have a key
		isContinuation := isIndented
		if isContinuation && currentKey != "" {
			currentValue = append(currentValue, trimmed)
			continue
		}

		if currentSection == "" {
			continue
		}

		sep := -1
		if idx := strings.Index(trimmed, "="); idx >= 0 {
			sep = idx
			if jdx := strings.Index(trimmed, ":"); jdx >= 0 && jdx < idx {
				sep = jdx
			}
		} else if idx := strings.Index(trimmed, ":"); idx >= 0 {
			sep = idx
		}
		if sep < 0 {
			continue
		}

		flush()
		key := strings.TrimSpace(trimmed[:sep])
		val := strings.TrimSpace(trimmed[sep+1:])
		key = strings.ReplaceAll(strings.ToLower(key), "-", "_")
		currentKey = key
		currentValue = []string{val}
	}
	flush()
	return result
}

// isSupportedPythonVersion returns true when v (in `X.Y` form) is part of
// the set of Python versions this action's matrix generator actively
// emits. It defers to `supportedPythonVersions` so the classifier-derived
// matrix path and the requires-python-derived matrix path always agree
// on which versions are buildable.
func isSupportedPythonVersion(v string) bool {
	for _, s := range supportedPythonVersions {
		if s == v {
			return true
		}
	}
	return false
}

// derivePythonVersionsFromClassifiers extracts Python `X.Y` versions from
// PEP-301 trove classifiers. Returns a deduplicated, version-sorted list
// filtered to the set of actively supported Python versions (3.9+); EOL
// versions (2.x, 3.6-3.8) are dropped so callers do not attempt to run
// against interpreters that GitHub-hosted runners no longer install.
func derivePythonVersionsFromClassifiers(classifiers []string) []string {
	re := regexp.MustCompile(`Programming Language\s*::\s*Python\s*::\s*(\d+\.\d+)`)
	seen := make(map[string]struct{})
	var versions []string
	for _, c := range classifiers {
		if matches := re.FindStringSubmatch(c); len(matches) > 1 {
			v := matches[1]
			if !isSupportedPythonVersion(v) {
				continue
			}
			if _, ok := seen[v]; !ok {
				seen[v] = struct{}{}
				versions = append(versions, v)
			}
		}
	}
	sort.Slice(versions, func(i, j int) bool {
		return compareVersions(versions[i], versions[j]) < 0
	})
	return versions
}

// detectDynamicProviderFromSetupCfg returns the name of the dynamic
// versioning provider in use, or empty string if the project's version is
// static. Recognised providers: "pbr", "setuptools-scm", "versioneer",
// "setuptools-dynamic".
func detectDynamicProviderFromSetupCfg(cfg map[string]map[string]setupCfgValue) string {
	if _, ok := cfg["pbr"]; ok {
		return "pbr"
	}
	if meta, ok := cfg["metadata"]; ok {
		if v, ok := meta["version"]; ok {
			s := strings.TrimSpace(v.Raw)
			if strings.HasPrefix(s, "attr:") || strings.HasPrefix(s, "file:") {
				return "setuptools-dynamic"
			}
		}
	}
	if opts, ok := cfg["options"]; ok {
		if v, ok := opts["setup_requires"]; ok {
			for _, line := range v.Lines {
				name := extractRequirementName(line)
				switch name {
				case "pbr":
					return "pbr"
				case "setuptools_scm", "setuptools-scm":
					return "setuptools-scm"
				case "versioneer":
					return "versioneer"
				}
			}
		}
	}
	return ""
}

// extractRequirementName returns the lowercased distribution name token
// at the start of a PEP 508 requirement line (e.g. `pbr>=2.0 ; ...` ->
// `pbr`). Returns an empty string when no valid name is found. This is
// deliberately stricter than substring matching so that requirements
// such as `sphinx-pbr-theme` do not get mistaken for `pbr`.
func extractRequirementName(line string) string {
	s := strings.TrimSpace(line)
	// Drop common surrounding punctuation left over from list/quoted forms.
	s = strings.Trim(s, "'\",[]() \t")
	nameRe := regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*`)
	match := nameRe.FindString(s)
	return strings.ToLower(match)
}

// extractSetupRequiresNames returns the lowercased distribution names of
// every requirement listed in a `setup_requires=[...]` keyword argument
// inside a setup.py source. Each requirement is parsed via
// `extractRequirementName` so that PEP 508 version specifiers, environment
// markers, and extras are stripped before name matching. Returns an empty
// slice when no `setup_requires=[...]` is found.
func extractSetupRequiresNames(text string) []string {
	listRe := regexp.MustCompile(`(?s)setup_requires\s*=\s*\[([^\]]*)\]`)
	itemRe := regexp.MustCompile(`['"]([^'"]+)['"]`)
	var names []string
	for _, list := range listRe.FindAllStringSubmatch(text, -1) {
		for _, item := range itemRe.FindAllStringSubmatch(list[1], -1) {
			if name := extractRequirementName(item[1]); name != "" {
				names = append(names, name)
			}
		}
	}
	return names
}

// detectDynamicProviderFromSetupPy returns the name of the dynamic
// versioning provider implied by a setup.py file, or empty string if the
// version appears static. The heuristics deliberately err on the side of
// recognising PBR/setuptools-scm/versioneer rather than the previous
// substring checks which only matched a handful of helper-function names.
func detectDynamicProviderFromSetupPy(text string) string {
	lower := strings.ToLower(text)

	// Parse the setup_requires=[...] list (if any) and extract the
	// distribution name for each entry. Matching on the parsed name
	// (rather than on a regex prefix inside the list) avoids treating
	// unrelated packages such as `sphinx-pbr-theme` or
	// `setuptools_scm_git_archive` as PBR / setuptools-scm providers.
	setupRequiresNames := extractSetupRequiresNames(text)
	hasPbrRequirement := false
	hasScmRequirement := false
	hasVersioneerRequirement := false
	for _, name := range setupRequiresNames {
		switch name {
		case "pbr":
			hasPbrRequirement = true
		case "setuptools-scm", "setuptools_scm":
			hasScmRequirement = true
		case "versioneer":
			hasVersioneerRequirement = true
		}
	}

	// `pbr=True` keyword argument to setup(...). Use word boundaries to
	// avoid matching unrelated identifiers ending in `pbr`.
	pbrKwarg := regexp.MustCompile(`\bpbr\s*=\s*true\b`).MatchString(lower)

	if pbrKwarg || hasPbrRequirement {
		return "pbr"
	}
	if strings.Contains(lower, "use_scm_version") || hasScmRequirement {
		return "setuptools-scm"
	}
	if hasVersioneerRequirement ||
		strings.Contains(lower, "versioneer.get_version") ||
		strings.Contains(lower, "versioneer.get_cmdclass") {
		return "versioneer"
	}
	// Legacy helper-call patterns (kept from the original implementation)
	if strings.Contains(text, "__version__") ||
		strings.Contains(text, "version=get_version") ||
		strings.Contains(text, "version=read_version") {
		return "runtime-attr"
	}
	return ""
}

// propagateFallbackPythonMatrix copies python-version-related metadata
// from a setup.py/setup.cfg fallback extraction back into the primary
// (pyproject.toml-derived) metadata map. `requires_python`,
// `version_matrix`, `matrix_json`, `build_version`, and
// `requires_python_source` are each propagated independently so that a
// classifier-derived matrix (which produces `requires_python_source =
// "classifiers"` without populating `requires_python`) is honoured even
// when `requires_python` itself is empty in the fallback.
func propagateFallbackPythonMatrix(metadata, fallbackMetadata *extractor.ProjectMetadata) {
	if metadata == nil || metadata.LanguageSpecific == nil || fallbackMetadata == nil || fallbackMetadata.LanguageSpecific == nil {
		return
	}
	if requiresPython, ok := fallbackMetadata.LanguageSpecific["requires_python"].(string); ok && requiresPython != "" {
		metadata.LanguageSpecific["requires_python"] = requiresPython
	}
	if matrix, ok := fallbackMetadata.LanguageSpecific["version_matrix"].([]string); ok && len(matrix) > 0 {
		metadata.LanguageSpecific["version_matrix"] = matrix
	}
	if matrixJSON, ok := fallbackMetadata.LanguageSpecific["matrix_json"].(string); ok && matrixJSON != "" {
		metadata.LanguageSpecific["matrix_json"] = matrixJSON
	}
	if buildVersion, ok := fallbackMetadata.LanguageSpecific["build_version"].(string); ok && buildVersion != "" {
		metadata.LanguageSpecific["build_version"] = buildVersion
	}
	if source, ok := fallbackMetadata.LanguageSpecific["requires_python_source"].(string); ok && source != "" {
		metadata.LanguageSpecific["requires_python_source"] = source
	}
}

// applyFallbackPythonMatrix populates Python version matrix metadata with
// a sensible default when the project does not declare a supported Python
// version range. Many legacy projects (notably PBR-based packages with a
// setup.cfg or setup.py that omits python_requires) rely on the build
// environment's installed Python rather than declaring a constraint.
// Without a fallback, downstream actions cannot determine which Python
// version to use for the build and fail outright.
//
// The fallback only fires when build_version is missing; projects that
// declared requires-python or version classifiers keep their derived
// matrix unchanged. The fallback emits requires_python_fallback=true so
// downstream consumers can surface a warning to the user.
func applyFallbackPythonMatrix(metadata *extractor.ProjectMetadata, source string) {
	if metadata == nil || metadata.LanguageSpecific == nil {
		return
	}
	if buildVersion, ok := metadata.LanguageSpecific["build_version"].(string); ok && buildVersion != "" {
		return
	}

	fallback := generatePythonVersionMatrix("")
	if len(fallback) == 0 {
		return
	}

	metadata.LanguageSpecific["version_matrix"] = fallback
	metadata.LanguageSpecific["matrix_json"] = fmt.Sprintf(`{"python-version": [%s]}`,
		strings.Join(quoteStrings(fallback), ", "))
	metadata.LanguageSpecific["build_version"] = fallback[len(fallback)-1]
	metadata.LanguageSpecific["requires_python_fallback"] = true
	// Mark the source of the resulting matrix so downstream consumers can
	// tell a fallback guess apart from a matrix derived from
	// `requires-python` or trove classifiers. Only set when no upstream
	// path has already declared a source (e.g. "classifiers").
	if src, ok := metadata.LanguageSpecific["requires_python_source"].(string); !ok || src == "" {
		metadata.LanguageSpecific["requires_python_source"] = "fallback"
	}

	fmt.Fprintf(os.Stderr,
		"[WARNING] %s does not declare requires-python or Python classifiers; using fallback Python matrix %v (build_version=%s)\n",
		source, fallback, fallback[len(fallback)-1])
}

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

// extractSetupPyInstallRequires returns the list of install_requires entries
// declared in a setup.py file. It handles single- and double-quoted entries
// inside the `install_requires=[...]` keyword argument. Empty/whitespace
// items are skipped. Returns nil when the keyword is absent.
func extractSetupPyInstallRequires(content string) []string {
	listRe := regexp.MustCompile(`(?s)install_requires\s*=\s*\[(.*?)\]`)
	listMatch := listRe.FindStringSubmatch(content)
	if len(listMatch) < 2 {
		return nil
	}
	itemRe := regexp.MustCompile(`['"]([^'"]+)['"]`)
	items := itemRe.FindAllStringSubmatch(listMatch[1], -1)
	var result []string
	for _, m := range items {
		if len(m) > 1 {
			if v := strings.TrimSpace(m[1]); v != "" {
				result = append(result, v)
			}
		}
	}
	return result
}

// extractSetupPyClassifiers returns the list of trove classifier strings
// declared in a setup.py file. It handles single- and double-quoted
// entries inside the `classifiers=[...]` keyword argument.
func extractSetupPyClassifiers(content string) []string {
	listRe := regexp.MustCompile(`(?s)classifiers\s*=\s*\[(.*?)\]`)
	listMatch := listRe.FindStringSubmatch(content)
	if len(listMatch) < 2 {
		return nil
	}
	itemRe := regexp.MustCompile(`['"]([^'"]+)['"]`)
	items := itemRe.FindAllStringSubmatch(listMatch[1], -1)
	var result []string
	for _, m := range items {
		if len(m) > 1 {
			result = append(result, m[1])
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

// supportedPythonVersions is the single source of truth for the set of
// Python versions this action actively supports. Both
// `generatePythonVersionMatrix` and `isSupportedPythonVersion` derive
// their behaviour from this slice, so the matrix produced from a
// `requires-python` specifier and the matrix derived from PEP-301 trove
// classifiers always agree on which versions are buildable.
//
// Update this slice (and only this slice) when Python's release cadence
// changes; the rest of the extractor follows.
var supportedPythonVersions = []string{"3.9", "3.10", "3.11", "3.12", "3.13", "3.14"}

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

	// Map minimum version to supported versions. The slices are derived
	// on the fly from `supportedPythonVersions` so adding (or retiring)
	// a Python version only needs to be done in one place.
	supportedVersions := map[string][]string{}
	for i, v := range supportedPythonVersions {
		supportedVersions[v] = append([]string(nil), supportedPythonVersions[i:]...)
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
			// Legacy / unsupported minimum version: route through to
			// the full supported set regardless of whether the request
			// was below 3.9 or above the known maximum.
			versions = append([]string(nil), supportedPythonVersions...)
		}
	}

	// If we couldn't determine, use a reasonable default
	if len(versions) == 0 {
		versions = append([]string(nil), supportedPythonVersions...)
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
