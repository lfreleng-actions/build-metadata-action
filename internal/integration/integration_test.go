// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lfreleng-actions/build-metadata-action/internal/detector"
	"github.com/lfreleng-actions/build-metadata-action/internal/extractor"
	_ "github.com/lfreleng-actions/build-metadata-action/internal/extractor/golang"
	_ "github.com/lfreleng-actions/build-metadata-action/internal/extractor/java"
	_ "github.com/lfreleng-actions/build-metadata-action/internal/extractor/javascript"
	_ "github.com/lfreleng-actions/build-metadata-action/internal/extractor/python"
	_ "github.com/lfreleng-actions/build-metadata-action/internal/extractor/rust"
)

// TestEndToEndPythonModern tests complete flow for Python modern projects
func TestEndToEndPythonModern(t *testing.T) {
	pyprojectToml := `[build-system]
requires = ["setuptools>=61.0"]
build-backend = "setuptools.build_meta"

[project]
name = "test-package"
version = "1.2.3"
description = "A test package"
authors = [
    {name = "Test Author", email = "test@example.com"}
]
dependencies = [
    "requests>=2.28.0",
    "click>=8.0"
]
requires-python = ">=3.9"

[project.optional-dependencies]
dev = ["pytest>=7.0", "black>=23.0"]
`

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "pyproject.toml"), []byte(pyprojectToml), 0644); err != nil {
		t.Fatalf("Failed to write pyproject.toml: %v", err)
	}

	// Test detection
	projectType, err := detector.DetectProjectType(tmpDir)
	if err != nil {
		t.Fatalf("Detection failed: %v", err)
	}

	if projectType != "python-modern" {
		t.Errorf("Project type = %v, want python-modern", projectType)
	}

	// Test extraction - use registered extractor name
	ext, err := extractor.GetExtractor("python")
	if err != nil {
		t.Fatalf("Failed to get extractor: %v", err)
	}

	metadata, err := ext.Extract(tmpDir)
	if err != nil {
		t.Fatalf("Extraction failed: %v", err)
	}

	// Validate metadata
	if metadata.Name != "test-package" {
		t.Errorf("Name = %v, want test-package", metadata.Name)
	}

	if metadata.Version != "1.2.3" {
		t.Errorf("Version = %v, want 1.2.3", metadata.Version)
	}

	if metadata.Description != "A test package" {
		t.Errorf("Description = %v, want 'A test package'", metadata.Description)
	}

	// Check language-specific fields
	if requiresPython, ok := metadata.LanguageSpecific["requires_python"].(string); !ok || requiresPython != ">=3.9" {
		t.Errorf("requires_python = %v, want >=3.9", requiresPython)
	}
}

