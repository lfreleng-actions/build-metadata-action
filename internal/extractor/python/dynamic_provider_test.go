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

// TestSetupPyProvider checks that the provider of a setup.py version
// comes from the keyword arguments of its setup() call alone, and never
// from a comment, a string or code outside the call.
func TestSetupPyProvider(t *testing.T) {
	const header = "from setuptools import setup\n\n"
	tests := []struct {
		name       string
		files      map[string]string
		versioning string
		provider   any
	}{
		{
			name:       "P1 use_scm_version=False",
			files:      map[string]string{"setup.py": header + "setup(name=\"scm-off\", version=\"1.0.0\", use_scm_version=False)\n"},
			versioning: "static",
		},
		{
			name: "P2 use_scm_version in a comment",
			files: map[string]string{"setup.py": header + "# setup(use_scm_version=True) was dropped in 2.0\n" +
				"setup(name=\"in-comment\", version=\"1.0.0\")\n"},
			versioning: "static",
		},
		{
			name: "P3 pbr=True in a docstring",
			files: map[string]string{"setup.py": "\"\"\"Build script. Once used setup(pbr=True).\"\"\"\n" + header +
				"setup(name=\"in-docstring\", version=\"1.0.0\")\n"},
			versioning: "static",
		},
		{
			name: "P4 __version__ in a comment",
			files: map[string]string{"setup.py": header + "# the package keeps __version__ in sync by hand\n" +
				"setup(name=\"mention\", version=\"1.0.0\")\n"},
			versioning: "static",
		},
		{
			name:       "P5 PBR shim",
			files:      map[string]string{"setup.py": "import setuptools\n\nsetuptools.setup(setup_requires=['pbr'], pbr=True)\n"},
			versioning: "dynamic",
			provider:   "pbr",
		},
		{
			name: "P6 use_scm_version=True",
			files: map[string]string{"setup.py": header +
				"setup(name=\"py-scm\", use_scm_version=True, setup_requires=[\"setuptools_scm\"])\n"},
			versioning: "dynamic",
			provider:   "setuptools-scm",
		},
		{
			name: "P7 versioneer",
			files: map[string]string{"setup.py": "import versioneer\n" + header +
				"setup(name=\"py-versioneer\", version=versioneer.get_version(), cmdclass=versioneer.get_cmdclass())\n"},
			versioning: "dynamic",
			provider:   "versioneer",
		},
		{
			name: "P8 __version__ imported",
			files: map[string]string{"setup.py": header + "from py_attr import __version__\n\n" +
				"setup(name=\"py-attr\", version=__version__)\n"},
			versioning: "dynamic",
			provider:   "runtime-attr",
		},
		{
			name:       "use_scm_version=None",
			files:      map[string]string{"setup.py": header + "setup(name=\"scm-none\", version=\"1.0.0\", use_scm_version=None)\n"},
			versioning: "static",
		},
		{
			name: "setup_requires passed as a variable",
			files: map[string]string{"setup.py": header + "setup_requires = [\"pbr\"]\n\n" +
				"setup(name=\"via-var\", version=\"1.0.0\", setup_requires=setup_requires)\n"},
			versioning: "static",
		},
		{
			name:       "pbr with a specifier in setup_requires",
			files:      map[string]string{"setup.py": header + "setup(name=\"pbr-spec\", setup_requires=[\"pbr>=2.0\"])\n"},
			versioning: "dynamic",
			provider:   "pbr",
		},
		{
			name: "signals inside a string argument",
			files: map[string]string{"setup.py": header +
				"setup(name=\"in-string\", version=\"1.0.0\", description=\"Once used pbr=True and __version__\")\n"},
			versioning: "static",
		},
		{
			name: "computed version starting with a name, binding __version__ in a lambda",
			files: map[string]string{"setup.py": header +
				"setup(name=\"in-lambda\", version=str((lambda __version__: \"1.0.0\")(None)))\n"},
			versioning: "dynamic",
			provider:   "runtime-attr",
		},
		{
			name: "computed version starting with a name, binding __version__ in a generator",
			files: map[string]string{"setup.py": header +
				"setup(name=\"in-genexp\", version=next(\"1.0.0\" for __version__ in [None]))\n"},
			versioning: "dynamic",
			provider:   "runtime-attr",
		},
		{
			name: "setup.cfg beside a setup.py whose pbr=True is commented out",
			files: map[string]string{
				"setup.cfg": "[metadata]\nname = cfg-static\nversion = 1.2.3\n",
				"setup.py":  header + "# setup(pbr=True)\nsetup()\n",
			},
			versioning: "static",
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
