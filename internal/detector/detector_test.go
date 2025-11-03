// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package detector

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDetectProjectType tests single project type detection
func TestDetectProjectType(t *testing.T) {
	tests := []struct {
		name         string
		setupFiles   map[string]string
		expectedType string
		expectError  bool
	}{
		{
			name: "Python modern (pyproject.toml)",
			setupFiles: map[string]string{
				"pyproject.toml": "[project]\nname = \"test\"",
			},
			expectedType: "python-modern",
			expectError:  false,
		},
		{
			name: "Python legacy (setup.py)",
			setupFiles: map[string]string{
				"setup.py": "from setuptools import setup",
			},
			expectedType: "python-legacy",
			expectError:  false,
		},
		{
			name: "JavaScript/Node.js",
			setupFiles: map[string]string{
				"package.json": `{"name": "test"}`,
			},
			expectedType: "javascript-npm",
			expectError:  false,
		},
		{
			name: "TypeScript",
			setupFiles: map[string]string{
				"package.json":  `{"name": "test"}`,
				"tsconfig.json": `{"compilerOptions": {}}`,
			},
			expectedType: "typescript-npm",
			expectError:  false,
		},
		{
			name: "Java Maven",
			setupFiles: map[string]string{
				"pom.xml": `<?xml version="1.0"?><project></project>`,
			},
			expectedType: "java-maven",
			expectError:  false,
		},
		{
			name: "Java Gradle (Groovy)",
			setupFiles: map[string]string{
				"build.gradle": "plugins { id 'java' }",
			},
			expectedType: "java-gradle",
			expectError:  false,
		},
		{
			name: "Java Gradle (Kotlin)",
			setupFiles: map[string]string{
				"build.gradle.kts": "plugins { id(\"java\") }",
			},
			expectedType: "kotlin-gradle", // Maps to java-gradle extractor
			expectError:  false,
		},
		{
			name: "Go module",
			setupFiles: map[string]string{
				"go.mod": "module example.com/test",
			},
			expectedType: "go-module",
			expectError:  false,
		},
		{
			name: "Rust Cargo",
			setupFiles: map[string]string{
				"Cargo.toml": "[package]\nname = \"test\"",
			},
			expectedType: "rust-cargo",
			expectError:  false,
		},
		{
			name: "Ruby gemspec",
			setupFiles: map[string]string{
				"test.gemspec": "Gem::Specification.new",
			},
			expectedType: "ruby-gemspec",
			expectError:  false,
		},
		{
			name: "Ruby Bundler",
			setupFiles: map[string]string{
				"Gemfile": "source 'https://rubygems.org'",
			},
			expectedType: "ruby-bundler",
			expectError:  false,
		},
		{
			name: "PHP Composer",
			setupFiles: map[string]string{
				"composer.json": `{"name": "test/test"}`,
			},
			expectedType: "php-composer",
			expectError:  false,
		},
		{
			name: "Swift Package",
			setupFiles: map[string]string{
				"Package.swift": "// swift-tools-version:5.9",
			},
			expectedType: "swift-package",
			expectError:  false,
		},
		{
			name: "Dart/Flutter",
			setupFiles: map[string]string{
				"pubspec.yaml": "name: test",
			},
			expectedType: "dart-flutter",
			expectError:  false,
		},
		{
			name: "C/C++ CMake",
			setupFiles: map[string]string{
				"CMakeLists.txt": "cmake_minimum_required(VERSION 3.10)",
			},
			expectedType: "c-cmake",
			expectError:  false,
		},
		{
			name: "Elixir Mix",
			setupFiles: map[string]string{
				"mix.exs": "defmodule Test.MixProject",
			},
			expectedType: "elixir-mix",
			expectError:  false,
		},
		{
			name: "Scala SBT",
			setupFiles: map[string]string{
				"build.sbt": "name := \"test\"",
			},
			expectedType: "scala-sbt",
			expectError:  false,
		},
		{
			name:         "Empty directory",
			setupFiles:   map[string]string{},
			expectedType: "",
			expectError:  true,
		},
		{
			name: "No matching files",
			setupFiles: map[string]string{
				"README.md": "# Test",
			},
			expectedType: "",
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			// Setup files
			for filename, content := range tt.setupFiles {
				path := filepath.Join(tmpDir, filename)
				if err := os.WriteFile(path, []byte(content), 0644); err != nil {
					t.Fatalf("Failed to write %s: %v", filename, err)
				}
			}

			// Test detection
			result, err := DetectProjectType(tmpDir)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if result != tt.expectedType {
					t.Errorf("DetectProjectType() = %v, want %v", result, tt.expectedType)
				}
			}
		})
	}
}

