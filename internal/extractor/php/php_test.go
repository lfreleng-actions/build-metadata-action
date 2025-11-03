// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package php

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractor_Name(t *testing.T) {
	e := NewExtractor()
	assert.Equal(t, "php", e.Name())
}

func TestExtractor_Priority(t *testing.T) {
	e := NewExtractor()
	assert.Equal(t, 1, e.Priority())
}

func TestExtractor_Detect(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(t *testing.T) string
		cleanup  func(string)
		expected bool
	}{
		{
			name: "valid composer.json",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				composerPath := filepath.Join(dir, "composer.json")
				err := os.WriteFile(composerPath, []byte(`{
  "name": "vendor/package",
  "version": "1.0.0"
}`), 0644)
				require.NoError(t, err)
				return dir
			},
			cleanup:  func(s string) {},
			expected: true,
		},
		{
			name: "missing composer.json",
			setup: func(t *testing.T) string {
				return t.TempDir()
			},
			cleanup:  func(s string) {},
			expected: false,
		},
		{
			name: "non-existent directory",
			setup: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "nonexistent")
			},
			cleanup:  func(s string) {},
			expected: false,
		},
	}

	e := NewExtractor()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup(t)
			defer tt.cleanup(path)
			result := e.Detect(path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractor_Extract_Basic(t *testing.T) {
	dir := t.TempDir()
	composerPath := filepath.Join(dir, "composer.json")

	composerContent := `{
  "name": "vendor/package",
  "description": "A test PHP package",
  "version": "1.2.3",
  "type": "library",
  "license": "MIT",
  "homepage": "https://example.com",
  "authors": [
    {
      "name": "John Doe",
      "email": "john@example.com"
    }
  ],
  "support": {
    "source": "https://github.com/vendor/package"
  },
  "require": {
    "php": "^8.1"
  }
}`

	err := os.WriteFile(composerPath, []byte(composerContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)
	require.NotNil(t, metadata)

	// Common metadata
	assert.Equal(t, "vendor/package", metadata.Name)
	assert.Equal(t, "1.2.3", metadata.Version)
	assert.Equal(t, "A test PHP package", metadata.Description)
	assert.Equal(t, "MIT", metadata.License)
	assert.Equal(t, "https://example.com", metadata.Homepage)
	assert.Equal(t, "https://github.com/vendor/package", metadata.Repository)
	assert.Equal(t, "composer.json", metadata.VersionSource)

	// Authors
	require.Len(t, metadata.Authors, 1)
	assert.Equal(t, "John Doe <john@example.com>", metadata.Authors[0])

	// PHP-specific metadata
	assert.Equal(t, "vendor/package", metadata.LanguageSpecific["package_name"])
	assert.Equal(t, "library", metadata.LanguageSpecific["package_type"])
	assert.Equal(t, "^8.1", metadata.LanguageSpecific["requires_php"])
	assert.Equal(t, true, metadata.LanguageSpecific["is_library"])
}

func TestExtractor_Extract_Dependencies(t *testing.T) {
	dir := t.TempDir()
	composerPath := filepath.Join(dir, "composer.json")

	composerContent := `{
  "name": "vendor/package",
  "version": "1.0.0",
  "require": {
    "php": "^8.1",
    "symfony/console": "^6.0",
    "guzzlehttp/guzzle": "^7.5",
    "ext-json": "*",
    "ext-mbstring": "*"
  },
  "require-dev": {
    "phpunit/phpunit": "^10.0",
    "phpstan/phpstan": "^1.0"
  }
}`

	err := os.WriteFile(composerPath, []byte(composerContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)

	// Check dependencies (excluding php and extensions)
	deps := metadata.LanguageSpecific["dependencies"]
	require.NotNil(t, deps)
	depsMap, ok := deps.(map[string]string)
	require.True(t, ok)
	assert.Equal(t, "^6.0", depsMap["symfony/console"])
	assert.Equal(t, "^7.5", depsMap["guzzlehttp/guzzle"])
	assert.Equal(t, 2, metadata.LanguageSpecific["dependency_count"])

	// Check dev dependencies
	devDeps := metadata.LanguageSpecific["dev_dependencies"]
	require.NotNil(t, devDeps)
	devDepsMap, ok := devDeps.(map[string]string)
	require.True(t, ok)
	assert.Equal(t, "^10.0", devDepsMap["phpunit/phpunit"])
	assert.Equal(t, 2, metadata.LanguageSpecific["dev_dependency_count"])

	// Check PHP extensions
	extensions := metadata.LanguageSpecific["php_extensions"]
	require.NotNil(t, extensions)
	extList, ok := extensions.([]string)
	require.True(t, ok)
	assert.Contains(t, extList, "json")
	assert.Contains(t, extList, "mbstring")
	assert.Equal(t, 2, metadata.LanguageSpecific["extension_count"])
}

func TestExtractor_Extract_Autoload(t *testing.T) {
	dir := t.TempDir()
	composerPath := filepath.Join(dir, "composer.json")

	composerContent := `{
  "name": "vendor/package",
  "version": "1.0.0",
  "autoload": {
    "psr-4": {
      "Vendor\\Package\\": "src/",
      "Vendor\\AnotherPackage\\": "lib/"
    },
    "psr-0": {
      "Vendor_Legacy_": "legacy/"
    },
    "classmap": ["classes/", "utils/"],
    "files": ["helpers.php", "functions.php"]
  }
}`

	err := os.WriteFile(composerPath, []byte(composerContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)

	// Check PSR-4
	psr4 := metadata.LanguageSpecific["psr4_namespaces"]
	require.NotNil(t, psr4)

	// Check PSR-0
	psr0 := metadata.LanguageSpecific["psr0_namespaces"]
	require.NotNil(t, psr0)

	// Check classmap
	classmap := metadata.LanguageSpecific["classmap_paths"]
	require.NotNil(t, classmap)

	// Check files
	files := metadata.LanguageSpecific["autoload_files"]
	require.NotNil(t, files)

	// Check autoload types
	autoloadTypes := metadata.LanguageSpecific["autoload_types"]
	require.NotNil(t, autoloadTypes)
	typesList, ok := autoloadTypes.([]string)
	require.True(t, ok)
	assert.Contains(t, typesList, "psr-4")
	assert.Contains(t, typesList, "psr-0")
	assert.Contains(t, typesList, "classmap")
	assert.Contains(t, typesList, "files")
}

func TestExtractor_Extract_PHPVersionMatrix(t *testing.T) {
	dir := t.TempDir()
	composerPath := filepath.Join(dir, "composer.json")

	composerContent := `{
  "name": "vendor/package",
  "version": "1.0.0",
  "require": {
    "php": "^8.1"
  }
}`

	err := os.WriteFile(composerPath, []byte(composerContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)

	// Check version matrix
	matrix := metadata.LanguageSpecific["php_version_matrix"]
	require.NotNil(t, matrix)

	matrixList, ok := matrix.([]string)
	require.True(t, ok)
	assert.Contains(t, matrixList, "8.1")
	assert.Contains(t, matrixList, "8.2")
	assert.Contains(t, matrixList, "8.3")

	// Check matrix JSON
	matrixJSON := metadata.LanguageSpecific["matrix_json"]
	require.NotNil(t, matrixJSON)
	assert.Contains(t, matrixJSON, "php-version")
	assert.Contains(t, matrixJSON, "8.1")
}

func TestExtractor_Extract_LaravelFramework(t *testing.T) {
	dir := t.TempDir()
	composerPath := filepath.Join(dir, "composer.json")

	composerContent := `{
  "name": "vendor/laravel-app",
  "version": "1.0.0",
  "require": {
    "php": "^8.1",
    "laravel/framework": "^10.0"
  }
}`

	err := os.WriteFile(composerPath, []byte(composerContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)

	framework := metadata.LanguageSpecific["framework"]
	assert.Equal(t, "Laravel", framework)
}

func TestExtractor_Extract_SymfonyFramework(t *testing.T) {
	dir := t.TempDir()
	composerPath := filepath.Join(dir, "composer.json")

	composerContent := `{
  "name": "vendor/symfony-app",
  "version": "1.0.0",
  "require": {
    "php": "^8.1",
    "symfony/framework-bundle": "^6.0"
  }
}`

	err := os.WriteFile(composerPath, []byte(composerContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)

	framework := metadata.LanguageSpecific["framework"]
	assert.Equal(t, "Symfony", framework)
}

func TestExtractor_Extract_Scripts(t *testing.T) {
	dir := t.TempDir()
	composerPath := filepath.Join(dir, "composer.json")

	composerContent := `{
  "name": "vendor/package",
  "version": "1.0.0",
  "scripts": {
    "test": "phpunit",
    "lint": "phpcs",
    "analyze": "phpstan analyze",
    "post-install-cmd": "echo 'Installation complete'"
  }
}`

	err := os.WriteFile(composerPath, []byte(composerContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)

	scripts := metadata.LanguageSpecific["scripts"]
	require.NotNil(t, scripts)

	scriptsList, ok := scripts.([]string)
	require.True(t, ok)
	assert.Contains(t, scriptsList, "test")
	assert.Contains(t, scriptsList, "lint")
	assert.Contains(t, scriptsList, "analyze")
	assert.Equal(t, 4, metadata.LanguageSpecific["script_count"])
}

func TestExtractor_Extract_MultipleAuthors(t *testing.T) {
	dir := t.TempDir()
	composerPath := filepath.Join(dir, "composer.json")

	composerContent := `{
  "name": "vendor/package",
  "version": "1.0.0",
  "authors": [
    {
      "name": "John Doe",
      "email": "john@example.com"
    },
    {
      "name": "Jane Smith"
    },
    {
      "email": "contact@example.com"
    }
  ]
}`

	err := os.WriteFile(composerPath, []byte(composerContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)

	require.Len(t, metadata.Authors, 2)
	assert.Equal(t, "John Doe <john@example.com>", metadata.Authors[0])
	assert.Equal(t, "Jane Smith", metadata.Authors[1])
}

func TestExtractor_Extract_MultipleLicenses(t *testing.T) {
	dir := t.TempDir()
	composerPath := filepath.Join(dir, "composer.json")

	composerContent := `{
  "name": "vendor/package",
  "version": "1.0.0",
  "license": ["MIT", "Apache-2.0"]
}`

	err := os.WriteFile(composerPath, []byte(composerContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)

	assert.Equal(t, "MIT, Apache-2.0", metadata.License)
}

func TestExtractor_Extract_Binaries(t *testing.T) {
	dir := t.TempDir()
	composerPath := filepath.Join(dir, "composer.json")

	composerContent := `{
  "name": "vendor/package",
  "version": "1.0.0",
  "bin": ["bin/console", "bin/deploy"]
}`

	err := os.WriteFile(composerPath, []byte(composerContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)

	binaries := metadata.LanguageSpecific["binaries"]
	require.NotNil(t, binaries)
	binList, ok := binaries.([]string)
	require.True(t, ok)
	assert.Contains(t, binList, "bin/console")
	assert.Contains(t, binList, "bin/deploy")
}

func TestExtractor_Extract_MinimumStability(t *testing.T) {
	dir := t.TempDir()
	composerPath := filepath.Join(dir, "composer.json")

	composerContent := `{
  "name": "vendor/package",
  "version": "1.0.0",
  "minimum-stability": "stable",
  "prefer-stable": true
}`

	err := os.WriteFile(composerPath, []byte(composerContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)

	assert.Equal(t, "stable", metadata.LanguageSpecific["minimum_stability"])
	assert.Equal(t, true, metadata.LanguageSpecific["prefer_stable"])
}

func TestExtractor_Extract_MissingFile(t *testing.T) {
	dir := t.TempDir()

	e := NewExtractor()
	_, err := e.Extract(dir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "composer.json not found")
}

func TestExtractor_Extract_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	composerPath := filepath.Join(dir, "composer.json")

	err := os.WriteFile(composerPath, []byte(`{invalid json`), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	_, err = e.Extract(dir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse")
}

func TestExtractor_Extract_MinimalComposer(t *testing.T) {
	dir := t.TempDir()
	composerPath := filepath.Join(dir, "composer.json")

	composerContent := `{
  "name": "vendor/minimal",
  "require": {
    "php": ">=7.4"
  }
}`

	err := os.WriteFile(composerPath, []byte(composerContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)

	assert.Equal(t, "vendor/minimal", metadata.Name)
}

func TestGeneratePHPVersionMatrix(t *testing.T) {
	tests := []struct {
		name          string
		constraint    string
		expectedCount int
		shouldContain []string
	}{
		{
			name:          "caret constraint 8.1",
			constraint:    "^8.1",
			expectedCount: 3,
			shouldContain: []string{"8.1", "8.2", "8.3"},
		},
		{
			name:          "caret constraint 8.0",
			constraint:    "^8.0",
			expectedCount: 4,
			shouldContain: []string{"8.0", "8.1", "8.2", "8.3"},
		},
		{
			name:          "greater than or equal 7.4",
			constraint:    ">=7.4",
			expectedCount: 5,
			shouldContain: []string{"7.4", "8.0", "8.3"},
		},
		{
			name:          "tilde constraint 8.2",
			constraint:    "~8.2",
			expectedCount: 2,
			shouldContain: []string{"8.2", "8.3"},
		},
		{
			name:          "unknown version defaults",
			constraint:    ">=99.0",
			expectedCount: 3,
			shouldContain: []string{"8.1", "8.2", "8.3"},
		},
		{
			name:          "empty constraint defaults",
			constraint:    "",
			expectedCount: 3,
			shouldContain: []string{"8.1", "8.2", "8.3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generatePHPVersionMatrix(tt.constraint)
			assert.Len(t, result, tt.expectedCount)
			for _, version := range tt.shouldContain {
				assert.Contains(t, result, version)
			}
		})
	}
}

func TestDetectPHPFramework(t *testing.T) {
	tests := []struct {
		name         string
		requirements map[string]string
		expected     string
	}{
		{
			name:         "Laravel",
			requirements: map[string]string{"laravel/framework": "^10.0"},
			expected:     "Laravel",
		},
		{
			name:         "Symfony",
			requirements: map[string]string{"symfony/symfony": "^6.0"},
			expected:     "Symfony",
		},
		{
			name:         "Symfony bundle",
			requirements: map[string]string{"symfony/framework-bundle": "^6.0"},
			expected:     "Symfony",
		},
		{
			name:         "CakePHP",
			requirements: map[string]string{"cakephp/cakephp": "^4.0"},
			expected:     "CakePHP",
		},
		{
			name:         "No framework",
			requirements: map[string]string{"guzzlehttp/guzzle": "^7.0"},
			expected:     "",
		},
		{
			name:         "Empty requirements",
			requirements: map[string]string{},
			expected:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectPHPFramework(tt.requirements)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestQuoteStrings(t *testing.T) {
	input := []string{"8.1", "8.2", "8.3"}
	expected := []string{`"8.1"`, `"8.2"`, `"8.3"`}

	result := quoteStrings(input)
	assert.Equal(t, expected, result)
}

func TestExtractor_Extract_Keywords(t *testing.T) {
	dir := t.TempDir()
	composerPath := filepath.Join(dir, "composer.json")

	composerContent := `{
  "name": "vendor/package",
  "version": "1.0.0",
  "keywords": ["framework", "web", "api", "rest"]
}`

	err := os.WriteFile(composerPath, []byte(composerContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)

	keywords := metadata.LanguageSpecific["keywords"]
	require.NotNil(t, keywords)
	keywordsList, ok := keywords.([]string)
	require.True(t, ok)
	assert.Contains(t, keywordsList, "framework")
	assert.Contains(t, keywordsList, "api")
}

func TestExtractor_Extract_SupportURLs(t *testing.T) {
	dir := t.TempDir()
	composerPath := filepath.Join(dir, "composer.json")

	composerContent := `{
  "name": "vendor/package",
  "version": "1.0.0",
  "support": {
    "issues": "https://github.com/vendor/package/issues",
    "docs": "https://docs.example.com"
  }
}`

	err := os.WriteFile(composerPath, []byte(composerContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)

	issuesURL := metadata.LanguageSpecific["issues_url"]
	assert.Equal(t, "https://github.com/vendor/package/issues", issuesURL)

	docsURL := metadata.LanguageSpecific["docs_url"]
	assert.Equal(t, "https://docs.example.com", docsURL)
}
