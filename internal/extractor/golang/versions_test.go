// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package golang

import (
	"reflect"
	"testing"

	"github.com/lfreleng-actions/build-metadata-action/internal/goversions"
)

// TestDefaultSupportedVersions verifies the package default is the
// static goversions fallback list, so `go test` and offline invocations
// never touch the network.
func TestDefaultSupportedVersions(t *testing.T) {
	SetSupportedVersions(nil)
	if !reflect.DeepEqual(activeSupportedVersions, goversions.GetFallbackVersions()) {
		t.Errorf("default supported set = %v, expected goversions fallback %v",
			activeSupportedVersions, goversions.GetFallbackVersions())
	}
}

// TestSetSupportedVersions verifies override and reset semantics.
func TestSetSupportedVersions(t *testing.T) {
	defer SetSupportedVersions(nil)

	custom := []string{"1.30", "1.31"}
	SetSupportedVersions(custom)
	if !reflect.DeepEqual(activeSupportedVersions, custom) {
		t.Errorf("supported set = %v, expected %v", activeSupportedVersions, custom)
	}

	// nil resets to the fallback list
	SetSupportedVersions(nil)
	if !reflect.DeepEqual(activeSupportedVersions, goversions.GetFallbackVersions()) {
		t.Errorf("supported set after reset = %v, expected goversions fallback %v",
			activeSupportedVersions, goversions.GetFallbackVersions())
	}

	// empty slice also resets to the fallback list
	SetSupportedVersions([]string{"1.30"})
	SetSupportedVersions([]string{})
	if !reflect.DeepEqual(activeSupportedVersions, goversions.GetFallbackVersions()) {
		t.Errorf("supported set after empty reset = %v, expected goversions fallback %v",
			activeSupportedVersions, goversions.GetFallbackVersions())
	}
}

// TestGenerateGoVersionMatrix exercises the matrix filter against an
// injected supported set for determinism.
func TestGenerateGoVersionMatrix(t *testing.T) {
	SetSupportedVersions([]string{"1.25", "1.26"})
	defer SetSupportedVersions(nil)

	tests := []struct {
		name      string
		goVersion string
		expected  []string
	}{
		{
			name:      "minimum below supported set returns full set",
			goVersion: "1.20",
			expected:  []string{"1.25", "1.26"},
		},
		{
			name:      "minimum at baseline returns full set",
			goVersion: "1.25",
			expected:  []string{"1.25", "1.26"},
		},
		{
			name:      "minimum at newest returns single entry",
			goVersion: "1.26",
			expected:  []string{"1.26"},
		},
		{
			name:      "patch component is normalized to major.minor",
			goVersion: "1.26.3",
			expected:  []string{"1.26"},
		},
		{
			name:      "minimum newer than supported set returns the minimum",
			goVersion: "1.27",
			expected:  []string{"1.27"},
		},
		{
			name:      "patch component on unsupported minimum is normalized",
			goVersion: "1.27.1",
			expected:  []string{"1.27"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matrix := generateGoVersionMatrix(tt.goVersion)
			if !reflect.DeepEqual(matrix, tt.expected) {
				t.Errorf("generateGoVersionMatrix(%q) = %v, expected %v",
					tt.goVersion, matrix, tt.expected)
			}
		})
	}
}

// TestGenerateGoVersionMatrixDefaultSet verifies matrix generation with
// the default (fallback) supported set: an old go.mod minimum widens to
// the full supported set rather than pinning stale/EOL toolchains.
func TestGenerateGoVersionMatrixDefaultSet(t *testing.T) {
	SetSupportedVersions(nil)

	matrix := generateGoVersionMatrix("1.17")
	if !reflect.DeepEqual(matrix, goversions.GetFallbackVersions()) {
		t.Errorf("generateGoVersionMatrix(\"1.17\") = %v, expected fallback %v",
			matrix, goversions.GetFallbackVersions())
	}
}

// TestNormalizeGoVersion verifies major.minor reduction of go.mod
// version directives.
func TestNormalizeGoVersion(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"1.25", "1.25"},
		{"1.25.4", "1.25"},
		{"1.26.0", "1.26"},
		{"2.0", "2.0"},
		{"invalid", "invalid"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := normalizeGoVersion(tt.input); got != tt.expected {
				t.Errorf("normalizeGoVersion(%q) = %q, expected %q",
					tt.input, got, tt.expected)
			}
		})
	}
}

// TestCompareGoVersionStrings verifies the major.minor comparator.
func TestCompareGoVersionStrings(t *testing.T) {
	tests := []struct {
		v1, v2   string
		expected int
	}{
		{"1.25", "1.26", -1},
		{"1.26", "1.25", 1},
		{"1.25", "1.25", 0},
		{"2.0", "1.99", 1},
		{"1.9", "1.10", -1},
	}

	for _, tt := range tests {
		t.Run(tt.v1+" vs "+tt.v2, func(t *testing.T) {
			if got := compareGoVersionStrings(tt.v1, tt.v2); got != tt.expected {
				t.Errorf("compareGoVersionStrings(%q, %q) = %d, expected %d",
					tt.v1, tt.v2, got, tt.expected)
			}
		})
	}
}
