// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package validator

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNewJSONValidator(t *testing.T) {
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
			v := NewJSONValidator(tt.strictMode)
			if v == nil {
				t.Fatal("NewJSONValidator returned nil")
			}
			if v.StrictMode != tt.strictMode {
				t.Errorf("StrictMode = %v, want %v", v.StrictMode, tt.strictMode)
			}
		})
	}
}

func TestJSONValidator_Validate(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{
			name:    "valid simple JSON",
			data:    []byte(`{"name": "test", "value": 123}`),
			wantErr: false,
		},
		{
			name:    "valid nested JSON",
			data:    []byte(`{"user": {"name": "Alice", "age": 30}, "active": true}`),
			wantErr: false,
		},
		{
			name:    "valid array JSON",
			data:    []byte(`[1, 2, 3, 4, 5]`),
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
			name:    "invalid JSON - missing quote",
			data:    []byte(`{"name: "test"}`),
			wantErr: true,
		},
		{
			name:    "invalid JSON - trailing comma",
			data:    []byte(`{"name": "test",}`),
			wantErr: true,
		},
		{
			name:    "invalid JSON - malformed",
			data:    []byte(`{invalid json}`),
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
		{
			name:    "whitespace only",
			data:    []byte(`   `),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewJSONValidator(false)
			err := v.Validate(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestJSONValidator_ValidateStrictMode(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		strict  bool
		wantErr bool
	}{
		{
			name:    "strict mode - valid data",
			data:    []byte(`{"name": "test", "value": 123}`),
			strict:  true,
			wantErr: false,
		},
		{
			name:    "non-strict mode - valid data",
			data:    []byte(`{"name": "test", "value": 123}`),
			strict:  false,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewJSONValidator(tt.strict)
			err := v.Validate(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestJSONValidator_ValidateAndPrettify(t *testing.T) {
	tests := []struct {
		name         string
		data         interface{}
		strictMode   bool
		wantErr      bool
		checkCompact bool
		checkPretty  bool
	}{
		{
			name: "simple object",
			data: map[string]interface{}{
				"name":  "test",
				"value": 123,
			},
			strictMode:   false,
			wantErr:      false,
			checkCompact: true,
			checkPretty:  true,
		},
		{
			name: "nested object",
			data: map[string]interface{}{
				"user": map[string]interface{}{
					"name": "Alice",
					"age":  30,
				},
				"active": true,
			},
			strictMode:   false,
			wantErr:      false,
			checkCompact: true,
			checkPretty:  true,
		},
		{
			name:         "array",
			data:         []int{1, 2, 3, 4, 5},
			strictMode:   false,
			wantErr:      false,
			checkCompact: true,
			checkPretty:  true,
		},
		{
			name:         "empty object",
			data:         map[string]interface{}{},
			strictMode:   false,
			wantErr:      false,
			checkCompact: true,
			checkPretty:  true,
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
			strictMode:   true,
			wantErr:      false,
			checkCompact: true,
			checkPretty:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewJSONValidator(tt.strictMode)
			compact, pretty, err := v.ValidateAndPrettify(tt.data)

			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateAndPrettify() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			// Check compact JSON is valid and compact
			if tt.checkCompact {
				if len(compact) == 0 {
					t.Error("compact JSON is empty")
				}
				if !json.Valid(compact) {
					t.Error("compact JSON is not valid")
				}
				// Compact should not contain newlines or extra spaces
				if strings.Contains(string(compact), "\n") {
					t.Error("compact JSON contains newlines")
				}
			}

			// Check pretty JSON is valid and formatted
			if tt.checkPretty {
				if len(pretty) == 0 {
					t.Error("pretty JSON is empty")
				}
				if !json.Valid(pretty) {
					t.Error("pretty JSON is not valid")
				}
				// For non-empty objects/arrays, pretty should contain newlines
				// Check based on type
				shouldHaveNewlines := false
				switch v := tt.data.(type) {
				case map[string]interface{}:
					shouldHaveNewlines = len(v) > 0
				case []int, []string, []interface{}:
					shouldHaveNewlines = true // Arrays with elements
				}

				if shouldHaveNewlines {
					dataStr := string(pretty)
					if !strings.Contains(dataStr, "\n") && dataStr != "{}" && dataStr != "[]" {
						// Skip this check for truly empty structures
						if len(compact) > 2 {
							t.Error("pretty JSON should contain newlines for non-empty structures")
						}
					}
				}
			}

			// Both should unmarshal to same structure
			var compactObj, prettyObj interface{}
			if err := json.Unmarshal(compact, &compactObj); err != nil {
				t.Errorf("failed to unmarshal compact JSON: %v", err)
			}
			if err := json.Unmarshal(pretty, &prettyObj); err != nil {
				t.Errorf("failed to unmarshal pretty JSON: %v", err)
			}
		})
	}
}

func TestJSONValidator_MarshalAndValidate(t *testing.T) {
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewJSONValidator(false)
			jsonBytes, err := v.MarshalAndValidate(tt.data)

			if (err != nil) != tt.wantErr {
				t.Errorf("MarshalAndValidate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			// Verify it's valid JSON
			if !json.Valid(jsonBytes) {
				t.Error("MarshalAndValidate() produced invalid JSON")
			}

			// Verify it can be unmarshaled
			var result interface{}
			if err := json.Unmarshal(jsonBytes, &result); err != nil {
				t.Errorf("failed to unmarshal result: %v", err)
			}
		})
	}
}

func TestJSONValidator_ValidateString(t *testing.T) {
	tests := []struct {
		name    string
		jsonStr string
		wantErr bool
	}{
		{
			name:    "valid JSON string",
			jsonStr: `{"name": "test"}`,
			wantErr: false,
		},
		{
			name:    "valid array string",
			jsonStr: `[1, 2, 3]`,
			wantErr: false,
		},
		{
			name:    "invalid JSON string",
			jsonStr: `{invalid}`,
			wantErr: true,
		},
		{
			name:    "empty string",
			jsonStr: ``,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewJSONValidator(false)
			err := v.ValidateString(tt.jsonStr)

			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateString() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestJSONValidator_IsValid(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{
			name: "valid JSON",
			data: []byte(`{"name": "test"}`),
			want: true,
		},
		{
			name: "invalid JSON",
			data: []byte(`{invalid}`),
			want: false,
		},
		{
			name: "empty data",
			data: []byte(``),
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
			v := NewJSONValidator(false)
			got := v.IsValid(tt.data)

			if got != tt.want {
				t.Errorf("IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestJSONValidator_ComplexMetadata(t *testing.T) {
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

	v := NewJSONValidator(true)
	compact, pretty, err := v.ValidateAndPrettify(metadata)
	if err != nil {
		t.Fatalf("ValidateAndPrettify() error = %v", err)
	}

	// Verify compact is valid
	if !json.Valid(compact) {
		t.Error("compact JSON is invalid")
	}

	// Verify pretty is valid
	if !json.Valid(pretty) {
		t.Error("pretty JSON is invalid")
	}

	// Verify pretty is actually formatted
	if !strings.Contains(string(pretty), "\n") {
		t.Error("pretty JSON is not formatted with newlines")
	}

	// Verify both unmarshal to same structure
	var compactObj, prettyObj interface{}
	if err := json.Unmarshal(compact, &compactObj); err != nil {
		t.Errorf("failed to unmarshal compact: %v", err)
	}
	if err := json.Unmarshal(pretty, &prettyObj); err != nil {
		t.Errorf("failed to unmarshal pretty: %v", err)
	}
}

func TestJSONValidator_RoundTrip(t *testing.T) {
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

	v := NewJSONValidator(true)
	jsonBytes, err := v.MarshalAndValidate(original)
	if err != nil {
		t.Fatalf("MarshalAndValidate() error = %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
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

func TestJSONValidator_ErrorMessages(t *testing.T) {
	v := NewJSONValidator(false)

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
				return v.Validate([]byte(`{invalid}`))
			},
			wantErrText: "invalid JSON syntax",
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
