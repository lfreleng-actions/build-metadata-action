// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package haskell

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewExtractor(t *testing.T) {
	e := NewExtractor()
	assert.NotNil(t, e)
	assert.Equal(t, "haskell", e.Name())
	assert.Equal(t, 1, e.Priority())
}

func TestDetect(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		expected bool
	}{
		{
			name: "cabal file present",
			files: map[string]string{
				"myproject.cabal": "name: myproject",
			},
			expected: true,
		},
		{
			name: "stack.yaml present",
			files: map[string]string{
				"stack.yaml": "resolver: lts-21.22",
			},
			expected: true,
		},
		{
			name: "package.yaml present",
			files: map[string]string{
				"package.yaml": "name: myproject",
			},
			expected: true,
		},
		{
			name: "cabal.project present",
			files: map[string]string{
				"cabal.project": "packages: .",
			},
			expected: true,
		},
		{
			name: "src directory with .hs files",
			files: map[string]string{
				"src/Main.hs": "module Main where",
			},
			expected: true,
		},
		{
			name:     "no Haskell indicators",
			files:    map[string]string{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			for path, content := range tt.files {
				fullPath := filepath.Join(tmpDir, path)
				err := os.MkdirAll(filepath.Dir(fullPath), 0755)
				require.NoError(t, err)
				err = os.WriteFile(fullPath, []byte(content), 0644)
				require.NoError(t, err)
			}

			e := NewExtractor()
			result := e.Detect(tmpDir)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractFromCabal(t *testing.T) {
	cabalContent := `name:           my-haskell-app
version:        1.2.3
synopsis:       A sample Haskell application
description:    This is a longer description of the app
homepage:       https://github.com/example/my-haskell-app
license:        BSD3
author:         John Doe
maintainer:     john@example.com
category:       Web
tested-with:    GHC == 9.4.8

build-depends:
    base >= 4.7 && < 5,
    text >= 1.2,
    bytestring,
    aeson >= 2.0

library
  exposed-modules:     MyLib
  build-depends:       base >= 4.7
  hs-source-dirs:      src
  default-language:    Haskell2010
`

	tmpDir := t.TempDir()
	cabalPath := filepath.Join(tmpDir, "my-haskell-app.cabal")
	err := os.WriteFile(cabalPath, []byte(cabalContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, metadata)

	assert.Equal(t, "my-haskell-app", metadata.Name)
	assert.Equal(t, "1.2.3", metadata.Version)
	assert.Equal(t, "my-haskell-app.cabal", metadata.VersionSource)
	assert.Equal(t, "A sample Haskell application", metadata.Description)
	assert.Equal(t, "https://github.com/example/my-haskell-app", metadata.Homepage)
	assert.Equal(t, "BSD3", metadata.License)

	assert.Len(t, metadata.Authors, 1)
	assert.Contains(t, metadata.Authors[0], "John Doe")

	assert.Equal(t, "Cabal", metadata.LanguageSpecific["build_tool"])
	assert.Equal(t, "john@example.com", metadata.LanguageSpecific["maintainer"])
	assert.Equal(t, "Web", metadata.LanguageSpecific["category"])

	deps := metadata.LanguageSpecific["dependencies"].([]string)
	assert.Greater(t, len(deps), 0)
	assert.Equal(t, len(deps), metadata.LanguageSpecific["dependency_count"])
}

func TestExtractMinimal(t *testing.T) {
	cabalContent := `name:    minimal-project
version: 0.1.0
`

	tmpDir := t.TempDir()
	cabalPath := filepath.Join(tmpDir, "minimal.cabal")
	err := os.WriteFile(cabalPath, []byte(cabalContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, metadata)

	assert.Equal(t, "minimal-project", metadata.Name)
	assert.Equal(t, "0.1.0", metadata.Version)
	assert.Equal(t, "Cabal", metadata.LanguageSpecific["build_tool"])
}

func TestExtractWithStack(t *testing.T) {
	cabalContent := `name: myapp
version: 1.0.0
`
	stackContent := `resolver: lts-21.22
packages:
- .
`

	tmpDir := t.TempDir()

	cabalPath := filepath.Join(tmpDir, "myapp.cabal")
	err := os.WriteFile(cabalPath, []byte(cabalContent), 0644)
	require.NoError(t, err)

	stackPath := filepath.Join(tmpDir, "stack.yaml")
	err = os.WriteFile(stackPath, []byte(stackContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, metadata)

	assert.Equal(t, "myapp", metadata.Name)
	assert.Equal(t, "Stack + Cabal", metadata.LanguageSpecific["build_tool"])
	assert.Equal(t, "lts-21.22", metadata.LanguageSpecific["stack_resolver"])
	assert.Equal(t, "9.4.8", metadata.LanguageSpecific["ghc_version"])
}

func TestExtractFromPackageYaml(t *testing.T) {
	packageYamlContent := `name: hpack-project
version: 2.0.0
github: example/hpack-project
`

	tmpDir := t.TempDir()
	packageYamlPath := filepath.Join(tmpDir, "package.yaml")
	err := os.WriteFile(packageYamlPath, []byte(packageYamlContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, metadata)

	assert.Equal(t, "hpack-project", metadata.Name)
	assert.Equal(t, "2.0.0", metadata.Version)
	assert.Equal(t, "package.yaml", metadata.VersionSource)
	assert.Equal(t, true, metadata.LanguageSpecific["uses_hpack"])
}

func TestParseDependenciesWithBasePackages(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "excludes base package",
			input:    "base >= 4.7 && < 5",
			expected: nil,
		},
		{
			name:     "preserves base-compat",
			input:    "base-compat >= 0.11",
			expected: []string{"base-compat"},
		},
		{
			name:     "preserves base-orphans",
			input:    "base-orphans >= 0.8",
			expected: []string{"base-orphans"},
		},
		{
			name:     "preserves base-unicode-symbols",
			input:    "base-unicode-symbols >= 0.2",
			expected: []string{"base-unicode-symbols"},
		},
		{
			name:     "mixed dependencies",
			input:    "base >= 4.7, text >= 1.2, base-compat >= 0.11, bytestring",
			expected: []string{"text", "base-compat", "bytestring"},
		},
		{
			name:     "regular packages",
			input:    "text >= 1.2, aeson >= 2.0, bytestring",
			expected: []string{"text", "aeson", "bytestring"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseDependencies(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractGHCVersionFromResolver(t *testing.T) {
	tests := []struct {
		name     string
		resolver string
		expected string
	}{
		{
			name:     "lts-22",
			resolver: "lts-22.0",
			expected: "9.6.4",
		},
		{
			name:     "lts-21",
			resolver: "lts-21.22",
			expected: "9.4.8",
		},
		{
			name:     "lts-20",
			resolver: "lts-20.26",
			expected: "9.2.8",
		},
		{
			name:     "nightly",
			resolver: "nightly-2024-01-01",
			expected: "9.6",
		},
		{
			name:     "unknown",
			resolver: "custom-resolver",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractGHCVersionFromResolver(tt.resolver)
			assert.Equal(t, tt.expected, result)
		})
	}
}
