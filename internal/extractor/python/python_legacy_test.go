// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package python

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// -----------------------------------------------------------------------
// Legacy project fixtures
//
// These tests cover the long tail of pre-PEP 621 / PBR / classifier-driven
// Python projects that the original implementation could not handle. Each
// fixture is modeled on an actual upstream pattern observed in OpenStack,
// LFIT, and older PyPI-published projects.
// -----------------------------------------------------------------------

// TestLegacy_PbrSetupCfg covers an OpenStack/LFIT-style PBR project that
// stores all metadata in setup.cfg, omits python_requires entirely, lists
// classifiers as a multi-line value, and uses hyphenated keys.
func TestLegacy_PbrSetupCfg(t *testing.T) {
	setupCfg := "[metadata]\n" +
		"name = lfdocs_conf\n" +
		"author = Linux Foundation Releng\n" +
		"author-email = releng@linuxfoundation.org\n" +
		"description-file = README.rst\n" +
		"license = EPL-1.0\n" +
		"classifier =\n" +
		"    Intended Audience :: Developers\n" +
		"    License :: OSI Approved :: Eclipse Public License 1.0 (EPL-1.0)\n" +
		"    Operating System :: OS Independent\n" +
		"    Programming Language :: Python\n" +
		"\n" +
		"[files]\n" +
		"packages = docs_conf\n"

	setupPy := "from setuptools import setup\n" +
		"setup(setup_requires=['pbr'], pbr=True)\n"

	requirements := "sphinx>=4.0\npbr>=5.0\n"

	tmpDir := createTempProject(t, map[string]string{
		"setup.cfg":        setupCfg,
		"setup.py":         setupPy,
		"requirements.txt": requirements,
	})
	defer os.RemoveAll(tmpDir)

	extractor := NewExtractor()
	metadata, err := extractor.Extract(tmpDir)
	require.NoError(t, err)

	assert.Equal(t, "lfdocs_conf", metadata.Name)
	assert.Equal(t, "EPL-1.0", metadata.License)
	assert.Equal(t, []string{"Linux Foundation Releng <releng@linuxfoundation.org>"}, metadata.Authors,
		"hyphenated author-email key must be normalised")

	// Versioning: PBR is the dynamic provider; setup.cfg version field is
	// absent, so build/version-patching consumers should treat the
	// version as unresolved until a tag is available.
	assert.Equal(t, "dynamic", metadata.LanguageSpecific["versioning_type"])
	assert.Equal(t, "pbr", metadata.LanguageSpecific["dynamic_provider"])
	assert.Equal(t, true, metadata.LanguageSpecific["version_unresolved"])

	// No python_requires, no version classifiers => fallback matrix.
	assert.Equal(t, true, metadata.LanguageSpecific["requires_python_fallback"])
	buildVersion, _ := metadata.LanguageSpecific["build_version"].(string)
	assert.NotEmpty(t, buildVersion)

	// requirements.txt should have been picked up as the dependency list
	// because install_requires is absent from setup.cfg.
	deps, ok := metadata.LanguageSpecific["dependencies"].([]string)
	require.True(t, ok)
	assert.Contains(t, deps, "sphinx>=4.0")
	assert.Equal(t, "requirements.txt", metadata.LanguageSpecific["dependencies_source"])

	// Classifiers should have been parsed despite the hyphenated singular
	// "classifier =" key form.
	classifiers, _ := metadata.LanguageSpecific["classifiers"].([]string)
	assert.Contains(t, classifiers, "Programming Language :: Python")
}

// TestLegacy_SetupCfgClassifiersDeriveMatrix covers projects that declare
// Python versions through classifiers rather than python_requires.
func TestLegacy_SetupCfgClassifiersDeriveMatrix(t *testing.T) {
	setupCfg := "[metadata]\n" +
		"name = sample-pkg\n" +
		"version = 1.2.3\n" +
		"classifiers =\n" +
		"    Programming Language :: Python :: 3\n" +
		"    Programming Language :: Python :: 3.10\n" +
		"    Programming Language :: Python :: 3.11\n" +
		"    Programming Language :: Python :: 3.12\n"

	tmpDir := createTempProject(t, map[string]string{"setup.cfg": setupCfg})
	defer os.RemoveAll(tmpDir)

	extractor := NewExtractor()
	metadata, err := extractor.Extract(tmpDir)
	require.NoError(t, err)

	matrix, _ := metadata.LanguageSpecific["version_matrix"].([]string)
	assert.Equal(t, []string{"3.10", "3.11", "3.12"}, matrix,
		"classifier-derived matrix should ignore the bare 3 and sort numerically")
	assert.Equal(t, "3.12", metadata.LanguageSpecific["build_version"])
	assert.Equal(t, "classifiers", metadata.LanguageSpecific["requires_python_source"])
	assert.Equal(t, "static", metadata.LanguageSpecific["versioning_type"])
	// Fallback should NOT fire when classifiers supply versions.
	assert.Nil(t, metadata.LanguageSpecific["requires_python_fallback"])
}

