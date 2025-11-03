// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package output

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestNewArtifactUploader tests creating new artifact uploader instances
func TestNewArtifactUploader(t *testing.T) {
	tests := []struct {
		name              string
		enabled           bool
		namePrefix        string
		formats           []string
		outputDir         string
		validateOutput    bool
		strictMode        bool
		expectedPrefix    string
		expectedFormats   []string
		expectedOutputDir string
	}{
		{
			name:              "default values",
			enabled:           true,
			namePrefix:        "",
			formats:           nil,
			outputDir:         "",
			validateOutput:    false,
			strictMode:        false,
			expectedPrefix:    "build-metadata",
			expectedFormats:   []string{"json", "yaml"},
			expectedOutputDir: os.TempDir(),
		},
		{
			name:              "custom values",
			enabled:           true,
			namePrefix:        "custom-metadata",
			formats:           []string{"json"},
			outputDir:         "/tmp/custom",
			validateOutput:    true,
			strictMode:        true,
			expectedPrefix:    "custom-metadata",
			expectedFormats:   []string{"json"},
			expectedOutputDir: "/tmp/custom",
		},
		{
			name:              "disabled uploader",
			enabled:           false,
			namePrefix:        "test",
			formats:           []string{"yaml"},
			outputDir:         "/tmp/test",
			validateOutput:    false,
			strictMode:        false,
			expectedPrefix:    "test",
			expectedFormats:   []string{"yaml"},
			expectedOutputDir: "/tmp/test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uploader := NewArtifactUploader(
				tt.enabled,
				tt.namePrefix,
				tt.formats,
				tt.outputDir,
				tt.validateOutput,
				tt.strictMode,
			)

			if uploader.Enabled != tt.enabled {
				t.Errorf("Expected enabled=%v, got %v", tt.enabled, uploader.Enabled)
			}

			if uploader.NamePrefix != tt.expectedPrefix {
				t.Errorf("Expected prefix=%q, got %q", tt.expectedPrefix, uploader.NamePrefix)
			}

			if len(uploader.Formats) != len(tt.expectedFormats) {
				t.Errorf("Expected %d formats, got %d", len(tt.expectedFormats), len(uploader.Formats))
			}

			if uploader.OutputDir != tt.expectedOutputDir {
				t.Errorf("Expected outputDir=%q, got %q", tt.expectedOutputDir, uploader.OutputDir)
			}

			if uploader.ValidateOutput != tt.validateOutput {
				t.Errorf("Expected validateOutput=%v, got %v", tt.validateOutput, uploader.ValidateOutput)
			}

			if uploader.StrictMode != tt.strictMode {
				t.Errorf("Expected strictMode=%v, got %v", tt.strictMode, uploader.StrictMode)
			}
		})
	}
}

// TestUpload_Disabled tests that disabled uploader returns nil
func TestUpload_Disabled(t *testing.T) {
	uploader := NewArtifactUploader(false, "test", []string{"json"}, "", false, false)

	metadata := map[string]interface{}{
		"project": "test",
		"version": "1.0.0",
	}

	result, err := uploader.Upload(metadata, "test-job")

	if err != nil {
		t.Errorf("Disabled uploader should not return error, got: %v", err)
	}

	if result != nil {
		t.Error("Disabled uploader should return nil result")
	}
}