// TestDetectAllProjectTypes tests detection of multiple project types
func TestDetectAllProjectTypes(t *testing.T) {
	tests := []struct {
		name          string
		setupFiles    map[string]string
		expectedTypes []string
		minCount      int
		expectError   bool
	}{
		{
			name: "Monorepo with Python and JavaScript",
			setupFiles: map[string]string{
				"pyproject.toml": "[project]\nname = \"test\"",
				"package.json":   `{"name": "test"}`,
			},
			expectedTypes: []string{"python-modern", "javascript-npm"},
			minCount:      2,
			expectError:   false,
		},
		{
			name: "Monorepo with Java (Maven and Gradle)",
			setupFiles: map[string]string{
				"pom.xml":      `<?xml version="1.0"?><project></project>`,
				"build.gradle": "plugins { id 'java' }",
			},
			expectedTypes: []string{"java-maven", "java-gradle"},
			minCount:      2,
			expectError:   false,
		},
		{
			name: "Full-stack monorepo",
			setupFiles: map[string]string{
				"pyproject.toml": "[project]\nname = \"backend\"",
				"package.json":   `{"name": "frontend"}`,
				"go.mod":         "module example.com/api",
				"Cargo.toml":     "[package]\nname = \"worker\"",
			},
			expectedTypes: []string{"python-modern", "javascript-npm", "go-module", "rust-cargo"},
			minCount:      4,
			expectError:   false,
		},
		{
			name: "Python with setup.py and pyproject.toml",
			setupFiles: map[string]string{
				"pyproject.toml": "[project]\nname = \"test\"",
				"setup.py":       "from setuptools import setup",
			},
			expectedTypes: []string{"python-modern", "python-legacy"},
			minCount:      2,
			expectError:   false,
		},
		{
			name:          "Empty directory",
			setupFiles:    map[string]string{},
			expectedTypes: nil,
			minCount:      0,
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			// Setup files
			for filename, content := range tt.setupFiles {
				path := filepath.Join(tmpDir, filename)
				if err := os.WriteFile(path, []byte(content), 0644); err != nil {
					t.Fatalf("Failed to write %s: %v", filename, err)
				}
			}

			// Test detection
			results, err := DetectAllProjectTypes(tmpDir)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if len(results) < tt.minCount {
				t.Errorf("Expected at least %d project types, got %d: %v", tt.minCount, len(results), results)
			}

			// Check that expected types are present
			found := make(map[string]bool)
			for _, result := range results {
				found[result] = true
			}

			for _, expected := range tt.expectedTypes {
				if !found[expected] {
					t.Errorf("Expected project type %s not found in %v", expected, results)
				}
			}
		})
	}
}

// TestDetectionPriority tests that higher priority types are detected first
func TestDetectionPriority(t *testing.T) {
	tests := []struct {
		name          string
		setupFiles    map[string]string
		expectedFirst string
	}{
		{
			name: "TypeScript over JavaScript",
			setupFiles: map[string]string{
				"package.json":  `{"name": "test"}`,
				"tsconfig.json": `{}`,
			},
			expectedFirst: "typescript-npm",
		},
		{
			name: "Modern Python over legacy",
			setupFiles: map[string]string{
				"pyproject.toml": "[project]\nname = \"test\"",
				"setup.py":       "from setuptools import setup",
			},
			expectedFirst: "python-modern",
		},
		{
			name: "Maven before Gradle (lower priority number)",
			setupFiles: map[string]string{
				"pom.xml":      `<?xml version="1.0"?><project></project>`,
				"build.gradle": "plugins { id 'java' }",
			},
			expectedFirst: "java-maven",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			// Setup files
			for filename, content := range tt.setupFiles {
				path := filepath.Join(tmpDir, filename)
				if err := os.WriteFile(path, []byte(content), 0644); err != nil {
					t.Fatalf("Failed to write %s: %v", filename, err)
				}
			}

			// Test that the first detected type matches expected
			result, err := DetectProjectType(tmpDir)
			if err != nil {
				t.Fatalf("Detection failed: %v", err)
			}

			if result != tt.expectedFirst {
				t.Errorf("Expected first detection to be %s, got %s", tt.expectedFirst, result)
			}
		})
	}
}

// TestWildcardMatching tests file pattern matching with wildcards
func TestWildcardMatching(t *testing.T) {
	tests := []struct {
		name         string
		setupFiles   map[string]string
		expectedType string
	}{
		{
			name: "C# project file with wildcard",
			setupFiles: map[string]string{
				"MyApp.csproj": `<Project Sdk="Microsoft.NET.Sdk">`,
			},
			expectedType: "csharp-project",
		},
		{
			name: "Ruby gemspec with wildcard",
			setupFiles: map[string]string{
				"mygem.gemspec": "Gem::Specification.new",
			},
			expectedType: "ruby-gemspec",
		},
		{
			name: "Haskell cabal with wildcard",
			setupFiles: map[string]string{
				"mypackage.cabal": "name: mypackage",
			},
			expectedType: "haskell-cabal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			// Setup files
			for filename, content := range tt.setupFiles {
				path := filepath.Join(tmpDir, filename)
				if err := os.WriteFile(path, []byte(content), 0644); err != nil {
					t.Fatalf("Failed to write %s: %v", filename, err)
				}
			}

			// Test detection
			result, err := DetectProjectType(tmpDir)
			if err != nil {
				t.Fatalf("Detection failed: %v", err)
			}

			if result != tt.expectedType {
				t.Errorf("DetectProjectType() = %v, want %v", result, tt.expectedType)
			}
		})
	}
}