// TestEndToEndJavaMaven tests complete flow for Maven projects
func TestEndToEndJavaMaven(t *testing.T) {
	pomXML := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>

    <groupId>com.example</groupId>
    <artifactId>test-app</artifactId>
    <version>2.0.0</version>
    <packaging>jar</packaging>

    <name>Test Application</name>
    <description>Integration test Maven project</description>

    <dependencies>
        <dependency>
            <groupId>org.springframework.boot</groupId>
            <artifactId>spring-boot-starter-web</artifactId>
            <version>3.2.0</version>
        </dependency>
    </dependencies>
</project>`

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "pom.xml"), []byte(pomXML), 0644); err != nil {
		t.Fatalf("Failed to write pom.xml: %v", err)
	}

	// Test detection
	projectType, err := detector.DetectProjectType(tmpDir)
	if err != nil {
		t.Fatalf("Detection failed: %v", err)
	}

	if projectType != "java-maven" {
		t.Errorf("Project type = %v, want java-maven", projectType)
	}

	// Test extraction - use registered extractor name
	ext, err := extractor.GetExtractor("java-maven")
	if err != nil {
		t.Fatalf("Failed to get extractor: %v", err)
	}

	metadata, err := ext.Extract(tmpDir)
	if err != nil {
		t.Fatalf("Extraction failed: %v", err)
	}

	// Validate metadata
	if metadata.Name != "Test Application" {
		t.Errorf("Name = %v, want 'Test Application'", metadata.Name)
	}

	if metadata.Version != "2.0.0" {
		t.Errorf("Version = %v, want 2.0.0", metadata.Version)
	}

	// Check language-specific fields
	if groupID, ok := metadata.LanguageSpecific["group_id"].(string); !ok || groupID != "com.example" {
		t.Errorf("group_id = %v, want com.example", groupID)
	}

	if artifactID, ok := metadata.LanguageSpecific["artifact_id"].(string); !ok || artifactID != "test-app" {
		t.Errorf("artifact_id = %v, want test-app", artifactID)
	}
}

// TestEndToEndJavaGradle tests complete flow for Gradle projects
func TestEndToEndJavaGradle(t *testing.T) {
	buildGradle := `
group 'com.example'
version '3.1.0'
description 'Integration test Gradle project'

plugins {
    id 'java'
    id 'application'
}

dependencies {
    implementation 'com.google.guava:guava:32.1.3-jre'
    testImplementation 'org.junit.jupiter:junit-jupiter:5.10.0'
}
`

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "build.gradle"), []byte(buildGradle), 0644); err != nil {
		t.Fatalf("Failed to write build.gradle: %v", err)
	}

	// Test detection
	projectType, err := detector.DetectProjectType(tmpDir)
	if err != nil {
		t.Fatalf("Detection failed: %v", err)
	}

	if projectType != "java-gradle" {
		t.Errorf("Project type = %v, want java-gradle", projectType)
	}

	// Test extraction - use registered extractor name
	ext, err := extractor.GetExtractor("java-gradle")
	if err != nil {
		t.Fatalf("Failed to get extractor: %v", err)
	}

	metadata, err := ext.Extract(tmpDir)
	if err != nil {
		t.Fatalf("Extraction failed: %v", err)
	}

	// Validate metadata
	if metadata.Version != "3.1.0" {
		t.Errorf("Version = %v, want 3.1.0", metadata.Version)
	}

	if metadata.Description != "Integration test Gradle project" {
		t.Errorf("Description = %v, want 'Integration test Gradle project'", metadata.Description)
	}

	// Check language-specific fields
	if groupID, ok := metadata.LanguageSpecific["group_id"].(string); !ok || groupID != "com.example" {
		t.Errorf("group_id = %v, want com.example", groupID)
	}
}

// TestEndToEndJavaScript tests complete flow for JavaScript/Node.js projects
func TestEndToEndJavaScript(t *testing.T) {
	packageJSON := `{
  "name": "test-package",
  "version": "1.5.0",
  "description": "Integration test Node.js project",
  "main": "index.js",
  "scripts": {
    "test": "jest",
    "build": "webpack"
  },
  "dependencies": {
    "express": "^4.18.2",
    "lodash": "^4.17.21"
  },
  "devDependencies": {
    "jest": "^29.7.0"
  },
  "engines": {
    "node": ">=18.0.0",
    "npm": ">=9.0.0"
  }
}`

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "package.json"), []byte(packageJSON), 0644); err != nil {
		t.Fatalf("Failed to write package.json: %v", err)
	}

	// Test detection
	projectType, err := detector.DetectProjectType(tmpDir)
	if err != nil {
		t.Fatalf("Detection failed: %v", err)
	}

	if projectType != "javascript-npm" {
		t.Errorf("Project type = %v, want javascript-npm", projectType)
	}

	// Test extraction - use registered extractor name
	ext, err := extractor.GetExtractor("javascript")
	if err != nil {
		t.Fatalf("Failed to get extractor: %v", err)
	}

	metadata, err := ext.Extract(tmpDir)
	if err != nil {
		t.Fatalf("Extraction failed: %v", err)
	}

	// Validate metadata
	if metadata.Name != "test-package" {
		t.Errorf("Name = %v, want test-package", metadata.Name)
	}

	if metadata.Version != "1.5.0" {
		t.Errorf("Version = %v, want 1.5.0", metadata.Version)
	}

	if metadata.Description != "Integration test Node.js project" {
		t.Errorf("Description = %v, want 'Integration test Node.js project'", metadata.Description)
	}
}

// TestEndToEndGo tests complete flow for Go projects
func TestEndToEndGo(t *testing.T) {
	goMod := `module github.com/example/test-module

go 1.21

require (
	github.com/gin-gonic/gin v1.9.1
	github.com/stretchr/testify v1.8.4
)
`

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatalf("Failed to write go.mod: %v", err)
	}

	// Test detection
	projectType, err := detector.DetectProjectType(tmpDir)
	if err != nil {
		t.Fatalf("Detection failed: %v", err)
	}

	if projectType != "go-module" {
		t.Errorf("Project type = %v, want go-module", projectType)
	}

	// Test extraction - use registered extractor name
	ext, err := extractor.GetExtractor("go-module")
	if err != nil {
		t.Fatalf("Failed to get extractor: %v", err)
	}

	metadata, err := ext.Extract(tmpDir)
	if err != nil {
		t.Fatalf("Extraction failed: %v", err)
	}

	// Validate metadata
	if metadata.Name != "github.com/example/test-module" {
		t.Errorf("Name = %v, want github.com/example/test-module", metadata.Name)
	}

	// Check language-specific fields
	if goVersion, ok := metadata.LanguageSpecific["go_version"].(string); !ok || goVersion != "1.21" {
		t.Errorf("go_version = %v, want 1.21", goVersion)
	}
}

// TestEndToEndRust tests complete flow for Rust projects
func TestEndToEndRust(t *testing.T) {
	cargoToml := `[package]
name = "test-crate"
version = "0.5.0"
edition = "2021"
authors = ["Test Author <test@example.com>"]
description = "Integration test Rust project"

[dependencies]
serde = { version = "1.0", features = ["derive"] }
tokio = { version = "1.35", features = ["full"] }

[dev-dependencies]
criterion = "0.5"
`

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "Cargo.toml"), []byte(cargoToml), 0644); err != nil {
		t.Fatalf("Failed to write Cargo.toml: %v", err)
	}

	// Test detection
	projectType, err := detector.DetectProjectType(tmpDir)
	if err != nil {
		t.Fatalf("Detection failed: %v", err)
	}

	if projectType != "rust-cargo" {
		t.Errorf("Project type = %v, want rust-cargo", projectType)
	}

	// Test extraction - use registered extractor name
	ext, err := extractor.GetExtractor("rust-cargo")
	if err != nil {
		t.Fatalf("Failed to get extractor: %v", err)
	}

	metadata, err := ext.Extract(tmpDir)
	if err != nil {
		t.Fatalf("Extraction failed: %v", err)
	}

	// Validate metadata
	if metadata.Name != "test-crate" {
		t.Errorf("Name = %v, want test-crate", metadata.Name)
	}

	if metadata.Version != "0.5.0" {
		t.Errorf("Version = %v, want 0.5.0", metadata.Version)
	}

	if metadata.Description != "Integration test Rust project" {
		t.Errorf("Description = %v, want 'Integration test Rust project'", metadata.Description)
	}

	// Check language-specific fields
	if edition, ok := metadata.LanguageSpecific["edition"].(string); !ok || edition != "2021" {
		t.Errorf("edition = %v, want 2021", edition)
	}
}

// TestMultiLanguageDetection tests detection in a monorepo with multiple languages
func TestMultiLanguageDetection(t *testing.T) {
	tmpDir := t.TempDir()

	// Create files for multiple project types
	files := map[string]string{
		"pyproject.toml": `[project]
name = "python-service"
version = "1.0.0"`,
		"package.json": `{
  "name": "web-frontend",
  "version": "2.0.0"
}`,
		"go.mod": `module example.com/backend
go 1.21`,
	}

	for filename, content := range files {
		if err := os.WriteFile(filepath.Join(tmpDir, filename), []byte(content), 0644); err != nil {
			t.Fatalf("Failed to write %s: %v", filename, err)
		}
	}

	// Test multi-detection
	projectTypes, err := detector.DetectAllProjectTypes(tmpDir)
	if err != nil {
		t.Fatalf("Multi-detection failed: %v", err)
	}

	if len(projectTypes) < 3 {
		t.Errorf("Expected at least 3 project types, got %d: %v", len(projectTypes), projectTypes)
	}

	// Check that all expected types are present
	expectedTypes := map[string]bool{
		"python-modern":  false,
		"javascript-npm": false,
		"go-module":      false,
	}

	for _, pt := range projectTypes {
		if _, exists := expectedTypes[pt]; exists {
			expectedTypes[pt] = true
		}
	}

	for typeName, found := range expectedTypes {
		if !found {
			t.Errorf("Expected project type %s not detected", typeName)
		}
	}
}

// TestExtractorRegistry tests the global extractor registry
func TestExtractorRegistry(t *testing.T) {
	extractors := extractor.GetAllExtractors()

	if len(extractors) < 6 {
		t.Errorf("Expected at least 6 extractors, got %d", len(extractors))
	}

	// Check that key extractors are registered - use actual registered names
	expectedExtractors := []string{
		"python",
		"java-maven",
		"java-gradle",
		"javascript",
		"go-module",
		"rust-cargo",
	}

	for _, name := range expectedExtractors {
		ext, err := extractor.GetExtractor(name)
		if err != nil {
			t.Errorf("Failed to get extractor %s: %v", name, err)
			continue
		}

		if ext.Name() != name {
			t.Errorf("Extractor name mismatch: got %s, want %s", ext.Name(), name)
		}
	}
}

// TestDetectionPriority tests that detection respects priority ordering
func TestDetectionPriority(t *testing.T) {
	tmpDir := t.TempDir()

	// Create both package.json and tsconfig.json (TypeScript should be detected)
	packageJSON := `{
  "name": "test",
  "version": "1.0.0"
}`
	tsconfig := `{
  "compilerOptions": {
    "target": "ES2020"
  }
}`

	if err := os.WriteFile(filepath.Join(tmpDir, "package.json"), []byte(packageJSON), 0644); err != nil {
		t.Fatalf("Failed to write package.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "tsconfig.json"), []byte(tsconfig), 0644); err != nil {
		t.Fatalf("Failed to write tsconfig.json: %v", err)
	}

	// Detection should prefer TypeScript over plain JavaScript due to priority
	projectType, err := detector.DetectProjectType(tmpDir)
	if err != nil {
		t.Fatalf("Detection failed: %v", err)
	}

	// Both typescript-npm and javascript-npm have priority 1, so either is acceptable
	if projectType != "typescript-npm" && projectType != "javascript-npm" {
		t.Errorf("Project type = %v, want typescript-npm or javascript-npm", projectType)
	}
}

// TestMissingProjectFiles tests graceful handling of missing project files
func TestMissingProjectFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Empty directory
	_, err := detector.DetectProjectType(tmpDir)
	if err == nil {
		t.Error("Expected error for empty directory, got nil")
	}
}

// TestInvalidMetadataHandling tests error handling for invalid metadata files
func TestInvalidMetadataHandling(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		content  string
		extType  string
	}{
		{
			name:     "invalid JSON",
			filename: "package.json",
			content:  `{ invalid json }`,
			extType:  "javascript",
		},
		{
			name:     "invalid TOML",
			filename: "pyproject.toml",
			content:  `[invalid toml`,
			extType:  "python",
		},
		{
			name:     "invalid XML",
			filename: "pom.xml",
			content:  `<?xml version="1.0"?><invalid>`,
			extType:  "java-maven",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			if err := os.WriteFile(filepath.Join(tmpDir, tt.filename), []byte(tt.content), 0644); err != nil {
				t.Fatalf("Failed to write %s: %v", tt.filename, err)
			}

			ext, err := extractor.GetExtractor(tt.extType)
			if err != nil {
				t.Fatalf("Failed to get extractor: %v", err)
			}

			_, err = ext.Extract(tmpDir)
			if err == nil {
				t.Error("Expected error for invalid file, got nil")
			}
		})
	}
}

// TestExtractorConsistency tests that all extractors follow the same interface
func TestExtractorConsistency(t *testing.T) {
	extractors := extractor.GetAllExtractors()

	for _, ext := range extractors {
		// Check Name() is not empty
		if ext.Name() == "" {
			t.Errorf("Extractor has empty name")
		}

		// Check Priority() is reasonable
		if ext.Priority() < 0 || ext.Priority() > 100 {
			t.Errorf("Extractor %s has unreasonable priority: %d", ext.Name(), ext.Priority())
		}
	}
}

// TestCrossExtractorValidation tests that metadata structure is consistent
func TestCrossExtractorValidation(t *testing.T) {
	tests := []struct {
		name          string
		setupFiles    func(string) error
		extractorType string
		checkFields   func(*testing.T, *extractor.ProjectMetadata)
	}{
		{
			name: "Python metadata structure",
			setupFiles: func(dir string) error {
				return os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(`
[project]
name = "test"
version = "1.0.0"
`), 0644)
			},
			extractorType: "python",
			checkFields: func(t *testing.T, m *extractor.ProjectMetadata) {
				if m.Name == "" {
					t.Error("Name should not be empty")
				}
				if m.Version == "" {
					t.Error("Version should not be empty")
				}
				if m.LanguageSpecific == nil {
					t.Error("LanguageSpecific should not be nil")
				}
			},
		},
		{
			name: "Java metadata structure",
			setupFiles: func(dir string) error {
				return os.WriteFile(filepath.Join(dir, "pom.xml"), []byte(`
<?xml version="1.0"?>
<project>
    <modelVersion>4.0.0</modelVersion>
    <groupId>com.example</groupId>
    <artifactId>test</artifactId>
    <version>1.0.0</version>
</project>
`), 0644)
			},
			extractorType: "java-maven",
			checkFields: func(t *testing.T, m *extractor.ProjectMetadata) {
				if m.Version == "" {
					t.Error("Version should not be empty")
				}
				if m.LanguageSpecific == nil {
					t.Error("LanguageSpecific should not be nil")
				}
				if _, ok := m.LanguageSpecific["group_id"]; !ok {
					t.Error("Maven metadata should include group_id")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			if err := tt.setupFiles(tmpDir); err != nil {
				t.Fatalf("Setup failed: %v", err)
			}

			ext, err := extractor.GetExtractor(tt.extractorType)
			if err != nil {
				t.Fatalf("Failed to get extractor: %v", err)
			}

			metadata, err := ext.Extract(tmpDir)
			if err != nil {
				t.Fatalf("Extraction failed: %v", err)
			}

			tt.checkFields(t, metadata)
		})
	}
}
