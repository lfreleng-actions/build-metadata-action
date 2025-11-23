// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package output

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// TestGenerateSummary_BasicMetadata tests summary generation with basic metadata
func TestGenerateSummary_BasicMetadata(t *testing.T) {
	metadata := map[string]interface{}{
		"common": map[string]interface{}{
			"project_type":    "python-modern",
			"project_name":    "example-project",
			"project_version": "1.0.0",
		},
	}

	summary := GenerateSummary(metadata)

	// Check header
	if !strings.Contains(summary, "🐍 Python Build") && !strings.Contains(summary, "🔧 Build Metadata") {
		t.Error("Summary should contain header")
	}

	// Check project information (now includes repository info when available)
	if !strings.Contains(summary, "### Project Information") {
		t.Error("Summary should contain project information section")
	}

	if !strings.Contains(summary, "Python (Modern)") {
		t.Error("Summary should contain formatted project type")
	}

	if !strings.Contains(summary, "example-project") {
		t.Error("Summary should contain project name")
	}

	if !strings.Contains(summary, "1.0.0") {
		t.Error("Summary should contain version")
	}

	// Check success indicator
	if !strings.Contains(summary, "✅ Metadata extraction successful") {
		t.Error("Summary should contain success indicator")
	}
}

// TestGenerateSummary_CompleteMetadata tests summary with all fields
func TestGenerateSummary_CompleteMetadata(t *testing.T) {
	buildTime := time.Date(2025, 1, 3, 12, 0, 0, 0, time.UTC)

	metadata := map[string]interface{}{
		"common": map[string]interface{}{
			"project_type":    "python-modern",
			"project_name":    "complete-project",
			"project_version": "2.0.0",
			"version_source":  "pyproject.toml",
			"versioning_type": "static",
			"build_timestamp": buildTime,
			"git_sha":         "abc123def456789012345678901234567890abcd",
			"git_branch":      "main",
			"git_tag":         "v2.0.0",
		},
		"environment": map[string]interface{}{
			"ci": map[string]interface{}{
				"platform":          "github-actions",
				"runner_os":         "Linux",
				"runner_arch":       "X64",
				"github_workflow":   "CI",
				"github_run_number": "42",
			},
			"tools": map[string]string{
				"python3": "3.11.0",
				"pip":     "23.0.1",
				"git":     "2.40.0",
			},
		},
		"language_specific": map[string]interface{}{
			"package_name":    "complete_project",
			"requires_python": ">=3.8",
			"build_backend":   "hatchling.build",
			"metadata_source": "pyproject.toml",
		},
	}

	summary := GenerateSummary(metadata)

	// Verify main section exists
	if !strings.Contains(summary, "### Project Information") {
		t.Error("Should contain project information")
	}

	// CI Environment and Language-Specific sections should NOT exist (consolidated into Project Information)
	if strings.Contains(summary, "## CI Environment") {
		t.Error("Should NOT contain separate CI Environment section")
	}

	if strings.Contains(summary, "## Tool Versions") {
		t.Error("Should NOT contain separate Tool Versions section")
	}

	if strings.Contains(summary, "## Language-Specific Metadata") {
		t.Error("Should NOT contain separate Language-Specific Metadata section")
	}

	// Verify specific content
	if !strings.Contains(summary, "complete-project") {
		t.Error("Should contain project name")
	}

	if !strings.Contains(summary, "2.0.0") {
		t.Error("Should contain version")
	}

	if !strings.Contains(summary, "pyproject.toml") {
		t.Error("Should contain version source")
	}

	if !strings.Contains(summary, "main") {
		t.Error("Should contain git branch")
	}

	if !strings.Contains(summary, "v2.0.0") {
		t.Error("Should contain git tag")
	}

	// CI environment details should NOT be in output anymore
	if strings.Contains(summary, "github-actions") {
		t.Error("Should NOT contain CI platform in output")
	}

	if strings.Contains(summary, "Runner OS") {
		t.Error("Should NOT contain runner OS in output")
	}

	if strings.Contains(summary, "Runner Arch") {
		t.Error("Should NOT contain runner arch in output")
	}

	// Check relevant tools are listed (Python-specific)
	if !strings.Contains(summary, "3.11.0") {
		t.Error("Should contain Python version in Project Information")
	}

	// Language-specific metadata should be in Project Information table
	if !strings.Contains(summary, "complete_project") {
		t.Error("Should contain package name in Project Information")
	}

	if !strings.Contains(summary, ">=3.8") {
		t.Error("Should contain requires_python in Project Information")
	}
}

