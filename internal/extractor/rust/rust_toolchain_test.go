// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package rust

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Repository content marked SECRET must never reach a warning or an
// output: the toolchain file is untrusted input.
const untrusted = "SECRET"

func TestParseToolchainFile(t *testing.T) {
	tests := []struct {
		name     string
		file     string
		content  string
		want     Toolchain
		warnings int
	}{
		{
			name: "legacy file naming a channel", file: "rust-toolchain",
			content: "1.85.0\n",
			want:    Toolchain{Kind: ToolchainChannel, Channel: "1.85.0"},
		},
		{
			name: "legacy file naming an absolute path", file: "rust-toolchain",
			content: "/" + untrusted + "/toolchain\n",
			want:    Toolchain{Kind: ToolchainPath},
		},
		{
			// rustup 1.29.1 reads each of these as one toolchain name.
			name: "legacy file with CRLF", file: "rust-toolchain",
			content: "1.85.0\r\n",
			want:    Toolchain{Kind: ToolchainChannel, Channel: "1.85.0"},
		},
		{
			name: "legacy file without a final newline", file: "rust-toolchain",
			content: "1.85.0",
			want:    Toolchain{Kind: ToolchainChannel, Channel: "1.85.0"},
		},
		{
			name: "legacy file padded with spaces", file: "rust-toolchain",
			content: "  1.85.0  \n",
			want:    Toolchain{Kind: ToolchainChannel, Channel: "1.85.0"},
		},
		{
			// rustup 1.29.1 parses each of these as TOML and rejects it.
			name: "legacy file with a trailing blank line", file: "rust-toolchain",
			content: "1.85.0\n\n",
			want:    Toolchain{Kind: ToolchainNone}, warnings: 1,
		},
		{
			name: "legacy file with a leading blank line", file: "rust-toolchain",
			content: "\n1.85.0\n",
			want:    Toolchain{Kind: ToolchainNone}, warnings: 1,
		},
		{
			name: "legacy file with a trailing blank CRLF line", file: "rust-toolchain",
			content: "1.85.0\r\n\r\n",
			want:    Toolchain{Kind: ToolchainNone}, warnings: 1,
		},
		{
			name: "legacy file with a leading blank CRLF line", file: "rust-toolchain",
			content: "\r\n1.85.0\r\n",
			want:    Toolchain{Kind: ToolchainNone}, warnings: 1,
		},
		{
			name: "legacy file with a trailing whitespace line", file: "rust-toolchain",
			content: "1.85.0\n \n",
			want:    Toolchain{Kind: ToolchainNone}, warnings: 1,
		},
		{
			name: "legacy file holding TOML", file: "rust-toolchain",
			content: "[toolchain]\nchannel = \"nightly-2026-01-01\"\n",
			want:    Toolchain{Kind: ToolchainChannel, Channel: "nightly-2026-01-01"},
		},
		{
			// rustup 1.29.1 rejects the reserved name "none" in either
			// form, and runs "None" as a custom toolchain.
			name: "legacy file naming the reserved toolchain none", file: "rust-toolchain",
			content: "none\n",
			want:    Toolchain{Kind: ToolchainNone}, warnings: 1,
		},
		{
			name: "channel naming the reserved toolchain none", file: "rust-toolchain.toml",
			content: "[toolchain]\nchannel = \"none\"\n",
			want:    Toolchain{Kind: ToolchainNone}, warnings: 1,
		},
		{
			name: "channel naming a custom toolchain None", file: "rust-toolchain.toml",
			content: "[toolchain]\nchannel = \"None\"\n",
			want:    Toolchain{Kind: ToolchainChannel, Channel: "None"},
		},
		{
			name: "every key", file: "rust-toolchain.toml",
			content: `[toolchain]
channel = "stable-x86_64-unknown-linux-gnu"
components = ["rust-src", "llvm-tools"]
targets = ["wasm32-unknown-unknown"]
profile = "minimal"
`,
			want: Toolchain{
				Kind:       ToolchainChannel,
				Channel:    "stable-x86_64-unknown-linux-gnu",
				Components: []string{"rust-src", "llvm-tools"},
				Targets:    []string{"wasm32-unknown-unknown"},
				Profile:    "minimal",
			},
		},
		{
			name: "path key", file: "rust-toolchain.toml",
			content: "[toolchain]\npath = \"/" + untrusted + "/toolchain\"\n",
			want:    Toolchain{Kind: ToolchainPath},
		},
		{
			name: "path key with options", file: "rust-toolchain.toml",
			content:  "[toolchain]\npath = \"/" + untrusted + "/toolchain\"\ncomponents = [\"clippy\"]\n",
			want:     Toolchain{Kind: ToolchainNone},
			warnings: 1,
		},
		{
			name: "path key with empty options", file: "rust-toolchain.toml",
			content:  "[toolchain]\npath = \"/" + untrusted + "\"\ntargets = []\nprofile = \"\"\n",
			want:     Toolchain{Kind: ToolchainNone},
			warnings: 1,
		},
		{
			name: "relative path key", file: "rust-toolchain.toml",
			content:  "[toolchain]\npath = \"../" + untrusted + "\"\n",
			want:     Toolchain{Kind: ToolchainNone},
			warnings: 1,
		},
		{
			name: "channel and path together", file: "rust-toolchain.toml",
			content:  "[toolchain]\nchannel = \"1.85.0\"\npath = \"/" + untrusted + "\"\n",
			want:     Toolchain{Kind: ToolchainNone},
			warnings: 1,
		},
		{
			name: "channel holding a path", file: "rust-toolchain.toml",
			content:  "[toolchain]\nchannel = \"../" + untrusted + "\"\n",
			want:     Toolchain{Kind: ToolchainNone},
			warnings: 1,
		},
		{
			name: "channel holding an absolute path", file: "rust-toolchain.toml",
			content:  "[toolchain]\nchannel = \"/" + untrusted + "\"\n",
			want:     Toolchain{Kind: ToolchainNone},
			warnings: 1,
		},
		{
			name: "legacy file naming a relative path", file: "rust-toolchain",
			content:  "../" + untrusted + "\n",
			want:     Toolchain{Kind: ToolchainNone},
			warnings: 1,
		},
		{
			name: "no channel selects the default toolchain", file: "rust-toolchain.toml",
			content: "[toolchain]\ncomponents = [\"clippy\"]\n",
			want:    Toolchain{Kind: ToolchainNone, Components: []string{"clippy"}},
		},
		{
			name: "no toolchain table", file: "rust-toolchain.toml",
			content:  "# " + untrusted + "\n",
			want:     Toolchain{Kind: ToolchainNone},
			warnings: 1,
		},
		{
			name: "empty toolchain table", file: "rust-toolchain.toml",
			content:  "[toolchain]\n",
			want:     Toolchain{Kind: ToolchainNone},
			warnings: 1,
		},
		{
			name: "profile alone", file: "rust-toolchain.toml",
			content:  "[toolchain]\nprofile = \"minimal\"\n",
			want:     Toolchain{Kind: ToolchainNone},
			warnings: 1,
		},
		{
			name: "empty components list selects the default toolchain", file: "rust-toolchain.toml",
			content: "[toolchain]\ncomponents = []\n",
			want:    Toolchain{Kind: ToolchainNone},
		},
		{
			name: "empty channel", file: "rust-toolchain.toml",
			content:  "[toolchain]\nchannel = \"\"\ncomponents = [\"clippy\"]\n",
			want:     Toolchain{Kind: ToolchainNone},
			warnings: 1,
		},
		{
			name: "unknown profile for a release channel", file: "rust-toolchain.toml",
			content:  "[toolchain]\nchannel = \"1.85.0\"\nprofile = \"" + untrusted + "\"\n",
			want:     Toolchain{Kind: ToolchainNone},
			warnings: 1,
		},
		{
			name: "unknown profile for a named channel", file: "rust-toolchain.toml",
			content:  "[toolchain]\nchannel = \"nightly-2026-01-01\"\nprofile = \"bogus\"\n",
			want:     Toolchain{Kind: ToolchainNone},
			warnings: 1,
		},
		{
			name: "unknown profile without a channel", file: "rust-toolchain.toml",
			content:  "[toolchain]\ncomponents = [\"clippy\"]\nprofile = \"bogus\"\n",
			want:     Toolchain{Kind: ToolchainNone},
			warnings: 1,
		},
		{
			name: "unknown profile for a custom toolchain", file: "rust-toolchain.toml",
			content:  "[toolchain]\nchannel = \"my-linked\"\nprofile = \"bogus\"\n",
			want:     Toolchain{Kind: ToolchainChannel, Channel: "my-linked"},
			warnings: 1,
		},
		{
			name: "bare name in the TOML file", file: "rust-toolchain.toml",
			content:  "1.85.0\n",
			want:     Toolchain{Kind: ToolchainNone},
			warnings: 1,
		},
		{
			name: "invalid TOML", file: "rust-toolchain.toml",
			content:  "[toolchain\nchannel = \"" + untrusted + "\"\n",
			want:     Toolchain{Kind: ToolchainNone},
			warnings: 1,
		},
		{
			name: "empty file", file: "rust-toolchain",
			content:  " \n\n",
			want:     Toolchain{Kind: ToolchainNone},
			warnings: 1,
		},
		{
			name: "invalid channel name", file: "rust-toolchain",
			content:  untrusted + " $(id)\n",
			want:     Toolchain{Kind: ToolchainNone},
			warnings: 1,
		},
		{
			name: "invalid names and profile", file: "rust-toolchain.toml",
			content: `[toolchain]
channel = "1.85.0"
components = ["clippy", "SECRET name"]
targets = ["SECRET$(id)"]
profile = "minimal"
`,
			want:     Toolchain{Kind: ToolchainChannel, Channel: "1.85.0", Components: []string{"clippy"}, Profile: "minimal"},
			warnings: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, warnings := parseToolchainFile(tt.file, []byte(tt.content))
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
			if len(warnings) != tt.warnings {
				t.Errorf("got %d warnings %q, want %d", len(warnings), warnings, tt.warnings)
			}
			assertNoUntrusted(t, got, warnings)
		})
	}
}

