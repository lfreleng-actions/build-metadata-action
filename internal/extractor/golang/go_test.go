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

// TestModuleVersionSuffix tests handling of semantic import versioning
func TestModuleVersionSuffix(t *testing.T) {
	tests := []struct {
		name             string
		modulePath       string
		expectedBaseName string
		description      string
	}{
		{
			name:             "v2 suffix",
			modulePath:       "github.com/moby/moby/v2",
			expectedBaseName: "moby",
			description:      "Should strip v2 suffix from semantic import version",
		},
		{
			name:             "v3 suffix",
			modulePath:       "github.com/go-chi/chi/v5",
			expectedBaseName: "chi",
			description:      "Should strip v5 suffix from semantic import version",
		},
		{
			name:             "v10 suffix",
			modulePath:       "github.com/example/repo/v10",
			expectedBaseName: "repo",
			description:      "Should strip v10 suffix (tests double-digit versions)",
		},
		{
			name:             "v100 suffix",
			modulePath:       "github.com/example/repo/v100",
			expectedBaseName: "repo",
			description:      "Should strip v100 suffix (tests triple-digit versions)",
		},
		{
			name:             "repo named v2",
			modulePath:       "github.com/user/v2",
			expectedBaseName: "v2",
			description:      "Should NOT strip when repo is literally named 'v2'",
		},
		{
			name:             "repo named v3",
			modulePath:       "github.com/company/v3",
			expectedBaseName: "v3",
			description:      "Should NOT strip when repo is literally named 'v3'",
		},
		{
			name:             "repo named v10",
			modulePath:       "github.com/api/v10",
			expectedBaseName: "v10",
			description:      "Should NOT strip when repo is literally named 'v10'",
		},
		{
			name:             "no version suffix",
			modulePath:       "github.com/example/project",
			expectedBaseName: "project",
			description:      "Should use last component when no version suffix",
		},
		{
			name:             "v1 suffix",
			modulePath:       "github.com/example/repo/v1",
			expectedBaseName: "v1",
			description:      "Should NOT strip v1 (Go doesn't use /v1 for semantic import versioning)",
		},
		{
			name:             "v0 suffix",
			modulePath:       "github.com/example/repo/v0",
			expectedBaseName: "v0",
			description:      "Should NOT strip v0 (Go doesn't use /v0 for semantic import versioning)",
		},
		{
			name:             "version-like name",
			modulePath:       "github.com/user/v2alpha",
			expectedBaseName: "v2alpha",
			description:      "Should NOT strip when not purely numeric after 'v'",
		},
		{
			name:             "gopkg.in style",
			modulePath:       "gopkg.in/yaml.v3",
			expectedBaseName: "yaml.v3",
			description:      "Should handle gopkg.in style (different convention)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			goModContent := "module " + tt.modulePath + "\n\ngo 1.21\n"

			tmpDir, err := os.MkdirTemp("", "go-extractor-test-*")
			if err != nil {
				t.Fatalf("Failed to create temp dir: %v", err)
			}
			defer func() { _ = os.RemoveAll(tmpDir) }()

			goModPath := filepath.Join(tmpDir, "go.mod")
			if err := os.WriteFile(goModPath, []byte(goModContent), 0644); err != nil {
				t.Fatalf("Failed to write go.mod: %v", err)
			}

			extractor := NewExtractor()
			metadata, err := extractor.Extract(tmpDir)
			if err != nil {
				t.Fatalf("Extract() error = %v", err)
			}

			baseName, ok := metadata.LanguageSpecific["base_name"].(string)
			if !ok {
				t.Fatalf("base_name is not a string")
			}

			if baseName != tt.expectedBaseName {
				t.Errorf("%s\nbase_name = %v, expected %v", tt.description, baseName, tt.expectedBaseName)
			}
		})
	}
}

