// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/lfreleng-actions/build-metadata-action/internal/detector"
	"github.com/sethvargo/go-githubactions"
)

// gitFixture writes files into a new directory and makes it a git
// repository with one commit tagged v9.9.9, so the git fallback is
// deterministic.
func gitFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{
		{"init", "-q"},
		{"add", "-A"},
		{"-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid",
			"-c", "commit.gpgsign=false", "commit", "-q", "-m", "Fixture"},
		{"-c", "tag.gpgsign=false", "tag", "v9.9.9"},
	} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

// TestPythonProjectVersion checks project_version, version_source and
// versioning_type for Python projects under both settings of
// use_version_extract: a static version the project declares wins, a
// project that declares none falls back to the latest tag, and
// versioning_type always equals python_versioning_type.
func TestPythonProjectVersion(t *testing.T) {
	type result struct{ version, source, versioning string }
	tests := []struct {
		name      string
		files     map[string]string
		extract   result
		noExtract result
	}{
		{
			name: "pyproject version below a script named version",
			files: map[string]string{"pyproject.toml": `[project.scripts]
version = "pkg.cli:version"

[project]
name = "pkg"
version = "4.5.6"
`},
			extract:   result{"4.5.6", "pyproject.toml", "static"},
			noExtract: result{"4.5.6", "pyproject.toml", "static"},
		},
		{
			name: "pyproject version with a comment holding =",
			files: map[string]string{"pyproject.toml": `[project]
name = "pkg-comment"
version = "1.2.3"  # bump with tag=release
`},
			extract:   result{"1.2.3", "pyproject.toml", "static"},
			noExtract: result{"1.2.3", "pyproject.toml", "static"},
		},
		{
			name: "dynamic version with a hatch version-file",
			files: map[string]string{"pyproject.toml": `[build-system]
requires = ["hatchling", "hatch-vcs"]
build-backend = "hatchling.build"

[project]
name = "dyn-hook"
dynamic = ["version"]

[tool.hatch.build.hooks.vcs]
version-file = "src/dyn_hook/_version.py"
`},
			extract:   result{"9.9.9", "git-tag", "dynamic"},
			noExtract: result{"", "", "dynamic"},
		},
		{
			name: "dynamic version and nothing else",
			files: map[string]string{"pyproject.toml": `[project]
name = "dyn-plain"
dynamic = ["version"]
`},
			extract:   result{"9.9.9", "git-tag", "dynamic"},
			noExtract: result{"", "", "dynamic"},
		},
		{
			name: "setup.cfg static version",
			files: map[string]string{"setup.cfg": `[metadata]
name = cfg-static
version = 1.2.3
`},
			extract:   result{"1.2.3", "setup.cfg", "static"},
			noExtract: result{"1.2.3", "setup.cfg", "static"},
		},
		{
			name: "setup.py static version",
			files: map[string]string{"setup.py": `from setuptools import setup

setup(name="py-static", version="0.4.0")
`},
			extract:   result{"0.4.0", "setup.py", "static"},
			noExtract: result{"0.4.0", "setup.py", "static"},
		},
		{
			name: "PBR project",
			files: map[string]string{
				"setup.cfg": "[metadata]\nname = pbr-project\n",
				"setup.py":  "import setuptools\n\nsetuptools.setup(setup_requires=[\"pbr\"], pbr=True)\n",
			},
			extract:   result{"9.9.9", "git-tag", "dynamic"},
			noExtract: result{"", "", "dynamic"},
		},
	}

	ctx := &appContext{action: githubactions.New(), isCI: false}
	for _, tt := range tests {
		dir := gitFixture(t, tt.files)
		projectType, err := detector.DetectProjectType(dir)
		if err != nil {
			t.Fatalf("%s: DetectProjectType: %v", tt.name, err)
		}
		for _, useExtract := range []bool{true, false} {
			want := tt.noExtract
			if useExtract {
				want = tt.extract
			}
			metadata := newMetadata(dir)
			cfg := runConfig{absPath: dir, useVersionExtract: useExtract}
			extractVersionInfo(ctx, cfg, metadata, projectType)
			extractProjectMetadata(ctx, metadata, projectType, dir)

			got := result{
				metadata.Common.ProjectVersion,
				metadata.Common.VersionSource,
				metadata.Common.VersioningType,
			}
			if got != want {
				t.Errorf("%s (use_version_extract=%v): got %+v, want %+v",
					tt.name, useExtract, got, want)
			}
			if pythonType := metadata.LanguageSpecific["versioning_type"]; pythonType != got.versioning {
				t.Errorf("%s (use_version_extract=%v): versioning_type %q, python_versioning_type %v",
					tt.name, useExtract, got.versioning, pythonType)
			}
		}
	}
}
