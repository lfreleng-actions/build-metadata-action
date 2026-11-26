// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package python

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDynamicProvider checks the provider reported for a dynamic
// version: pyproject.toml names it from [build-system].requires, a
// static pyproject.toml version has none, and setup.cfg and setup.py
// report what their own readers find.
func TestDynamicProvider(t *testing.T) {
	dynamicPyproject := func(requires string) string {
		return "[build-system]\nrequires = [" + requires + "]\n\n" +
			"[project]\nname = \"dyn\"\ndynamic = [\"version\"]\n"
	}
	tests := []struct {
		name       string
		files      map[string]string
		versioning string
		provider   any
	}{
		{
			name:       "setuptools-scm with extras and a specifier",
			files:      map[string]string{"pyproject.toml": dynamicPyproject(`"setuptools>=64", "setuptools_scm[toml]>=8"`)},
			versioning: "dynamic",
			provider:   "setuptools-scm",
		},
		{
			name:       "hatch-vcs",
			files:      map[string]string{"pyproject.toml": dynamicPyproject(`"hatchling", "Hatch_VCS"`)},
			versioning: "dynamic",
			provider:   "hatch-vcs",
		},
		{
			name:       "pbr",
			files:      map[string]string{"pyproject.toml": dynamicPyproject(`"pbr>=6.1", "setuptools"`)},
			versioning: "dynamic",
			provider:   "pbr",
		},
		{
			name:       "no known provider",
			files:      map[string]string{"pyproject.toml": dynamicPyproject(`"hatchling", "sphinx-pbr-theme"`)},
			versioning: "dynamic",
			provider:   "pyproject-dynamic",
		},
		{
			name:       "the earliest provider in the table wins",
			files:      map[string]string{"pyproject.toml": dynamicPyproject(`"pbr", "hatch-vcs", "setuptools-scm"`)},
			versioning: "dynamic",
			provider:   "setuptools-scm",
		},
		{
			name: "static version with setuptools_scm in requires",
			files: map[string]string{"pyproject.toml": "[build-system]\nrequires = [\"setuptools_scm\"]\n\n" +
				"[project]\nname = \"static\"\nversion = \"1.0.0\"\n"},
			versioning: "static",
			provider:   nil,
		},
		{
			name:       "setup.cfg pbr section",
			files:      map[string]string{"setup.cfg": "[metadata]\nname = cfg-pbr\n\n[pbr]\nskip_authors = true\n"},
			versioning: "dynamic",
			provider:   "pbr",
		},
		{
			name:       "setup.py use_scm_version",
			files:      map[string]string{"setup.py": "from setuptools import setup\n\nsetup(name=\"py-scm\", use_scm_version=True)\n"},
			versioning: "dynamic",
			provider:   "setuptools-scm",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metadata, err := NewExtractor().Extract(createTempProject(t, tt.files))
			require.NoError(t, err)
			assert.Equal(t, tt.versioning, metadata.LanguageSpecific["versioning_type"])
			assert.Equal(t, tt.provider, metadata.LanguageSpecific["dynamic_provider"])
		})
	}
}
