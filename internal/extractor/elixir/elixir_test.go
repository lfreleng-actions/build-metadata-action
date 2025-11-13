// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package elixir

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
	assert.Equal(t, "elixir", e.Name())
	assert.Equal(t, 1, e.Priority())
}

func TestDetect(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		expected bool
	}{
		{
			name: "mix.exs present",
			files: map[string]string{
				"mix.exs": "defmodule MyApp.MixProject do",
			},
			expected: true,
		},
		{
			name: "lib directory with .ex files",
			files: map[string]string{
				"lib/my_app.ex": "defmodule MyApp do",
			},
			expected: true,
		},
		{
			name: "root .ex file",
			files: map[string]string{
				"app.ex": "defmodule App do",
			},
			expected: true,
		},
		{
			name:     "no Elixir indicators",
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

func TestExtractFromMixExs(t *testing.T) {
	mixExsContent := `defmodule MyApp.MixProject do
  use Mix.Project

  def project do
    [
      app: :my_app,
      version: "1.2.3",
      elixir: "~> 1.14",
      description: "A sample Elixir application",
      package: package(),
      deps: deps()
    ]
  end

  defp package do
    [
      licenses: ["Apache-2.0"],
      links: %{
        "GitHub" => "https://github.com/example/my_app",
        "Homepage" => "https://example.com"
      }
    ]
  end

  defp deps do
    [
      {:phoenix, "~> 1.7.0"},
      {:ecto, "~> 3.10"},
      {:jason, "~> 1.4"}
    ]
  end
end
`

	tmpDir := t.TempDir()
	mixExsPath := filepath.Join(tmpDir, "mix.exs")
	err := os.WriteFile(mixExsPath, []byte(mixExsContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, metadata)

	assert.Equal(t, "my_app", metadata.Name)
	assert.Equal(t, "1.2.3", metadata.Version)
	assert.Equal(t, "mix.exs", metadata.VersionSource)
	assert.Equal(t, "A sample Elixir application", metadata.Description)
	assert.Equal(t, "Apache-2.0", metadata.License)
	assert.Equal(t, "https://example.com", metadata.Homepage)

	assert.Equal(t, "Mix", metadata.LanguageSpecific["build_tool"])
	assert.Equal(t, "~> 1.14", metadata.LanguageSpecific["elixir_version"])
	assert.Equal(t, "Phoenix", metadata.LanguageSpecific["framework"])

	deps := metadata.LanguageSpecific["dependencies"].([]string)
	assert.Len(t, deps, 3)
	assert.Contains(t, deps, "phoenix:~> 1.7.0")
	assert.Contains(t, deps, "ecto:~> 3.10")
	assert.Contains(t, deps, "jason:~> 1.4")
	assert.Equal(t, 3, metadata.LanguageSpecific["dependency_count"])
}

func TestExtractMinimal(t *testing.T) {
	mixExsContent := `defmodule Minimal.MixProject do
  use Mix.Project

  def project do
    [
      app: :minimal,
      version: "0.1.0"
    ]
  end
end
`

	tmpDir := t.TempDir()
	mixExsPath := filepath.Join(tmpDir, "mix.exs")
	err := os.WriteFile(mixExsPath, []byte(mixExsContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, metadata)

	assert.Equal(t, "minimal", metadata.Name)
	assert.Equal(t, "0.1.0", metadata.Version)
	assert.Equal(t, "Mix", metadata.LanguageSpecific["build_tool"])
}

func TestGenerateElixirVersionMatrix(t *testing.T) {
	tests := []struct {
		name        string
		requirement string
		expected    []string
	}{
		{
			name:        "Elixir 1.16",
			requirement: "~> 1.16",
			expected:    []string{"1.16", "1.17"},
		},
		{
			name:        "Elixir 1.15",
			requirement: "~> 1.15",
			expected:    []string{"1.15", "1.16", "1.17"},
		},
		{
			name:        "Elixir 1.14",
			requirement: ">= 1.14",
			expected:    []string{"1.14", "1.15", "1.16"},
		},
		{
			name:        "Elixir 1.13",
			requirement: "~> 1.13.0",
			expected:    []string{"1.13", "1.14", "1.15"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateElixirVersionMatrix(tt.requirement)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDetectFramework(t *testing.T) {
	tests := []struct {
		name         string
		dependencies []string
		expected     string
	}{
		{
			name:         "Phoenix framework",
			dependencies: []string{"phoenix:~> 1.7", "ecto:~> 3.10"},
			expected:     "Phoenix",
		},
		{
			name:         "Nerves framework",
			dependencies: []string{"nerves:~> 1.10", "ring_logger:~> 0.8"},
			expected:     "Nerves",
		},
		{
			name:         "Plug only",
			dependencies: []string{"plug:~> 1.14", "cowboy:~> 2.10"},
			expected:     "Plug",
		},
		{
			name:         "No framework",
			dependencies: []string{"jason:~> 1.4", "httpoison:~> 2.0"},
			expected:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectFramework(tt.dependencies)
			assert.Equal(t, tt.expected, result)
		})
	}
}
