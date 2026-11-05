// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package python

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests exercise the constraint forms that previously only
// existed as integration cases in `python-supported-versions-action`
// (the now-superseded shell action). They drive the full extractor
// pipeline end-to-end, so coverage is not just at the constraint
// solver level (already in `internal/pyversions/constraints_test.go`)
// but also through the pyproject.toml / setup.cfg / setup.py readers,
// the source-taxonomy decoration, and matrix emission.
//
// Each table row mirrors a fixture from
// `tests/scripts/unit/test_constraint_utils.sh` in the shell action's
// repo so the Go suite is functionally a strict superset.

// withOfflinePolicy installs an offline policy with the action's static
// supported set so each test runs deterministically without hitting the
// live endoflife.date API.
func withOfflinePolicy(t *testing.T) {
	t.Helper()
	prev := ActivePolicy()
	SetActivePolicy(&Policy{
		Offline:      true,
		SupportedSet: append([]string(nil), supportedPythonVersions...),
		EOLVersions:  map[string]bool{},
	})
	t.Cleanup(func() { SetActivePolicy(prev) })
}

// TestConstraintForms_PyProject covers every PEP 440 / Poetry
// constraint form via the pyproject.toml `requires-python` path.
func TestConstraintForms_PyProject(t *testing.T) {
	tests := []struct {
		name     string
		requires string
		matrix   []string
	}{
		{
			name:     "floor below baseline (>=3.10)",
			requires: ">=3.10",
			matrix:   []string{"3.11", "3.12", "3.13", "3.14", "3.15"},
		},
		{
			name:     "exclusion constraint (>=3.11,!=3.12)",
			requires: ">=3.11,!=3.12",
			matrix:   []string{"3.11", "3.13", "3.14", "3.15"},
		},
		{
			name:     "exact pin (==3.12)",
			requires: "==3.12",
			matrix:   []string{"3.12"},
		},
		{
			name:     "wildcard exact (==3.11.*)",
			requires: "==3.11.*",
			matrix:   []string{"3.11"},
		},
		{
			name:     "compatible release without patch (~=3.11)",
			requires: "~=3.11",
			matrix:   []string{"3.11", "3.12", "3.13", "3.14", "3.15"},
		},
		{
			name:     "compatible release with patch (~=3.11.5)",
			requires: "~=3.11.5",
			matrix:   []string{"3.11"},
		},
		{
			name:     "greater than (>3.11)",
			requires: ">3.11",
			matrix:   []string{"3.12", "3.13", "3.14", "3.15"},
		},
		{
			name:     "multi-bound with exclusion (>=3.11,<3.15,!=3.13)",
			requires: ">=3.11,<3.15,!=3.13",
			matrix:   []string{"3.11", "3.12", "3.14"},
		},
		{
			name:     "inclusive upper bound (>=3.11,<=3.13)",
			requires: ">=3.11,<=3.13",
			matrix:   []string{"3.11", "3.12", "3.13"},
		},
		{
			name:     "tightest bound wins (>=3.11,<=3.14,<3.13)",
			requires: ">=3.11,<=3.14,<3.13",
			matrix:   []string{"3.11", "3.12"},
		},
		{
			name:     "tie-break exclusive over inclusive (<=3.13,<3.13)",
			requires: ">=3.11,<=3.13,<3.13",
			matrix:   []string{"3.11", "3.12"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withOfflinePolicy(t)
			pyproject := "[project]\n" +
				"name = \"constraint-test\"\n" +
				"version = \"1.0\"\n" +
				"requires-python = \"" + tt.requires + "\"\n"

			tmpDir := createTempProject(t, map[string]string{
				"pyproject.toml": pyproject,
			})
			t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

			ex := NewExtractor()
			metadata, err := ex.Extract(tmpDir)
			require.NoError(t, err)

			got, _ := metadata.LanguageSpecific["version_matrix"].([]string)
			assert.Equal(t, tt.matrix, got)
			assert.Equal(t, "requires-python",
				metadata.LanguageSpecific["requires_python_source"])
		})
	}
}

