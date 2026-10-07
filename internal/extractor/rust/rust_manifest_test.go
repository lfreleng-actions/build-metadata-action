// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package rust

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func extractManifest(t *testing.T, manifest string) map[string]interface{} {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	metadata, err := NewExtractor().Extract(dir)
	if err != nil {
		t.Fatalf("Extract() failed on a valid manifest: %v", err)
	}
	return metadata.LanguageSpecific
}

const inheritingWorkspace = `
[workspace]
members = []

[workspace.package]
documentation = "https://docs.example.com"
license-file = "LICENSE.txt"
readme = "README.workspace.md"
publish = ["internal"]
`

// Cargo accepts these forms; each used to abort the whole extraction
// with a type error, losing every rust_* output for the project.
func TestManifestFieldForms(t *testing.T) {
	tests := []struct {
		name      string
		pkgFields string
		want      map[string]interface{}
		absent    []string
	}{
		{
			name:      "build script disabled",
			pkgFields: `build = false`,
			absent:    []string{"has_build_script", "build_script"},
		},
		{
			name:      "build script path",
			pkgFields: `build = "tools/build.rs"`,
			want: map[string]interface{}{
				"has_build_script": true,
				"build_script":     "tools/build.rs",
			},
		},
		{
			name: "fields inherited from the workspace",
			pkgFields: `documentation.workspace = true
license-file.workspace = true
readme.workspace = true
publish.workspace = true`,
			want: map[string]interface{}{
				"documentation": "https://docs.example.com",
				"license_file":  "LICENSE.txt",
				"readme":        "README.workspace.md",
				"publish":       []interface{}{"internal"},
			},
		},
		{
			name: "fields set on the package",
			pkgFields: `documentation = "https://docs.rs/demo"
license-file = "COPYING"
readme = "README.md"
publish = false`,
			want: map[string]interface{}{
				"documentation": "https://docs.rs/demo",
				"license_file":  "COPYING",
				"readme":        "README.md",
				"publish":       false,
			},
		},
		{
			name:      "readme detection disabled",
			pkgFields: `readme = false`,
			absent:    []string{"readme"},
		},
		{
			// Cargo inherits only through the marker, verified with
			// cargo 1.99 metadata.
			name:   "workspace fields without the inheritance marker",
			absent: []string{"documentation", "license_file", "readme", "publish"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest := "[package]\nname = \"demo\"\nversion = \"0.1.0\"\n" +
				tt.pkgFields + "\n" + inheritingWorkspace
			got := extractManifest(t, manifest)
			for key, want := range tt.want {
				if !reflect.DeepEqual(got[key], want) {
					t.Errorf("%s = %#v, want %#v", key, got[key], want)
				}
			}
			for _, key := range tt.absent {
				if value, ok := got[key]; ok {
					t.Errorf("%s = %#v, want it absent", key, value)
				}
			}
		})
	}
}

// An inherited field the workspace does not define stays absent rather
// than leaking the raw {workspace = true} marker into the outputs.
func TestInheritedPublishWithoutWorkspaceValue(t *testing.T) {
	got := extractManifest(t, `[package]
name = "demo"
version = "0.1.0"
publish.workspace = true

[workspace]
members = []
`)
	if value, ok := got["publish"]; ok {
		t.Errorf("publish = %#v, want it absent", value)
	}
}