// TestMultiFileRules tests detection rules that require multiple files
func TestMultiFileRules(t *testing.T) {
	tests := []struct {
		name         string
		setupFiles   map[string]string
		expectedType string
		expectError  bool
	}{
		{
			name: "TypeScript requires both package.json and tsconfig.json",
			setupFiles: map[string]string{
				"package.json":  `{"name": "test"}`,
				"tsconfig.json": `{}`,
			},
			expectedType: "typescript-npm",
			expectError:  false,
		},
		{
			name: "Only package.json detects JavaScript",
			setupFiles: map[string]string{
				"package.json": `{"name": "test"}`,
			},
			expectedType: "javascript-npm",
			expectError:  false,
		},
		{
			name: "Only tsconfig.json does not detect TypeScript",
			setupFiles: map[string]string{
				"tsconfig.json": `{}`,
			},
			expectedType: "",
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			// Setup files
			for filename, content := range tt.setupFiles {
				path := filepath.Join(tmpDir, filename)
				if err := os.WriteFile(path, []byte(content), 0644); err != nil {
					t.Fatalf("Failed to write %s: %v", filename, err)
				}
			}

			// Test detection
			result, err := DetectProjectType(tmpDir)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if result != tt.expectedType {
					t.Errorf("DetectProjectType() = %v, want %v", result, tt.expectedType)
				}
			}
		})
	}
}

// TestGetDetectionRules tests retrieval of all detection rules
func TestGetDetectionRules(t *testing.T) {
	rules := GetDetectionRules()

	if len(rules) == 0 {
		t.Error("GetDetectionRules() returned empty slice")
	}

	// Check that rules have required fields
	for i, rule := range rules {
		if rule.Type == "" {
			t.Errorf("Rule %d has empty Type", i)
		}
		if len(rule.Files) == 0 {
			t.Errorf("Rule %d (%s) has no Files", i, rule.Type)
		}
	}
}

// TestAddDetectionRule tests adding custom detection rules
func TestAddDetectionRule(t *testing.T) {
	initialCount := len(GetDetectionRules())

	// Add a custom rule
	customRule := DetectionRule{
		Type:     "custom",
		Subtype:  "test",
		Files:    []string{"custom.test"},
		Priority: 99,
	}

	AddDetectionRule(customRule)

	newCount := len(GetDetectionRules())
	if newCount != initialCount+1 {
		t.Errorf("Expected %d rules after adding custom rule, got %d", initialCount+1, newCount)
	}

	// Test detection with custom rule
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "custom.test"), []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to write custom.test: %v", err)
	}

	result, err := DetectProjectType(tmpDir)
	if err != nil {
		t.Fatalf("Detection failed: %v", err)
	}

	if result != "custom-test" {
		t.Errorf("DetectProjectType() = %v, want custom-test", result)
	}
}

// TestProjectTypeString tests the ProjectType.String() method
func TestProjectTypeString(t *testing.T) {
	tests := []struct {
		pt       ProjectType
		expected string
	}{
		{
			pt:       ProjectType{Type: "python", Subtype: "modern"},
			expected: "python-modern",
		},
		{
			pt:       ProjectType{Type: "java", Subtype: "maven"},
			expected: "java-maven",
		},
		{
			pt:       ProjectType{Type: "go", Subtype: "module"},
			expected: "go-module",
		},
		{
			pt:       ProjectType{Type: "standalone", Subtype: ""},
			expected: "standalone",
		},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.pt.String()
			if result != tt.expected {
				t.Errorf("ProjectType.String() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestSubdirectoryDetection tests that files in subdirectories don't trigger detection
func TestSubdirectoryDetection(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a subdirectory with a project file
	subdir := filepath.Join(tmpDir, "subproject")
	if err := os.MkdirAll(subdir, 0755); err != nil {
		t.Fatalf("Failed to create subdirectory: %v", err)
	}

	if err := os.WriteFile(filepath.Join(subdir, "pom.xml"), []byte(`<?xml version="1.0"?><project></project>`), 0644); err != nil {
		t.Fatalf("Failed to write pom.xml in subdir: %v", err)
	}

	// Root directory should not detect the subdirectory's project
	_, err := DetectProjectType(tmpDir)
	if err == nil {
		t.Error("Expected error for project file in subdirectory, got nil")
	}
}

// TestCaseInsensitivity tests file name case handling
func TestCaseInsensitivity(t *testing.T) {
	// Note: This test may behave differently on case-insensitive filesystems (macOS/Windows)
	tmpDir := t.TempDir()

	// Create file with exact case
	if err := os.WriteFile(filepath.Join(tmpDir, "Cargo.toml"), []byte("[package]\nname = \"test\""), 0644); err != nil {
		t.Fatalf("Failed to write Cargo.toml: %v", err)
	}

	// Detection should work with exact case
	result, err := DetectProjectType(tmpDir)
	if err != nil {
		t.Fatalf("Detection failed: %v", err)
	}

	if result != "rust-cargo" {
		t.Errorf("DetectProjectType() = %v, want rust-cargo", result)
	}
}
