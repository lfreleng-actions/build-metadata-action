// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package environment

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// installRecordingTool puts a fake tool on PATH that writes the directory
// it runs in to a record file, then prints a version line.
func installRecordingTool(t *testing.T, binDir, name, record string) {
	t.Helper()
	script := "#!/bin/sh\npwd -P > '" + record + "'\necho '" + name + " 1.2.3'\n"
	if err := os.WriteFile(filepath.Join(binDir, name), []byte(script), 0o755); err != nil {
		t.Fatalf("writing fake %s: %v", name, err)
	}
}

func recordedDir(t *testing.T, record string) string {
	t.Helper()
	body, err := os.ReadFile(record)
	if err != nil {
		t.Fatalf("tool never ran: %v", err)
	}
	return strings.TrimSpace(string(body))
}

// A rustup proxy run from the checkout honours the repository's
// rust-toolchain file, and a `path` entry there makes it execute binaries
// the repository supplies. The probe must run somewhere no repository
// file can reach, while other tools keep running in the caller's
// directory.
func TestRustupProxiesIgnoreTheProjectDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake tools are POSIX shell scripts")
	}

	project, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	toolchainFile := "[toolchain]\npath = \"/nonexistent/toolchain\"\n"
	if err := os.WriteFile(filepath.Join(project, "rust-toolchain.toml"), []byte(toolchainFile), 0o644); err != nil {
		t.Fatal(err)
	}

	binDir := t.TempDir()
	records := t.TempDir()
	for _, tool := range []string{"cargo", "rustc", "make"} {
		installRecordingTool(t, binDir, tool, filepath.Join(records, tool))
	}
	t.Setenv("PATH", binDir)
	t.Setenv("GITHUB_WORKSPACE", project)
	t.Chdir(project)

	for _, tool := range []string{"cargo", "rustc"} {
		if got := getToolVersion(tool, "--version"); got != "1.2.3" {
			t.Errorf("getToolVersion(%s) = %q, want 1.2.3", tool, got)
		}
		dir := recordedDir(t, filepath.Join(records, tool))
		if dir == project || strings.HasPrefix(dir, project+string(os.PathSeparator)) {
			t.Errorf("%s ran in %s, inside the project; a toolchain file there would select it", tool, dir)
		}
	}

	// Control: other tools still run where the action was invoked.
	getToolVersion("make", "--version")
	if dir := recordedDir(t, filepath.Join(records, "make")); dir != project {
		t.Errorf("make ran in %s, want the current directory %s", dir, project)
	}
}

// TMPDIR is configurable, and a self-hosted runner may point it into the
// checkout. A probe directory inside GITHUB_WORKSPACE, or under any
// directory holding a toolchain file, would let the repository select
// the toolchain again, so the probe must not run there at all.
func TestRustupProxyProbeDirectoryMustBeTrusted(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake tools are POSIX shell scripts")
	}

	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(base, "workspace")
	mkdir := func(dir string) string {
		t.Helper()
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	mkdir(filepath.Join(workspace, "tmp"))
	if err := os.Symlink(filepath.Join(workspace, "tmp"), filepath.Join(base, "link-into-workspace")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"rust-toolchain", "rust-toolchain.toml"} {
		parent := mkdir(filepath.Join(base, "under-"+name))
		if err := os.WriteFile(filepath.Join(parent, name), []byte("[toolchain]\npath = \"/nonexistent\"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		mkdir(filepath.Join(parent, "tmp"))
	}

	tests := []struct {
		name    string
		tmpdir  string
		wantRun bool
	}{
		{"workspace itself", workspace, false},
		{"inside the workspace", filepath.Join(workspace, "tmp"), false},
		{"symlink into the workspace", filepath.Join(base, "link-into-workspace"), false},
		{"under a legacy toolchain file", filepath.Join(base, "under-rust-toolchain", "tmp"), false},
		{"under a toolchain file", filepath.Join(base, "under-rust-toolchain.toml", "tmp"), false},
		{"trusted directory", mkdir(filepath.Join(base, "trusted")), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			binDir := t.TempDir()
			records := t.TempDir()
			record := filepath.Join(records, "cargo")
			installRecordingTool(t, binDir, "cargo", record)
			t.Setenv("PATH", binDir)
			t.Setenv("GITHUB_WORKSPACE", workspace)
			t.Setenv("TMPDIR", tt.tmpdir)

			got := getToolVersion("cargo", "--version")
			_, statErr := os.Stat(record)
			if ran := statErr == nil; ran != tt.wantRun {
				t.Fatalf("probe ran = %v, want %v (version %q)", ran, tt.wantRun, got)
			}
			if tt.wantRun && recordedDir(t, record) != tt.tmpdir {
				t.Errorf("cargo ran in %s, want %s", recordedDir(t, record), tt.tmpdir)
			}
		})
	}
}