// TestUpload_JSON tests JSON artifact upload
func TestUpload_JSON(t *testing.T) {
	tmpDir := t.TempDir()

	uploader := NewArtifactUploader(true, "test-metadata", []string{"json"}, tmpDir, false, false)

	metadata := map[string]interface{}{
		"common": map[string]interface{}{
			"project_type":    "python-modern",
			"project_name":    "test-project",
			"project_version": "1.0.0",
		},
	}

	result, err := uploader.Upload(metadata, "test-job")

	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}

	if result == nil {
		t.Fatal("Upload result should not be nil")
	}

	// Check artifact name format
	expectedPrefix := "test-metadata-test-job-"
	if !strings.HasPrefix(result.Name, expectedPrefix) {
		t.Errorf("Expected artifact name to start with %q, got %q", expectedPrefix, result.Name)
	}

	// Check suffix length (should be 4 characters)
	if len(result.Suffix) != 4 {
		t.Errorf("Expected suffix length 4, got %d", len(result.Suffix))
	}

	// Check files were created
	if len(result.Files) != 2 {
		t.Errorf("Expected 2 JSON files (compact and pretty), got %d", len(result.Files))
	}

	// Verify compact JSON file exists and is valid
	compactPath := filepath.Join(result.Path, "metadata.json")
	compactData, err := os.ReadFile(compactPath)
	if err != nil {
		t.Errorf("Failed to read compact JSON: %v", err)
	}

	var compactMeta map[string]interface{}
	if err := json.Unmarshal(compactData, &compactMeta); err != nil {
		t.Errorf("Compact JSON is not valid: %v", err)
	}

	// Verify pretty JSON file exists and is valid
	prettyPath := filepath.Join(result.Path, "metadata-pretty.json")
	prettyData, err := os.ReadFile(prettyPath)
	if err != nil {
		t.Errorf("Failed to read pretty JSON: %v", err)
	}

	var prettyMeta map[string]interface{}
	if err := json.Unmarshal(prettyData, &prettyMeta); err != nil {
		t.Errorf("Pretty JSON is not valid: %v", err)
	}

	// Check that pretty JSON is actually formatted (has newlines)
	if !strings.Contains(string(prettyData), "\n") {
		t.Error("Pretty JSON should contain newlines")
	}
}

// TestUpload_YAML tests YAML artifact upload
func TestUpload_YAML(t *testing.T) {
	tmpDir := t.TempDir()

	uploader := NewArtifactUploader(true, "test-metadata", []string{"yaml"}, tmpDir, false, false)

	metadata := map[string]interface{}{
		"common": map[string]interface{}{
			"project_type":    "go-module",
			"project_name":    "test-go-app",
			"project_version": "v0.1.0",
		},
	}

	result, err := uploader.Upload(metadata, "build")

	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}

	if result == nil {
		t.Fatal("Upload result should not be nil")
	}

	// Check files were created
	if len(result.Files) != 1 {
		t.Errorf("Expected 1 YAML file, got %d", len(result.Files))
	}

	// Verify YAML file exists and is valid
	yamlPath := filepath.Join(result.Path, "metadata.yaml")
	yamlData, err := os.ReadFile(yamlPath)
	if err != nil {
		t.Errorf("Failed to read YAML: %v", err)
	}

	var yamlMeta map[string]interface{}
	if err := yaml.Unmarshal(yamlData, &yamlMeta); err != nil {
		t.Errorf("YAML is not valid: %v", err)
	}

	// Verify content
	if common, ok := yamlMeta["common"].(map[string]interface{}); ok {
		if projectType, ok := common["project_type"].(string); !ok || projectType != "go-module" {
			t.Error("YAML should contain correct project_type")
		}
	}
}

// TestUpload_BothFormats tests uploading both JSON and YAML
func TestUpload_BothFormats(t *testing.T) {
	tmpDir := t.TempDir()

	uploader := NewArtifactUploader(true, "test", []string{"json", "yaml"}, tmpDir, false, false)

	metadata := map[string]interface{}{
		"project": "test",
		"version": "1.0.0",
	}

	result, err := uploader.Upload(metadata, "multi-format")

	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}

	// Should have 3 files total: 2 JSON (compact + pretty) + 1 YAML
	if len(result.Files) != 3 {
		t.Errorf("Expected 3 files (2 JSON + 1 YAML), got %d", len(result.Files))
	}

	// Verify all expected files exist
	expectedFiles := []string{"metadata.json", "metadata-pretty.json", "metadata.yaml"}
	for _, expectedFile := range expectedFiles {
		filePath := filepath.Join(result.Path, expectedFile)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			t.Errorf("Expected file %q does not exist", expectedFile)
		}
	}
}

// TestUpload_UnsupportedFormat tests handling of unsupported formats
func TestUpload_UnsupportedFormat(t *testing.T) {
	tmpDir := t.TempDir()

	uploader := NewArtifactUploader(true, "test", []string{"xml"}, tmpDir, false, false)

	metadata := map[string]interface{}{
		"project": "test",
	}

	_, err := uploader.Upload(metadata, "test-job")

	if err == nil {
		t.Error("Expected error for unsupported format")
	}

	if !strings.Contains(err.Error(), "unsupported artifact format") {
		t.Errorf("Expected 'unsupported artifact format' error, got: %v", err)
	}
}

