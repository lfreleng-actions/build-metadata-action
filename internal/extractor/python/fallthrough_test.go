// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package python

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPyProjectFallThrough checks which file supplies the metadata of a
// project whose pyproject.toml names no project: setup.cfg or setup.py
// when one sits beside it, whatever tool tables pyproject.toml holds,
// and pyproject.toml itself only when neither does.
func TestPyProjectFallThrough(t *testing.T) {
	const buildSystem = "[build-system]\nrequires = [\"setuptools\"]\nbuild-backend = \"setuptools.build_meta\"\n"
	pbrFiles := func(extra string) map[string]string {
		return map[string]string{
			"pyproject.toml": buildSystem + extra,
			"setup.cfg":      "[metadata]\nname = cfg-pbr\npython_requires = >=3.9\n",
			"setup.py":       "import setuptools\n\nsetuptools.setup(setup_requires=['pbr'], pbr=True)\n",
		}
	}
	staticCfgFiles := func(extra string) map[string]string {
		return map[string]string{
			"pyproject.toml": buildSystem + extra,
			"setup.cfg":      "[metadata]\nname = cfg-static\nversion = 1.2.3\n\n[options]\npython_requires = >=3.9\n",
		}
	}
	tests := []struct {
		name          string
		files         map[string]string
		project       string
		source        string
		version       string
		versionSource string
		versioning    string
		provider      any
	}{
		{
			name:          "F1 build-system alone beside PBR files",
			files:         pbrFiles(""),
			project:       "cfg-pbr",
			source:        "setup.cfg",
			versionSource: "setup.cfg",
			versioning:    "dynamic",
			provider:      "pbr",
		},
		{
			name:          "F2 tool.setuptools beside PBR files",
			files:         pbrFiles("\n[tool.setuptools]\nzip-safe = false\n"),
			project:       "cfg-pbr",
			source:        "setup.cfg",
			versionSource: "setup.cfg",
			versioning:    "dynamic",
			provider:      "pbr",
		},
		{
			name:          "F3 tool.ruff beside a static setup.cfg",
			files:         staticCfgFiles("\n[tool.ruff]\nline-length = 88\n"),
			project:       "cfg-static",
			source:        "setup.cfg",
			version:       "1.2.3",
			versionSource: "setup.cfg",
			versioning:    "static",
		},
		{
			name:          "F4 tool.hatch beside a static setup.cfg",
			files:         staticCfgFiles("\n[tool.hatch.envs.default]\ndependencies = [\"pytest\"]\n"),
			project:       "cfg-static",
			source:        "setup.cfg",
			version:       "1.2.3",
			versionSource: "setup.cfg",
			versioning:    "static",
		},
		{
			name:          "F5 tool.poetry without a name beside a static setup.cfg",
			files:         staticCfgFiles("\n[tool.poetry.group.dev.dependencies]\npytest = \"*\"\n"),
			project:       "cfg-static",
			source:        "setup.cfg",
			version:       "1.2.3",
			versionSource: "setup.cfg",
			versioning:    "static",
		},
		{
			name: "F6 tool.pdm beside a static setup.py",
			files: map[string]string{
				"pyproject.toml": buildSystem + "\n[tool.pdm.scripts]\ntest = \"pytest\"\n",
				"setup.py":       "from setuptools import setup\n\nsetup(name=\"py-static\", version=\"0.4.0\", python_requires=\">=3.9\")\n",
			},
			project:       "py-static",
			source:        "setup.py",
			version:       "0.4.0",
			versionSource: "setup.py",
			versioning:    "static",
		},
		{
			name:          "tool.setuptools and no legacy file",
			files:         map[string]string{"pyproject.toml": buildSystem + "\n[tool.setuptools]\nzip-safe = false\n"},
			source:        "pyproject.toml",
			versionSource: "pyproject.toml",
			versioning:    "static",
		},
		{
			name: "a named pyproject.toml wins over setup.cfg",
			files: map[string]string{
				"pyproject.toml": buildSystem + "\n[project]\nname = \"named\"\nversion = \"3.0.0\"\nrequires-python = \">=3.9\"\n",
				"setup.cfg":      "[metadata]\nname = other\nversion = 1.2.3\n",
			},
			project:       "named",
			source:        "pyproject.toml",
			version:       "3.0.0",
			versionSource: "pyproject.toml",
			versioning:    "static",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metadata, err := NewExtractor().Extract(createTempProject(t, tt.files))
			require.NoError(t, err)
			assert.Equal(t, tt.project, metadata.Name)
			assert.Equal(t, tt.source, metadata.LanguageSpecific["metadata_source"])
			assert.Equal(t, tt.version, metadata.Version)
			assert.Equal(t, tt.versionSource, metadata.VersionSource)
			assert.Equal(t, tt.versioning, metadata.LanguageSpecific["versioning_type"])
			assert.Equal(t, tt.provider, metadata.LanguageSpecific["dynamic_provider"])
			assert.Equal(t, "setuptools.build_meta", metadata.LanguageSpecific["build_backend"])
			if tt.source != "pyproject.toml" {
				assert.Equal(t, ">=3.9", metadata.LanguageSpecific["requires_python"])
			}
			assert.NotEmpty(t, metadata.LanguageSpecific["version_matrix"])
		})
	}
}