// TestGenerateSummary_EmptyMetadata tests handling of empty metadata
func TestGenerateSummary_EmptyMetadata(t *testing.T) {
	metadata := map[string]interface{}{}

	summary := GenerateSummary(metadata)

	// Should still generate header and success indicator
	if !strings.Contains(summary, "🔧 Build Metadata") {
		t.Error("Should contain header even with empty metadata")
	}

	if !strings.Contains(summary, "✅ Metadata extraction successful") {
		t.Error("Should contain success indicator")
	}
}

// TestGenerateSummary_NilMetadata tests handling of nil metadata
func TestGenerateSummary_NilMetadata(t *testing.T) {
	summary := GenerateSummary(nil)

	// Should still generate something
	if summary == "" {
		t.Error("Should generate non-empty summary for nil metadata")
	}

	if !strings.Contains(summary, "🔧 Build Metadata") {
		t.Error("Should contain header")
	}
}

// TestGenerateSummary_PythonProject tests Python-specific formatting
func TestGenerateSummary_PythonProject(t *testing.T) {
	metadata := map[string]interface{}{
		"common": map[string]interface{}{
			"project_type":    "python-modern",
			"project_name":    "my-python-pkg",
			"project_version": "1.2.3",
		},
		"language_specific": map[string]interface{}{
			"package_name":    "my_python_pkg",
			"requires_python": ">=3.9",
			"build_backend":   "hatchling.build",
			"metadata_source": "pyproject.toml",
			"matrix_json":     `{"python-version":["3.9","3.10","3.11"]}`,
		},
	}

	summary := GenerateSummary(metadata)

	if !strings.Contains(summary, "### Python Project Details") {
		t.Error("Should contain Python project details section")
	}

	if !strings.Contains(summary, "my_python_pkg") {
		t.Error("Should contain package name")
	}

	if !strings.Contains(summary, ">=3.9") {
		t.Error("Should contain Python version requirement")
	}

	if !strings.Contains(summary, "hatchling.build") {
		t.Error("Should contain build backend")
	}

	if !strings.Contains(summary, "### Build Matrix") {
		t.Error("Should contain build matrix section")
	}

	if !strings.Contains(summary, "```json") {
		t.Error("Should contain JSON code block for matrix")
	}
}

// TestGenerateSummary_JavaMavenProject tests Java Maven-specific formatting
func TestGenerateSummary_JavaMavenProject(t *testing.T) {
	metadata := map[string]interface{}{
		"common": map[string]interface{}{
			"project_type":    "java-maven",
			"project_name":    "my-java-app",
			"project_version": "2.0.0-SNAPSHOT",
		},
		"language_specific": map[string]interface{}{
			"group_id":    "com.example",
			"artifact_id": "my-java-app",
			"packaging":   "jar",
		},
	}

	summary := GenerateSummary(metadata)

	if !strings.Contains(summary, "### Java Project Details") {
		t.Error("Should contain Java project details section")
	}

	if !strings.Contains(summary, "com.example") {
		t.Error("Should contain group ID")
	}

	if !strings.Contains(summary, "my-java-app") {
		t.Error("Should contain artifact ID")
	}

	if !strings.Contains(summary, "jar") {
		t.Error("Should contain packaging type")
	}
}

// TestGenerateSummary_JavaGradleProject tests Java Gradle-specific formatting
func TestGenerateSummary_JavaGradleProject(t *testing.T) {
	metadata := map[string]interface{}{
		"common": map[string]interface{}{
			"project_type":    "java-gradle",
			"project_name":    "gradle-app",
			"project_version": "3.0.0",
		},
		"language_specific": map[string]interface{}{
			"group_id":    "org.example",
			"artifact_id": "gradle-app",
			"packaging":   "war",
		},
	}

	summary := GenerateSummary(metadata)

	if !strings.Contains(summary, "### Java Project Details") {
		t.Error("Should contain Java project details section")
	}

	if !strings.Contains(summary, "org.example") {
		t.Error("Should contain group ID")
	}

	if !strings.Contains(summary, "gradle-app") {
		t.Error("Should contain artifact ID")
	}
}