// TestUpload_WithValidation tests upload with validation enabled
func TestUpload_WithValidation(t *testing.T) {
	tmpDir := t.TempDir()

	uploader := NewArtifactUploader(true, "test", []string{"json"}, tmpDir, true, true)

	metadata := map[string]interface{}{
		"common": map[string]interface{}{
			"project_type":    "python-modern",
			"project_name":    "validated-project",
			"project_version": "2.0.0",
		},
	}

	result, err := uploader.Upload(metadata, "validate-job")

	if err != nil {
		t.Fatalf("Upload with validation failed: %v", err)
	}

	if result == nil {
		t.Fatal("Upload result should not be nil")
	}

	// Verify files exist
	compactPath := filepath.Join(result.Path, "metadata.json")
	if _, err := os.Stat(compactPath); os.IsNotExist(err) {
		t.Error("Validated JSON file should exist")
	}
}

// TestUpload_ComplexMetadata tests upload with complex nested metadata
func TestUpload_ComplexMetadata(t *testing.T) {
	tmpDir := t.TempDir()

	uploader := NewArtifactUploader(true, "complex", []string{"json", "yaml"}, tmpDir, false, false)

	metadata := map[string]interface{}{
		"common": map[string]interface{}{
			"project_type":    "java-maven",
			"project_name":    "complex-app",
			"project_version": "3.0.0-SNAPSHOT",
		},
		"environment": map[string]interface{}{
			"ci": map[string]interface{}{
				"platform":   "github-actions",
				"runner_os":  "Linux",
				"run_number": "123",
			},
			"tools": map[string]string{
				"java":  "17.0.0",
				"maven": "3.9.0",
			},
		},
		"language_specific": map[string]interface{}{
			"group_id":    "com.example",
			"artifact_id": "complex-app",
			"packaging":   "jar",
			"dependencies": []string{
				"junit:junit:4.13.2",
				"org.springframework:spring-core:6.0.0",
			},
		},
	}

	result, err := uploader.Upload(metadata, "complex-build")

	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}

	// Verify JSON content
	jsonPath := filepath.Join(result.Path, "metadata.json")
	jsonData, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("Failed to read JSON: %v", err)
	}

	var parsedJSON map[string]interface{}
	if err := json.Unmarshal(jsonData, &parsedJSON); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	// Verify nested structure
	if common, ok := parsedJSON["common"].(map[string]interface{}); ok {
		if projectName, ok := common["project_name"].(string); !ok || projectName != "complex-app" {
			t.Error("JSON should preserve nested structure")
		}
	} else {
		t.Error("JSON should have 'common' section")
	}

	// Verify YAML content
	yamlPath := filepath.Join(result.Path, "metadata.yaml")
	yamlData, err := os.ReadFile(yamlPath)
	if err != nil {
		t.Fatalf("Failed to read YAML: %v", err)
	}

	var parsedYAML map[string]interface{}
	if err := yaml.Unmarshal(yamlData, &parsedYAML); err != nil {
		t.Fatalf("Failed to parse YAML: %v", err)
	}

	// Verify nested structure in YAML
	if env, ok := parsedYAML["environment"].(map[string]interface{}); ok {
		if tools, ok := env["tools"].(map[string]interface{}); ok {
			if javaVersion, ok := tools["java"].(string); !ok || javaVersion != "17.0.0" {
				t.Error("YAML should preserve nested structure")
			}
		}
	}
}

// TestGenerateSuffix tests suffix generation
func TestGenerateSuffix(t *testing.T) {
	// Generate multiple suffixes
	suffixes := make(map[string]bool)
	for i := 0; i < 100; i++ {
		suffix, err := generateSuffix()
		if err != nil {
			t.Fatalf("generateSuffix failed: %v", err)
		}

		// Check length
		if len(suffix) != 4 {
			t.Errorf("Expected suffix length 4, got %d", len(suffix))
		}

		// Check charset (alphanumeric lowercase)
		for _, c := range suffix {
			if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
				t.Errorf("Suffix contains invalid character: %c", c)
			}
		}

		suffixes[suffix] = true
	}

	// Check that we got some variety (at least 80 unique suffixes out of 100)
	if len(suffixes) < 80 {
		t.Errorf("Expected at least 80 unique suffixes, got %d", len(suffixes))
	}
}

