// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package validator

import (
	"encoding/json"
	"fmt"
	"reflect"
)

// JSONValidator validates JSON data
type JSONValidator struct {
	StrictMode bool
}

// NewJSONValidator creates a new JSON validator
func NewJSONValidator(strictMode bool) *JSONValidator {
	return &JSONValidator{
		StrictMode: strictMode,
	}
}

// Validate checks if the provided JSON bytes are valid
func (v *JSONValidator) Validate(data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("JSON data is empty")
	}

	// Syntax validation
	var obj interface{}
	if err := json.Unmarshal(data, &obj); err != nil {
		return fmt.Errorf("invalid JSON syntax: %w", err)
	}

	// Round-trip test to ensure no data loss
	marshaled, err := json.Marshal(obj)
	if err != nil {
		return fmt.Errorf("JSON marshal failed during validation: %w", err)
	}

	var roundTrip interface{}
	if err := json.Unmarshal(marshaled, &roundTrip); err != nil {
		return fmt.Errorf("JSON round-trip validation failed: %w", err)
	}

	// In strict mode, ensure round-trip produces equivalent structure
	if v.StrictMode {
		if !reflect.DeepEqual(obj, roundTrip) {
			return fmt.Errorf("JSON round-trip produced different structure")
		}
	}

	return nil
}

// ValidateAndPrettify validates data and returns both compact and pretty-printed JSON
func (v *JSONValidator) ValidateAndPrettify(data interface{}) (compact, pretty []byte, err error) {
	// Generate compact JSON
	compact, err = json.Marshal(data)
	if err != nil {
		return nil, nil, fmt.Errorf("compact JSON marshal failed: %w", err)
	}

	// Validate compact
	if err := v.Validate(compact); err != nil {
		return nil, nil, fmt.Errorf("compact JSON validation failed: %w", err)
	}

	// Generate pretty JSON
	pretty, err = json.MarshalIndent(data, "", "  ")
	if err != nil {
		return nil, nil, fmt.Errorf("pretty JSON marshal failed: %w", err)
	}

	// Validate pretty
	if err := v.Validate(pretty); err != nil {
		return nil, nil, fmt.Errorf("pretty JSON validation failed: %w", err)
	}

	return compact, pretty, nil
}

// MarshalAndValidate marshals data to JSON and validates it
func (v *JSONValidator) MarshalAndValidate(data interface{}) ([]byte, error) {
	// Marshal to JSON
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("JSON marshal failed: %w", err)
	}

	// Validate
	if err := v.Validate(jsonBytes); err != nil {
		return nil, err
	}

	return jsonBytes, nil
}

// ValidateString validates a JSON string
func (v *JSONValidator) ValidateString(jsonStr string) error {
	return v.Validate([]byte(jsonStr))
}

// IsValid checks if data is valid JSON without returning an error
func (v *JSONValidator) IsValid(data []byte) bool {
	return v.Validate(data) == nil
}