// TestGenerateSummary_JavaScriptProject tests JavaScript-specific formatting
func TestGenerateSummary_JavaScriptProject(t *testing.T) {
	metadata := map[string]interface{}{
		"common": map[string]interface{}{
			"project_type":    "javascript-npm",
			"project_name":    "my-node-app",
			"project_version": "1.0.0",
		},
		"language_specific": map[string]interface{}{
			"package_manager": "npm",
			"engines": map[string]interface{}{
				"node": ">=18.0.0",
			},
		},
	}

	summary := GenerateSummary(metadata)

	if !strings.Contains(summary, "### Node.js Project Details") {
		t.Errorf("Should contain Node.js project details section\nGot:\n%s", summary)
	}

	if !strings.Contains(summary, "npm") {
		t.Errorf("Should contain package manager\nGot:\n%s", summary)
	}

	if !strings.Contains(summary, ">=18.0.0") {
		t.Errorf("Should contain node version requirement\nGot:\n%s", summary)
	}
}

// TestGenerateSummary_GoProject tests Go-specific formatting
func TestGenerateSummary_GoProject(t *testing.T) {
	metadata := map[string]interface{}{
		"common": map[string]interface{}{
			"project_type":    "go-module",
			"project_name":    "my-go-app",
			"project_version": "v0.1.0",
		},
		"language_specific": map[string]interface{}{
			"module":     "github.com/example/my-go-app",
			"go_version": "1.21",
		},
	}

	summary := GenerateSummary(metadata)

	if !strings.Contains(summary, "### Go Module Details") {
		t.Error("Should contain Go module details section")
	}

	if !strings.Contains(summary, "github.com/example/my-go-app") {
		t.Error("Should contain module path")
	}

	if !strings.Contains(summary, "1.21") {
		t.Error("Should contain Go version")
	}
}

// TestGenerateSummary_RustProject tests Rust-specific formatting
func TestGenerateSummary_RustProject(t *testing.T) {
	metadata := map[string]interface{}{
		"common": map[string]interface{}{
			"project_type":    "rust-cargo",
			"project_name":    "my-rust-crate",
			"project_version": "0.2.0",
		},
		"language_specific": map[string]interface{}{
			"edition": "2021",
		},
	}

	summary := GenerateSummary(metadata)

	if !strings.Contains(summary, "### Rust Crate Details") {
		t.Error("Should contain Rust crate details section")
	}

	if !strings.Contains(summary, "2021") {
		t.Error("Should contain Rust edition")
	}
}

// TestGenerateSummary_DotNetProject tests .NET-specific formatting
func TestGenerateSummary_DotNetProject(t *testing.T) {
	metadata := map[string]interface{}{
		"common": map[string]interface{}{
			"project_type":    "csharp-project",
			"project_name":    "MyApp",
			"project_version": "1.0.0",
		},
		"language_specific": map[string]interface{}{
			"framework": "net8.0",
		},
	}

	summary := GenerateSummary(metadata)

	if !strings.Contains(summary, "### .NET Project Details") {
		t.Error("Should contain .NET project details section")
	}

	if !strings.Contains(summary, "net8.0") {
		t.Error("Should contain target framework")
	}
}

// TestGenerateSummary_DynamicVersioning tests dynamic versioning display
func TestGenerateSummary_DynamicVersioning(t *testing.T) {
	tests := []struct {
		name      string
		isDynamic bool
		expected  string
	}{
		{
			name:      "static versioning",
			isDynamic: false,
			expected:  "static",
		},
		{
			name:      "dynamic versioning",
			isDynamic: true,
			expected:  "dynamic",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metadata := map[string]interface{}{
				"common": map[string]interface{}{
					"project_type":    "python-modern",
					"project_name":    "test",
					"project_version": "1.0.0",
					"versioning_type": func() string {
						if tt.isDynamic {
							return "dynamic"
						}
						return "static"
					}(),
				},
			}

			summary := GenerateSummary(metadata)

			if !strings.Contains(summary, "Versioning Type") {
				t.Error("Should contain versioning type field")
			}

			if !strings.Contains(summary, tt.expected) {
				t.Errorf("Expected versioning type to be %s", tt.expected)
			}
		})
	}
}

