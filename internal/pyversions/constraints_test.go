// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package pyversions

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseConstraints(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    []Constraint
		shouldError bool
	}{
		{
			name:  "basic >= constraint",
			input: ">=3.10",
			expected: []Constraint{
				{Operator: ">=", Version: "3.10"},
			},
		},
		{
			name:  "complex constraint with both bounds",
			input: ">=3.10,<3.14",
			expected: []Constraint{
				{Operator: ">=", Version: "3.10"},
				{Operator: "<", Version: "3.14"},
			},
		},
		{
			name:  "exact version constraint",
			input: "==3.11",
			expected: []Constraint{
				{Operator: "==", Version: "3.11"},
			},
		},
		{
			name:  "exclusion constraint",
			input: "!=3.11",
			expected: []Constraint{
				{Operator: "!=", Version: "3.11"},
			},
		},
		{
			name:  "compatible release",
			input: "~=3.10",
			expected: []Constraint{
				{Operator: ">=", Version: "3.10"},
				{Operator: "<", Version: "3.11"},
			},
		},
		{
			name:  "poetry caret",
			input: "^3.10",
			expected: []Constraint{
				{Operator: ">=", Version: "3.10"},
				{Operator: "<", Version: "4.0"},
			},
		},
		{
			name:  "wildcard constraint",
			input: "==3.10.*",
			expected: []Constraint{
				{Operator: ">=", Version: "3.10"},
				{Operator: "<", Version: "3.11"},
			},
		},
		{
			name:  "constraint with patch version",
			input: ">=3.10.1",
			expected: []Constraint{
				{Operator: ">=", Version: "3.10"},
			},
		},
		{
			name:  "greater than constraint",
			input: ">3.10",
			expected: []Constraint{
				{Operator: ">", Version: "3.10"},
			},
		},
		{
			name:  "less than or equal constraint",
			input: "<=3.12",
			expected: []Constraint{
				{Operator: "<=", Version: "3.12"},
			},
		},
		{
			name:  "multiple constraints with spaces",
			input: ">= 3.10 , < 3.13",
			expected: []Constraint{
				{Operator: ">=", Version: "3.10"},
				{Operator: "<", Version: "3.13"},
			},
		},
		{
			name:        "empty constraint",
			input:       "",
			shouldError: true,
		},
		{
			name:        "invalid constraint format",
			input:       "invalid",
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseConstraints(tt.input)

			if tt.shouldError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestNormalizeConstraint(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "poetry caret",
			input:    "^3.10",
			expected: ">=3.10,<4.0",
		},
		{
			name:     "poetry caret with patch",
			input:    "^3.10.1",
			expected: ">=3.10,<4.0",
		},
		{
			name:     "compatible release",
			input:    "~=3.10",
			expected: ">=3.10,<3.11",
		},
		{
			name:     "compatible release with patch",
			input:    "~=3.10.1",
			expected: ">=3.10,<3.11",
		},
		{
			name:     "wildcard constraint",
			input:    "==3.10.*",
			expected: ">=3.10,<3.11",
		},
		{
			name:     "strip patch from bounds",
			input:    "<3.13.0",
			expected: "<3.13",
		},
		{
			name:     "strip patch from multiple bounds",
			input:    ">=3.10.5,<3.13.0",
			expected: ">=3.10,<3.13",
		},
		{
			name:     "exclusion unchanged",
			input:    "!=3.11",
			expected: "!=3.11",
		},
		{
			name:     "basic constraint unchanged",
			input:    ">=3.10",
			expected: ">=3.10",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeConstraint(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		v1       string
		v2       string
		expected int
	}{
		{"3.10", "3.9", 1},
		{"3.9", "3.10", -1},
		{"3.10", "3.10", 0},
		{"3.11", "3.10", 1},
		{"3.10", "3.11", -1},
		{"3.13", "3.9", 1},
		{"4.0", "3.13", 1},
		{"3.13", "4.0", -1},
	}

	for _, tt := range tests {
		t.Run(tt.v1+" vs "+tt.v2, func(t *testing.T) {
			result := compareVersions(tt.v1, tt.v2)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMatchesConstraint(t *testing.T) {
	tests := []struct {
		version    string
		constraint Constraint
		expected   bool
	}{
		{"3.10", Constraint{Operator: ">=", Version: "3.10"}, true},
		{"3.11", Constraint{Operator: ">=", Version: "3.10"}, true},
		{"3.9", Constraint{Operator: ">=", Version: "3.10"}, false},
		{"3.10", Constraint{Operator: ">", Version: "3.10"}, false},
		{"3.11", Constraint{Operator: ">", Version: "3.10"}, true},
		{"3.10", Constraint{Operator: "<=", Version: "3.10"}, true},
		{"3.11", Constraint{Operator: "<=", Version: "3.10"}, false},
		{"3.10", Constraint{Operator: "<", Version: "3.11"}, true},
		{"3.11", Constraint{Operator: "<", Version: "3.11"}, false},
		{"3.10", Constraint{Operator: "==", Version: "3.10"}, true},
		{"3.11", Constraint{Operator: "==", Version: "3.10"}, false},
		{"3.11", Constraint{Operator: "!=", Version: "3.10"}, true},
		{"3.10", Constraint{Operator: "!=", Version: "3.10"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.version+" "+tt.constraint.Operator+" "+tt.constraint.Version, func(t *testing.T) {
			result := matchesConstraint(tt.version, tt.constraint)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFilterVersions(t *testing.T) {
	allVersions := []string{"3.9", "3.10", "3.11", "3.12", "3.13"}

	tests := []struct {
		name        string
		versions    []string
		constraints []Constraint
		expected    []string
	}{
		{
			name:     "basic >= constraint",
			versions: allVersions,
			constraints: []Constraint{
				{Operator: ">=", Version: "3.10"},
			},
			expected: []string{"3.10", "3.11", "3.12", "3.13"},
		},
		{
			name:     "complex constraint",
			versions: allVersions,
			constraints: []Constraint{
				{Operator: ">=", Version: "3.10"},
				{Operator: "<", Version: "3.13"},
			},
			expected: []string{"3.10", "3.11", "3.12"},
		},
		{
			name:     "exact version",
			versions: allVersions,
			constraints: []Constraint{
				{Operator: "==", Version: "3.11"},
			},
			expected: []string{"3.11"},
		},
		{
			name:     "exclusion",
			versions: allVersions,
			constraints: []Constraint{
				{Operator: ">=", Version: "3.10"},
				{Operator: "!=", Version: "3.11"},
			},
			expected: []string{"3.10", "3.12", "3.13"},
		},
		{
			name:     "greater than",
			versions: allVersions,
			constraints: []Constraint{
				{Operator: ">", Version: "3.10"},
			},
			expected: []string{"3.11", "3.12", "3.13"},
		},
		{
			name:        "no constraints",
			versions:    allVersions,
			constraints: []Constraint{},
			expected:    allVersions,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := FilterVersions(tt.versions, tt.constraints)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestResolveVersions(t *testing.T) {
	supportedVersions := []string{"3.9", "3.10", "3.11", "3.12", "3.13"}

	tests := []struct {
		name           string
		requiresPython string
		supported      []string
		expected       []string
		shouldError    bool
	}{
		{
			name:           "basic >=3.10",
			requiresPython: ">=3.10",
			supported:      supportedVersions,
			expected:       []string{"3.10", "3.11", "3.12", "3.13"},
		},
		{
			name:           "complex >=3.10,<3.14",
			requiresPython: ">=3.10,<3.14",
			supported:      supportedVersions,
			expected:       []string{"3.10", "3.11", "3.12", "3.13"},
		},
		{
			name:           "exact ==3.11",
			requiresPython: "==3.11",
			supported:      supportedVersions,
			expected:       []string{"3.11"},
		},
		{
			name:           "greater than >3.10",
			requiresPython: ">3.10",
			supported:      supportedVersions,
			expected:       []string{"3.11", "3.12", "3.13"},
		},
		{
			name:           "compatible release ~=3.10",
			requiresPython: "~=3.10",
			supported:      supportedVersions,
			expected:       []string{"3.10"},
		},
		{
			name:           "poetry caret ^3.10",
			requiresPython: "^3.10",
			supported:      supportedVersions,
			expected:       []string{"3.10", "3.11", "3.12", "3.13"},
		},
		{
			name:           "wildcard ==3.11.*",
			requiresPython: "==3.11.*",
			supported:      supportedVersions,
			expected:       []string{"3.11"},
		},
		{
			name:           "strict >=3.12",
			requiresPython: ">=3.12",
			supported:      supportedVersions,
			expected:       []string{"3.12", "3.13"},
		},
		{
			name:           "inclusive upper bound >=3.10,<=3.12",
			requiresPython: ">=3.10,<=3.12",
			supported:      supportedVersions,
			expected:       []string{"3.10", "3.11", "3.12"},
		},
		{
			name:           "empty constraint",
			requiresPython: "",
			supported:      supportedVersions,
			shouldError:    true,
		},
		{
			name:           "no supported versions",
			requiresPython: ">=3.10",
			supported:      []string{},
			shouldError:    true,
		},
		{
			name:           "no matching versions",
			requiresPython: ">=4.0",
			supported:      supportedVersions,
			shouldError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ResolveVersions(tt.requiresPython, tt.supported)

			if tt.shouldError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestStripPatchVersion(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"3.10.1", "3.10"},
		{"3.10.0", "3.10"},
		{"3.11.5", "3.11"},
		{"3.10", "3.10"},
		{"4.0.0", "4.0"},
		{"3", "3"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := stripPatchVersion(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHasSameMajorVersion(t *testing.T) {
	tests := []struct {
		v1       string
		v2       string
		expected bool
	}{
		{"3.10", "3.11", true},
		{"3.10", "3.9", true},
		{"3.10", "4.0", false},
		{"4.0", "3.13", false},
		{"3.11", "3.11", true},
	}

	for _, tt := range tests {
		t.Run(tt.v1+" vs "+tt.v2, func(t *testing.T) {
			result := hasSameMajorVersion(tt.v1, tt.v2)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHasSameMinorVersion(t *testing.T) {
	tests := []struct {
		v1       string
		v2       string
		expected bool
	}{
		{"3.10", "3.10", true},
		{"3.10", "3.11", false},
		{"3.11", "3.10", false},
		{"4.0", "3.0", false},
		{"3.9", "3.9", true},
	}

	for _, tt := range tests {
		t.Run(tt.v1+" vs "+tt.v2, func(t *testing.T) {
			result := hasSameMinorVersion(tt.v1, tt.v2)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Integration test that mimics real-world scenarios from python-supported-versions-action fixtures
func TestIntegrationScenarios(t *testing.T) {
	// Simulated supported versions from API (current as of 2025)
	supportedVersions := []string{"3.9", "3.10", "3.11", "3.12", "3.13", "3.14"}

	tests := []struct {
		name           string
		requiresPython string
		expectedCount  int
		expectedMin    string
		expectedMax    string
	}{
		{
			name:           "pyproject_requires_python_basic",
			requiresPython: ">=3.10",
			expectedCount:  5,
			expectedMin:    "3.10",
			expectedMax:    "3.14",
		},
		{
			name:           "pyproject_complex_constraint",
			requiresPython: ">=3.10,<3.14",
			expectedCount:  4,
			expectedMin:    "3.10",
			expectedMax:    "3.13",
		},
		{
			name:           "pyproject_requires_python_exact",
			requiresPython: "==3.11",
			expectedCount:  1,
			expectedMin:    "3.11",
			expectedMax:    "3.11",
		},
		{
			name:           "pyproject_requires_python_greater",
			requiresPython: ">3.10",
			expectedCount:  4,
			expectedMin:    "3.11",
			expectedMax:    "3.14",
		},
		{
			name:           "pyproject_requires_python_strict",
			requiresPython: ">=3.12",
			expectedCount:  3,
			expectedMin:    "3.12",
			expectedMax:    "3.14",
		},
		{
			name:           "pyproject_inclusive_constraint",
			requiresPython: ">=3.10,<=3.12",
			expectedCount:  3,
			expectedMin:    "3.10",
			expectedMax:    "3.12",
		},
		{
			name:           "pyproject_unsupported_constraint (compatible release)",
			requiresPython: "~=3.10.0",
			expectedCount:  1,
			expectedMin:    "3.10",
			expectedMax:    "3.10",
		},
		{
			name:           "pyproject_poetry_caret",
			requiresPython: "^3.10",
			expectedCount:  5,
			expectedMin:    "3.10",
			expectedMax:    "3.14",
		},
		{
			name:           "pyproject_poetry_compatible",
			requiresPython: "~=3.11",
			expectedCount:  1,
			expectedMin:    "3.11",
			expectedMax:    "3.11",
		},
		{
			name:           "pyproject_poetry_exact",
			requiresPython: "==3.12",
			expectedCount:  1,
			expectedMin:    "3.12",
			expectedMax:    "3.12",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ResolveVersions(tt.requiresPython, supportedVersions)
			require.NoError(t, err)

			assert.Len(t, result, tt.expectedCount, "version count mismatch")

			if len(result) > 0 {
				assert.Equal(t, tt.expectedMin, result[0], "minimum version mismatch")
				assert.Equal(t, tt.expectedMax, result[len(result)-1], "maximum version mismatch")
			}
		})
	}
}
