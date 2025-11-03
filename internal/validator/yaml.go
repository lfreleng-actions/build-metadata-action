// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package validator

import (
	"fmt"
	"reflect"

	"gopkg.in/yaml.v3"
)

// YAMLValidator validates YAML data
type YAMLValidator struct {
	StrictMode bool
}

// NewYAMLValidator creates a new YAML validator
func NewYAMLValidator(strictMode bool) *YAMLValidator {
	return &YAMLValidator{
		StrictMode: strictMode,
	}
}

// Validate checks if the provided YAML bytes are valid
func (v *YAMLValidator) Validate(data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("YAML data is empty")
	}

	// Syntax validation
	var obj interface{}
	if err := yaml.Unmarshal(data, &obj); err != nil {
		return fmt.Errorf("invalid YAML syntax: %w", err)
	}

	// Round-trip test to ensure no data loss
	marshaled, err := yaml.Marshal(obj)
	if err != nil {
		return fmt.Errorf("YAML marshal failed during validation: %w", err)
	}

	var roundTrip interface{}
	if err := yaml.Unmarshal(marshaled, &roundTrip); err != nil {
		return fmt.Errorf("YAML round-trip validation failed: %w", err)
	}

	// In strict mode, ensure round-trip produces equivalent structure
	if v.StrictMode {
		if !reflect.DeepEqual(obj, roundTrip) {
			return fmt.Errorf("YAML round-trip produced different structure")
		}
	}

	return nil
}

// MarshalAndValidate marshals data to YAML and validates it
func (v *YAMLValidator) MarshalAndValidate(data interface{}) ([]byte, error) {
	// Marshal to YAML
	yamlBytes, err := yaml.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("YAML marshal failed: %w", err)
	}

	// Validate
	if err := v.Validate(yamlBytes); err != nil {
		return nil, err
	}

	return yamlBytes, nil
}

// ValidateString validates a YAML string
func (v *YAMLValidator) ValidateString(yamlStr string) error {
	return v.Validate([]byte(yamlStr))
}

// IsValid checks if data is valid YAML without returning an error
func (v *YAMLValidator) IsValid(data []byte) bool {
	return v.Validate(data) == nil
}

// ValidateWithSchema validates YAML against a schema (placeholder for future enhancement)
func (v *YAMLValidator) ValidateWithSchema(data []byte, schema interface{}) error {
	// First validate basic YAML syntax
	if err := v.Validate(data); err != nil {
		return err
	}

	// TODO: Implement schema validation using a YAML schema library
	// This is a placeholder for future enhancement
	return nil
}

// NormalizeYAML unmarshals and remarshals YAML to produce consistent formatting
func (v *YAMLValidator) NormalizeYAML(data []byte) ([]byte, error) {
	var obj interface{}
	if err := yaml.Unmarshal(data, &obj); err != nil {
		return nil, fmt.Errorf("failed to unmarshal YAML: %w", err)
	}

	normalized, err := yaml.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal normalized YAML: %w", err)
	}

	return normalized, nil
}