// TestConstraintForms_PoetryCaret covers the Poetry-specific caret
// constraint form via [tool.poetry.dependencies].python. The bare
// caret expands the upper bound to the next major (4.0), so the matrix
// includes every supported version at or above the floor.
func TestConstraintForms_PoetryCaret(t *testing.T) {
	tests := []struct {
		name   string
		python string
		matrix []string
	}{
		{
			name:   "caret without patch (^3.11)",
			python: "^3.11",
			matrix: []string{"3.11", "3.12", "3.13", "3.14", "3.15"},
		},
		{
			name:   "caret with patch (^3.11.5)",
			python: "^3.11.5",
			matrix: []string{"3.11", "3.12", "3.13", "3.14", "3.15"},
		},
		{
			name:   "caret below floor (^3.10)",
			python: "^3.10",
			matrix: []string{"3.11", "3.12", "3.13", "3.14", "3.15"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withOfflinePolicy(t)
			pyproject := "[tool.poetry]\n" +
				"name = \"poetry-test\"\n" +
				"version = \"0.1.0\"\n" +
				"\n" +
				"[tool.poetry.dependencies]\n" +
				"python = \"" + tt.python + "\"\n"

			tmpDir := createTempProject(t, map[string]string{
				"pyproject.toml": pyproject,
			})
			t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

			ex := NewExtractor()
			metadata, err := ex.Extract(tmpDir)
			require.NoError(t, err)

			got, _ := metadata.LanguageSpecific["version_matrix"].([]string)
			assert.Equal(t, tt.matrix, got)
			assert.Equal(t, "poetry-dependencies",
				metadata.LanguageSpecific["requires_python_source"])
		})
	}
}

// TestConstraintForms_SetupCfg covers the setup.cfg python_requires
// path with the same constraint forms the shell action tested.
func TestConstraintForms_SetupCfg(t *testing.T) {
	tests := []struct {
		name     string
		requires string
		matrix   []string
	}{
		{
			name:     "exclusion (!=3.12)",
			requires: ">=3.11,!=3.12",
			matrix:   []string{"3.11", "3.13", "3.14", "3.15"},
		},
		{
			name:     "exact (==3.13)",
			requires: "==3.13",
			matrix:   []string{"3.13"},
		},
		{
			name:     "compatible release with patch (~=3.12.7)",
			requires: "~=3.12.7",
			matrix:   []string{"3.12"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withOfflinePolicy(t)
			cfg := "[metadata]\n" +
				"name = cfg-constraint-test\n" +
				"version = 1.0\n" +
				"python_requires = " + tt.requires + "\n"

			tmpDir := createTempProject(t, map[string]string{
				"setup.cfg": cfg,
			})
			t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

			ex := NewExtractor()
			metadata, err := ex.Extract(tmpDir)
			require.NoError(t, err)

			got, _ := metadata.LanguageSpecific["version_matrix"].([]string)
			assert.Equal(t, tt.matrix, got)
			assert.Equal(t, "requires-python",
				metadata.LanguageSpecific["requires_python_source"])
		})
	}
}

// TestConstraintForms_SetupPy covers the setup.py python_requires
// path. The regex extractor on the setup.py side surfaces single- and
// double-quoted constraint strings.
func TestConstraintForms_SetupPy(t *testing.T) {
	tests := []struct {
		name   string
		setup  string
		matrix []string
	}{
		{
			name: "tilde with single quotes",
			setup: "from setuptools import setup\n" +
				"setup(name='tilde-pkg', version='1.0', python_requires='~=3.11')\n",
			matrix: []string{"3.11", "3.12", "3.13", "3.14", "3.15"},
		},
		{
			name: "exact with double quotes",
			setup: "from setuptools import setup\n" +
				"setup(name=\"exact-pkg\", version=\"1.0\", python_requires=\"==3.12\")\n",
			matrix: []string{"3.12"},
		},
		{
			name: "exclusion in multi-clause",
			setup: "from setuptools import setup\n" +
				"setup(name='excl-pkg', version='1.0', python_requires='>=3.11,!=3.12')\n",
			matrix: []string{"3.11", "3.13", "3.14", "3.15"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withOfflinePolicy(t)
			tmpDir := createTempProject(t, map[string]string{
				"setup.py": tt.setup,
			})
			t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

			ex := NewExtractor()
			metadata, err := ex.Extract(tmpDir)
			require.NoError(t, err)

			got, _ := metadata.LanguageSpecific["version_matrix"].([]string)
			assert.Equal(t, tt.matrix, got)
			assert.Equal(t, "requires-python",
				metadata.LanguageSpecific["requires_python_source"])
		})
	}
}

