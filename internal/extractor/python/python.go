// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package python

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

// pythonProjectFiles records which Python project descriptor files are
// present alongside the human-readable found/not-found lists used in
// error messages.
type pythonProjectFiles struct {
	pyprojectPath string
	setupCfgPath  string
	setupPyPath   string

	pyprojectExists bool
	setupCfgExists  bool
	setupPyExists   bool

	filesFound    []string
	filesNotFound []string
}

// detectPythonFiles stats the three Python descriptor files in a fixed
// order (pyproject.toml, setup.cfg, setup.py) and records their presence.
func detectPythonFiles(projectPath string) pythonProjectFiles {
	files := pythonProjectFiles{
		pyprojectPath: filepath.Join(projectPath, "pyproject.toml"),
		setupCfgPath:  filepath.Join(projectPath, "setup.cfg"),
		setupPyPath:   filepath.Join(projectPath, "setup.py"),
	}

	for _, entry := range []struct {
		name   string
		path   string
		exists *bool
	}{
		{"pyproject.toml", files.pyprojectPath, &files.pyprojectExists},
		{"setup.cfg", files.setupCfgPath, &files.setupCfgExists},
		{"setup.py", files.setupPyPath, &files.setupPyExists},
	} {
		if _, err := os.Stat(entry.path); err == nil {
			*entry.exists = true
			files.filesFound = append(files.filesFound, entry.name)
		} else {
			files.filesNotFound = append(files.filesNotFound, entry.name)
		}
	}

	return files
}

// Extract retrieves metadata from a Python project
func (e *Extractor) Extract(projectPath string) (*extractor.ProjectMetadata, error) {
	metadata := &extractor.ProjectMetadata{
		LanguageSpecific: make(map[string]interface{}),
	}

	files := detectPythonFiles(projectPath)

	// Try pyproject.toml first (modern Python)
	if files.pyprojectExists {
		handled, err := e.extractViaPyProject(projectPath, files, metadata)
		if err != nil {
			return nil, err
		}
		if handled {
			return metadata, nil
		}
		// pyproject.toml exists but has no [project] section; fall
		// through to try setup.cfg or setup.py.
	}

	// Try setup.cfg (intermediate format)
	if files.setupCfgExists {
		if err := e.extractFromSetupCfg(files.setupCfgPath, metadata); err != nil {
			return nil, fmt.Errorf("found setup.cfg but failed to parse it: %w\n\nFiles found: %s\nFiles not found: %s",
				err, strings.Join(files.filesFound, ", "), strings.Join(files.filesNotFound, ", "))
		}
		// Canonical PBR layout pairs declarative setup.cfg with a tiny
		// setup.py shim such as `setup(setup_requires=['pbr'], pbr=True)`.
		// Cross-reference setup.py when the cfg-derived versioning_type is
		// still static, so PBR/setuptools-scm/versioneer projects that
		// only signal their dynamic provider from setup.py are not
		// misclassified.
		if files.setupPyExists {
			crossCheckDynamicFromSetupPy(files.setupPyPath, metadata)
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
	if files.setupPyExists {
		if err := e.extractFromSetupPy(files.setupPyPath, metadata); err != nil {
			return nil, fmt.Errorf("found setup.py but failed to parse it: %w\n\nFiles found: %s\nFiles not found: %s",
				err, strings.Join(files.filesFound, ", "), strings.Join(files.filesNotFound, ", "))
		}
		if _, hasDeps := metadata.LanguageSpecific["dependencies"]; !hasDeps {
			loadRequirementsTxt(projectPath, metadata)
		}
		applyFallbackPythonMatrix(metadata, "setup.py")
		return metadata, nil
	}

	return nil, fmt.Errorf("no Python project files found in %s\n\nSearched for: pyproject.toml, setup.cfg, setup.py\nFiles found: %s\nFiles not found: %s",
		projectPath, strings.Join(files.filesFound, ", "), strings.Join(files.filesNotFound, ", "))
}

// extractViaPyProject runs the pyproject.toml extraction path. It returns
// handled=true when pyproject.toml yielded usable metadata (a [project]
// section or a recognised tool config) so the caller should stop, or
// handled=false when the file lacked usable metadata and the caller
// should fall through to setup.cfg/setup.py.
func (e *Extractor) extractViaPyProject(projectPath string, files pythonProjectFiles, metadata *extractor.ProjectMetadata) (bool, error) {
	if err := e.extractFromPyProject(files.pyprojectPath, metadata); err != nil {
		return false, fmt.Errorf("found pyproject.toml but failed to parse it: %w\n\nFiles found: %s\nFiles not found: %s\n\nThis error often occurs due to:\n- Invalid TOML syntax (check for merge conflict markers like <<<<<<<, =======, >>>>>>>)\n- Malformed data structures\n- Encoding issues",
			err, strings.Join(files.filesFound, ", "), strings.Join(files.filesNotFound, ", "))
	}

	// Consider it valid if we have a [project] section OR tool-specific configs.
	hasProjectSection := metadata.Name != ""
	hasToolConfig := metadata.LanguageSpecific["poetry_config"] == true ||
		metadata.LanguageSpecific["pdm_config"] == true ||
		metadata.LanguageSpecific["hatch_config"] == true ||
		metadata.LanguageSpecific["setuptools_config"] == true

	if !hasProjectSection && !hasToolConfig {
		return false, nil
	}

	// We have usable metadata, but might still need requires_python from
	// a sibling setup.py/setup.cfg.
	if metadata.LanguageSpecific["requires_python"] == nil || metadata.LanguageSpecific["requires_python"] == "" {
		e.fillRequiresPythonFromFallback(files, metadata)
	}
	applyFallbackPythonMatrix(metadata, "pyproject.toml")
	return true, nil
}

// fillRequiresPythonFromFallback tries setup.py then setup.cfg to recover
// python-version metadata that a pyproject.toml without a requires-python
// declaration left empty, propagating any matrix the fallback derived.
func (e *Extractor) fillRequiresPythonFromFallback(files pythonProjectFiles, metadata *extractor.ProjectMetadata) {
	if files.setupPyExists {
		fallbackMetadata := &extractor.ProjectMetadata{
			LanguageSpecific: make(map[string]interface{}),
		}
		if err := e.extractFromSetupPy(files.setupPyPath, fallbackMetadata); err == nil {
			propagateFallbackPythonMatrix(metadata, fallbackMetadata)
		}
	}
	if (metadata.LanguageSpecific["requires_python"] == nil || metadata.LanguageSpecific["requires_python"] == "") && files.setupCfgExists {
		fallbackMetadata := &extractor.ProjectMetadata{
			LanguageSpecific: make(map[string]interface{}),
		}
		if err := e.extractFromSetupCfg(files.setupCfgPath, fallbackMetadata); err == nil {
			propagateFallbackPythonMatrix(metadata, fallbackMetadata)
		}
	}
}

// Detect checks if this extractor can handle the project
func (e *Extractor) Detect(projectPath string) bool {
	if _, err := os.Stat(filepath.Join(projectPath, "pyproject.toml")); err == nil {
		return true
	}

	if _, err := os.Stat(filepath.Join(projectPath, "setup.cfg")); err == nil {
		return true
	}

	if _, err := os.Stat(filepath.Join(projectPath, "setup.py")); err == nil {
		return true
	}

	return false
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

// init registers the Python extractor
func init() {
	extractor.RegisterExtractor(NewExtractor())
}