// TestLegacy_SetupCfgAttrVersionIsDynamic covers attr: style version
// indirection (setuptools dynamic versioning without PBR).
func TestLegacy_SetupCfgAttrVersionIsDynamic(t *testing.T) {
	setupCfg := "[metadata]\n" +
		"name = attr-pkg\n" +
		"version = attr: attr_pkg.__version__\n" +
		"python_requires = >=3.9\n"

	tmpDir := createTempProject(t, map[string]string{"setup.cfg": setupCfg})
	defer os.RemoveAll(tmpDir)

	extractor := NewExtractor()
	metadata, err := extractor.Extract(tmpDir)
	require.NoError(t, err)

	assert.Equal(t, "dynamic", metadata.LanguageSpecific["versioning_type"])
	assert.Equal(t, "setuptools-dynamic", metadata.LanguageSpecific["dynamic_provider"])
	assert.Equal(t, ">=3.9", metadata.LanguageSpecific["requires_python"])
}

// TestLegacy_SetupCfgFileVersionIsDynamic covers file: style indirection.
func TestLegacy_SetupCfgFileVersionIsDynamic(t *testing.T) {
	setupCfg := "[metadata]\n" +
		"name = file-pkg\n" +
		"version = file: VERSION\n"

	tmpDir := createTempProject(t, map[string]string{"setup.cfg": setupCfg})
	defer os.RemoveAll(tmpDir)

	extractor := NewExtractor()
	metadata, err := extractor.Extract(tmpDir)
	require.NoError(t, err)

	assert.Equal(t, "dynamic", metadata.LanguageSpecific["versioning_type"])
	assert.Equal(t, "setuptools-dynamic", metadata.LanguageSpecific["dynamic_provider"])
}

// TestLegacy_SetupCfgSetuptoolsScmInSetupRequires covers the
// "setup_requires = setuptools_scm" declarative-setup convention.
func TestLegacy_SetupCfgSetuptoolsScmInSetupRequires(t *testing.T) {
	setupCfg := "[metadata]\n" +
		"name = scm-pkg\n" +
		"\n" +
		"[options]\n" +
		"setup_requires =\n" +
		"    setuptools_scm\n" +
		"python_requires = >=3.10\n"

	tmpDir := createTempProject(t, map[string]string{"setup.cfg": setupCfg})
	defer os.RemoveAll(tmpDir)

	extractor := NewExtractor()
	metadata, err := extractor.Extract(tmpDir)
	require.NoError(t, err)

	assert.Equal(t, "dynamic", metadata.LanguageSpecific["versioning_type"])
	assert.Equal(t, "setuptools-scm", metadata.LanguageSpecific["dynamic_provider"])
	assert.Equal(t, ">=3.10", metadata.LanguageSpecific["requires_python"])
}

// TestLegacy_SetupCfgColonSeparator verifies that setup.cfg files written
// with ":" separators (RawConfigParser accepts both "=" and ":") parse OK.
func TestLegacy_SetupCfgColonSeparator(t *testing.T) {
	setupCfg := "[metadata]\n" +
		"name: colon-pkg\n" +
		"version: 2.0.0\n" +
		"python_requires: >=3.11\n"

	tmpDir := createTempProject(t, map[string]string{"setup.cfg": setupCfg})
	defer os.RemoveAll(tmpDir)

	extractor := NewExtractor()
	metadata, err := extractor.Extract(tmpDir)
	require.NoError(t, err)

	assert.Equal(t, "colon-pkg", metadata.Name)
	assert.Equal(t, "2.0.0", metadata.Version)
	assert.Equal(t, ">=3.11", metadata.LanguageSpecific["requires_python"])
}