// TestPyProjectFallThroughDropsProjectVersioning checks that a nameless
// [project] table that declares a dynamic version and dependencies
// leaves neither behind when setup.cfg supplies the metadata.
func TestPyProjectFallThroughDropsProjectVersioning(t *testing.T) {
	const pyproject = "[build-system]\nrequires = [\"setuptools\", \"setuptools_scm\"]\n\n" +
		"[project]\ndynamic = [\"version\"]\ndependencies = [\"requests\"]\n\n" +
		"[tool.setuptools]\nzip-safe = false\n"
	tests := []struct {
		name       string
		files      map[string]string
		versioning string
		provider   any
		deps       any
	}{
		{
			name: "static setup.cfg and requirements.txt",
			files: map[string]string{
				"pyproject.toml":   pyproject,
				"setup.cfg":        "[metadata]\nname = cfg-static\nversion = 1.2.3\n",
				"requirements.txt": "click\n",
			},
			versioning: "static",
			deps:       []string{"click"},
		},
		{
			name: "setup.cfg beside a PBR shim",
			files: map[string]string{
				"pyproject.toml": pyproject,
				"setup.cfg":      "[metadata]\nname = cfg-pbr\n",
				"setup.py":       "import setuptools\n\nsetuptools.setup(setup_requires=['pbr'], pbr=True)\n",
			},
			versioning: "dynamic",
			provider:   "pbr",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metadata, err := NewExtractor().Extract(createTempProject(t, tt.files))
			require.NoError(t, err)
			assert.Equal(t, "setup.cfg", metadata.LanguageSpecific["metadata_source"])
			assert.Equal(t, tt.versioning, metadata.LanguageSpecific["versioning_type"])
			assert.Equal(t, tt.provider, metadata.LanguageSpecific["dynamic_provider"])
			assert.Equal(t, tt.deps, metadata.LanguageSpecific["dependencies"])
		})
	}
}

// TestPyProjectFallThroughDropsProjectPythonData checks that a nameless
// [project] table leaves no requires-python, matrix, classifiers or
// keywords behind when setup.cfg, which declares none, supplies the
// metadata; only the [build-system] fields stay.
func TestPyProjectFallThroughDropsProjectPythonData(t *testing.T) {
	files := map[string]string{
		"pyproject.toml": "[build-system]\nrequires = [\"setuptools\"]\nbuild-backend = \"setuptools.build_meta\"\n\n" +
			"[project]\nrequires-python = \"==3.12.*\"\nkeywords = [\"stale\"]\n" +
			"classifiers = [\"Programming Language :: Python :: 3.12\"]\n",
		"setup.cfg": "[metadata]\nname = cfg-static\nversion = 1.2.3\n",
	}
	metadata, err := NewExtractor().Extract(createTempProject(t, files))
	require.NoError(t, err)
	assert.Equal(t, "cfg-static", metadata.Name)
	assert.Equal(t, "setup.cfg", metadata.LanguageSpecific["metadata_source"])
	assert.Empty(t, metadata.LanguageSpecific["requires_python"])
	assert.Nil(t, metadata.LanguageSpecific["keywords"])
	assert.Nil(t, metadata.LanguageSpecific["classifiers"])
	assert.Equal(t, "static-fallback", metadata.LanguageSpecific["requires_python_source"])
	assert.NotEqual(t, []string{"3.12"}, metadata.LanguageSpecific["version_matrix"])
	assert.Equal(t, "setuptools.build_meta", metadata.LanguageSpecific["build_backend"])
	assert.Equal(t, []string{"setuptools"}, metadata.LanguageSpecific["build_requires"])
}
