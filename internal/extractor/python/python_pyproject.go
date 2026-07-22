// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package python

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/lfreleng-actions/build-metadata-action/internal/extractor"
)

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

// extractFromPyProject extracts metadata from pyproject.toml
func (e *Extractor) extractFromPyProject(path string, metadata *extractor.ProjectMetadata) error {
	pyproject, fileContent, err := decodePyProject(path)
	if err != nil {
		return err
	}

	warnMissingPyProjectFields(pyproject, fileContent)

	applyPyProjectCoreMetadata(metadata, pyproject)
	applyPyProjectLanguageSpecific(metadata, pyproject)
	poetryPythonConstraint := applyPyProjectToolConfig(metadata, pyproject)

	if err := generatePyProjectMatrix(metadata, pyproject, poetryPythonConstraint); err != nil {
		return err
	}

	if metadata.Name != "" && pyproject.Project.Name != "" {
		// Package name is project name with dashes replaced by underscores
		packageName := strings.ReplaceAll(pyproject.Project.Name, "-", "_")
		metadata.LanguageSpecific["project_match_package"] = metadata.Name == packageName
	}

	return nil
}

// decodePyProject reads and decodes pyproject.toml, returning both the
// parsed struct and the raw bytes (the latter is used for diagnostics on
// fields that failed to bind). It surfaces detailed errors for the two
// most common corruption modes: an unquoted version value and general
// TOML syntax faults such as merge-conflict markers.
func decodePyProject(path string) (PyProjectTOML, []byte, error) {
	var pyproject PyProjectTOML

	fileContent, readErr := os.ReadFile(path)
	if readErr != nil {
		return pyproject, nil, fmt.Errorf("failed to read pyproject.toml: %w", readErr)
	}

	// Detect an unquoted version value (invalid TOML syntax) in the
	// [project] table, for example `version = 1.0.0` written by a buggy
	// patching tool. The descriptive error is returned to the caller,
	// which surfaces it to the user, rather than printed here.
	if raw := projectTableUnquotedVersion(string(fileContent)); raw != "" {
		return pyproject, fileContent, fmt.Errorf(
			"pyproject.toml [project].version has invalid TOML syntax: "+
				"unquoted value %q (should be version = %q)", raw, raw)
	}

	if _, err := toml.DecodeFile(path, &pyproject); err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "expected") || strings.Contains(errMsg, "invalid") {
			// Verbose-only diagnostics; the returned error already carries the
			// detail, and the file preview must not leak into normal CI logs.
			debugf("[ERROR] TOML parsing failed for %s\n", path)
			debugf("[ERROR] Error: %v\n", err)
			preview := string(fileContent)
			if len(preview) > 500 {
				preview = preview[:500] + "..."
			}
			debugf("[ERROR] File preview:\n%s\n", preview)
			return pyproject, fileContent, fmt.Errorf("TOML parsing failed - file contains invalid TOML syntax: %w\n\nCommon causes:\n- Git merge conflict markers (<<<<<<<, =======, >>>>>>>)\n- Unclosed strings or brackets\n- Invalid escape sequences\n- Incorrect indentation or structure", err)
		}
		return pyproject, fileContent, fmt.Errorf("TOML parsing failed: %w", err)
	}

	return pyproject, fileContent, nil
}

// projectTableUnquotedVersion returns the raw value when the [project]
// table declares `version` with an unquoted (invalid TOML) value such
// as `version = 1.0.0`. It returns "" when the version is absent or
// quoted. The scan is limited to the [project] table so unrelated
// tables (for example [tool.*]) cannot trigger a false positive.
func projectTableUnquotedVersion(content string) string {
	unquoted := regexp.MustCompile(`^\s*version\s*=\s*([^"'\s][^\s]*)\s*$`)
	inProject := false
	for _, line := range strings.Split(content, "\n") {
		if trimmed := strings.TrimSpace(line); strings.HasPrefix(trimmed, "[") {
			inProject = trimmed == "[project]"
			continue
		}
		if !inProject {
			continue
		}
		if m := unquoted.FindStringSubmatch(line); len(m) > 1 {
			return m[1]
		}
	}
	return ""
}

// warnMissingPyProjectFields emits verbose diagnostics for empty core
// fields. requires-python gets extra attention: when it is missing from
// the struct but present in the raw text we log a manual extraction so
// operators can see the field exists but failed to bind. Output is gated
// behind INPUT_VERBOSE via debugf so normal runs stay quiet.
func warnMissingPyProjectFields(pyproject PyProjectTOML, fileContent []byte) {
	if pyproject.Project.Name == "" {
		debugf("[WARNING] pyproject.toml parsed successfully but [project].name is empty\n")
	}
	if pyproject.Project.Version == "" {
		debugf("[WARNING] pyproject.toml parsed successfully but [project].version is empty\n")
	}
	if pyproject.Project.RequiresPython == "" {
		debugf("[WARNING] pyproject.toml parsed successfully but [project].requires-python is empty or missing\n")
		if strings.Contains(string(fileContent), "requires-python") {
			debugf("[WARNING] requires-python field EXISTS in file but was not parsed into struct\n")
			re := regexp.MustCompile(`requires-python\s*=\s*"([^"]+)"`)
			if matches := re.FindStringSubmatch(string(fileContent)); len(matches) > 1 {
				debugf("[INFO] Manual extraction found: requires-python = %q\n", matches[1])
			}
		}
	}
}