// TestLegacy_SetupPyPbrInSetupRequires covers a plain setup.py shim that
// delegates to PBR.
func TestLegacy_SetupPyPbrInSetupRequires(t *testing.T) {
	setupPy := "from setuptools import setup\n" +
		"setup(name='pbr-shim',\n" +
		"      setup_requires=['pbr>=2.0'],\n" +
		"      pbr=True)\n"

	tmpDir := createTempProject(t, map[string]string{"setup.py": setupPy})
	defer os.RemoveAll(tmpDir)

	extractor := NewExtractor()
	metadata, err := extractor.Extract(tmpDir)
	require.NoError(t, err)

	assert.Equal(t, "dynamic", metadata.LanguageSpecific["versioning_type"])
	assert.Equal(t, "pbr", metadata.LanguageSpecific["dynamic_provider"])
	assert.Equal(t, true, metadata.LanguageSpecific["requires_python_fallback"])
}

// TestLegacy_SetupPyUseScmVersion covers setuptools-scm via setup.py.
func TestLegacy_SetupPyUseScmVersion(t *testing.T) {
	setupPy := "from setuptools import setup\n" +
		"setup(name='scm-pkg',\n" +
		"      use_scm_version=True,\n" +
		"      python_requires='>=3.10')\n"

	tmpDir := createTempProject(t, map[string]string{"setup.py": setupPy})
	defer os.RemoveAll(tmpDir)

	extractor := NewExtractor()
	metadata, err := extractor.Extract(tmpDir)
	require.NoError(t, err)

	assert.Equal(t, "dynamic", metadata.LanguageSpecific["versioning_type"])
	assert.Equal(t, "setuptools-scm", metadata.LanguageSpecific["dynamic_provider"])
}

// TestLegacy_SetupPyVersioneer covers the legacy versioneer pattern.
func TestLegacy_SetupPyVersioneer(t *testing.T) {
	setupPy := "from setuptools import setup\n" +
		"import versioneer\n" +
		"setup(name='vsn-pkg',\n" +
		"      version=versioneer.get_version(),\n" +
		"      cmdclass=versioneer.get_cmdclass())\n"

	tmpDir := createTempProject(t, map[string]string{"setup.py": setupPy})
	defer os.RemoveAll(tmpDir)

	extractor := NewExtractor()
	metadata, err := extractor.Extract(tmpDir)
	require.NoError(t, err)

	assert.Equal(t, "dynamic", metadata.LanguageSpecific["versioning_type"])
	assert.Equal(t, "versioneer", metadata.LanguageSpecific["dynamic_provider"])
}

// TestLegacy_SetupPyClassifiersDeriveMatrix covers setup.py-only projects
// that omit python_requires but list Python version classifiers.
func TestLegacy_SetupPyClassifiersDeriveMatrix(t *testing.T) {
	setupPy := "from setuptools import setup\n" +
		"setup(name='classifier-pkg',\n" +
		"      version='1.0',\n" +
		"      classifiers=[\n" +
		"          'Programming Language :: Python :: 3.10',\n" +
		"          'Programming Language :: Python :: 3.11',\n" +
		"      ])\n"

	tmpDir := createTempProject(t, map[string]string{"setup.py": setupPy})
	defer os.RemoveAll(tmpDir)

	extractor := NewExtractor()
	metadata, err := extractor.Extract(tmpDir)
	require.NoError(t, err)

	matrix, _ := metadata.LanguageSpecific["version_matrix"].([]string)
	assert.Equal(t, []string{"3.10", "3.11"}, matrix)
	assert.Equal(t, "3.11", metadata.LanguageSpecific["build_version"])
	assert.Equal(t, "classifiers", metadata.LanguageSpecific["requires_python_source"])
	assert.Nil(t, metadata.LanguageSpecific["requires_python_fallback"])
}

// TestLegacy_BareSetupPyFallback covers the absolute-minimal setup.py.
func TestLegacy_BareSetupPyFallback(t *testing.T) {
	setupPy := "from setuptools import setup\nsetup()\n"

	tmpDir := createTempProject(t, map[string]string{"setup.py": setupPy})
	defer os.RemoveAll(tmpDir)

	extractor := NewExtractor()
	metadata, err := extractor.Extract(tmpDir)
	require.NoError(t, err)

	assert.Equal(t, true, metadata.LanguageSpecific["requires_python_fallback"])
	buildVersion, _ := metadata.LanguageSpecific["build_version"].(string)
	assert.NotEmpty(t, buildVersion)
	matrixJSON, _ := metadata.LanguageSpecific["matrix_json"].(string)
	assert.Contains(t, matrixJSON, "python-version")
}

