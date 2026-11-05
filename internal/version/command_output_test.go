// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package version

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// installFakeTool writes a POSIX shell script named name into a fresh
// directory and returns the script's path.
func installFakeTool(t *testing.T, name, script string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake tools are POSIX shell scripts")
	}
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatalf("writing fake %s: %v", name, err)
	}
	return path
}

// git describe exits 0 yet warns on stderr when a ref names an
// annotated tag under another name; the warning is not a version.
const fakeGitWithWarnings = `case "$*" in
*describe*)
	echo "warning: tag 'v1-alias' is externally known as 'v1.0.0'" >&2
	echo v1.0.0
	;;
*"tag --list"*)
	echo "warning: ignoring broken ref refs/tags/broken" >&2
	printf 'v1.0.0\n\nv1.1.0\n'
	;;
esac
exit 0
`

func useFakeGit(t *testing.T) {
	t.Helper()
	git := installFakeTool(t, "git", fakeGitWithWarnings)
	t.Setenv("PATH", filepath.Dir(git))
}

func TestGetLatestGitTagIgnoresStderr(t *testing.T) {
	useFakeGit(t)
	got, err := GetLatestGitTag(t.TempDir())
	if err != nil {
		t.Fatalf("GetLatestGitTag: %v", err)
	}
	if got != "v1.0.0" {
		t.Errorf("GetLatestGitTag = %q, want v1.0.0", got)
	}
}

func TestGetAllGitTagsIgnoresStderr(t *testing.T) {
	useFakeGit(t)
	got, err := GetAllGitTags(t.TempDir())
	if err != nil {
		t.Fatalf("GetAllGitTags: %v", err)
	}
	want := []string{"v1.0.0", "v1.1.0"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("GetAllGitTags = %q, want %q", got, want)
	}
}

func TestExtractFromGitIgnoresStderr(t *testing.T) {
	useFakeGit(t)
	info, err := extractFromGit(t.TempDir())
	if err != nil {
		t.Fatalf("extractFromGit: %v", err)
	}
	if info.Version != "1.0.0" || info.Source != "git-tag" {
		t.Errorf("extractFromGit = %+v, want version 1.0.0 from git-tag", info)
	}
}

func TestExtractWithToolParsesJSON(t *testing.T) {
	tests := []struct {
		name        string
		json        string
		wantVersion string
		wantSource  string
		wantDynamic bool
	}{
		{
			name:        "static",
			json:        `{"success":true,"version":"2.3.4","project_type":"Python","subtype":"Modern (pyproject.toml)","file":"./pyproject.toml","matched_by":"version","version_source":"static"}`,
			wantVersion: "2.3.4",
			wantSource:  "pyproject.toml",
		},
		{
			name:        "dynamic git tag",
			json:        `{"success":true,"version":"2.1.0","project_type":"Python","subtype":"Modern (pyproject.toml)","file":"./pyproject.toml","matched_by":"dynamic-git-tag","version_source":"dynamic-git-tag","git_tag":"v2.1.0"}`,
			wantVersion: "2.1.0",
			wantSource:  "pyproject.toml",
			wantDynamic: true,
		},
		{
			name:        "nested manifest",
			json:        `{"success":true,"version":"1.0.0","file":"./sub/pom.xml","version_source":"static"}`,
			wantVersion: "1.0.0",
			wantSource:  "sub/pom.xml",
		},
		{
			name:        "no file",
			json:        `{"success":true,"version":"1.0.0","version_source":"static"}`,
			wantVersion: "1.0.0",
			wantSource:  "version-extract-action",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := installFakeTool(t, "version-extract", "echo \"warning: something odd\" >&2\necho '"+tt.json+"'\n")
			info, err := extractWithTool(t.TempDir(), "python-modern", tool)
			if err != nil {
				t.Fatalf("extractWithTool: %v", err)
			}
			if info.Version != tt.wantVersion {
				t.Errorf("Version = %q, want %q", info.Version, tt.wantVersion)
			}
			if info.Source != tt.wantSource {
				t.Errorf("Source = %q, want %q", info.Source, tt.wantSource)
			}
			if info.IsDynamic != tt.wantDynamic {
				t.Errorf("IsDynamic = %v, want %v", info.IsDynamic, tt.wantDynamic)
			}
		})
	}
}

func TestToolSourceAbsolutePath(t *testing.T) {
	root := t.TempDir()
	if got := toolSource(root, filepath.Join(root, "package.json")); got != "package.json" {
		t.Errorf("toolSource = %q, want package.json", got)
	}
}

func TestExtractWithToolRejectsFailures(t *testing.T) {
	tests := []struct {
		name   string
		script string
	}{
		{"success false", `echo "no version file" >&2
echo '{"success":false,"version":"","error":"not found"}'
`},
		{"empty version", `echo '{"success":true,"version":""}'
`},
		{"invalid JSON", `echo "no version file" >&2
echo 'not json'
`},
		{"non-zero exit", `echo "no version file" >&2
exit 1
`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := installFakeTool(t, "version-extract", tt.script)
			info, err := extractWithTool(t.TempDir(), "python-modern", tool)
			if err == nil {
				t.Fatalf("extractWithTool = %+v, want an error", info)
			}
			if strings.Contains(tt.script, ">&2") && !strings.Contains(err.Error(), "no version file") {
				t.Errorf("error %q does not carry the tool's stderr", err)
			}
		})
	}
}