// applyPyProjectCoreMetadata copies the PEP 621 `[project]` scalar and
// list fields (name, version, description, license, authors, URLs) onto
// the shared metadata struct.
func applyPyProjectCoreMetadata(metadata *extractor.ProjectMetadata, pyproject PyProjectTOML) {
	metadata.Name = pyproject.Project.Name
	metadata.Version = pyproject.Project.Version
	metadata.Description = pyproject.Project.Description

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

	for key, value := range pyproject.Project.URLs {
		lowerKey := strings.ToLower(key)
		switch lowerKey {
		case "homepage", "home":
			metadata.Homepage = value
		case "repository", "source":
			metadata.Repository = value
		}
	}
}

// applyPyProjectLanguageSpecific records the Python-specific metadata
// (package name, build backend, keywords, classifiers, versioning type,
// dependencies) that downstream consumers read from LanguageSpecific.
func applyPyProjectLanguageSpecific(metadata *extractor.ProjectMetadata, pyproject PyProjectTOML) {
	metadata.LanguageSpecific["package_name"] = pyproject.Project.Name
	// Store requires_python even if empty (for diagnostics)
	metadata.LanguageSpecific["requires_python"] = pyproject.Project.RequiresPython
	metadata.LanguageSpecific["build_backend"] = pyproject.BuildSystem.BuildBackend
	metadata.LanguageSpecific["build_requires"] = pyproject.BuildSystem.Requires

	requiresPythonValue := pyproject.Project.RequiresPython
	debugf("[DEBUG] pyproject.Project.RequiresPython = %q (len=%d, empty=%v)\n",
		requiresPythonValue, len(requiresPythonValue), requiresPythonValue == "")
	if requiresPythonValue == "" {
		debugf("[DEBUG] RequiresPython is EMPTY - matrix generation will be skipped\n")
	}
	metadata.LanguageSpecific["metadata_source"] = "pyproject.toml"
	metadata.LanguageSpecific["keywords"] = pyproject.Project.Keywords
	metadata.LanguageSpecific["classifiers"] = pyproject.Project.Classifiers

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

	if len(pyproject.Project.Dependencies) > 0 {
		metadata.LanguageSpecific["dependencies"] = pyproject.Project.Dependencies
		metadata.LanguageSpecific["dependency_count"] = len(pyproject.Project.Dependencies)
		metadata.LanguageSpecific["dependencies_source"] = "pyproject.toml"
	}
}

// applyPyProjectToolConfig records `[tool.*]` configuration for Poetry,
// PDM, Hatch, and setuptools. It returns the Poetry Python constraint
// (from `[tool.poetry.dependencies].python`), which is used as a matrix
// source when no PEP 621 `requires-python` is declared.
func applyPyProjectToolConfig(metadata *extractor.ProjectMetadata, pyproject PyProjectTOML) string {
	if pyproject.Tool == nil {
		return ""
	}

	poetryPythonConstraint := ""
	if poetry, ok := pyproject.Tool["poetry"].(map[string]interface{}); ok {
		metadata.LanguageSpecific["poetry_config"] = true
		if version, ok := poetry["version"].(string); ok && metadata.Version == "" {
			metadata.Version = version
			metadata.VersionSource = "pyproject.toml (poetry)"
		}
		// Poetry projects without a PEP 621 `[project]` table express
		// the Python constraint in `[tool.poetry.dependencies].python`.
		// Surface it so the matrix generator behaves the same as for
		// standard `requires-python` projects.
		if deps, ok := poetry["dependencies"].(map[string]interface{}); ok {
			if py, ok := deps["python"].(string); ok {
				poetryPythonConstraint = strings.TrimSpace(py)
			}
		}
	}

	if pdm, ok := pyproject.Tool["pdm"].(map[string]interface{}); ok {
		metadata.LanguageSpecific["pdm_config"] = true
		if version, ok := pdm["version"].(map[string]interface{}); ok {
			metadata.LanguageSpecific["pdm_version_source"] = version["source"]
		}
	}

	if hatch, ok := pyproject.Tool["hatch"].(map[string]interface{}); ok {
		metadata.LanguageSpecific["hatch_config"] = true
		if version, ok := hatch["version"].(map[string]interface{}); ok {
			metadata.LanguageSpecific["hatch_version_source"] = version["source"]
		}
	}

	if _, ok := pyproject.Tool["setuptools"].(map[string]interface{}); ok {
		metadata.LanguageSpecific["setuptools_config"] = true
	}

	return poetryPythonConstraint
}

// generatePyProjectMatrix emits the Python version matrix from the
// declared `requires-python`, falling back to the Poetry Python
// constraint when the PEP 621 field is absent.
func generatePyProjectMatrix(metadata *extractor.ProjectMetadata, pyproject PyProjectTOML, poetryPythonConstraint string) error {
	effectiveRequires := pyproject.Project.RequiresPython
	effectiveSource := ""
	if effectiveRequires != "" {
		effectiveSource = "requires-python"
	} else if poetryPythonConstraint != "" {
		effectiveRequires = poetryPythonConstraint
		effectiveSource = "poetry-dependencies"
		metadata.LanguageSpecific["requires_python"] = poetryPythonConstraint
		debugf(
			"[DEBUG] Using tool.poetry.dependencies.python = %q (no [project].requires-python declared)\n",
			poetryPythonConstraint)
	}
	if effectiveRequires == "" {
		debugf("[DEBUG] RequiresPython is empty, skipping matrix generation\n")
		return nil
	}
	debugf("[DEBUG] Generating matrix for requires_python: %q\n", effectiveRequires)
	return resolveAndEmitMatrix(metadata, effectiveRequires, effectiveSource)
}