// TestLegacy_RequirementsTxtFallback covers a setup.cfg project that
// declares install_requires in a sibling requirements.txt instead of in
// the setup.cfg [options] section.
func TestLegacy_RequirementsTxtFallback(t *testing.T) {
	setupCfg := "[metadata]\n" +
		"name = req-pkg\n" +
		"version = 0.1.0\n" +
		"python_requires = >=3.11\n"

	requirements := "# Runtime dependencies\n" +
		"requests>=2.31\n" +
		"click>=8.0\n" +
		"-r common.txt\n" + // pip directive - must be skipped
		"\n"

	tmpDir := createTempProject(t, map[string]string{
		"setup.cfg":        setupCfg,
		"requirements.txt": requirements,
	})
	defer os.RemoveAll(tmpDir)

	extractor := NewExtractor()
	metadata, err := extractor.Extract(tmpDir)
	require.NoError(t, err)

	deps, ok := metadata.LanguageSpecific["dependencies"].([]string)
	require.True(t, ok)
	assert.Equal(t, []string{"requests>=2.31", "click>=8.0"}, deps,
		"pip directives and comments must be filtered out")
	assert.Equal(t, "requirements.txt", metadata.LanguageSpecific["dependencies_source"])
}

// TestLegacy_PyProjectWithoutRequiresPythonFallback covers a modern-ish
// pyproject.toml that declares a [project] table but omits requires-python.
func TestLegacy_PyProjectWithoutRequiresPythonFallback(t *testing.T) {
	pyproject := "[project]\n" +
		"name = \"sparse-pkg\"\n" +
		"version = \"1.0\"\n"

	tmpDir := createTempProject(t, map[string]string{"pyproject.toml": pyproject})
	defer os.RemoveAll(tmpDir)

	extractor := NewExtractor()
	metadata, err := extractor.Extract(tmpDir)
	require.NoError(t, err)

	assert.Equal(t, true, metadata.LanguageSpecific["requires_python_fallback"])
	assert.NotEmpty(t, metadata.LanguageSpecific["build_version"])
}

// TestLegacy_ParseSetupCfgContinuationLines verifies the new INI parser
// handles indented continuation lines, hyphenated keys, mixed "="/":"
// separators, and full-line comments.
func TestLegacy_ParseSetupCfgContinuationLines(t *testing.T) {
	input := "# leading comment\n" +
		"[metadata]\n" +
		"name = continuation-pkg\n" +
		"author-email: someone@example.org\n" +
		"classifiers =\n" +
		"    First\n" +
		"    Second\n" +
		"    Third\n" +
		"\n" +
		"[options]\n" +
		"install_requires =\n" +
		"    foo\n" +
		"    bar\n"

	cfg := parseSetupCfg(input)

	require.Contains(t, cfg, "metadata")
	assert.Equal(t, "continuation-pkg", cfg["metadata"]["name"].Raw)
	assert.Equal(t, "someone@example.org", cfg["metadata"]["author_email"].Raw,
		"hyphenated key with colon separator should be normalised")
	assert.Equal(t, []string{"First", "Second", "Third"},
		cfg["metadata"]["classifiers"].Lines)

	require.Contains(t, cfg, "options")
	assert.Equal(t, []string{"foo", "bar"},
		cfg["options"]["install_requires"].Lines)
}

// TestLegacy_DerivePythonVersionsFromClassifiers exercises the helper
// directly with edge cases: duplicates, the bare-major 3, non-Python
// classifiers, and out-of-order entries.
func TestLegacy_DerivePythonVersionsFromClassifiers(t *testing.T) {
	input := []string{
		"Programming Language :: Python :: 3",
		"Programming Language :: Python :: 3.11",
		"Programming Language :: Python :: 3.9",
		"Programming Language :: Python :: 3.11", // duplicate
		"Operating System :: OS Independent",
		"Programming Language :: Python :: 3 :: Only",
	}
	result := derivePythonVersionsFromClassifiers(input)
	assert.Equal(t, []string{"3.9", "3.11"}, result)
}

// TestLegacy_DetectDynamicProviderFromSetupPy covers each provider branch.
func TestLegacy_DetectDynamicProviderFromSetupPy(t *testing.T) {
	cases := map[string]string{
		"setup(pbr=True)":                        "pbr",
		"setup_requires=['pbr']":                 "pbr",
		"setup_requires=['setuptools_scm>=6.0']": "setuptools-scm",
		"setup_requires = [ 'setuptools-scm' ]":  "setuptools-scm",
		"setup(use_scm_version=True)":            "setuptools-scm",
		"versioneer.get_version()":               "versioneer",
		"setup(version='1.0')":                   "",
		"setup(version=__version__)":             "runtime-attr",
	}
	for input, want := range cases {
		t.Run(input, func(t *testing.T) {
			assert.Equal(t, want, detectDynamicProviderFromSetupPy(input))
		})
	}
}