// TestGetMetadataJSON tests JSON string generation
func TestGetMetadataJSON(t *testing.T) {
	metadata := map[string]interface{}{
		"project": "test-json",
		"version": "1.0.0",
	}

	jsonStr, err := GetMetadataJSON(metadata, false)
	if err != nil {
		t.Fatalf("GetMetadataJSON failed: %v", err)
	}

	if jsonStr == "" {
		t.Error("JSON string should not be empty")
	}

	// Verify it's valid JSON
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		t.Errorf("Generated JSON is not valid: %v", err)
	}

	// Verify content
	if project, ok := parsed["project"].(string); !ok || project != "test-json" {
		t.Error("JSON should contain correct project name")
	}
}

// TestGetMetadataJSON_WithValidation tests JSON generation with validation
func TestGetMetadataJSON_WithValidation(t *testing.T) {
	metadata := map[string]interface{}{
		"common": map[string]interface{}{
			"project_type":    "python-modern",
			"project_name":    "validated",
			"project_version": "1.0.0",
		},
	}

	jsonStr, err := GetMetadataJSON(metadata, true)
	if err != nil {
		t.Fatalf("GetMetadataJSON with validation failed: %v", err)
	}

	if jsonStr == "" {
		t.Error("JSON string should not be empty")
	}

	// Verify valid JSON
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		t.Errorf("Generated JSON is not valid: %v", err)
	}
}

// TestGetMetadataYAML tests YAML string generation
func TestGetMetadataYAML(t *testing.T) {
	metadata := map[string]interface{}{
		"project": "test-yaml",
		"version": "2.0.0",
	}

	yamlStr, err := GetMetadataYAML(metadata, false)
	if err != nil {
		t.Fatalf("GetMetadataYAML failed: %v", err)
	}

	if yamlStr == "" {
		t.Error("YAML string should not be empty")
	}

	// Verify it's valid YAML
	var parsed map[string]interface{}
	if err := yaml.Unmarshal([]byte(yamlStr), &parsed); err != nil {
		t.Errorf("Generated YAML is not valid: %v", err)
	}

	// Verify content
	if project, ok := parsed["project"].(string); !ok || project != "test-yaml" {
		t.Error("YAML should contain correct project name")
	}
}

// TestGetMetadataYAML_WithValidation tests YAML generation with validation
func TestGetMetadataYAML_WithValidation(t *testing.T) {
	metadata := map[string]interface{}{
		"common": map[string]interface{}{
			"project_type":    "go-module",
			"project_name":    "go-validated",
			"project_version": "v0.5.0",
		},
	}

	yamlStr, err := GetMetadataYAML(metadata, true)
	if err != nil {
		t.Fatalf("GetMetadataYAML with validation failed: %v", err)
	}

	if yamlStr == "" {
		t.Error("YAML string should not be empty")
	}

	// Verify valid YAML
	var parsed map[string]interface{}
	if err := yaml.Unmarshal([]byte(yamlStr), &parsed); err != nil {
		t.Errorf("Generated YAML is not valid: %v", err)
	}
}

// TestUpload_SpecialCharacters tests handling special characters in metadata
func TestUpload_SpecialCharacters(t *testing.T) {
	tmpDir := t.TempDir()

	uploader := NewArtifactUploader(true, "test", []string{"json", "yaml"}, tmpDir, false, false)

	metadata := map[string]interface{}{
		"common": map[string]interface{}{
			"project_name": "test-项目-🚀",
			"description":  "A test with 特殊字符 and émojis 🎉",
			"version":      "1.0.0-beta+build.123",
		},
	}

	result, err := uploader.Upload(metadata, "unicode-test")

	if err != nil {
		t.Fatalf("Upload with special characters failed: %v", err)
	}

	// Verify JSON preserves Unicode
	jsonPath := filepath.Join(result.Path, "metadata.json")
	jsonData, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("Failed to read JSON: %v", err)
	}

	var parsedJSON map[string]interface{}
	if err := json.Unmarshal(jsonData, &parsedJSON); err != nil {
		t.Fatalf("Failed to parse JSON with Unicode: %v", err)
	}

	// Verify YAML preserves Unicode
	yamlPath := filepath.Join(result.Path, "metadata.yaml")
	yamlData, err := os.ReadFile(yamlPath)
	if err != nil {
		t.Fatalf("Failed to read YAML: %v", err)
	}

	var parsedYAML map[string]interface{}
	if err := yaml.Unmarshal(yamlData, &parsedYAML); err != nil {
		t.Fatalf("Failed to parse YAML with Unicode: %v", err)
	}
}