func assertNoUntrusted(t *testing.T, toolchain Toolchain, warnings []string) {
	t.Helper()
	if strings.Contains(fmt.Sprintf("%+v %q", toolchain, warnings), untrusted) {
		t.Errorf("repository content leaked: %+v %q", toolchain, warnings)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mkdir(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

const channel185 = "[toolchain]\nchannel = \"1.85.0\"\n"

func TestDetectToolchain(t *testing.T) {
	t.Run("no file", func(t *testing.T) {
		root := t.TempDir()
		t.Setenv("GITHUB_WORKSPACE", root)
		got, warnings := detectToolchain(mkdir(t, filepath.Join(root, "project")))
		if !reflect.DeepEqual(got, Toolchain{Kind: ToolchainNone}) || len(warnings) != 0 {
			t.Errorf("got %+v %q, want kind none and no warnings", got, warnings)
		}
	})

	t.Run("nearest parent wins", func(t *testing.T) {
		root := t.TempDir()
		t.Setenv("GITHUB_WORKSPACE", root)
		writeFile(t, filepath.Join(root, "rust-toolchain.toml"), "[toolchain]\nchannel = \"1.80.0\"\n")
		writeFile(t, filepath.Join(root, "a", "rust-toolchain.toml"), channel185)
		got, _ := detectToolchain(mkdir(t, filepath.Join(root, "a", "b", "c")))
		if got.Channel != "1.85.0" || got.File != "../../rust-toolchain.toml" {
			t.Errorf("got %+v, want 1.85.0 from ../../rust-toolchain.toml", got)
		}
	})

	t.Run("legacy file wins over TOML", func(t *testing.T) {
		root := t.TempDir()
		t.Setenv("GITHUB_WORKSPACE", root)
		writeFile(t, filepath.Join(root, "rust-toolchain"), "1.80.0\n")
		writeFile(t, filepath.Join(root, "rust-toolchain.toml"), channel185)
		got, warnings := detectToolchain(root)
		if got.Channel != "1.80.0" || got.File != "rust-toolchain" || len(warnings) != 1 {
			t.Errorf("got %+v %q, want 1.80.0 from rust-toolchain with one warning", got, warnings)
		}
	})

	t.Run("search stops at the workspace", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, filepath.Join(root, "rust-toolchain.toml"), channel185)
		workspace := mkdir(t, filepath.Join(root, "workspace"))
		project := mkdir(t, filepath.Join(workspace, "project"))

		t.Setenv("GITHUB_WORKSPACE", workspace)
		if got, warnings := detectToolchain(project); !reflect.DeepEqual(got, Toolchain{Kind: ToolchainNone}) || len(warnings) != 0 {
			t.Errorf("searched above GITHUB_WORKSPACE: %+v %q", got, warnings)
		}
		// Control: without the boundary the same file is found.
		t.Setenv("GITHUB_WORKSPACE", "")
		if got, _ := detectToolchain(project); got.Channel != "1.85.0" {
			t.Errorf("without GITHUB_WORKSPACE got %+v, want the parent file", got)
		}
	})

	t.Run("workspace and project spelled through a symlink", func(t *testing.T) {
		root, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(root, "rust-toolchain.toml"), "[toolchain]\nchannel = \"SECRET\"\n")
		real := mkdir(t, filepath.Join(root, "real"))
		mkdir(t, filepath.Join(real, "project"))
		link := filepath.Join(root, "link")
		if err := os.Symlink(real, link); err != nil {
			t.Fatal(err)
		}
		for _, spelling := range [][2]string{{link, real}, {real, link}} {
			t.Setenv("GITHUB_WORKSPACE", spelling[0])
			got, warnings := detectToolchain(filepath.Join(spelling[1], "project"))
			if !reflect.DeepEqual(got, Toolchain{Kind: ToolchainNone}) || len(warnings) != 0 {
				t.Errorf("workspace %s, project under %s: searched above the workspace: %+v %q",
					filepath.Base(spelling[0]), filepath.Base(spelling[1]), got, warnings)
			}
		}
		// Control: a file inside the workspace is still found.
		writeFile(t, filepath.Join(real, "rust-toolchain.toml"), channel185)
		got, _ := detectToolchain(filepath.Join(real, "project"))
		if got.Channel != "1.85.0" || got.File != "../rust-toolchain.toml" {
			t.Errorf("file inside the workspace: got %+v", got)
		}
	})

	t.Run("project linking out of the workspace", func(t *testing.T) {
		root, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(root, "rust-toolchain.toml"), "[toolchain]\nchannel = \"SECRET\"\n")
		outside := mkdir(t, filepath.Join(root, "outside"))
		workspace := mkdir(t, filepath.Join(root, "workspace"))
		if err := os.Symlink(outside, filepath.Join(workspace, "project")); err != nil {
			t.Fatal(err)
		}
		t.Setenv("GITHUB_WORKSPACE", workspace)
		got, warnings := detectToolchain(filepath.Join(workspace, "project"))
		if got.Kind != ToolchainNone || len(warnings) != 1 || !strings.Contains(warnings[0], "outside the workspace") {
			t.Errorf("got %+v %q, want kind none and one warning", got, warnings)
		}
		assertNoUntrusted(t, got, warnings)
	})

	t.Run("project outside the workspace", func(t *testing.T) {
		root, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		workspace := mkdir(t, filepath.Join(root, "workspace"))
		writeFile(t, filepath.Join(root, "rust-toolchain.toml"), "[toolchain]\nchannel = \"SECRET\"\n")
		above := mkdir(t, filepath.Join(root, "outside", "project"))
		own := mkdir(t, filepath.Join(root, "own"))
		writeFile(t, filepath.Join(own, "rust-toolchain.toml"), "[toolchain]\nchannel = \"SECRET\"\n")

		t.Setenv("GITHUB_WORKSPACE", workspace)
		for _, project := range []string{above, own, root} {
			got, warnings := detectToolchain(project)
			if got.Kind != ToolchainNone || len(warnings) != 1 || !strings.Contains(warnings[0], "outside the workspace") {
				t.Errorf("%s: got %+v %q, want kind none and one warning", project, got, warnings)
			}
			assertNoUntrusted(t, got, warnings)
		}
	})

	t.Run("workspace that cannot be resolved", func(t *testing.T) {
		root, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(root, "project", "rust-toolchain.toml"), "[toolchain]\nchannel = \"SECRET\"\n")
		t.Setenv("GITHUB_WORKSPACE", filepath.Join(root, "missing"))
		got, warnings := detectToolchain(filepath.Join(root, "project"))
		if got.Kind != ToolchainNone || len(warnings) != 1 {
			t.Errorf("got %+v %q, want kind none and one warning", got, warnings)
		}
		assertNoUntrusted(t, got, warnings)
	})

	t.Run("symlinks", func(t *testing.T) {
		root := t.TempDir()
		workspace := mkdir(t, filepath.Join(root, "workspace"))
		t.Setenv("GITHUB_WORKSPACE", workspace)
		writeFile(t, filepath.Join(root, "outside.toml"), "[toolchain]\nchannel = \"SECRET\"\n")
		writeFile(t, filepath.Join(workspace, "shared.toml"), channel185)

		outside := mkdir(t, filepath.Join(workspace, "outside"))
		if err := os.Symlink(filepath.Join(root, "outside.toml"), filepath.Join(outside, "rust-toolchain.toml")); err != nil {
			t.Fatal(err)
		}
		got, warnings := detectToolchain(outside)
		if got.Kind != ToolchainNone || len(warnings) != 1 || !strings.Contains(warnings[0], "outside the workspace") {
			t.Errorf("link out of the workspace: got %+v %q", got, warnings)
		}
		assertNoUntrusted(t, got, warnings)

		inside := mkdir(t, filepath.Join(workspace, "inside"))
		if err := os.Symlink(filepath.Join(workspace, "shared.toml"), filepath.Join(inside, "rust-toolchain.toml")); err != nil {
			t.Fatal(err)
		}
		if got, warnings := detectToolchain(inside); got.Channel != "1.85.0" || len(warnings) != 0 {
			t.Errorf("link within the workspace: got %+v %q", got, warnings)
		}
	})

	t.Run("oversize file", func(t *testing.T) {
		root := t.TempDir()
		t.Setenv("GITHUB_WORKSPACE", root)
		writeFile(t, filepath.Join(root, "rust-toolchain.toml"),
			channel185+"# "+strings.Repeat("x", maxToolchainFileSize)+"\n")
		got, warnings := detectToolchain(root)
		if got.Kind != ToolchainNone || len(warnings) != 1 || !strings.Contains(warnings[0], "64 KiB") {
			t.Errorf("got %+v %q, want kind none and a size warning", got, warnings)
		}
	})

	t.Run("directory in place of the file", func(t *testing.T) {
		root := t.TempDir()
		t.Setenv("GITHUB_WORKSPACE", root)
		mkdir(t, filepath.Join(root, "rust-toolchain.toml"))
		got, warnings := detectToolchain(root)
		if got.Kind != ToolchainNone || len(warnings) != 1 {
			t.Errorf("got %+v %q, want kind none and one warning", got, warnings)
		}
	})
}

