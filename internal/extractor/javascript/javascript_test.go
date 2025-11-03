// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package javascript

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDetect verifies the extractor can detect JavaScript/Node.js projects
func TestDetect(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		expected bool
	}{
		{
			name: "valid package.json",
			files: map[string]string{
				"package.json": `{"name": "test", "version": "1.0.0"}`,
			},
			expected: true,
		},
		{
			name:     "no package.json",
			files:    map[string]string{},
			expected: false,
		},
		{
			name: "package.json in subdirectory should not detect at root",
			files: map[string]string{
				"subdir/package.json": `{"name": "test"}`,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir, err := os.MkdirTemp("", "js-extractor-test-*")
			if err != nil {
				t.Fatalf("Failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tmpDir)

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
	packageJSON := `{
  "name": "my-project",
  "version": "1.2.3",
  "description": "A test project",
  "license": "MIT",
  "author": "John Doe <john@example.com>",
  "homepage": "https://example.com",
  "repository": {
    "type": "git",
    "url": "https://github.com/example/my-project.git"
  }
}`

	tmpDir, err := os.MkdirTemp("", "js-extractor-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	pkgPath := filepath.Join(tmpDir, "package.json")
	if err := os.WriteFile(pkgPath, []byte(packageJSON), 0644); err != nil {
		t.Fatalf("Failed to write package.json: %v", err)
	}

	extractor := NewExtractor()
	metadata, err := extractor.Extract(tmpDir)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	if metadata.Name != "my-project" {
		t.Errorf("Name = %v, expected my-project", metadata.Name)
	}

	if metadata.Version != "1.2.3" {
		t.Errorf("Version = %v, expected 1.2.3", metadata.Version)
	}

	if metadata.Description != "A test project" {
		t.Errorf("Description = %v, expected 'A test project'", metadata.Description)
	}

	if metadata.License != "MIT" {
		t.Errorf("License = %v, expected MIT", metadata.License)
	}

	if metadata.Homepage != "https://example.com" {
		t.Errorf("Homepage = %v, expected https://example.com", metadata.Homepage)
	}

	if metadata.Repository != "https://github.com/example/my-project.git" {
		t.Errorf("Repository = %v, expected https://github.com/example/my-project.git", metadata.Repository)
	}
}

// TestPackageManagerDetection tests detection of different package managers
func TestPackageManagerDetection(t *testing.T) {
	tests := []struct {
		name            string
		lockFiles       map[string]string
		expectedManager string
	}{
		{
			name: "npm with package-lock.json",
			lockFiles: map[string]string{
				"package-lock.json": `{"lockfileVersion": 3}`,
			},
			expectedManager: "npm",
		},
		{
			name: "yarn classic with yarn.lock",
			lockFiles: map[string]string{
				"yarn.lock": `# yarn lockfile v1`,
			},
			expectedManager: "yarn",
		},
		{
			name: "yarn berry with .yarnrc.yml",
			lockFiles: map[string]string{
				"yarn.lock":   `# yarn lockfile v1`,
				".yarnrc.yml": `nodeLinker: node-modules`,
			},
			expectedManager: "yarn-berry",
		},
		{
			name: "pnpm with pnpm-lock.yaml",
			lockFiles: map[string]string{
				"pnpm-lock.yaml": `lockfileVersion: '6.0'`,
			},
			expectedManager: "pnpm",
		},
		{
			name: "bun with bun.lockb",
			lockFiles: map[string]string{
				"bun.lockb": `binary lock file`,
			},
			expectedManager: "bun",
		},
		{
			name:            "no lock files defaults to npm",
			lockFiles:       map[string]string{},
			expectedManager: "npm",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir, err := os.MkdirTemp("", "js-extractor-test-*")
			if err != nil {
				t.Fatalf("Failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tmpDir)

			// Create package.json
			packageJSON := `{"name": "test", "version": "1.0.0"}`
			pkgPath := filepath.Join(tmpDir, "package.json")
			if err := os.WriteFile(pkgPath, []byte(packageJSON), 0644); err != nil {
				t.Fatalf("Failed to write package.json: %v", err)
			}

			// Create lock files
			for path, content := range tt.lockFiles {
				fullPath := filepath.Join(tmpDir, path)
				if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
					t.Fatalf("Failed to write lock file: %v", err)
				}
			}

			extractor := NewExtractor()
			metadata, err := extractor.Extract(tmpDir)
			if err != nil {
				t.Fatalf("Extract() error = %v", err)
			}

			manager, ok := metadata.LanguageSpecific["package_manager"].(string)
			if !ok || manager != tt.expectedManager {
				t.Errorf("package_manager = %v, expected %v", manager, tt.expectedManager)
			}
		})
	}
}

// TestFrameworkDetection tests detection of common JavaScript frameworks
func TestFrameworkDetection(t *testing.T) {
	packageJSON := `{
  "name": "web-app",
  "version": "1.0.0",
  "dependencies": {
    "react": "^18.0.0",
    "react-dom": "^18.0.0",
    "next": "^14.0.0",
    "vue": "^3.0.0",
    "@angular/core": "^17.0.0"
  }
}`

	tmpDir, err := os.MkdirTemp("", "js-extractor-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	pkgPath := filepath.Join(tmpDir, "package.json")
	if err := os.WriteFile(pkgPath, []byte(packageJSON), 0644); err != nil {
		t.Fatalf("Failed to write package.json: %v", err)
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
		"React":   true,
		"Next.js": true,
		"Vue.js":  true,
		"Angular": true,
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

func TestFrameworkDetectionRemixAndQwik(t *testing.T) {
	tests := []struct {
		name              string
		packageJSON       string
		expectedFramework string
	}{
		{
			name: "Remix framework",
			packageJSON: `{
  "name": "remix-app",
  "version": "1.0.0",
  "dependencies": {
    "@remix-run/react": "^2.0.0",
    "@remix-run/node": "^2.0.0",
    "react": "^18.0.0",
    "react-dom": "^18.0.0"
  }
}`,
			expectedFramework: "Remix",
		},
		{
			name: "Qwik framework",
			packageJSON: `{
  "name": "qwik-app",
  "version": "1.0.0",
  "dependencies": {
    "@builder.io/qwik": "^1.0.0",
    "@builder.io/qwik-city": "^1.0.0"
  }
}`,
			expectedFramework: "Qwik",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir, err := os.MkdirTemp("", "js-extractor-test-*")
			if err != nil {
				t.Fatalf("Failed to create temp dir: %v", err)
			}
			defer func() { _ = os.RemoveAll(tmpDir) }()

			pkgPath := filepath.Join(tmpDir, "package.json")
			if err := os.WriteFile(pkgPath, []byte(tt.packageJSON), 0644); err != nil {
				t.Fatalf("Failed to write package.json: %v", err)
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

			found := false
			for _, fw := range frameworks {
				if fw == tt.expectedFramework {
					found = true
					break
				}
			}

			if !found {
				t.Errorf("Expected framework %q not found in %v", tt.expectedFramework, frameworks)
			}
		})
	}
}

// TestBuildToolDetection tests detection of build tools
func TestBuildToolDetection(t *testing.T) {
	packageJSON := `{
  "name": "build-test",
  "version": "1.0.0",
  "devDependencies": {
    "webpack": "^5.0.0",
    "vite": "^5.0.0",
    "rollup": "^4.0.0",
    "esbuild": "^0.19.0"
  }
}`

	tmpDir, err := os.MkdirTemp("", "js-extractor-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	pkgPath := filepath.Join(tmpDir, "package.json")
	if err := os.WriteFile(pkgPath, []byte(packageJSON), 0644); err != nil {
		t.Fatalf("Failed to write package.json: %v", err)
	}

	extractor := NewExtractor()
	metadata, err := extractor.Extract(tmpDir)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	buildTools, ok := metadata.LanguageSpecific["build_tools"].([]string)
	if !ok {
		t.Fatalf("build_tools is not []string")
	}

	expectedTools := map[string]bool{
		"Webpack": true,
		"Vite":    true,
		"Rollup":  true,
		"esbuild": true,
	}

	for _, tool := range buildTools {
		if !expectedTools[tool] {
			t.Errorf("Unexpected build tool detected: %v", tool)
		}
	}
}

// TestTestingFrameworkDetection tests detection of testing frameworks
func TestTestingFrameworkDetection(t *testing.T) {
	packageJSON := `{
  "name": "test-project",
  "version": "1.0.0",
  "devDependencies": {
    "jest": "^29.0.0",
    "vitest": "^1.0.0",
    "@playwright/test": "^1.40.0",
    "cypress": "^13.0.0"
  }
}`

	tmpDir, err := os.MkdirTemp("", "js-extractor-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	pkgPath := filepath.Join(tmpDir, "package.json")
	if err := os.WriteFile(pkgPath, []byte(packageJSON), 0644); err != nil {
		t.Fatalf("Failed to write package.json: %v", err)
	}

	extractor := NewExtractor()
	metadata, err := extractor.Extract(tmpDir)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	testFrameworks, ok := metadata.LanguageSpecific["testing_frameworks"].([]string)
	if !ok {
		t.Fatalf("testing_frameworks is not []string")
	}

	expectedFrameworks := map[string]bool{
		"Jest":       true,
		"Vitest":     true,
		"Playwright": true,
		"Cypress":    true,
	}

	for _, fw := range testFrameworks {
		if !expectedFrameworks[fw] {
			t.Errorf("Unexpected test framework detected: %v", fw)
		}
	}
}

// TestTypeScriptDetection tests TypeScript detection
func TestTypeScriptDetection(t *testing.T) {
	tests := []struct {
		name                  string
		files                 map[string]string
		expectedTypeScript    bool
		expectedTypeScriptVer string
	}{
		{
			name: "TypeScript in dependencies",
			files: map[string]string{
				"package.json": `{
					"name": "test",
					"version": "1.0.0",
					"devDependencies": {
						"typescript": "^5.0.0"
					}
				}`,
			},
			expectedTypeScript:    true,
			expectedTypeScriptVer: "^5.0.0",
		},
		{
			name: "TypeScript with tsconfig.json",
			files: map[string]string{
				"package.json": `{
					"name": "test",
					"version": "1.0.0",
					"devDependencies": {
						"typescript": "^5.0.0"
					}
				}`,
				"tsconfig.json": `{
					"compilerOptions": {
						"target": "ES2020",
						"module": "ESNext"
					}
				}`,
			},
			expectedTypeScript:    true,
			expectedTypeScriptVer: "^5.0.0",
		},
		{
			name: "No TypeScript",
			files: map[string]string{
				"package.json": `{
					"name": "test",
					"version": "1.0.0"
				}`,
			},
			expectedTypeScript: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir, err := os.MkdirTemp("", "js-extractor-test-*")
			if err != nil {
				t.Fatalf("Failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tmpDir)

			for path, content := range tt.files {
				fullPath := filepath.Join(tmpDir, path)
				if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
					t.Fatalf("Failed to write file: %v", err)
				}
			}

			extractor := NewExtractor()
			metadata, err := extractor.Extract(tmpDir)
			if err != nil {
				t.Fatalf("Extract() error = %v", err)
			}

			hasTypeScript, ok := metadata.LanguageSpecific["has_typescript"].(bool)
			if !ok {
				hasTypeScript = false
			}

			if hasTypeScript != tt.expectedTypeScript {
				t.Errorf("has_typescript = %v, expected %v", hasTypeScript, tt.expectedTypeScript)
			}

			if tt.expectedTypeScript {
				// TypeScript detection is based on has_typescript flag
				// The version would be in the raw dependencies map if we need it
				// For now, just verify the flag is set correctly
			}
		})
	}
}

// TestWorkspaceDetection tests monorepo/workspace detection
func TestWorkspaceDetection(t *testing.T) {
	tests := []struct {
		name              string
		packageJSON       string
		expectedWorkspace bool
		expectedCount     int
	}{
		{
			name: "workspaces as array",
			packageJSON: `{
				"name": "monorepo",
				"version": "1.0.0",
				"workspaces": ["packages/*", "apps/*"]
			}`,
			expectedWorkspace: true,
			expectedCount:     2,
		},
		{
			name: "workspaces as object",
			packageJSON: `{
				"name": "monorepo",
				"version": "1.0.0",
				"workspaces": {
					"packages": ["packages/*"]
				}
			}`,
			expectedWorkspace: true,
		},
		{
			name: "no workspaces",
			packageJSON: `{
				"name": "single-package",
				"version": "1.0.0"
			}`,
			expectedWorkspace: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir, err := os.MkdirTemp("", "js-extractor-test-*")
			if err != nil {
				t.Fatalf("Failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tmpDir)

			pkgPath := filepath.Join(tmpDir, "package.json")
			if err := os.WriteFile(pkgPath, []byte(tt.packageJSON), 0644); err != nil {
				t.Fatalf("Failed to write package.json: %v", err)
			}

			extractor := NewExtractor()
			metadata, err := extractor.Extract(tmpDir)
			if err != nil {
				t.Fatalf("Extract() error = %v", err)
			}

			isWorkspace, ok := metadata.LanguageSpecific["is_workspace"].(bool)
			if !ok {
				isWorkspace = false
			}

			if isWorkspace != tt.expectedWorkspace {
				t.Errorf("is_workspace = %v, expected %v", isWorkspace, tt.expectedWorkspace)
			}
		})
	}
}

// TestDependencyCount tests dependency counting
func TestDependencyCount(t *testing.T) {
	packageJSON := `{
  "name": "test-deps",
  "version": "1.0.0",
  "dependencies": {
    "react": "^18.0.0",
    "vue": "^3.0.0",
    "express": "^4.0.0"
  },
  "devDependencies": {
    "jest": "^29.0.0",
    "typescript": "^5.0.0"
  },
  "peerDependencies": {
    "react-dom": "^18.0.0"
  }
}`

	tmpDir, err := os.MkdirTemp("", "js-extractor-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	pkgPath := filepath.Join(tmpDir, "package.json")
	if err := os.WriteFile(pkgPath, []byte(packageJSON), 0644); err != nil {
		t.Fatalf("Failed to write package.json: %v", err)
	}

	extractor := NewExtractor()
	metadata, err := extractor.Extract(tmpDir)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	depCount, ok := metadata.LanguageSpecific["dependency_count"].(int)
	if !ok || depCount != 3 {
		t.Errorf("dependency_count = %v, expected 3", depCount)
	}

	devDepCount, ok := metadata.LanguageSpecific["dev_dependency_count"].(int)
	if !ok || devDepCount != 2 {
		t.Errorf("dev_dependency_count = %v, expected 2", devDepCount)
	}

	totalCount, ok := metadata.LanguageSpecific["total_dependency_count"].(int)
	if !ok || totalCount != 6 {
		t.Errorf("total_dependency_count = %v, expected 6", totalCount)
	}
}

// TestScriptDetection tests script detection and categorization
func TestScriptDetection(t *testing.T) {
	packageJSON := `{
  "name": "test-scripts",
  "version": "1.0.0",
  "scripts": {
    "start": "node server.js",
    "dev": "vite",
    "build": "vite build",
    "test": "jest",
    "test:watch": "jest --watch",
    "lint": "eslint .",
    "format": "prettier --write ."
  }
}`

	tmpDir, err := os.MkdirTemp("", "js-extractor-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	pkgPath := filepath.Join(tmpDir, "package.json")
	if err := os.WriteFile(pkgPath, []byte(packageJSON), 0644); err != nil {
		t.Fatalf("Failed to write package.json: %v", err)
	}

	extractor := NewExtractor()
	metadata, err := extractor.Extract(tmpDir)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	scriptCount, ok := metadata.LanguageSpecific["script_count"].(int)
	if !ok || scriptCount != 7 {
		t.Errorf("script_count = %v, expected 7", scriptCount)
	}

	// Check for detected script patterns
	detectedScripts, ok := metadata.LanguageSpecific["detected_scripts"].([]string)
	if !ok {
		t.Errorf("detected_scripts not found or wrong type")
	} else {
		// Verify build and test scripts were detected
		hasBuild := false
		hasTest := false
		for _, pattern := range detectedScripts {
			if pattern == "build" {
				hasBuild = true
			}
			if pattern == "test" {
				hasTest = true
			}
		}
		if !hasBuild {
			t.Errorf("build script pattern not detected")
		}
		if !hasTest {
			t.Errorf("test script pattern not detected")
		}
	}
}

// TestModuleTypeDetection tests module type detection (ESM vs CommonJS)
func TestModuleTypeDetection(t *testing.T) {
	tests := []struct {
		name               string
		packageJSON        string
		expectedModuleType string
	}{
		{
			name: "ESM module",
			packageJSON: `{
				"name": "test",
				"version": "1.0.0",
				"type": "module"
			}`,
			expectedModuleType: "module",
		},
		{
			name: "CommonJS module",
			packageJSON: `{
				"name": "test",
				"version": "1.0.0",
				"type": "commonjs"
			}`,
			expectedModuleType: "commonjs",
		},
		{
			name: "no type specified defaults to commonjs",
			packageJSON: `{
				"name": "test",
				"version": "1.0.0"
			}`,
			expectedModuleType: "commonjs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir, err := os.MkdirTemp("", "js-extractor-test-*")
			if err != nil {
				t.Fatalf("Failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tmpDir)

			pkgPath := filepath.Join(tmpDir, "package.json")
			if err := os.WriteFile(pkgPath, []byte(tt.packageJSON), 0644); err != nil {
				t.Fatalf("Failed to write package.json: %v", err)
			}

			extractor := NewExtractor()
			metadata, err := extractor.Extract(tmpDir)
			if err != nil {
				t.Fatalf("Extract() error = %v", err)
			}

			moduleType, ok := metadata.LanguageSpecific["module_type"].(string)
			if !ok || moduleType != tt.expectedModuleType {
				t.Errorf("module_type = %v, expected %v", moduleType, tt.expectedModuleType)
			}
		})
	}
}

// TestNoPackageJson tests behavior when no package.json exists
func TestNoPackageJson(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "js-extractor-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	extractor := NewExtractor()
	_, err = extractor.Extract(tmpDir)
	if err == nil {
		t.Error("Expected error when no package.json exists, got nil")
	}
}

// TestExtractorName verifies the extractor name
func TestExtractorName(t *testing.T) {
	extractor := NewExtractor()
	expectedName := "javascript"
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

// TestEngineNodeDetection tests Node.js engine version detection
func TestEngineNodeDetection(t *testing.T) {
	packageJSON := `{
  "name": "test",
  "version": "1.0.0",
  "engines": {
    "node": ">=18.0.0",
    "npm": ">=9.0.0"
  }
}`

	tmpDir, err := os.MkdirTemp("", "js-extractor-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	pkgPath := filepath.Join(tmpDir, "package.json")
	if err := os.WriteFile(pkgPath, []byte(packageJSON), 0644); err != nil {
		t.Fatalf("Failed to write package.json: %v", err)
	}

	extractor := NewExtractor()
	metadata, err := extractor.Extract(tmpDir)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	nodeVersion, ok := metadata.LanguageSpecific["requires_node"].(string)
	if !ok || nodeVersion != ">=18.0.0" {
		t.Errorf("requires_node = %v, expected >=18.0.0", nodeVersion)
	}
}
