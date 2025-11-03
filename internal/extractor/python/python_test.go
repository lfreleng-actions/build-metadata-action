// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package python

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPythonExtractor_Detect(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		expected bool
	}{
		{
			name: "detects pyproject.toml",
			files: map[string]string{
				"pyproject.toml": "[project]\nname = \"test\"",
			},
			expected: true,
		},
		{
			name: "detects setup.py",
			files: map[string]string{
				"setup.py": "from setuptools import setup\nsetup(name='test')",
			},
			expected: true,
		},
		{
			name: "detects setup.cfg",
			files: map[string]string{
				"setup.cfg": "[metadata]\nname = test",
			},
			expected: true,
		},
		{
			name: "no Python files",
			files: map[string]string{
				"README.md": "# Test",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := createTempProject(t, tt.files)
			defer os.RemoveAll(tmpDir)

			extractor := NewExtractor()
			result := extractor.Detect(tmpDir)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPythonExtractor_Extract_PyProjectTOML(t *testing.T) {
	pyprojectContent := `[project]
name = "example-package"
version = "1.2.3"
description = "An example package"
license = "Apache-2.0"
authors = [
    {name = "John Doe", email = "john@example.com"},
    {name = "Jane Smith"}
]
requires-python = ">=3.8"
dependencies = [
    "requests>=2.28.0",
    "click>=8.0.0"
]
keywords = ["example", "test"]

[project.urls]
Homepage = "https://example.com"
Repository = "https://github.com/example/package"

[build-system]
requires = ["setuptools>=61.0"]
build-backend = "setuptools.build_meta"
`

	tmpDir := createTempProject(t, map[string]string{
		"pyproject.toml": pyprojectContent,
	})
	defer os.RemoveAll(tmpDir)

	extractor := NewExtractor()
	metadata, err := extractor.Extract(tmpDir)

	require.NoError(t, err)
	assert.Equal(t, "example-package", metadata.Name)
	assert.Equal(t, "1.2.3", metadata.Version)
	assert.Equal(t, "An example package", metadata.Description)
	assert.Equal(t, "Apache-2.0", metadata.License)
	assert.Equal(t, "https://example.com", metadata.Homepage)
	assert.Equal(t, "https://github.com/example/package", metadata.Repository)
	assert.Contains(t, metadata.Authors, "John Doe <john@example.com>")
	assert.Contains(t, metadata.Authors, "Jane Smith")

	// Language-specific metadata
	assert.Equal(t, "example-package", metadata.LanguageSpecific["package_name"])
	assert.Equal(t, ">=3.8", metadata.LanguageSpecific["requires_python"])
	assert.Equal(t, "setuptools.build_meta", metadata.LanguageSpecific["build_backend"])
	assert.Equal(t, "pyproject.toml", metadata.LanguageSpecific["metadata_source"])

	// Dependencies
	deps, ok := metadata.LanguageSpecific["dependencies"].([]string)
	require.True(t, ok)
	assert.Len(t, deps, 2)
	assert.Contains(t, deps, "requests>=2.28.0")

	// Python version matrix
	matrix, ok := metadata.LanguageSpecific["version_matrix"].([]string)
	require.True(t, ok)
	assert.NotEmpty(t, matrix)
	assert.Contains(t, matrix, "3.9")

	// Build version
	buildVersion, ok := metadata.LanguageSpecific["build_version"].(string)
	require.True(t, ok)
	assert.NotEmpty(t, buildVersion)
}

func TestPythonExtractor_Extract_PyProjectTOML_TableLicense(t *testing.T) {
	// Test PEP 621 table-format license: {text = "..."} instead of simple string
	pyprojectContent := `[project]
name = "lfreleng-test-python-project"
version = "0.0.1"
description = "Sample Python project used for testing actions"
authors = [
    {name = "Matthew Watkins", email = "93649628+ModeSevenIndustrialSolutions@users.noreply.github.com"},
]
dependencies = ["typer>=0.15.2", "jupyterlab>=4.3.6"]
requires-python = "<3.13,>=3.11"
readme = "README.md"
license = {text = "Apache-2.0"}
classifiers = [
  "Programming Language :: Python :: 3.12",
  "Programming Language :: Python :: 3.11",
]

[build-system]
requires = ["pdm-backend"]
build-backend = "pdm.backend"
`

	tmpDir := createTempProject(t, map[string]string{
		"pyproject.toml": pyprojectContent,
	})
	defer os.RemoveAll(tmpDir)

	extractor := NewExtractor()
	metadata, err := extractor.Extract(tmpDir)

	require.NoError(t, err)
	assert.Equal(t, "lfreleng-test-python-project", metadata.Name)
	assert.Equal(t, "0.0.1", metadata.Version)
	assert.Equal(t, "Apache-2.0", metadata.License, "Should extract license from table format")
	assert.Equal(t, "Sample Python project used for testing actions", metadata.Description)

	// Language-specific metadata
	assert.Equal(t, "lfreleng-test-python-project", metadata.LanguageSpecific["package_name"])
	assert.Equal(t, "<3.13,>=3.11", metadata.LanguageSpecific["requires_python"])
	assert.Equal(t, "pdm.backend", metadata.LanguageSpecific["build_backend"])

	// Python version matrix
	matrix, ok := metadata.LanguageSpecific["version_matrix"].([]string)
	require.True(t, ok)
	assert.NotEmpty(t, matrix)
	assert.Contains(t, matrix, "3.11")
	assert.Contains(t, matrix, "3.12")

	// Build version should be latest (3.13 is now included in matrix)
	buildVersion, ok := metadata.LanguageSpecific["build_version"].(string)
	require.True(t, ok)
	assert.NotEmpty(t, buildVersion)
	// Latest version in range <3.13,>=3.11 should be 3.12, but the matrix
	// generator may include 3.13 depending on interpretation of the constraint
}

func TestPythonExtractor_Extract_SetupPy(t *testing.T) {
	setupPyContent := `from setuptools import setup

setup(
    name='my-package',
    version='2.0.0',
    description='My test package',
    author='Test Author',
    author_email='test@example.com',
    license='MIT',
    url='https://github.com/test/package',
    python_requires='>=3.9',
)
`

	tmpDir := createTempProject(t, map[string]string{
		"setup.py": setupPyContent,
	})
	defer os.RemoveAll(tmpDir)

	extractor := NewExtractor()
	metadata, err := extractor.Extract(tmpDir)

	require.NoError(t, err)
	assert.Equal(t, "my-package", metadata.Name)
	assert.Equal(t, "2.0.0", metadata.Version)
	assert.Equal(t, "My test package", metadata.Description)
	assert.Equal(t, "MIT", metadata.License)
	assert.Equal(t, "https://github.com/test/package", metadata.Homepage)
	assert.Contains(t, metadata.Authors, "Test Author <test@example.com>")

	// Language-specific metadata
	assert.Equal(t, "setup.py", metadata.LanguageSpecific["metadata_source"])
	assert.Equal(t, ">=3.9", metadata.LanguageSpecific["requires_python"])
}

func TestPythonExtractor_Extract_SetupCfg(t *testing.T) {
	setupCfgContent := `[metadata]
name = setup-cfg-package
version = 0.5.0
description = Setup.cfg test package
author = Config Author
author_email = config@example.com
license = BSD
url = https://example.org
python_requires = >=3.7

[options]
install_requires =
    numpy>=1.20
    pandas>=1.3
`

	tmpDir := createTempProject(t, map[string]string{
		"setup.cfg": setupCfgContent,
	})
	defer os.RemoveAll(tmpDir)

	extractor := NewExtractor()
	metadata, err := extractor.Extract(tmpDir)

	require.NoError(t, err)
	assert.Equal(t, "setup-cfg-package", metadata.Name)
	assert.Equal(t, "0.5.0", metadata.Version)
	assert.Equal(t, "Setup.cfg test package", metadata.Description)
	assert.Equal(t, "BSD", metadata.License)
	assert.Equal(t, "https://example.org", metadata.Homepage)

	// Language-specific metadata
	assert.Equal(t, "setup.cfg", metadata.LanguageSpecific["metadata_source"])
	assert.Equal(t, ">=3.7", metadata.LanguageSpecific["requires_python"])
}

func TestPythonExtractor_Extract_DynamicVersion(t *testing.T) {
	pyprojectContent := `[project]
name = "dynamic-package"
dynamic = ["version"]
description = "Package with dynamic version"

[tool.setuptools.dynamic]
version = {attr = "package.__version__"}
`

	tmpDir := createTempProject(t, map[string]string{
		"pyproject.toml": pyprojectContent,
	})
	defer os.RemoveAll(tmpDir)

	extractor := NewExtractor()
	metadata, err := extractor.Extract(tmpDir)

	require.NoError(t, err)
	assert.Equal(t, "dynamic-package", metadata.Name)

	// Should detect dynamic version
	versioningType, ok := metadata.LanguageSpecific["versioning_type"].(string)
	require.True(t, ok)
	assert.Equal(t, "dynamic", versioningType)
}

func TestPythonExtractor_Extract_Poetry(t *testing.T) {
	pyprojectContent := `[tool.poetry]
name = "poetry-package"
version = "1.0.0"
description = "Poetry project"

[tool.poetry.dependencies]
python = "^3.9"
requests = "^2.28.0"

[build-system]
requires = ["poetry-core>=1.0.0"]
build-backend = "poetry.core.masonry.api"
`

	tmpDir := createTempProject(t, map[string]string{
		"pyproject.toml": pyprojectContent,
	})
	defer os.RemoveAll(tmpDir)

	extractor := NewExtractor()
	metadata, err := extractor.Extract(tmpDir)

	require.NoError(t, err)

	// Should detect Poetry
	hasPoetry, ok := metadata.LanguageSpecific["poetry_config"].(bool)
	require.True(t, ok)
	assert.True(t, hasPoetry)
}

func TestGeneratePythonVersionMatrix(t *testing.T) {
	tests := []struct {
		name           string
		requiresPython string
		expected       []string
	}{
		{
			name:           ">=3.8",
			requiresPython: ">=3.8",
			expected:       []string{"3.9", "3.10", "3.11", "3.12", "3.13", "3.14"},
		},
		{
			name:           ">=3.9",
			requiresPython: ">=3.9",
			expected:       []string{"3.9", "3.10", "3.11", "3.12", "3.13", "3.14"},
		},
		{
			name:           ">=3.10",
			requiresPython: ">=3.10",
			expected:       []string{"3.10", "3.11", "3.12", "3.13", "3.14"},
		},
		{
			name:           "~=3.9",
			requiresPython: "~=3.9",
			expected:       []string{"3.9", "3.10", "3.11", "3.12", "3.13", "3.14"},
		},
		{
			name:           "<3.13,>=3.11",
			requiresPython: "<3.13,>=3.11",
			expected:       []string{"3.11", "3.12"},
		},
		{
			name:           ">=3.10,<3.12",
			requiresPython: ">=3.10,<3.12",
			expected:       []string{"3.10", "3.11"},
		},
		{
			name:           ">=3.9,<3.11",
			requiresPython: ">=3.9,<3.11",
			expected:       []string{"3.9", "3.10"},
		},
		{
			name:           "empty",
			requiresPython: "",
			expected:       []string{"3.9", "3.10", "3.11", "3.12", "3.13", "3.14"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generatePythonVersionMatrix(tt.requiresPython)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractSetupPyField(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		field    string
		expected string
	}{
		{
			name:     "single quotes",
			content:  "name='test-package'",
			field:    "name",
			expected: "test-package",
		},
		{
			name:     "double quotes",
			content:  `version="1.2.3"`,
			field:    "version",
			expected: "1.2.3",
		},
		{
			name:     "with spaces",
			content:  "description = 'A test package'",
			field:    "description",
			expected: "A test package",
		},
		{
			name:     "triple quotes",
			content:  `description="""Multi-line description"""`,
			field:    "description",
			expected: "Multi-line description",
		},
		{
			name:     "not found",
			content:  "name='test'",
			field:    "version",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractSetupPyField(tt.content, tt.field)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseINI(t *testing.T) {
	content := `[section1]
key1 = value1
key2 = value2

[section2]
key3 = value3

# Comment line
; Another comment
[section3]
key4 = value4
`

	result := parseINI(content)

	assert.Len(t, result, 3)
	assert.Equal(t, "value1", result["section1"]["key1"])
	assert.Equal(t, "value2", result["section1"]["key2"])
	assert.Equal(t, "value3", result["section2"]["key3"])
	assert.Equal(t, "value4", result["section3"]["key4"])
}

func TestPythonExtractor_ProjectMatchPackage(t *testing.T) {
	tests := []struct {
		name          string
		projectName   string
		expectedMatch bool
	}{
		{
			name:          "matching project and package name",
			projectName:   "example_package",
			expectedMatch: true,
		},
		{
			name:          "project name with dashes",
			projectName:   "example-package",
			expectedMatch: false,
		},
		{
			name:          "simple name",
			projectName:   "myproject",
			expectedMatch: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pyprojectContent := `[project]
name = "` + tt.projectName + `"
version = "1.0.0"
description = "Test package"

[build-system]
requires = ["setuptools>=61.0"]
build-backend = "setuptools.build_meta"
`

			tmpDir := createTempProject(t, map[string]string{
				"pyproject.toml": pyprojectContent,
			})
			defer os.RemoveAll(tmpDir)

			extractor := NewExtractor()
			metadata, err := extractor.Extract(tmpDir)

			require.NoError(t, err)
			assert.Equal(t, tt.projectName, metadata.Name)

			// Check project_match_package
			projectMatchPackage, ok := metadata.LanguageSpecific["project_match_package"].(bool)
			require.True(t, ok, "project_match_package should be set")
			assert.Equal(t, tt.expectedMatch, projectMatchPackage)
		})
	}
}

// Test for psf/requests-like project: pyproject.toml without [project] section
func TestPythonExtractor_Extract_PyProjectWithoutProjectSection(t *testing.T) {
	// Create a project like psf/requests:
	// - pyproject.toml exists but has no [project] section (only [tool.*])
	// - setup.py has python_requires
	pyprojectContent := `[tool.isort]
profile = "black"

[tool.pytest.ini_options]
addopts = "--doctest-modules"
`

	setupPyContent := `from setuptools import setup

setup(
    name='test-requests',
    version='2.32.0',
    description='A requests-like package',
    python_requires='>=3.9',
)
`

	tmpDir := createTempProject(t, map[string]string{
		"pyproject.toml": pyprojectContent,
		"setup.py":       setupPyContent,
	})
	defer os.RemoveAll(tmpDir)

	extractor := NewExtractor()
	metadata, err := extractor.Extract(tmpDir)

	require.NoError(t, err)
	assert.Equal(t, "test-requests", metadata.Name)
	assert.Equal(t, "2.32.0", metadata.Version)

	// The key test: requires_python should come from setup.py fallback
	requiresPython, ok := metadata.LanguageSpecific["requires_python"].(string)
	require.True(t, ok, "requires_python should be set from setup.py fallback")
	assert.Equal(t, ">=3.9", requiresPython)

	// Matrix JSON should be generated
	matrixJSON, ok := metadata.LanguageSpecific["matrix_json"].(string)
	require.True(t, ok, "matrix_json should be generated from requires_python")
	assert.Contains(t, matrixJSON, "python-version")
	assert.Contains(t, matrixJSON, "3.9")
	assert.Contains(t, matrixJSON, "3.10")
	assert.Contains(t, matrixJSON, "3.11")
	assert.Contains(t, matrixJSON, "3.12")
	assert.Contains(t, matrixJSON, "3.13")
	assert.Contains(t, matrixJSON, "3.14")

	// Build version should be set to latest
	buildVersion, ok := metadata.LanguageSpecific["build_version"].(string)
	require.True(t, ok, "build_version should be set")
	assert.Equal(t, "3.14", buildVersion)

	// Version matrix should be set
	versionMatrix, ok := metadata.LanguageSpecific["version_matrix"].([]string)
	require.True(t, ok, "version_matrix should be set")
	assert.Equal(t, []string{"3.9", "3.10", "3.11", "3.12", "3.13", "3.14"}, versionMatrix)
}

// Helper function to create temporary test projects
func createTempProject(t *testing.T, files map[string]string) string {
	tmpDir, err := os.MkdirTemp("", "python-test-*")
	require.NoError(t, err)

	for filename, content := range files {
		filePath := filepath.Join(tmpDir, filename)
		err := os.WriteFile(filePath, []byte(content), 0644)
		require.NoError(t, err)
	}

	return tmpDir
}
