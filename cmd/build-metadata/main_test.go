// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package main

import "testing"

// TestVersionPropertiesMatch locks in the comparator semantics: a
// "true"/"false" string when both sides are present, ignoring one
// trailing -SNAPSHOT on the project version, and "" (not comparable)
// when either side is empty.
func TestVersionPropertiesMatch(t *testing.T) {
	tests := []struct {
		name           string
		propsVersion   string
		projectVersion string
		want           string
	}{
		{
			name:           "matching versions",
			propsVersion:   "1.1.0",
			projectVersion: "1.1.0",
			want:           "true",
		},
		{
			name:           "mismatched versions",
			propsVersion:   "1.9.0",
			projectVersion: "2.0.0",
			want:           "false",
		},
		{
			name:           "development SNAPSHOT matches release",
			propsVersion:   "3.8.6",
			projectVersion: "3.8.6-SNAPSHOT",
			want:           "true",
		},
		{
			name:           "differing SNAPSHOT does not match",
			propsVersion:   "3.8.6",
			projectVersion: "3.9.0-SNAPSHOT",
			want:           "false",
		},
		{
			name:           "identical SNAPSHOT versions still match",
			propsVersion:   "3.8.6-SNAPSHOT",
			projectVersion: "3.8.6-SNAPSHOT",
			want:           "true",
		},
		{
			name:           "only one SNAPSHOT suffix is removed",
			propsVersion:   "3.8.6",
			projectVersion: "3.8.6-SNAPSHOT-SNAPSHOT",
			want:           "false",
		},
		{
			name:           "SNAPSHOT in version.properties is not stripped",
			propsVersion:   "3.8.6-SNAPSHOT",
			projectVersion: "3.8.6",
			want:           "false",
		},
		{
			name:           "no version.properties version",
			propsVersion:   "",
			projectVersion: "1.0.0",
			want:           "",
		},
		{
			name:           "no project version",
			propsVersion:   "1.0.0",
			projectVersion: "",
			want:           "",
		},
		{
			name:           "both empty",
			propsVersion:   "",
			projectVersion: "",
			want:           "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := versionPropertiesMatch(tt.propsVersion, tt.projectVersion)
			if got != tt.want {
				t.Errorf("versionPropertiesMatch(%q, %q) = %q, want %q",
					tt.propsVersion, tt.projectVersion, got, tt.want)
			}
		})
	}
}

// TestSynthesizeSnapshotVersion locks in the synthesis rules:
// version.properties is authoritative over the project version, a
// base already carrying the (case-insensitive) suffix passes through
// unchanged, and no base yields no snapshot.
func TestSynthesizeSnapshotVersion(t *testing.T) {
	tests := []struct {
		name           string
		propsVersion   string
		projectVersion string
		want           string
	}{
		{
			name:           "version.properties wins over project version",
			propsVersion:   "1.1.0",
			projectVersion: "2.0.0",
			want:           "1.1.0-SNAPSHOT",
		},
		{
			name:           "falls back to project version",
			propsVersion:   "",
			projectVersion: "0.2.0",
			want:           "0.2.0-SNAPSHOT",
		},
		{
			name:           "existing SNAPSHOT suffix passes through",
			propsVersion:   "3.0.1-SNAPSHOT",
			projectVersion: "",
			want:           "3.0.1-SNAPSHOT",
		},
		{
			name:           "lowercase snapshot suffix not double-appended",
			propsVersion:   "",
			projectVersion: "3.0.1-snapshot",
			want:           "3.0.1-snapshot",
		},
		{
			name:           "no versions yields no snapshot",
			propsVersion:   "",
			projectVersion: "",
			want:           "",
		},
		{
			name:           "pre-release suffix still gains SNAPSHOT",
			propsVersion:   "1.2.3-rc1",
			projectVersion: "",
			want:           "1.2.3-rc1-SNAPSHOT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := synthesizeSnapshotVersion(tt.propsVersion, tt.projectVersion)
			if got != tt.want {
				t.Errorf("synthesizeSnapshotVersion(%q, %q) = %q, want %q",
					tt.propsVersion, tt.projectVersion, got, tt.want)
			}
		})
	}
}
