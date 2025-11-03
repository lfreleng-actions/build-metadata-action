// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package julia

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
	assert.Equal(t, "julia", e.Name())
	assert.Equal(t, 1, e.Priority())
}

func TestDetect(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		expected bool
	}{
		{
			name: "Project.toml present",
			files: map[string]string{
				"Project.toml": `name = "MyPackage"`,
			},
			expected: true,
		},
		{
			name: "JuliaProject.toml present",
			files: map[string]string{
				"JuliaProject.toml": `name = "MyPackage"`,
			},
			expected: true,
		},
		{
			name: "Manifest.toml present",
			files: map[string]string{
				"Manifest.toml": `julia_version = "1.9.0"`,
			},
			expected: true,
		},
		{
			name: "src directory with .jl files",
			files: map[string]string{
				"src/MyPackage.jl": `module MyPackage end`,
			},
			expected: true,
		},
		{
			name: "root .jl file",
			files: map[string]string{
				"script.jl": `println("Hello")`,
			},
			expected: true,
		},
		{
			name:     "no Julia indicators",
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

func TestExtractFromProjectToml(t *testing.T) {
	projectTomlContent := `name = "MyJuliaPackage"
uuid = "12345678-1234-1234-1234-123456789abc"
version = "1.2.3"
authors = ["Jane Doe <jane@example.com>", "John Smith"]

[deps]
DataFrames = "a93c6f00-e57d-5684-b7b6-d8193f3e46c0"
CSV = "336ed68f-0bac-5ca0-87d4-7b16caf5d00b"
Plots = "91a5bcdd-55d7-5caf-9e0b-520d859cae80"

[compat]
julia = "1.9"
DataFrames = "1.6"
CSV = "0.10"
`

	tmpDir := t.TempDir()
	projectTomlPath := filepath.Join(tmpDir, "Project.toml")
	err := os.WriteFile(projectTomlPath, []byte(projectTomlContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, metadata)

	assert.Equal(t, "MyJuliaPackage", metadata.Name)
	assert.Equal(t, "1.2.3", metadata.Version)
	assert.Equal(t, "Project.toml", metadata.VersionSource)

	assert.Len(t, metadata.Authors, 2)
	assert.Contains(t, metadata.Authors, "Jane Doe <jane@example.com>")
	assert.Contains(t, metadata.Authors, "John Smith")

	assert.Equal(t, "12345678-1234-1234-1234-123456789abc", metadata.LanguageSpecific["uuid"])
	assert.Equal(t, "Pkg", metadata.LanguageSpecific["build_tool"])
	assert.Equal(t, "1.9", metadata.LanguageSpecific["julia_version"])

	deps := metadata.LanguageSpecific["dependencies"].([]string)
	assert.Len(t, deps, 3)
	assert.Contains(t, deps, "DataFrames")
	assert.Contains(t, deps, "CSV")
	assert.Contains(t, deps, "Plots")
	assert.Equal(t, 3, metadata.LanguageSpecific["dependency_count"])
}

func TestExtractMinimal(t *testing.T) {
	projectTomlContent := `name = "MinimalPackage"
version = "0.1.0"
`

	tmpDir := t.TempDir()
	projectTomlPath := filepath.Join(tmpDir, "Project.toml")
	err := os.WriteFile(projectTomlPath, []byte(projectTomlContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, metadata)

	assert.Equal(t, "MinimalPackage", metadata.Name)
	assert.Equal(t, "0.1.0", metadata.Version)
	assert.Equal(t, "Pkg", metadata.LanguageSpecific["build_tool"])
}

func TestExtractWithManifest(t *testing.T) {
	projectTomlContent := `name = "TestPackage"
version = "1.0.0"
`
	manifestTomlContent := `julia_version = "1.9.0"
manifest_format = "2.0"
`

	tmpDir := t.TempDir()

	projectTomlPath := filepath.Join(tmpDir, "Project.toml")
	err := os.WriteFile(projectTomlPath, []byte(projectTomlContent), 0644)
	require.NoError(t, err)

	manifestTomlPath := filepath.Join(tmpDir, "Manifest.toml")
	err = os.WriteFile(manifestTomlPath, []byte(manifestTomlContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, metadata)

	assert.Equal(t, "TestPackage", metadata.Name)
	assert.Equal(t, true, metadata.LanguageSpecific["has_manifest"])
}

func TestDetectPackageType(t *testing.T) {
	tests := []struct {
		name            string
		files           map[string]string
		expectedType    string
		expectTests     bool
		expectDocs      bool
		expectNotebooks bool
	}{
		{
			name: "full package structure",
			files: map[string]string{
				"Project.toml":       `name = "FullPackage"`,
				"src/FullPackage.jl": `module FullPackage end`,
				"test/runtests.jl":   `using Test`,
				"docs/make.jl":       `using Documenter`,
				"notebook.ipynb":     `{"cells": []}`,
			},
			expectedType:    "package",
			expectTests:     true,
			expectDocs:      true,
			expectNotebooks: true,
		},
		{
			name: "package without extras",
			files: map[string]string{
				"Project.toml":         `name = "SimplePackage"`,
				"src/SimplePackage.jl": `module SimplePackage end`,
			},
			expectedType:    "package",
			expectTests:     false,
			expectDocs:      false,
			expectNotebooks: false,
		},
		{
			name: "script only",
			files: map[string]string{
				"Project.toml": `name = "Script"`,
				"script.jl":    `println("Hello")`,
			},
			expectedType:    "",
			expectTests:     false,
			expectDocs:      false,
			expectNotebooks: false,
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
			metadata, err := e.Extract(tmpDir)
			require.NoError(t, err)
			require.NotNil(t, metadata)

			if tt.expectedType != "" {
				assert.Equal(t, tt.expectedType, metadata.LanguageSpecific["package_type"])
			}

			if tt.expectTests {
				assert.Equal(t, true, metadata.LanguageSpecific["has_tests"])
			}

			if tt.expectDocs {
				assert.Equal(t, true, metadata.LanguageSpecific["has_docs"])
			}

			if tt.expectNotebooks {
				assert.Equal(t, true, metadata.LanguageSpecific["has_notebooks"])
			}
		})
	}
}

func TestGenerateJuliaVersionMatrix(t *testing.T) {
	tests := []struct {
		name        string
		versionSpec string
		expected    []string
	}{
		{
			name:        "caret notation 1.9",
			versionSpec: "^1.9",
			expected:    []string{"1.9", "1.10"},
		},
		{
			name:        "tilde notation 1.9",
			versionSpec: "~1.9",
			expected:    []string{"1.9"},
		},
		{
			name:        "tilde notation 1.10",
			versionSpec: "~1.10",
			expected:    []string{"1.10"},
		},
		{
			name:        "range notation",
			versionSpec: "1.6-1.9",
			expected:    []string{"1.6", "1.9"},
		},
		{
			name:        "exact version",
			versionSpec: "1.9.4",
			expected:    []string{"1.9"},
		},
		{
			name:        "default for unknown",
			versionSpec: "unknown",
			expected:    []string{"1.9", "1.10"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateJuliaVersionMatrix(tt.versionSpec)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNormalizeVersion(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		expected string
	}{
		{
			name:     "three parts",
			version:  "1.9.4",
			expected: "1.9",
		},
		{
			name:     "two parts",
			version:  "1.9",
			expected: "1.9",
		},
		{
			name:     "one part",
			version:  "1",
			expected: "1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeVersion(tt.version)
			assert.Equal(t, tt.expected, result)
		})
	}
}