// TestGenerateSummary_ToolVersionsSorting tests that tools are sorted alphabetically
func TestGenerateSummary_ToolVersionsSorting(t *testing.T) {
	metadata := map[string]interface{}{
		"common": map[string]interface{}{
			"project_type":    "python-modern",
			"project_name":    "test",
			"project_version": "1.0.0",
		},
		"environment": map[string]interface{}{
			"tools": map[string]string{
				"python3": "3.11.0",
				"pip":     "23.0.1",
				"python":  "3.11.0",
			},
		},
	}

	summary := GenerateSummary(metadata)

	// Find positions of tools in summary
	pipPos := strings.Index(summary, "pip Version")
	python3Pos := strings.Index(summary, "Python 3 Version")

	if pipPos == -1 || python3Pos == -1 {
		t.Errorf("Python-relevant tools should be present in summary\nGot: %s", summary)
	}

	// Check alphabetical ordering (pip comes before python3 alphabetically)
	if !(pipPos < python3Pos) {
		t.Errorf("Tools should be sorted alphabetically\npip position: %d, python3 position: %d\nGot: %s", pipPos, python3Pos, summary)
	}
}

// TestFormatProjectType tests project type formatting
func TestFormatProjectType(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"python-modern", "Python (Modern)"},
		{"python-legacy", "Python (Legacy)"},
		{"javascript-npm", "JavaScript (npm)"},
		{"java-maven", "Java (Maven)"},
		{"java-gradle", "Java (Gradle)"},
		{"java-gradle-kts", "Java (Gradle Kotlin DSL)"},
		{"csharp-project", "C# (.NET Project)"},
		{"csharp-solution", "C# (.NET Solution)"},
		{"go-module", "Go (Module)"},
		{"rust-cargo", "Rust (Cargo)"},
		{"ruby-gemspec", "Ruby (Gem)"},
		{"ruby-bundler", "Ruby (Bundler)"},
		{"unknown-type", "Unknown Type"},
		{"custom-format", "Custom Format"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := formatProjectType(tt.input)
			if result != tt.expected {
				t.Errorf("formatProjectType(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestConvertToMap tests metadata to map conversion
func TestConvertToMap(t *testing.T) {
	type TestStruct struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}

	tests := []struct {
		name     string
		input    interface{}
		expected map[string]interface{}
	}{
		{
			name: "simple struct",
			input: TestStruct{
				Name:    "test",
				Version: "1.0.0",
			},
			expected: map[string]interface{}{
				"name":    "test",
				"version": "1.0.0",
			},
		},
		{
			name:     "map input",
			input:    map[string]interface{}{"key": "value"},
			expected: map[string]interface{}{"key": "value"},
		},
		{
			name:     "nil input",
			input:    nil,
			expected: map[string]interface{}{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertToMap(tt.input)

			if tt.input == nil {
				if len(result) != 0 {
					t.Error("Expected empty map for nil input")
				}
				return
			}

			// Compare keys
			for key, expectedValue := range tt.expected {
				if result[key] != expectedValue {
					t.Errorf("Expected %v for key %s, got %v", expectedValue, key, result[key])
				}
			}
		})
	}
}

// TestSortMapKeys tests map key sorting
func TestSortMapKeys(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]string
		expected []string
	}{
		{
			name:     "already sorted",
			input:    map[string]string{"a": "1", "b": "2", "c": "3"},
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "reverse order",
			input:    map[string]string{"z": "1", "m": "2", "a": "3"},
			expected: []string{"a", "m", "z"},
		},
		{
			name:     "mixed case",
			input:    map[string]string{"Zebra": "1", "alpha": "2", "Beta": "3"},
			expected: []string{"Beta", "Zebra", "alpha"},
		},
		{
			name:     "empty map",
			input:    map[string]string{},
			expected: []string{},
		},
		{
			name:     "single key",
			input:    map[string]string{"only": "one"},
			expected: []string{"only"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sortMapKeys(tt.input)

			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d keys, got %d", len(tt.expected), len(result))
				return
			}

			for i, key := range result {
				if key != tt.expected[i] {
					t.Errorf("At position %d: expected %q, got %q", i, tt.expected[i], key)
				}
			}
		})
	}
}

// TestGenerateMarkdown tests markdown generation
func TestGenerateMarkdown(t *testing.T) {
	metadata := map[string]interface{}{
		"common": map[string]interface{}{
			"project_type":    "python-modern",
			"project_name":    "test-project",
			"project_version": "1.0.0",
		},
	}

	markdown := GenerateMarkdown(metadata)

	// Currently GenerateMarkdown is an alias for GenerateSummary
	// so we just check it produces output
	if markdown == "" {
		t.Error("GenerateMarkdown should produce non-empty output")
	}

	if !strings.Contains(markdown, "# 🔧 Build Metadata") {
		t.Error("Markdown should contain header")
	}
}

