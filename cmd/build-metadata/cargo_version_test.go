// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package main

import "testing"

// TestCargoProjectVersion checks project_version, version_source and
// versioning_type for Cargo projects under both settings of
// use_version_extract. Each fixture is a git repository tagged v9.9.9,
// so "9.9.9, git-tag" is the fallback; with use_version_extract off
// there is no fallback.
func TestCargoProjectVersion(t *testing.T) {
	tests := []struct {
		name  string
		cargo string
		// extract is the result with use_version_extract on; with it
		// off the result is the same, or no version and no source
		// where this names the tag, unless noExtract says otherwise.
		extract   versionResult
		noExtract *versionResult
	}{
		{
			name:    "C1 trailing comment",
			cargo:   "[package]\nname = \"c1\"\nversion = \"1.2.3\" # bump on release\n",
			extract: versionResult{"1.2.3", "Cargo.toml", "static"},
		},
		{
			name: "C2 inline workspace table",
			cargo: `[package]
name = "c2"
version = { workspace = true }

[workspace]

[workspace.package]
version = "1.2.3"
`,
			extract: versionResult{"1.2.3", "Cargo.toml", "static"},
		},
		{
			name: "C3 dotted workspace key above workspace.package",
			cargo: `[package]
name = "c3"
version.workspace = true

[workspace]

[workspace.package]
version = "1.2.3"
`,
			extract: versionResult{"1.2.3", "Cargo.toml", "static"},
		},
		{
			name:    "C4 comment on the table header",
			cargo:   "[package] # the crate\nname = \"c4\"\nversion = \"1.2.3\"\n",
			extract: versionResult{"1.2.3", "Cargo.toml", "static"},
		},
		{
			name:    "C5 quoted key",
			cargo:   "[package]\nname = \"c5\"\n\"version\" = \"1.2.3\"\n",
			extract: versionResult{"1.2.3", "Cargo.toml", "static"},
		},
		{
			name:    "C6 virtual workspace",
			cargo:   "[workspace]\nmembers = []\n\n[workspace.package]\nversion = \"1.2.3\"\n",
			extract: versionResult{"1.2.3", "Cargo.toml", "static"},
		},
		{
			name:    "C7 placeholder 0.0.0",
			cargo:   "[package]\nname = \"c7\"\nversion = \"0.0.0\"\n",
			extract: versionResult{"0.0.0", "Cargo.toml", "dynamic"},
		},
		{
			name:    "placeholder 0.1.0-dev",
			cargo:   "[package]\nname = \"dev\"\nversion = \"0.1.0-dev\"\n",
			extract: versionResult{"0.1.0-dev", "Cargo.toml", "dynamic"},
		},
		{
			name:    "C8 inherits a version the manifest does not hold",
			cargo:   "[package]\nname = \"c8\"\nversion.workspace = true\n",
			extract: versionResult{"9.9.9", "git-tag", "static"},
		},
		{
			name:    "C9 no version",
			cargo:   "[package]\nname = \"c9\"\n",
			extract: versionResult{"9.9.9", "git-tag", "static"},
		},
		{
			name: "C9 no version beside workspace.package",
			cargo: `[package]
name = "c9"

[workspace]

[workspace.package]
version = "1.2.3"
`,
			extract: versionResult{"9.9.9", "git-tag", "static"},
		},
		{
			name:    "manifest that does not parse",
			cargo:   "[package]\nversion = \"1.2.3\"\nname = \n",
			extract: versionResult{"9.9.9", "git-tag", "static"},
			// The extractor fails, so nothing sets a versioning type.
			noExtract: &versionResult{},
		},
	}

	for _, tt := range tests {
		dir := gitFixture(t, map[string]string{"Cargo.toml": tt.cargo})
		noExtract := tt.extract
		switch {
		case tt.noExtract != nil:
			noExtract = *tt.noExtract
		case tt.extract.source == "git-tag":
			noExtract = versionResult{"", "", tt.extract.versioning}
		}
		for _, useExtract := range []bool{true, false} {
			want := noExtract
			if useExtract {
				want = tt.extract
			}
			if got, _ := resolveProjectVersion(t, dir, "rust-cargo", useExtract); got != want {
				t.Errorf("%s (use_version_extract=%v): got %+v, want %+v",
					tt.name, useExtract, got, want)
			}
		}
	}
}
