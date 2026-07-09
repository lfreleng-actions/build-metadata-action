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

	// Note: The implementation does not output a success indicator message
	// The presence of the table with project information indicates success
}

// TestGenerateSummary_VersionProperties tests rendering of the
// version.properties, version match and snapshot version rows
func TestGenerateSummary_VersionProperties(t *testing.T) {
	metadata := map[string]interface{}{
		"common": map[string]interface{}{
			"project_type":               "javascript-npm",
			"project_name":               "onap-ui-common",
			"project_version":            "1.1.0",
			"version_source":             "package.json",
			"version_properties_version": "1.1.0",
			"version_properties_match":   "true",
			"snapshot_version":           "1.1.0-SNAPSHOT",
		},
	}

	summary := GenerateSummary(metadata)

	if !strings.Contains(summary, "| version.properties | 1.1.0 |") {
		t.Error("Summary should contain the version.properties row")
	}
	if !strings.Contains(summary, "| Version Match | true ✅ |") {
		t.Error("Summary should contain the version match row")
	}
	if !strings.Contains(summary, "| Snapshot Version | 1.1.0-SNAPSHOT |") {
		t.Error("Summary should contain the snapshot version row")
	}
}

// TestGenerateSummary_VersionPropertiesMismatch tests the mismatch
// rendering and that rows are omitted when version.properties is absent
func TestGenerateSummary_VersionPropertiesMismatch(t *testing.T) {
	metadata := map[string]interface{}{
		"common": map[string]interface{}{
			"project_type":               "javascript-npm",
			"project_name":               "frontend",
			"project_version":            "2.0.0",
			"version_source":             "package.json",
			"version_properties_version": "1.9.0",
			"version_properties_match":   "false",
			"snapshot_version":           "1.9.0-SNAPSHOT",
		},
	}

	summary := GenerateSummary(metadata)

	if !strings.Contains(summary, "| Version Match | false ❌ |") {
		t.Error("Summary should render a mismatch with false ❌")
	}

	absent := map[string]interface{}{
		"common": map[string]interface{}{
			"project_type":    "javascript-npm",
			"project_name":    "frontend",
			"project_version": "2.0.0",
			// No version.properties file: the action still synthesizes
			// snapshot_version from project_version, so the summary
			// omits the version.properties/match rows but keeps the
			// snapshot row
			"snapshot_version": "2.0.0-SNAPSHOT",
		},
	}
	summary = GenerateSummary(absent)
	if strings.Contains(summary, "version.properties") ||
		strings.Contains(summary, "Version Match") {
		t.Error("Summary should omit version.properties rows when absent")
	}
	if !strings.Contains(summary, "| Snapshot Version | 2.0.0-SNAPSHOT |") {
		t.Error("Summary should render the snapshot version synthesized" +
			" from project_version when version.properties is absent")
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
			"build_version":   "3.11",
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
	// Note: python3 is intentionally excluded from summary for Python projects because:
	// 1. The "Build Python" field shows the recommended version from project metadata
	// 2. The detected python3 version is the system Python, not the matrix job's Python
	// 3. build-metadata-action runs BEFORE setup-python, so the detected version is misleading
	// We check for pip instead, and Build Python should show the version from metadata
	if !strings.Contains(summary, "23.0.1") {
		t.Error("Should contain pip version in Project Information")
	}
	if !strings.Contains(summary, "Build Python") {
		t.Error("Should contain Build Python field from language_specific metadata")
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

	// Should still generate header
	if !strings.Contains(summary, "🔧 Build Metadata") {
		t.Error("Should contain header even with empty metadata")
	}

	// Note: The implementation does not output a success indicator message
	// The presence of the header indicates the function executed successfully
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

	// Implementation uses consolidated "Project Information" section
	if !strings.Contains(summary, "### Project Information") {
		t.Error("Should contain Project Information section")
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

	// Matrix JSON is included in the table, not as a separate section
	if !strings.Contains(summary, "python-version") {
		t.Error("Should contain matrix JSON in table")
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

	// Implementation uses consolidated "Project Information" section
	if !strings.Contains(summary, "### Project Information") {
		t.Error("Should contain Project Information section")
	}

	// Note: Java language-specific fields (group_id, artifact_id) are not currently
	// added to the table by addLanguageSpecificToTable for java-maven projects
	// The test verifies the basic project info is present
	if !strings.Contains(summary, "my-java-app") {
		t.Error("Should contain project name")
	}

	if !strings.Contains(summary, "Java (Maven)") {
		t.Error("Should contain formatted project type")
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

	// Implementation uses consolidated "Project Information" section
	if !strings.Contains(summary, "### Project Information") {
		t.Error("Should contain Project Information section")
	}

	if !strings.Contains(summary, "gradle-app") {
		t.Error("Should contain project name")
	}

	if !strings.Contains(summary, "Java (Gradle)") {
		t.Error("Should contain formatted project type")
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
		},
	}

	summary := GenerateSummary(metadata)

	// Implementation uses consolidated "Project Information" section
	if !strings.Contains(summary, "### Project Information") {
		t.Errorf("Should contain Project Information section\nGot:\n%s", summary)
	}

	if !strings.Contains(summary, "npm") {
		t.Errorf("Should contain package manager\nGot:\n%s", summary)
	}

	if !strings.Contains(summary, "JavaScript (npm)") {
		t.Errorf("Should contain formatted project type\nGot:\n%s", summary)
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

	// Implementation uses consolidated "Project Information" section
	if !strings.Contains(summary, "### Project Information") {
		t.Error("Should contain Project Information section")
	}

	if !strings.Contains(summary, "my-go-app") {
		t.Error("Should contain project name")
	}

	if !strings.Contains(summary, "Go (Module)") {
		t.Error("Should contain formatted project type")
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

	// Implementation uses consolidated "Project Information" section
	if !strings.Contains(summary, "### Project Information") {
		t.Error("Should contain Project Information section")
	}

	if !strings.Contains(summary, "my-rust-crate") {
		t.Error("Should contain project name")
	}

	if !strings.Contains(summary, "Rust (Cargo)") {
		t.Error("Should contain formatted project type")
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

	// Implementation uses consolidated "Project Information" section
	if !strings.Contains(summary, "### Project Information") {
		t.Error("Should contain Project Information section")
	}

	if !strings.Contains(summary, "MyApp") {
		t.Error("Should contain project name")
	}

	if !strings.Contains(summary, "C# (.NET Project)") {
		t.Error("Should contain formatted project type")
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

// TestGenerateSummary_ToolVersionsForPython tests that Python projects show pip but not python3
// python3 is excluded because:
// 1. The "Build Python" field already shows the recommended version from project metadata
// 2. The detected python3 version is the system Python, not the matrix job's Python
// 3. build-metadata-action runs BEFORE setup-python, so the detected version is misleading
func TestGenerateSummary_ToolVersionsForPython(t *testing.T) {
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

	// pip should be present
	pipPos := strings.Index(summary, "pip Version")
	if pipPos == -1 {
		t.Errorf("pip Version should be present in summary\nGot: %s", summary)
	}

	// python3 should NOT be present (it's misleading for matrix jobs)
	python3Pos := strings.Index(summary, "Python 3 Version")
	if python3Pos != -1 {
		t.Errorf("Python 3 Version should NOT be present in summary for Python projects (misleading for matrix jobs)\nGot: %s", summary)
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

	// Check timestamp format (implementation uses "2006-01-02 15:04:05 UTC" format)
	expectedFormat := "2025-01-03 15:30:45 UTC"
	if !strings.Contains(summary, expectedFormat) {
		t.Errorf("Should contain timestamp in expected format: %s\nGot:\n%s", expectedFormat, summary)
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