func TestExtractReportsToolchain(t *testing.T) {
	testdata, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GITHUB_WORKSPACE", testdata)

	tests := []struct {
		fixture string
		want    map[string]interface{}
	}{
		{"toolchain", map[string]interface{}{
			"toolchain_kind":       ToolchainChannel,
			"toolchain_file":       "rust-toolchain.toml",
			"toolchain_channel":    "1.85.0",
			"toolchain_components": []string{"clippy", "rustfmt"},
			"toolchain_targets":    []string{"wasm32-unknown-unknown", "x86_64-unknown-linux-musl"},
			"toolchain_profile":    "minimal",
		}},
		{"toolchain-path", map[string]interface{}{
			"toolchain_kind": ToolchainPath,
			"toolchain_file": "rust-toolchain.toml",
		}},
		{"complete", map[string]interface{}{
			"toolchain_kind": ToolchainNone,
		}},
	}

	for _, tt := range tests {
		t.Run(tt.fixture, func(t *testing.T) {
			metadata, err := NewExtractor().Extract(filepath.Join(testdata, tt.fixture))
			if err != nil {
				t.Fatal(err)
			}
			got := map[string]interface{}{}
			for key, value := range metadata.LanguageSpecific {
				if strings.HasPrefix(key, "toolchain_") {
					got[key] = value
				}
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
