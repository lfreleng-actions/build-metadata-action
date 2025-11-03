// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package rust

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestDetect verifies the extractor can detect Rust projects
func TestDetect(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		expected bool
	}{
		{
			name: "valid Cargo.toml",
			files: map[string]string{
				"Cargo.toml": "[package]\nname = \"test\"\nversion = \"0.1.0\"\n",
			},
			expected: true,
		},
		{
			name:     "no Cargo.toml",
			files:    map[string]string{},
			expected: false,
		},
		{
			name: "Cargo.toml in subdirectory should not detect at root",
			files: map[string]string{
				"subdir/Cargo.toml": "[package]\nname = \"test\"\n",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary directory
			tmpDir, err := os.MkdirTemp("", "rust-extractor-test-*")
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
	cargoToml := `[package]
name = "my-crate"
version = "0.1.0"
edition = "2021"
authors = ["John Doe <john@example.com>"]
description = "A test crate"
license = "MIT"
homepage = "https://example.com"
repository = "https://github.com/example/my-crate"
`

	tmpDir, err := os.MkdirTemp("", "rust-extractor-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cargoPath := filepath.Join(tmpDir, "Cargo.toml")
	if err := os.WriteFile(cargoPath, []byte(cargoToml), 0644); err != nil {
		t.Fatalf("Failed to write Cargo.toml: %v", err)
	}

	extractor := NewExtractor()
	metadata, err := extractor.Extract(tmpDir)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	// Verify common metadata
	if metadata.Name != "my-crate" {
		t.Errorf("Name = %v, expected my-crate", metadata.Name)
	}

	if metadata.Version != "0.1.0" {
		t.Errorf("Version = %v, expected 0.1.0", metadata.Version)
	}

	if metadata.Description != "A test crate" {
		t.Errorf("Description = %v, expected 'A test crate'", metadata.Description)
	}

	if metadata.License != "MIT" {
		t.Errorf("License = %v, expected MIT", metadata.License)
	}

	if metadata.Homepage != "https://example.com" {
		t.Errorf("Homepage = %v, expected https://example.com", metadata.Homepage)
	}

	if metadata.Repository != "https://github.com/example/my-crate" {
		t.Errorf("Repository = %v, expected https://github.com/example/my-crate", metadata.Repository)
	}

	// Verify language-specific metadata
	if metadata.LanguageSpecific["edition"] != "2021" {
		t.Errorf("edition = %v, expected 2021", metadata.LanguageSpecific["edition"])
	}

	if metadata.LanguageSpecific["package_name"] != "my-crate" {
		t.Errorf("package_name = %v, expected my-crate", metadata.LanguageSpecific["package_name"])
	}
}

// TestExtractDependencies tests dependency extraction
func TestExtractDependencies(t *testing.T) {
	cargoToml := `[package]
name = "test-deps"
version = "1.0.0"
edition = "2021"

[dependencies]
serde = "1.0"
tokio = { version = "1.35", features = ["full"] }
reqwest = { version = "0.11", optional = true }

[dev-dependencies]
criterion = "0.5"

[build-dependencies]
cc = "1.0"
`

	tmpDir, err := os.MkdirTemp("", "rust-extractor-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cargoPath := filepath.Join(tmpDir, "Cargo.toml")
	if err := os.WriteFile(cargoPath, []byte(cargoToml), 0644); err != nil {
		t.Fatalf("Failed to write Cargo.toml: %v", err)
	}

	extractor := NewExtractor()
	metadata, err := extractor.Extract(tmpDir)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	// Check dependency count
	depCount, ok := metadata.LanguageSpecific["dependency_count"].(int)
	if !ok || depCount != 3 {
		t.Errorf("dependency_count = %v, expected 3", depCount)
	}

	// Check dev dependencies
	devDepCount, ok := metadata.LanguageSpecific["dev_dependency_count"].(int)
	if !ok || devDepCount != 1 {
		t.Errorf("dev_dependency_count = %v, expected 1", devDepCount)
	}

	// Check build dependencies
	buildDepCount, ok := metadata.LanguageSpecific["build_dependency_count"].(int)
	if !ok || buildDepCount != 1 {
		t.Errorf("build_dependency_count = %v, expected 1", buildDepCount)
	}

	// Check total dependencies
	totalDepCount, ok := metadata.LanguageSpecific["total_dependency_count"].(int)
	if !ok || totalDepCount != 5 {
		t.Errorf("total_dependency_count = %v, expected 5", totalDepCount)
	}

	// Check optional dependencies
	optionalDeps, ok := metadata.LanguageSpecific["optional_dependencies"].([]string)
	if !ok {
		t.Fatalf("optional_dependencies is not []string")
	}

	if len(optionalDeps) != 1 || optionalDeps[0] != "reqwest" {
		t.Errorf("optional_dependencies = %v, expected [reqwest]", optionalDeps)
	}
}

// TestFrameworkDetection tests detection of common Rust frameworks
func TestFrameworkDetection(t *testing.T) {
	cargoToml := `[package]
name = "web-app"
version = "1.0.0"
edition = "2021"

[dependencies]
tokio = { version = "1.35", features = ["full"] }
actix-web = "4.0"
serde = { version = "1.0", features = ["derive"] }
diesel = "2.0"
clap = "4.0"
`

	tmpDir, err := os.MkdirTemp("", "rust-extractor-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cargoPath := filepath.Join(tmpDir, "Cargo.toml")
	if err := os.WriteFile(cargoPath, []byte(cargoToml), 0644); err != nil {
		t.Fatalf("Failed to write Cargo.toml: %v", err)
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
		"Tokio (Async Runtime)":     true,
		"Actix Web (Web Framework)": true,
		"Serde (Serialization)":     true,
		"Diesel (ORM)":              true,
		"Clap (CLI Parser)":         true,
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

// TestRustVersionWithMSRV tests MSRV extraction and version matrix
func TestRustVersionWithMSRV(t *testing.T) {
	cargoToml := `[package]
name = "msrv-test"
version = "1.0.0"
edition = "2021"
rust-version = "1.70"
`

	tmpDir, err := os.MkdirTemp("", "rust-extractor-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cargoPath := filepath.Join(tmpDir, "Cargo.toml")
	if err := os.WriteFile(cargoPath, []byte(cargoToml), 0644); err != nil {
		t.Fatalf("Failed to write Cargo.toml: %v", err)
	}

	extractor := NewExtractor()
	metadata, err := extractor.Extract(tmpDir)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	// Check MSRV
	rustVersion, ok := metadata.LanguageSpecific["rust_version"].(string)
	if !ok || rustVersion != "1.70" {
		t.Errorf("rust_version = %v, expected 1.70", rustVersion)
	}

	msrv, ok := metadata.LanguageSpecific["msrv"].(string)
	if !ok || msrv != "1.70" {
		t.Errorf("msrv = %v, expected 1.70", msrv)
	}

	// Check version matrix
	matrix, ok := metadata.LanguageSpecific["rust_version_matrix"].([]string)
	if !ok {
		t.Fatalf("rust_version_matrix is not []string")
	}

	// The matrix should include at least MSRV (1.70) and stable
	// It may include more versions if dynamic fetching succeeds (which is good!)
	if len(matrix) < 2 {
		t.Errorf("rust_version_matrix = %v, expected at least 2 versions", matrix)
	}

	if matrix[0] != "1.70" {
		t.Errorf("rust_version_matrix[0] = %v, expected 1.70 (MSRV)", matrix[0])
	}

	if matrix[len(matrix)-1] != "stable" {
		t.Errorf("rust_version_matrix last element = %v, expected stable", matrix[len(matrix)-1])
	}

	// Check matrix_json
	matrixJSON, ok := metadata.LanguageSpecific["matrix_json"].(string)
	if !ok || matrixJSON == "" {
		t.Errorf("matrix_json is missing or not a string")
	}
}

// TestFeatures tests feature extraction
func TestFeatures(t *testing.T) {
	cargoToml := `[package]
name = "feature-test"
version = "1.0.0"
edition = "2021"

[features]
default = ["feature1"]
feature1 = []
feature2 = ["dep:optional-dep"]
full = ["feature1", "feature2"]

[dependencies]
optional-dep = { version = "1.0", optional = true }
`

	tmpDir, err := os.MkdirTemp("", "rust-extractor-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cargoPath := filepath.Join(tmpDir, "Cargo.toml")
	if err := os.WriteFile(cargoPath, []byte(cargoToml), 0644); err != nil {
		t.Fatalf("Failed to write Cargo.toml: %v", err)
	}

	extractor := NewExtractor()
	metadata, err := extractor.Extract(tmpDir)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	// Check feature count
	featureCount, ok := metadata.LanguageSpecific["feature_count"].(int)
	if !ok || featureCount != 4 {
		t.Errorf("feature_count = %v, expected 4", featureCount)
	}

	// Check feature names
	featureNames, ok := metadata.LanguageSpecific["feature_names"].([]string)
	if !ok {
		t.Fatalf("feature_names is not []string")
	}

	expectedFeatures := map[string]bool{
		"default":  true,
		"feature1": true,
		"feature2": true,
		"full":     true,
	}

	for _, name := range featureNames {
		if !expectedFeatures[name] {
			t.Errorf("Unexpected feature: %v", name)
		}
	}
}

// TestWorkspace tests workspace detection
func TestWorkspace(t *testing.T) {
	cargoToml := `[workspace]
members = ["crate1", "crate2", "crate3"]
resolver = "2"

[package]
name = "workspace-root"
version = "1.0.0"
edition = "2021"
`

	tmpDir, err := os.MkdirTemp("", "rust-extractor-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cargoPath := filepath.Join(tmpDir, "Cargo.toml")
	if err := os.WriteFile(cargoPath, []byte(cargoToml), 0644); err != nil {
		t.Fatalf("Failed to write Cargo.toml: %v", err)
	}

	extractor := NewExtractor()
	metadata, err := extractor.Extract(tmpDir)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	// Check workspace flag
	isWorkspace, ok := metadata.LanguageSpecific["is_workspace"].(bool)
	if !ok || !isWorkspace {
		t.Errorf("is_workspace = %v, expected true", isWorkspace)
	}

	// Check workspace members
	members, ok := metadata.LanguageSpecific["workspace_members"].([]string)
	if !ok {
		t.Fatalf("workspace_members is not []string")
	}

	if len(members) != 3 {
		t.Errorf("workspace_members length = %d, expected 3", len(members))
	}

	// Check workspace resolver
	resolver, ok := metadata.LanguageSpecific["workspace_resolver"].(string)
	if !ok || resolver != "2" {
		t.Errorf("workspace_resolver = %v, expected 2", resolver)
	}
}

// TestBinaryTargets tests binary target extraction
func TestBinaryTargets(t *testing.T) {
	cargoToml := `[package]
name = "multi-bin"
version = "1.0.0"
edition = "2021"

[[bin]]
name = "app1"
path = "src/bin/app1.rs"

[[bin]]
name = "app2"
path = "src/bin/app2.rs"
`

	tmpDir, err := os.MkdirTemp("", "rust-extractor-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cargoPath := filepath.Join(tmpDir, "Cargo.toml")
	if err := os.WriteFile(cargoPath, []byte(cargoToml), 0644); err != nil {
		t.Fatalf("Failed to write Cargo.toml: %v", err)
	}

	extractor := NewExtractor()
	metadata, err := extractor.Extract(tmpDir)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	// Check binary count
	binCount, ok := metadata.LanguageSpecific["binary_count"].(int)
	if !ok || binCount != 2 {
		t.Errorf("binary_count = %v, expected 2", binCount)
	}

	// Check binary names
	binNames, ok := metadata.LanguageSpecific["binary_targets"].([]string)
	if !ok {
		t.Fatalf("binary_targets is not []string")
	}

	expectedBins := map[string]bool{
		"app1": true,
		"app2": true,
	}

	for _, name := range binNames {
		if !expectedBins[name] {
			t.Errorf("Unexpected binary target: %v", name)
		}
	}
}

// TestNoCargoToml tests behavior when no Cargo.toml exists
func TestNoCargoToml(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "rust-extractor-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	extractor := NewExtractor()
	_, err = extractor.Extract(tmpDir)
	if err == nil {
		t.Error("Expected error when no Cargo.toml exists, got nil")
	}
}

// TestExtractorName verifies the extractor name matches the detector
func TestExtractorName(t *testing.T) {
	extractor := NewExtractor()
	expectedName := "rust-cargo"
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

// TestVersionMatrixFromEdition tests fallback to edition-based matrix
func TestVersionMatrixFromEdition(t *testing.T) {
	cargoToml := `[package]
name = "edition-test"
version = "1.0.0"
edition = "2021"
`

	tmpDir, err := os.MkdirTemp("", "rust-extractor-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cargoPath := filepath.Join(tmpDir, "Cargo.toml")
	if err := os.WriteFile(cargoPath, []byte(cargoToml), 0644); err != nil {
		t.Fatalf("Failed to write Cargo.toml: %v", err)
	}

	extractor := NewExtractor()
	metadata, err := extractor.Extract(tmpDir)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	// Should have version matrix based on edition
	matrix, ok := metadata.LanguageSpecific["rust_version_matrix"].([]string)
	if !ok {
		t.Fatalf("rust_version_matrix is not []string")
	}

	// Edition 2021 should give us ["1.56", "stable"]
	if len(matrix) != 2 || matrix[0] != "1.56" || matrix[1] != "stable" {
		t.Errorf("rust_version_matrix = %v, expected [1.56, stable] for edition 2021", matrix)
	}
}

// TestRustVersionCaching verifies the cache works correctly
func TestRustVersionCaching(t *testing.T) {
	// Clear cache before test
	rustVersionCache.Lock()
	rustVersionCache.versions = nil
	rustVersionCache.fetchedAt = time.Time{}
	rustVersionCache.Unlock()

	// First call should fetch from network (or fail gracefully)
	versions1, err1 := fetchRustVersions()

	// If network call fails, we expect an error
	// If it succeeds, we expect versions
	if err1 == nil {
		if len(versions1) == 0 {
			t.Error("fetchRustVersions() returned empty versions without error")
		}

		// Check that cache was populated
		rustVersionCache.RLock()
		cachedVersions := rustVersionCache.versions
		cacheTime := rustVersionCache.fetchedAt
		rustVersionCache.RUnlock()

		if len(cachedVersions) == 0 {
			t.Error("Cache was not populated after successful fetch")
		}

		if cacheTime.IsZero() {
			t.Error("Cache timestamp was not set")
		}

		// Second call should use cache (no network call)
		versions2, err2 := fetchRustVersions()
		if err2 != nil {
			t.Errorf("Second fetchRustVersions() returned error: %v", err2)
		}

		// Verify we got the same versions from cache
		if len(versions1) != len(versions2) {
			t.Errorf("Cached versions differ: first=%v, second=%v", versions1, versions2)
		}

		// Verify cache time didn't change (proving we used cache)
		rustVersionCache.RLock()
		cacheTime2 := rustVersionCache.fetchedAt
		rustVersionCache.RUnlock()

		if !cacheTime.Equal(cacheTime2) {
			t.Error("Cache timestamp changed, suggesting a second network call was made")
		}
	}
	// If network call fails, that's acceptable in tests (offline scenarios)
}

// TestRustVersionCacheExpiration verifies cache expires after TTL
func TestRustVersionCacheExpiration(t *testing.T) {
	// Set a short TTL for testing
	originalTTL := rustVersionCache.cacheTTL
	defer func() {
		rustVersionCache.cacheTTL = originalTTL
	}()

	rustVersionCache.Lock()
	rustVersionCache.cacheTTL = 1 * time.Millisecond
	rustVersionCache.versions = []string{"1.70", "1.71", "stable"}
	rustVersionCache.fetchedAt = time.Now()
	rustVersionCache.Unlock()

	// Wait for cache to expire
	time.Sleep(10 * time.Millisecond)

	// This should attempt to fetch again (cache expired)
	// We don't assert success because network might not be available
	_, _ = fetchRustVersions()

	// Verify the cache was either:
	// 1. Updated with new data if fetch succeeded, OR
	// 2. Still has old data if fetch failed
	rustVersionCache.RLock()
	hasData := len(rustVersionCache.versions) > 0
	rustVersionCache.RUnlock()

	if !hasData {
		t.Error("Cache should have data (either old or new)")
	}
}

// TestRustVersionCacheConcurrency verifies cache is thread-safe
func TestRustVersionCacheConcurrency(t *testing.T) {
	// Populate cache
	rustVersionCache.Lock()
	rustVersionCache.versions = []string{"1.70", "1.71", "stable"}
	rustVersionCache.fetchedAt = time.Now()
	rustVersionCache.cacheTTL = 1 * time.Hour
	rustVersionCache.Unlock()

	// Run concurrent reads
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			_, _ = fetchRustVersions()
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// If we get here without deadlock or race condition, the test passes
}

// TestWorkspaceInheritance verifies that workspace inheritance is handled correctly
func TestWorkspaceInheritance(t *testing.T) {
	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "rust-workspace-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a Cargo.toml with workspace inheritance
	cargoToml := `[package]
name = "test-workspace-member"
version = { workspace = true }
authors = { workspace = true }
edition = { workspace = true }
description = { workspace = true }
license = { workspace = true }
repository = { workspace = true }
homepage = { workspace = true }
keywords = { workspace = true }
categories = { workspace = true }

[workspace]
members = ["member1", "member2"]

[workspace.package]
version = "0.2.0"
authors = ["Workspace Author <author@example.com>"]
edition = "2021"
description = "Workspace-level description"
license = "MIT"
repository = "https://github.com/example/repo"
homepage = "https://example.com"
keywords = ["test", "workspace"]
categories = ["development-tools"]

[dependencies]
serde = "1.0"
`

	cargoPath := filepath.Join(tmpDir, "Cargo.toml")
	if err := os.WriteFile(cargoPath, []byte(cargoToml), 0644); err != nil {
		t.Fatalf("Failed to write Cargo.toml: %v", err)
	}

	// Extract metadata
	extractor := NewExtractor()
	metadata, err := extractor.Extract(tmpDir)
	if err != nil {
		t.Fatalf("Failed to extract metadata: %v", err)
	}

	// Verify workspace-inherited fields
	if metadata.Name != "test-workspace-member" {
		t.Errorf("Expected name 'test-workspace-member', got '%s'", metadata.Name)
	}

	if metadata.Version != "0.2.0" {
		t.Errorf("Expected version '0.2.0' (from workspace), got '%s'", metadata.Version)
	}

	if metadata.Description != "Workspace-level description" {
		t.Errorf("Expected workspace description, got '%s'", metadata.Description)
	}

	if metadata.License != "MIT" {
		t.Errorf("Expected license 'MIT' (from workspace), got '%s'", metadata.License)
	}

	if metadata.Repository != "https://github.com/example/repo" {
		t.Errorf("Expected workspace repository, got '%s'", metadata.Repository)
	}

	if metadata.Homepage != "https://example.com" {
		t.Errorf("Expected workspace homepage, got '%s'", metadata.Homepage)
	}

	if len(metadata.Authors) != 1 || metadata.Authors[0] != "Workspace Author <author@example.com>" {
		t.Errorf("Expected workspace authors, got %v", metadata.Authors)
	}

	// Verify edition is inherited
	if edition, ok := metadata.LanguageSpecific["edition"].(string); !ok || edition != "2021" {
		t.Errorf("Expected edition '2021' (from workspace), got '%v'", metadata.LanguageSpecific["edition"])
	}

	// Verify keywords are inherited
	if keywords, ok := metadata.LanguageSpecific["keywords"].([]string); !ok || len(keywords) != 2 {
		t.Errorf("Expected 2 keywords (from workspace), got %v", metadata.LanguageSpecific["keywords"])
	}

	// Verify categories are inherited
	if categories, ok := metadata.LanguageSpecific["categories"].([]string); !ok || len(categories) != 1 {
		t.Errorf("Expected 1 category (from workspace), got %v", metadata.LanguageSpecific["categories"])
	}

	// Verify workspace information is captured
	if isWorkspace, ok := metadata.LanguageSpecific["is_workspace"].(bool); !ok || !isWorkspace {
		t.Error("Expected is_workspace to be true")
	}

	if members, ok := metadata.LanguageSpecific["workspace_members"].([]string); !ok || len(members) != 2 {
		t.Errorf("Expected 2 workspace members, got %v", metadata.LanguageSpecific["workspace_members"])
	}
}
