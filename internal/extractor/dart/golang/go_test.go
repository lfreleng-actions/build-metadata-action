// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package golang

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDetect verifies the extractor can detect Go projects
func TestDetect(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		expected bool
	}{
		{
			name: "valid go.mod",
			files: map[string]string{
				"go.mod": "module github.com/example/project\n\ngo 1.21\n",
			},
			expected: true,
		},
		{
			name:     "no go.mod",
			files:    map[string]string{},
			expected: false,
		},
		{
			name: "go.mod in subdirectory should not detect at root",
			files: map[string]string{
				"subdir/go.mod": "module github.com/example/project\n",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary directory
			tmpDir, err := os.MkdirTemp("", "go-extractor-test-*")
			if err != nil {
				t.Fatalf("Failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tmpDir)

			// Create test files
			for path, content := range tt.files {
				fullPath := filepath.Join(tmpDir, path)
				dir := filepath.Dir(fullPath)
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatalf("Failed to create directory: %v", err)
				}
				if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
					t.Fatalf("Failed to write file: %v", err)
				}
			}

			// Test detection
			extractor := NewExtractor()
			result := extractor.Detect(tmpDir)
			if result != tt.expected {
				t.Errorf("Detect() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

// TestExtractBasic tests basic metadata extraction
func TestExtractBasic(t *testing.T) {
	goModContent := `module github.com/example/myproject

go 1.21

require (
	github.com/gin-gonic/gin v1.9.1
	github.com/sirupsen/logrus v1.9.3
)
`

	tmpDir, err := os.MkdirTemp("", "go-extractor-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	goModPath := filepath.Join(tmpDir, "go.mod")
	if err := os.WriteFile(goModPath, []byte(goModContent), 0644); err != nil {
		t.Fatalf("Failed to write go.mod: %v", err)
	}

	extractor := NewExtractor()
	metadata, err := extractor.Extract(tmpDir)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	// Verify common metadata
	if metadata.Name != "github.com/example/myproject" {
		t.Errorf("Name = %v, expected github.com/example/myproject", metadata.Name)
	}

	if metadata.Repository != "https://github.com/example/myproject" {
		t.Errorf("Repository = %v, expected https://github.com/example/myproject", metadata.Repository)
	}

	// Verify language-specific metadata
	if metadata.LanguageSpecific["module_path"] != "github.com/example/myproject" {
		t.Errorf("module_path = %v, expected github.com/example/myproject", metadata.LanguageSpecific["module_path"])
	}

	if metadata.LanguageSpecific["go_version"] != "1.21" {
		t.Errorf("go_version = %v, expected 1.21", metadata.LanguageSpecific["go_version"])
	}

	if metadata.LanguageSpecific["base_name"] != "myproject" {
		t.Errorf("base_name = %v, expected myproject", metadata.LanguageSpecific["base_name"])
	}
}

// TestExtractDependencies tests dependency extraction
func TestExtractDependencies(t *testing.T) {
	goModContent := `module github.com/example/project

go 1.22

require (
	github.com/gin-gonic/gin v1.9.1
	github.com/stretchr/testify v1.8.4 // indirect
	gorm.io/gorm v1.25.5
)

replace github.com/old/module => github.com/new/module v1.0.0

exclude github.com/bad/module v1.0.0

retract v1.0.0
`

	tmpDir, err := os.MkdirTemp("", "go-extractor-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	goModPath := filepath.Join(tmpDir, "go.mod")
	if err := os.WriteFile(goModPath, []byte(goModContent), 0644); err != nil {
		t.Fatalf("Failed to write go.mod: %v", err)
	}

	extractor := NewExtractor()
	metadata, err := extractor.Extract(tmpDir)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	// Check dependency count
	depCount, ok := metadata.LanguageSpecific["dependency_count"].(int)
	if !ok || depCount != 2 {
		t.Errorf("dependency_count = %v, expected 2 direct dependencies", depCount)
	}

	totalDepCount, ok := metadata.LanguageSpecific["total_dependency_count"].(int)
	if !ok || totalDepCount != 3 {
		t.Errorf("total_dependency_count = %v, expected 3 total dependencies", totalDepCount)
	}

	// Check dependencies exist
	deps, ok := metadata.LanguageSpecific["dependencies"].([]string)
	if !ok {
		t.Fatalf("dependencies is not []string")
	}

	expectedDeps := map[string]bool{
		"github.com/gin-gonic/gin@v1.9.1": true,
		"gorm.io/gorm@v1.25.5":            true,
	}

	for _, dep := range deps {
		if !expectedDeps[dep] {
			t.Errorf("unexpected direct dependency: %v", dep)
		}
	}

	// Check indirect dependencies
	indirectDeps, ok := metadata.LanguageSpecific["indirect_dependencies"].([]string)
	if !ok {
		t.Fatalf("indirect_dependencies is not []string")
	}

	if len(indirectDeps) != 1 || indirectDeps[0] != "github.com/stretchr/testify@v1.8.4" {
		t.Errorf("indirect_dependencies = %v, expected [github.com/stretchr/testify@v1.8.4]", indirectDeps)
	}

	// Check replace directives
	replaceCount, ok := metadata.LanguageSpecific["replace_count"].(int)
	if !ok || replaceCount != 1 {
		t.Errorf("replace_count = %v, expected 1", replaceCount)
	}

	// Check exclude directives
	excludeCount, ok := metadata.LanguageSpecific["exclude_count"].(int)
	if !ok || excludeCount != 1 {
		t.Errorf("exclude_count = %v, expected 1", excludeCount)
	}

	// Check retract directives
	retractCount, ok := metadata.LanguageSpecific["retract_count"].(int)
	if !ok || retractCount != 1 {
		t.Errorf("retract_count = %v, expected 1", retractCount)
	}
}

// TestFrameworkDetection tests detection of common Go frameworks
func TestFrameworkDetection(t *testing.T) {
	goModContent := `module github.com/example/api

go 1.21

require (
	github.com/gin-gonic/gin v1.9.1
	gorm.io/gorm v1.25.5
	github.com/spf13/cobra v1.8.0
	go.uber.org/zap v1.26.0
)
`

	tmpDir, err := os.MkdirTemp("", "go-extractor-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	goModPath := filepath.Join(tmpDir, "go.mod")
	if err := os.WriteFile(goModPath, []byte(goModContent), 0644); err != nil {
		t.Fatalf("Failed to write go.mod: %v", err)
	}

	extractor := NewExtractor()
	metadata, err := extractor.Extract(tmpDir)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	frameworks, ok := metadata.LanguageSpecific["frameworks"].([]string)
	if !ok {
		t.Fatalf("frameworks is not []string")
	}

	expectedFrameworks := map[string]bool{
		"Gin (Web Framework)": true,
		"GORM (ORM)":          true,
		"Cobra (CLI)":         true,
		"Zap (Logging)":       true,
	}

	if len(frameworks) != len(expectedFrameworks) {
		t.Errorf("Expected %d frameworks, got %d: %v", len(expectedFrameworks), len(frameworks), frameworks)
	}

	for _, fw := range frameworks {
		if !expectedFrameworks[fw] {
			t.Errorf("Unexpected framework detected: %v", fw)
		}
	}
}

// TestVersionMatrix tests Go version matrix generation
func TestVersionMatrix(t *testing.T) {
	tests := []struct {
		name           string
		goVersion      string
		expectedMatrix []string
		expectedInJSON bool
	}{
		{
			name:           "Go 1.21",
			goVersion:      "1.21",
			expectedMatrix: []string{"1.21", "1.22", "1.23"},
			expectedInJSON: true,
		},
		{
			name:           "Go 1.20",
			goVersion:      "1.20",
			expectedMatrix: []string{"1.20", "1.21", "1.22", "1.23"},
			expectedInJSON: true,
		},
		{
			name:           "Go 1.22",
			goVersion:      "1.22",
			expectedMatrix: []string{"1.22", "1.23"},
			expectedInJSON: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			goModContent := "module github.com/example/project\n\ngo " + tt.goVersion + "\n"

			tmpDir, err := os.MkdirTemp("", "go-extractor-test-*")
			if err != nil {
				t.Fatalf("Failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tmpDir)

			goModPath := filepath.Join(tmpDir, "go.mod")
			if err := os.WriteFile(goModPath, []byte(goModContent), 0644); err != nil {
				t.Fatalf("Failed to write go.mod: %v", err)
			}

			extractor := NewExtractor()
			metadata, err := extractor.Extract(tmpDir)
			if err != nil {
				t.Fatalf("Extract() error = %v", err)
			}

			matrix, ok := metadata.LanguageSpecific["go_version_matrix"].([]string)
			if !ok {
				t.Fatalf("go_version_matrix is not []string")
			}

			if len(matrix) != len(tt.expectedMatrix) {
				t.Errorf("Matrix length = %d, expected %d", len(matrix), len(tt.expectedMatrix))
			}

			for i, v := range tt.expectedMatrix {
				if i >= len(matrix) || matrix[i] != v {
					t.Errorf("Matrix[%d] = %v, expected %v", i, matrix[i], v)
				}
			}

			// Check matrix_json exists
			if tt.expectedInJSON {
				matrixJSON, ok := metadata.LanguageSpecific["matrix_json"].(string)
				if !ok || matrixJSON == "" {
					t.Errorf("matrix_json is missing or not a string")
				}
			}
		})
	}
}

// TestToolchain tests toolchain extraction
func TestToolchain(t *testing.T) {
	goModContent := `module github.com/example/project

go 1.21

toolchain go1.21.5
`

	tmpDir, err := os.MkdirTemp("", "go-extractor-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	goModPath := filepath.Join(tmpDir, "go.mod")
	if err := os.WriteFile(goModPath, []byte(goModContent), 0644); err != nil {
		t.Fatalf("Failed to write go.mod: %v", err)
	}

	extractor := NewExtractor()
	metadata, err := extractor.Extract(tmpDir)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	toolchain, ok := metadata.LanguageSpecific["toolchain"].(string)
	if !ok || toolchain != "go1.21.5" {
		t.Errorf("toolchain = %v, expected go1.21.5", toolchain)
	}
}

// TestNoGoMod tests behavior when no go.mod exists
func TestNoGoMod(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "go-extractor-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	extractor := NewExtractor()
	_, err = extractor.Extract(tmpDir)
	if err == nil {
		t.Error("Expected error when no go.mod exists, got nil")
	}
}

// TestExtractorName verifies the extractor name matches the detector
func TestExtractorName(t *testing.T) {
	extractor := NewExtractor()
	expectedName := "go-module"
	if extractor.Name() != expectedName {
		t.Errorf("Name() = %v, expected %v", extractor.Name(), expectedName)
	}
}

// TestExtractorPriority verifies the extractor priority
func TestExtractorPriority(t *testing.T) {
	extractor := NewExtractor()
	if extractor.Priority() <= 0 {
		t.Errorf("Priority() = %v, expected positive value", extractor.Priority())
	}
}