// TestUpload_EmptyMetadata tests handling empty metadata
func TestUpload_EmptyMetadata(t *testing.T) {
	tmpDir := t.TempDir()

	uploader := NewArtifactUploader(true, "test", []string{"json"}, tmpDir, false, false)

	metadata := map[string]interface{}{}

	result, err := uploader.Upload(metadata, "empty-test")

	if err != nil {
		t.Fatalf("Upload with empty metadata failed: %v", err)
	}

	// Should still create files
	if len(result.Files) != 2 {
		t.Errorf("Expected 2 files even with empty metadata, got %d", len(result.Files))
	}

	// Verify JSON is valid (should be {})
	jsonPath := filepath.Join(result.Path, "metadata.json")
	jsonData, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("Failed to read JSON: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(jsonData, &parsed); err != nil {
		t.Errorf("Empty metadata JSON is not valid: %v", err)
	}
}

// TestUpload_DirectoryCreation tests that directories are created properly
func TestUpload_DirectoryCreation(t *testing.T) {
	tmpDir := t.TempDir()
	nestedDir := filepath.Join(tmpDir, "nested", "deep", "path")

	uploader := NewArtifactUploader(true, "test", []string{"json"}, nestedDir, false, false)

	metadata := map[string]interface{}{
		"test": "directory creation",
	}

	result, err := uploader.Upload(metadata, "dir-test")

	if err != nil {
		t.Fatalf("Upload with nested directory failed: %v", err)
	}

	// Verify directory was created
	if _, err := os.Stat(result.Path); os.IsNotExist(err) {
		t.Error("Artifact directory should have been created")
	}

	// Verify it's a subdirectory of the nested path
	if !strings.HasPrefix(result.Path, nestedDir) {
		t.Errorf("Expected artifact path to be under %q, got %q", nestedDir, result.Path)
	}
}

// TestUpload_MultipleJobs tests uploading from multiple jobs
func TestUpload_MultipleJobs(t *testing.T) {
	tmpDir := t.TempDir()

	uploader := NewArtifactUploader(true, "shared", []string{"json"}, tmpDir, false, false)

	jobs := []string{"job1", "job2", "job3"}
	artifactNames := make(map[string]bool)

	for _, job := range jobs {
		metadata := map[string]interface{}{
			"job": job,
		}

		result, err := uploader.Upload(metadata, job)
		if err != nil {
			t.Fatalf("Upload for %s failed: %v", job, err)
		}

		// Check uniqueness
		if artifactNames[result.Name] {
			t.Errorf("Duplicate artifact name: %s", result.Name)
		}
		artifactNames[result.Name] = true

		// Verify artifact directory exists
		if _, err := os.Stat(result.Path); os.IsNotExist(err) {
			t.Errorf("Artifact directory for %s should exist", job)
		}
	}

	// Should have 3 unique artifacts
	if len(artifactNames) != 3 {
		t.Errorf("Expected 3 unique artifacts, got %d", len(artifactNames))
	}
}

// TestArtifactResult tests ArtifactResult structure
func TestArtifactResult(t *testing.T) {
	result := &ArtifactResult{
		Path:   "/tmp/test-artifact",
		Suffix: "abc1",
		Name:   "test-artifact-job-abc1",
		Files:  []string{"metadata.json", "metadata.yaml"},
	}

	if result.Path != "/tmp/test-artifact" {
		t.Error("Path should be set correctly")
	}

	if result.Suffix != "abc1" {
		t.Error("Suffix should be set correctly")
	}

	if result.Name != "test-artifact-job-abc1" {
		t.Error("Name should be set correctly")
	}

	if len(result.Files) != 2 {
		t.Error("Files should contain 2 items")
	}
}

// TestContains tests the contains helper function
func TestContains(t *testing.T) {
	tests := []struct {
		name     string
		slice    []string
		str      string
		expected bool
	}{
		{
			name:     "found in slice",
			slice:    []string{"a", "b", "c"},
			str:      "b",
			expected: true,
		},
		{
			name:     "not found in slice",
			slice:    []string{"a", "b", "c"},
			str:      "d",
			expected: false,
		},
		{
			name:     "empty slice",
			slice:    []string{},
			str:      "a",
			expected: false,
		},
		{
			name:     "nil slice",
			slice:    nil,
			str:      "a",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := contains(tt.slice, tt.str)
			if result != tt.expected {
				t.Errorf("contains(%v, %q) = %v, want %v", tt.slice, tt.str, result, tt.expected)
			}
		})
	}
}
