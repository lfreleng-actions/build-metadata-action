// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package helm

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractor_Name(t *testing.T) {
	e := NewExtractor()
	assert.Equal(t, "helm", e.Name())
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
			name: "valid chart",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				chartPath := filepath.Join(dir, "Chart.yaml")
				err := os.WriteFile(chartPath, []byte(`apiVersion: v2
name: test-chart
version: 1.0.0`), 0644)
				require.NoError(t, err)
				return dir
			},
			cleanup:  func(s string) {},
			expected: true,
		},
		{
			name: "missing Chart.yaml",
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
	chartPath := filepath.Join(dir, "Chart.yaml")

	chartContent := `apiVersion: v2
name: my-chart
description: A test Helm chart
version: 1.0.0
appVersion: "1.0"
type: application
home: https://example.com
keywords:
  - test
  - demo
maintainers:
  - name: Test Maintainer
    email: test@example.com
sources:
  - https://github.com/example/my-chart`

	err := os.WriteFile(chartPath, []byte(chartContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)
	require.NotNil(t, metadata)

	// Common metadata
	assert.Equal(t, "my-chart", metadata.Name)
	assert.Equal(t, "1.0.0", metadata.Version)
	assert.Equal(t, "A test Helm chart", metadata.Description)
	assert.Equal(t, "https://example.com", metadata.Homepage)
	assert.Equal(t, "https://github.com/example/my-chart", metadata.Repository)
	assert.Equal(t, "Chart.yaml", metadata.VersionSource)

	// Authors
	require.Len(t, metadata.Authors, 1)
	assert.Equal(t, "Test Maintainer <test@example.com>", metadata.Authors[0])

	// Helm-specific metadata
	assert.Equal(t, "my-chart", metadata.LanguageSpecific["chart_name"])
	assert.Equal(t, "v2", metadata.LanguageSpecific["api_version"])
	assert.Equal(t, "1.0", metadata.LanguageSpecific["app_version"])
	assert.Equal(t, "application", metadata.LanguageSpecific["chart_type"])
	assert.Equal(t, false, metadata.LanguageSpecific["is_library_chart"])
}

func TestExtractor_Extract_WithDependencies(t *testing.T) {
	dir := t.TempDir()
	chartPath := filepath.Join(dir, "Chart.yaml")

	chartContent := `apiVersion: v2
name: my-chart
version: 1.0.0
dependencies:
  - name: redis
    version: "17.0.0"
    repository: "https://charts.bitnami.com/bitnami"
  - name: postgresql
    version: "12.0.0"
    repository: "https://charts.bitnami.com/bitnami"
    condition: postgresql.enabled`

	err := os.WriteFile(chartPath, []byte(chartContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)

	// Check dependencies
	deps := metadata.LanguageSpecific["dependencies"]
	require.NotNil(t, deps)

	depsList, ok := deps.([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, depsList, 2)

	// Check first dependency
	assert.Equal(t, "redis", depsList[0]["name"])
	assert.Equal(t, "17.0.0", depsList[0]["version"])
	assert.Equal(t, "https://charts.bitnami.com/bitnami", depsList[0]["repository"])

	// Check dependency count
	assert.Equal(t, 2, metadata.LanguageSpecific["dependency_count"])
}

func TestExtractor_Extract_LibraryChart(t *testing.T) {
	dir := t.TempDir()
	chartPath := filepath.Join(dir, "Chart.yaml")

	chartContent := `apiVersion: v2
name: my-library
version: 1.0.0
type: library`

	err := os.WriteFile(chartPath, []byte(chartContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)

	assert.Equal(t, "library", metadata.LanguageSpecific["chart_type"])
	assert.Equal(t, true, metadata.LanguageSpecific["is_library_chart"])
}

func TestExtractor_Extract_WithKubeVersion(t *testing.T) {
	dir := t.TempDir()
	chartPath := filepath.Join(dir, "Chart.yaml")

	chartContent := `apiVersion: v2
name: my-chart
version: 1.0.0
kubeVersion: ">=1.27.0"`

	err := os.WriteFile(chartPath, []byte(chartContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)

	assert.Equal(t, ">=1.27.0", metadata.LanguageSpecific["kube_version"])

	// Check version matrix
	matrix := metadata.LanguageSpecific["kubernetes_version_matrix"]
	require.NotNil(t, matrix)

	matrixList, ok := matrix.([]string)
	require.True(t, ok)
	assert.Contains(t, matrixList, "1.27")
	assert.Contains(t, matrixList, "1.28")

	// Check matrix JSON
	matrixJSON := metadata.LanguageSpecific["matrix_json"]
	require.NotNil(t, matrixJSON)
	assert.Contains(t, matrixJSON, "kubernetes-version")
}

func TestExtractor_Extract_MissingFile(t *testing.T) {
	dir := t.TempDir()

	e := NewExtractor()
	_, err := e.Extract(dir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Chart.yaml not found")
}

func TestExtractor_Extract_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	chartPath := filepath.Join(dir, "Chart.yaml")

	// Invalid YAML
	err := os.WriteFile(chartPath, []byte(`invalid: yaml: content:`), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	_, err = e.Extract(dir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse")
}

func TestExtractor_Extract_MinimalChart(t *testing.T) {
	dir := t.TempDir()
	chartPath := filepath.Join(dir, "Chart.yaml")

	// Minimal valid Chart.yaml
	chartContent := `apiVersion: v2
name: minimal-chart
version: 0.1.0`

	err := os.WriteFile(chartPath, []byte(chartContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)

	assert.Equal(t, "minimal-chart", metadata.Name)
	assert.Equal(t, "0.1.0", metadata.Version)
}

func TestGenerateKubernetesVersionMatrix(t *testing.T) {
	tests := []struct {
		name          string
		constraint    string
		expectedCount int
		shouldContain []string
	}{
		{
			name:          "greater than or equal 1.27",
			constraint:    ">=1.27.0",
			expectedCount: 4,
			shouldContain: []string{"1.27", "1.28", "1.29", "1.30"},
		},
		{
			name:          "greater than or equal 1.20",
			constraint:    ">=1.20.0-0",
			expectedCount: 6,
			shouldContain: []string{"1.20", "1.21", "1.22"},
		},
		{
			name:          "caret constraint",
			constraint:    "^1.25.0",
			expectedCount: 4,
			shouldContain: []string{"1.25", "1.26", "1.27"},
		},
		{
			name:          "tilde constraint",
			constraint:    "~1.28.0",
			expectedCount: 3,
			shouldContain: []string{"1.28", "1.29", "1.30"},
		},
		{
			name:          "unknown version defaults",
			constraint:    ">=99.99.0",
			expectedCount: 4,
			shouldContain: []string{"1.27", "1.28", "1.29", "1.30"},
		},
		{
			name:          "empty constraint defaults",
			constraint:    "",
			expectedCount: 4,
			shouldContain: []string{"1.27", "1.28", "1.29", "1.30"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateKubernetesVersionMatrix(tt.constraint)
			assert.Len(t, result, tt.expectedCount)
			for _, version := range tt.shouldContain {
				assert.Contains(t, result, version)
			}
		})
	}
}

func TestQuoteStrings(t *testing.T) {
	input := []string{"1.27", "1.28", "1.29"}
	expected := []string{`"1.27"`, `"1.28"`, `"1.29"`}

	result := quoteStrings(input)
	assert.Equal(t, expected, result)
}

func TestExtractor_Extract_WithAnnotations(t *testing.T) {
	dir := t.TempDir()
	chartPath := filepath.Join(dir, "Chart.yaml")

	chartContent := `apiVersion: v2
name: annotated-chart
version: 1.0.0
annotations:
  category: Database
  licenses: Apache-2.0
  images: |
    - name: redis
      image: docker.io/bitnami/redis:7.2.0`

	err := os.WriteFile(chartPath, []byte(chartContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)

	annotations := metadata.LanguageSpecific["annotations"]
	require.NotNil(t, annotations)

	annotationsMap, ok := annotations.(map[string]string)
	require.True(t, ok)
	assert.Equal(t, "Database", annotationsMap["category"])
	assert.Equal(t, "Apache-2.0", annotationsMap["licenses"])
}

func TestExtractor_Extract_DeprecatedChart(t *testing.T) {
	dir := t.TempDir()
	chartPath := filepath.Join(dir, "Chart.yaml")

	chartContent := `apiVersion: v2
name: old-chart
version: 1.0.0
deprecated: true`

	err := os.WriteFile(chartPath, []byte(chartContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)

	assert.Equal(t, true, metadata.LanguageSpecific["deprecated"])
}