// TestExtractBaseNameFromModulePath tests the helper function for extracting base names
func TestExtractBaseNameFromModulePath(t *testing.T) {
	tests := []struct {
		name     string
		parts    []string
		expected string
	}{
		{
			name:     "empty parts",
			parts:    []string{},
			expected: "",
		},
		{
			name:     "single component",
			parts:    []string{"repo"},
			expected: "repo",
		},
		{
			name:     "standard path",
			parts:    []string{"github.com", "user", "repo"},
			expected: "repo",
		},
		{
			name:     "v2 suffix with 4 components",
			parts:    []string{"github.com", "user", "repo", "v2"},
			expected: "repo",
		},
		{
			name:     "v3 suffix with 4 components",
			parts:    []string{"github.com", "user", "repo", "v3"},
			expected: "repo",
		},
		{
			name:     "v10 suffix",
			parts:    []string{"github.com", "user", "repo", "v10"},
			expected: "repo",
		},
		{
			name:     "v100 suffix",
			parts:    []string{"gitlab.com", "org", "project", "v100"},
			expected: "project",
		},
		{
			name:     "repo named v2 (3 components)",
			parts:    []string{"github.com", "user", "v2"},
			expected: "v2",
		},
		{
			name:     "repo named v10 (3 components)",
			parts:    []string{"example.com", "org", "v10"},
			expected: "v10",
		},
		{
			name:     "v1 suffix should not be stripped",
			parts:    []string{"github.com", "user", "repo", "v1"},
			expected: "v1",
		},
		{
			name:     "v0 suffix should not be stripped",
			parts:    []string{"github.com", "user", "repo", "v0"},
			expected: "v0",
		},
		{
			name:     "non-numeric version",
			parts:    []string{"github.com", "user", "repo", "v2alpha"},
			expected: "v2alpha",
		},
		{
			name:     "nested path with version",
			parts:    []string{"github.com", "org", "project", "subpkg", "v2"},
			expected: "subpkg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractBaseNameFromModulePath(tt.parts)
			if result != tt.expected {
				t.Errorf("extractBaseNameFromModulePath(%v) = %q, expected %q", tt.parts, result, tt.expected)
			}
		})
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

// TestVersionValidation tests that major version indicators are rejected
func TestVersionValidation(t *testing.T) {
	tests := []struct {
		name           string
		versionContent string
		expectVersion  bool
		description    string
	}{
		{
			name:           "semantic version v1.2.3",
			versionContent: "v1.2.3",
			expectVersion:  true,
			description:    "Should accept semantic version with dots",
		},
		{
			name:           "semantic version v2.0.0",
			versionContent: "v2.0.0",
			expectVersion:  true,
			description:    "Should accept semantic version even if major is v2",
		},
		{
			name:           "semantic version v10.5.3",
			versionContent: "v10.5.3",
			expectVersion:  true,
			description:    "Should accept semantic version with multi-digit major",
		},
		{
			name:           "version without v prefix",
			versionContent: "1.2.3",
			expectVersion:  true,
			description:    "Should accept version without v prefix",
		},
		{
			name:           "major version indicator v2",
			versionContent: "v2",
			expectVersion:  false,
			description:    "Should reject v2 as it's a module path component",
		},
		{
			name:           "major version indicator v10",
			versionContent: "v10",
			expectVersion:  false,
			description:    "Should reject v10 as it's a module path component",
		},
		{
			name:           "major version indicator v99",
			versionContent: "v99",
			expectVersion:  false,
			description:    "Should reject v99 (previously would pass with length check)",
		},
		{
			name:           "major version indicator v100",
			versionContent: "v100",
			expectVersion:  false,
			description:    "Should reject v100 (previously would fail with length <= 3 check)",
		},
		{
			name:           "major version indicator v9999",
			versionContent: "v9999",
			expectVersion:  false,
			description:    "Should reject v9999 regardless of digit count",
		},
		{
			name:           "prerelease version",
			versionContent: "v2.0.0-rc1",
			expectVersion:  true,
			description:    "Should accept prerelease version",
		},
		{
			name:           "version with metadata",
			versionContent: "v1.2.3+build123",
			expectVersion:  true,
			description:    "Should accept version with build metadata",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir, err := os.MkdirTemp("", "go-extractor-test-*")
			if err != nil {
				t.Fatalf("Failed to create temp dir: %v", err)
			}
			defer func() { _ = os.RemoveAll(tmpDir) }()

			// Create go.mod
			goModContent := `module github.com/example/project

go 1.21
`
			goModPath := filepath.Join(tmpDir, "go.mod")
			if err := os.WriteFile(goModPath, []byte(goModContent), 0644); err != nil {
				t.Fatalf("Failed to write go.mod: %v", err)
			}

			// Create VERSION file
			versionPath := filepath.Join(tmpDir, "VERSION")
			if err := os.WriteFile(versionPath, []byte(tt.versionContent), 0644); err != nil {
				t.Fatalf("Failed to write VERSION file: %v", err)
			}

			extractor := NewExtractor()
			metadata, err := extractor.Extract(tmpDir)
			if err != nil {
				t.Fatalf("Extract() error = %v", err)
			}

			hasVersion := metadata.Version != ""
			if hasVersion != tt.expectVersion {
				t.Errorf("%s\nGot version=%q (hasVersion=%v), expected hasVersion=%v",
					tt.description, metadata.Version, hasVersion, tt.expectVersion)
			}

			if tt.expectVersion && metadata.Version != tt.versionContent {
				t.Errorf("Expected version=%q, got %q", tt.versionContent, metadata.Version)
			}
		})
	}
}