// TestGenerateSummary_SpecialCharacters tests handling of special characters
func TestGenerateSummary_SpecialCharacters(t *testing.T) {
	metadata := map[string]interface{}{
		"common": map[string]interface{}{
			"project_type":    "python-modern",
			"project_name":    "test-project-名前-🚀",
			"project_version": "1.0.0-beta.1+build.123",
		},
	}

	summary := GenerateSummary(metadata)

	if !strings.Contains(summary, "test-project-名前-🚀") {
		t.Error("Should handle Unicode characters in project name")
	}

	if !strings.Contains(summary, "1.0.0-beta.1+build.123") {
		t.Error("Should handle complex version strings")
	}
}

// TestGenerateSummary_LongValues tests handling of very long values
func TestGenerateSummary_LongValues(t *testing.T) {
	longName := strings.Repeat("very-long-project-name-", 10)

	metadata := map[string]interface{}{
		"common": map[string]interface{}{
			"project_type":    "python-modern",
			"project_name":    longName,
			"project_version": "1.0.0",
		},
	}

	summary := GenerateSummary(metadata)

	if !strings.Contains(summary, longName) {
		t.Error("Should handle long project names")
	}
}

// TestGenerateSummary_MissingFields tests handling of missing optional fields
func TestGenerateSummary_MissingFields(t *testing.T) {
	metadata := map[string]interface{}{
		"common": map[string]interface{}{
			"project_type": "python-modern",
			// Missing project_name and project_version
		},
	}

	summary := GenerateSummary(metadata)

	// Should still generate valid output
	if !strings.Contains(summary, "# 🔧 Build Metadata") {
		t.Error("Should contain header even with missing fields")
	}

	// Should contain project type
	if !strings.Contains(summary, "Python (Modern)") {
		t.Error("Should contain project type")
	}
}

// TestGenerateSummary_TimestampFormatting tests timestamp formatting
func TestGenerateSummary_TimestampFormatting(t *testing.T) {
	timestamp := time.Date(2025, 1, 3, 15, 30, 45, 0, time.UTC)

	metadata := map[string]interface{}{
		"common": map[string]interface{}{
			"project_type":    "python-modern",
			"project_name":    "test",
			"project_version": "1.0.0",
			"build_timestamp": timestamp,
		},
	}

	summary := GenerateSummary(metadata)

	// Check RFC3339 format
	expectedFormat := "2025-01-03T15:30:45Z"
	if !strings.Contains(summary, expectedFormat) {
		t.Errorf("Should contain timestamp in RFC3339 format: %s", expectedFormat)
	}
}

// TestGenerateSummary_AllProjectTypes tests all supported project types
func TestGenerateSummary_AllProjectTypes(t *testing.T) {
	projectTypes := []string{
		"python-modern",
		"python-legacy",
		"javascript-npm",
		"typescript-npm",
		"java-maven",
		"java-gradle",
		"java-gradle-kts",
		"csharp-project",
		"csharp-solution",
		"go-module",
		"rust-cargo",
		"ruby-gemspec",
		"ruby-bundler",
	}

	for _, projectType := range projectTypes {
		t.Run(projectType, func(t *testing.T) {
			metadata := map[string]interface{}{
				"common": map[string]interface{}{
					"project_type":    projectType,
					"project_name":    "test-project",
					"project_version": "1.0.0",
				},
			}

			summary := GenerateSummary(metadata)

			if summary == "" {
				t.Error("Should generate non-empty summary")
			}

			if !strings.Contains(summary, "# 🔧 Build Metadata") {
				t.Error("Should contain header")
			}
		})
	}
}

// TestGenerateSummary_JSONMarshaling tests that metadata can be JSON marshaled
func TestGenerateSummary_JSONMarshaling(t *testing.T) {
	// Create metadata with various types
	metadata := map[string]interface{}{
		"common": map[string]interface{}{
			"project_type":    "python-modern",
			"project_name":    "test",
			"project_version": "1.0.0",
			"versioning_type": "dynamic",
			"build_timestamp": time.Now(),
		},
	}

	// Ensure it can be marshaled to JSON (used in convertToMap)
	jsonBytes, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("Failed to marshal metadata: %v", err)
	}

	// Ensure it can be unmarshaled back
	var result map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		t.Fatalf("Failed to unmarshal metadata: %v", err)
	}

	// Generate summary with the result
	summary := GenerateSummary(result)

	if summary == "" {
		t.Error("Should generate non-empty summary from unmarshaled data")
	}
}
