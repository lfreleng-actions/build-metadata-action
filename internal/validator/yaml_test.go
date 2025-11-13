// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package validator

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestNewYAMLValidator(t *testing.T) {
	tests := []struct {
		name       string
		strictMode bool
	}{
		{
			name:       "strict mode enabled",
			strictMode: true,
		},
		{
			name:       "strict mode disabled",
			strictMode: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewYAMLValidator(tt.strictMode)
			if v == nil {
				t.Fatal("NewYAMLValidator returned nil")
			}
			if v.StrictMode != tt.strictMode {
				t.Errorf("StrictMode = %v, want %v", v.StrictMode, tt.strictMode)
			}
		})
	}
}

func TestYAMLValidator_Validate(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{
			name: "valid simple YAML",
			data: []byte(`name: test
value: 123`),
			wantErr: false,
		},
		{
			name: "valid nested YAML",
			data: []byte(`user:
  name: Alice
  age: 30
active: true`),
			wantErr: false,
		},
		{
			name: "valid array YAML",
			data: []byte(`- 1
- 2
- 3
- 4
- 5`),
			wantErr: false,
		},
		{
			name:    "valid empty object",
			data:    []byte(`{}`),
			wantErr: false,
		},
		{
			name:    "valid empty array",
			data:    []byte(`[]`),
			wantErr: false,
		},
		{
			name:    "valid null",
			data:    []byte(`null`),
			wantErr: false,
		},
		{
			name:    "valid string",
			data:    []byte(`"hello"`),
			wantErr: false,
		},
		{
			name:    "valid number",
			data:    []byte(`42`),
			wantErr: false,
		},
		{
			name:    "valid boolean",
			data:    []byte(`true`),
			wantErr: false,
		},
		{
			name: "valid multiline string",
			data: []byte(`description: |
  This is a
  multiline string`),
			wantErr: false,
		},
		{
			name: "valid with comments",
			data: []byte(`# This is a comment
name: test
# Another comment
value: 123`),
			wantErr: false,
		},
		{
			name: "invalid YAML - bad indentation",
			data: []byte(`name: test
 value: 123`),
			wantErr: true,
		},
		{
			name:    "empty data",
			data:    []byte(``),
			wantErr: true,
		},
		{
			name:    "nil data",
			data:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewYAMLValidator(false)
			err := v.Validate(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestYAMLValidator_ValidateStrictMode(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		strict  bool
		wantErr bool
	}{
		{
			name: "strict mode - valid data",
			data: []byte(`name: test
value: 123`),
			strict:  true,
			wantErr: false,
		},
		{
			name: "non-strict mode - valid data",
			data: []byte(`name: test
value: 123`),
			strict:  false,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewYAMLValidator(tt.strict)
			err := v.Validate(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestYAMLValidator_MarshalAndValidate(t *testing.T) {
	tests := []struct {
		name    string
		data    interface{}
		wantErr bool
	}{
		{
			name: "valid struct-like data",
			data: map[string]interface{}{
				"name":    "test",
				"version": "1.0.0",
				"count":   42,
			},
			wantErr: false,
		},
		{
			name:    "valid array",
			data:    []string{"a", "b", "c"},
			wantErr: false,
		},
		{
			name:    "valid string",
			data:    "hello",
			wantErr: false,
		},
		{
			name:    "valid number",
			data:    123,
			wantErr: false,
		},
		{
			name:    "valid boolean",
			data:    true,
			wantErr: false,
		},
		{
			name:    "valid null",
			data:    nil,
			wantErr: false,
		},
		{
			name: "complex nested structure",
			data: map[string]interface{}{
				"metadata": map[string]interface{}{
					"version": "1.0.0",
					"build": map[string]interface{}{
						"timestamp": "2025-01-03T12:00:00Z",
						"number":    42,
					},
				},
				"dependencies": []string{"dep1", "dep2"},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewYAMLValidator(false)
			yamlBytes, err := v.MarshalAndValidate(tt.data)

			if (err != nil) != tt.wantErr {
				t.Errorf("MarshalAndValidate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			// Verify it's valid YAML
			var result interface{}
			if err := yaml.Unmarshal(yamlBytes, &result); err != nil {
				t.Errorf("produced YAML is invalid: %v", err)
			}

			// Verify it can be re-marshaled
			_, err = yaml.Marshal(result)
			if err != nil {
				t.Errorf("failed to remarshal: %v", err)
			}
		})
	}
}

func TestYAMLValidator_ValidateString(t *testing.T) {
	tests := []struct {
		name    string
		yamlStr string
		wantErr bool
	}{
		{
			name:    "valid YAML string",
			yamlStr: "name: test\nvalue: 123",
			wantErr: false,
		},
		{
			name:    "valid array string",
			yamlStr: "- 1\n- 2\n- 3",
			wantErr: false,
		},
		{
			name:    "invalid YAML string",
			yamlStr: "name: test\n value: 123",
			wantErr: true,
		},
		{
			name:    "empty string",
			yamlStr: "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewYAMLValidator(false)
			err := v.ValidateString(tt.yamlStr)

			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateString() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestYAMLValidator_IsValid(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{
			name: "valid YAML",
			data: []byte("name: test"),
			want: true,
		},
		{
			name: "invalid YAML",
			data: []byte("name: test\n value: 123"),
			want: false,
		},
		{
			name: "empty data",
			data: []byte(""),
			want: false,
		},
		{
			name: "nil data",
			data: nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewYAMLValidator(false)
			got := v.IsValid(tt.data)

			if got != tt.want {
				t.Errorf("IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestYAMLValidator_ValidateWithSchema(t *testing.T) {
	// Test the placeholder schema validation function
	data := []byte("name: test\nvalue: 123")

	v := NewYAMLValidator(false)
	err := v.ValidateWithSchema(data, nil)

	// Should validate basic YAML syntax even with nil schema
	if err != nil {
		t.Errorf("ValidateWithSchema() unexpected error = %v", err)
	}

	// Test with invalid YAML
	invalidData := []byte("name: test\n value: 123")
	err = v.ValidateWithSchema(invalidData, nil)
	if err == nil {
		t.Error("ValidateWithSchema() expected error for invalid YAML")
	}
}

func TestYAMLValidator_NormalizeYAML(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
		check   func(*testing.T, []byte)
	}{
		{
			name:    "normalize simple YAML",
			data:    []byte("name:   test\nvalue:    123"),
			wantErr: false,
			check: func(t *testing.T, normalized []byte) {
				var obj interface{}
				if err := yaml.Unmarshal(normalized, &obj); err != nil {
					t.Errorf("normalized YAML is invalid: %v", err)
				}
			},
		},
		{
			name:    "normalize with extra whitespace",
			data:    []byte("name:  test  \nvalue:  123  "),
			wantErr: false,
			check: func(t *testing.T, normalized []byte) {
				if len(normalized) == 0 {
					t.Error("normalized YAML is empty")
				}
			},
		},
		{
			name:    "normalize array",
			data:    []byte("- 1\n-   2\n-    3"),
			wantErr: false,
			check: func(t *testing.T, normalized []byte) {
				var arr []int
				if err := yaml.Unmarshal(normalized, &arr); err != nil {
					t.Errorf("normalized array is invalid: %v", err)
				}
				if len(arr) != 3 {
					t.Errorf("array length = %d, want 3", len(arr))
				}
			},
		},
		{
			name:    "invalid YAML",
			data:    []byte("name: test\n value: 123"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewYAMLValidator(false)
			normalized, err := v.NormalizeYAML(tt.data)

			if (err != nil) != tt.wantErr {
				t.Errorf("NormalizeYAML() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			if tt.check != nil {
				tt.check(t, normalized)
			}
		})
	}
}

func TestYAMLValidator_ComplexMetadata(t *testing.T) {
	// Test with realistic build metadata structure
	metadata := map[string]interface{}{
		"common": map[string]interface{}{
			"project_type":    "python-modern",
			"project_name":    "example-project",
			"project_version": "1.2.3",
			"build_timestamp": "2025-01-03T12:00:00Z",
		},
		"language_specific": map[string]interface{}{
			"package_name":    "example_project",
			"requires_python": ">=3.8",
			"build_backend":   "poetry",
		},
		"environment": map[string]interface{}{
			"ci": map[string]interface{}{
				"platform":  "github",
				"runner_os": "Linux",
				"is_ci":     true,
			},
			"tools": map[string]string{
				"python": "3.9.7",
				"git":    "2.34.1",
			},
		},
		"dependencies": []string{"requests", "pytest", "black"},
	}

	v := NewYAMLValidator(true)
	yamlBytes, err := v.MarshalAndValidate(metadata)
	if err != nil {
		t.Fatalf("MarshalAndValidate() error = %v", err)
	}

	// Verify YAML is valid
	var result interface{}
	if err := yaml.Unmarshal(yamlBytes, &result); err != nil {
		t.Errorf("YAML is invalid: %v", err)
	}

	// Verify it's actually YAML format (contains newlines and colons)
	yamlStr := string(yamlBytes)
	if !strings.Contains(yamlStr, "\n") {
		t.Error("YAML does not contain newlines")
	}
	if !strings.Contains(yamlStr, ":") {
		t.Error("YAML does not contain colons")
	}
}

func TestYAMLValidator_RoundTrip(t *testing.T) {
	// Test that data survives round-trip marshaling
	original := map[string]interface{}{
		"string":  "test",
		"number":  42,
		"float":   3.14,
		"boolean": true,
		"null":    nil,
		"array":   []interface{}{1, 2, 3},
		"object": map[string]interface{}{
			"nested": "value",
		},
	}

	v := NewYAMLValidator(true)
	yamlBytes, err := v.MarshalAndValidate(original)
	if err != nil {
		t.Fatalf("MarshalAndValidate() error = %v", err)
	}

	var decoded map[string]interface{}
	if err := yaml.Unmarshal(yamlBytes, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	// Verify key fields exist and have correct types
	if decoded["string"] != "test" {
		t.Errorf("string value mismatch")
	}
	if decoded["boolean"] != true {
		t.Errorf("boolean value mismatch")
	}
	if decoded["null"] != nil {
		t.Errorf("null value should be nil")
	}
}

func TestYAMLValidator_ErrorMessages(t *testing.T) {
	v := NewYAMLValidator(false)

	tests := []struct {
		name        string
		operation   func() error
		wantErrText string
	}{
		{
			name: "empty data error",
			operation: func() error {
				return v.Validate([]byte{})
			},
			wantErrText: "empty",
		},
		{
			name: "invalid syntax error",
			operation: func() error {
				return v.Validate([]byte("name: test\n value: 123"))
			},
			wantErrText: "invalid YAML syntax",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.operation()
			if err == nil {
				t.Error("expected error, got nil")
				return
			}
			if !strings.Contains(err.Error(), tt.wantErrText) {
				t.Errorf("error message %q does not contain %q", err.Error(), tt.wantErrText)
			}
		})
	}
}

func TestYAMLValidator_SpecialCharacters(t *testing.T) {
	// Test YAML with special characters
	data := map[string]interface{}{
		"with-dashes":      "value",
		"with_underscores": "value",
		"with:colons":      "value",
		"with spaces":      "value",
		"with\"quotes\"":   "value",
	}

	v := NewYAMLValidator(false)
	yamlBytes, err := v.MarshalAndValidate(data)
	if err != nil {
		t.Fatalf("MarshalAndValidate() error = %v", err)
	}

	// Verify it can be unmarshaled back
	var result map[string]interface{}
	if err := yaml.Unmarshal(yamlBytes, &result); err != nil {
		t.Errorf("failed to unmarshal YAML with special characters: %v", err)
	}
}

func TestYAMLValidator_UnicodeSupport(t *testing.T) {
	// Test YAML with Unicode characters
	data := map[string]interface{}{
		"emoji":    "🎉",
		"chinese":  "你好",
		"japanese": "こんにちは",
		"arabic":   "مرحبا",
	}

	v := NewYAMLValidator(false)
	yamlBytes, err := v.MarshalAndValidate(data)
	if err != nil {
		t.Fatalf("MarshalAndValidate() error = %v", err)
	}

	// Verify it can be unmarshaled back
	var result map[string]interface{}
	if err := yaml.Unmarshal(yamlBytes, &result); err != nil {
		t.Errorf("failed to unmarshal YAML with Unicode: %v", err)
	}

	// Verify Unicode is preserved
	if result["emoji"] != "🎉" {
		t.Error("emoji not preserved")
	}
}