// TestConstraintForms_OutOfRangeResolution covers the shell action's
// "no matching versions" cases: a constraint that resolves to an empty
// matrix after intersecting with supported versions falls back to the
// static supported set (so consumers always get a runnable matrix) AND
// surfaces the situation via `requires_python_source =
// "out-of-range-fallback"` plus a ::warning::, so downstream callers
// can detect that the matrix is wider than the project actually
// requested.
func TestConstraintForms_OutOfRangeResolution(t *testing.T) {
	withOfflinePolicy(t)
	pyproject := "[project]\n" +
		"name = \"out-of-range\"\n" +
		"version = \"1.0\"\n" +
		"requires-python = \">=4.0\"\n"

	tmpDir := createTempProject(t, map[string]string{
		"pyproject.toml": pyproject,
	})
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	ex := NewExtractor()
	metadata, err := ex.Extract(tmpDir)
	require.NoError(t, err)

	got, _ := metadata.LanguageSpecific["version_matrix"].([]string)
	// `>=4.0` matches no supported Python; the matrix widens to the
	// policy's supported set rather than emitting an unrunnable empty
	// matrix.
	assert.Equal(t, supportedPythonVersions, got)
	assert.Equal(t, "out-of-range-fallback",
		metadata.LanguageSpecific["requires_python_source"],
		"source must record that the matrix was widened beyond the project's constraint")
}

// TestConstraintForms_EOLMinorOnlyFallsBack covers a project pinned to
// 3.10, which is below the supported baseline: no supported version
// satisfies it, so the matrix widens and the source records the fallback.
func TestConstraintForms_EOLMinorOnlyFallsBack(t *testing.T) {
	withOfflinePolicy(t)
	pyproject := "[project]\n" +
		"name = \"eol-only\"\n" +
		"version = \"1.0\"\n" +
		"requires-python = \"==3.10.*\"\n"

	tmpDir := createTempProject(t, map[string]string{
		"pyproject.toml": pyproject,
	})
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	ex := NewExtractor()
	metadata, err := ex.Extract(tmpDir)
	require.NoError(t, err)

	got, _ := metadata.LanguageSpecific["version_matrix"].([]string)
	assert.Equal(t, supportedPythonVersions, got)
	assert.NotContains(t, got, "3.10")
	assert.Equal(t, "out-of-range-fallback",
		metadata.LanguageSpecific["requires_python_source"])
}

// TestConstraintForms_PoetryCaretBeatsRequiresPython covers a real-world
// edge case: a Poetry pyproject.toml that DOES carry both a [project]
// table (with requires-python) AND a [tool.poetry.dependencies].python.
// PEP 621 wins -- requires-python must take precedence -- and the
// source must be reported as "requires-python", not "poetry-dependencies".
func TestConstraintForms_PoetryCaretBeatsRequiresPython(t *testing.T) {
	withOfflinePolicy(t)
	pyproject := "[project]\n" +
		"name = \"hybrid\"\n" +
		"version = \"1.0\"\n" +
		"requires-python = \">=3.12\"\n" +
		"\n" +
		"[tool.poetry.dependencies]\n" +
		"python = \"^3.10\"\n"

	tmpDir := createTempProject(t, map[string]string{
		"pyproject.toml": pyproject,
	})
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	ex := NewExtractor()
	metadata, err := ex.Extract(tmpDir)
	require.NoError(t, err)

	got, _ := metadata.LanguageSpecific["version_matrix"].([]string)
	assert.Equal(t, []string{"3.12", "3.13", "3.14", "3.15"}, got,
		"PEP 621 requires-python must beat Poetry's caret when both are present")
	assert.Equal(t, "requires-python",
		metadata.LanguageSpecific["requires_python_source"])
}
