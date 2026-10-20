// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sethvargo/go-githubactions"
)

func writeTree(t *testing.T, dir string, files ...string) {
	t.Helper()
	for _, name := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("creating parent of %s: %v", name, err)
		}
		if err := os.WriteFile(path, []byte("FROM scratch\n"), 0o600); err != nil {
			t.Fatalf("writing %s: %v", name, err)
		}
	}
}

// The common ONAP shape: a Maven project with a Dockerfile resolves to
// java-maven, yet the Java lane needs to know an image build exists.
func TestRepositoryFactsMavenWithDockerfile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pom.xml"), []byte(minimalPOM), 0o600); err != nil {
		t.Fatal(err)
	}
	writeTree(t, dir, "Dockerfile", "src/main/docker/Dockerfile")

	metadata := newMetadata(dir)
	applyRepositoryFacts(metadata, dir, nil)

	if got := projectTypesJSON(metadata.Common.ProjectTypes); got != `["java-maven","docker"]` {
		t.Errorf("project_types = %s, want [\"java-maven\",\"docker\"]", got)
	}
	if !metadata.Common.HasDockerfiles {
		t.Error("HasDockerfiles = false, want true")
	}
	if metadata.Common.DockerfileCount != 2 {
		t.Errorf("DockerfileCount = %d, want 2", metadata.Common.DockerfileCount)
	}
}

func TestRepositoryFactsEmptyTree(t *testing.T) {
	dir := t.TempDir()
	metadata := newMetadata(dir)
	applyRepositoryFacts(metadata, dir, nil)

	if got := projectTypesJSON(metadata.Common.ProjectTypes); got != "[]" {
		t.Errorf("project_types = %s, want []", got)
	}
	if metadata.Common.HasDockerfiles || metadata.Common.DockerfileCount != 0 {
		t.Errorf("Dockerfiles = (%t, %d), want (false, 0)",
			metadata.Common.HasDockerfiles, metadata.Common.DockerfileCount)
	}
}

// BuildKit's per-Dockerfile ignore file is not a Dockerfile.
func TestRepositoryFactsDockerignoreOnly(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, "Dockerfile.dockerignore")

	metadata := newMetadata(dir)
	applyRepositoryFacts(metadata, dir, nil)

	if metadata.Common.HasDockerfiles || metadata.Common.DockerfileCount != 0 {
		t.Errorf("Dockerfiles = (%t, %d), want (false, 0)",
			metadata.Common.HasDockerfiles, metadata.Common.DockerfileCount)
	}
}

func TestRepositoryFactsDockerfileWithDockerignore(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, "Dockerfile", "Dockerfile.dockerignore")

	metadata := newMetadata(dir)
	applyRepositoryFacts(metadata, dir, nil)

	if !metadata.Common.HasDockerfiles || metadata.Common.DockerfileCount != 1 {
		t.Errorf("Dockerfiles = (%t, %d), want (true, 1)",
			metadata.Common.HasDockerfiles, metadata.Common.DockerfileCount)
	}
}

func TestCountDockerfiles(t *testing.T) {
	tests := []struct {
		name     string
		files    []string
		excludes []string
		want     int
	}{
		{
			name:  "naming conventions",
			files: []string{"Dockerfile", "images/Dockerfile.alpine", "api.Dockerfile", "docs/dockerfile.md", "Dockerfiles/README"},
			want:  3,
		},
		{
			name:  "dockerignore beside Dockerfile",
			files: []string{"Dockerfile", "Dockerfile.dockerignore", ".dockerignore"},
			want:  1,
		},
		{
			name:  "dockerignore beside variant",
			files: []string{"Dockerfile.prod", "Dockerfile.prod.dockerignore"},
			want:  1,
		},
		{
			name:  "plain dockerignore",
			files: []string{".dockerignore", "app/.dockerignore"},
			want:  0,
		},
		{
			name:  "built-in excludes",
			files: []string{"node_modules/x/Dockerfile", "vendor/y/Dockerfile", "target/docker/Dockerfile", ".git/Dockerfile"},
			want:  0,
		},
		{
			name:     "exclude by base name",
			files:    []string{"app/Dockerfile", "test/Dockerfile", "a/test/Dockerfile"},
			excludes: []string{"test"},
			want:     1,
		},
		{
			name:     "exclude by relative path glob",
			files:    []string{"app/Dockerfile", "docker/dev/Dockerfile", "docker/prod/Dockerfile"},
			excludes: []string{"docker/dev/"},
			want:     2,
		},
		{
			name:     "exclude a file pattern",
			files:    []string{"Dockerfile", "Dockerfile.test"},
			excludes: []string{"Dockerfile.*"},
			want:     1,
		},
		{
			name:  "depth bound",
			files: []string{"1/2/3/4/5/6/7/Dockerfile", "1/2/3/4/5/6/7/8/Dockerfile"},
			want:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeTree(t, dir, tt.files...)
			excludes := append(append([]string{}, defaultDockerfileExcludes...), tt.excludes...)
			if got := countDockerfiles(dir, excludes); got != tt.want {
				t.Errorf("countDockerfiles() = %d, want %d", got, tt.want)
			}
		})
	}
}

// A symlink must not count, nor lead the walk outside the tree.
func TestCountDockerfilesIgnoresSymlinks(t *testing.T) {
	outside := t.TempDir()
	writeTree(t, outside, "Dockerfile", "nested/Dockerfile")

	dir := t.TempDir()
	if err := os.Symlink(filepath.Join(outside, "Dockerfile"), filepath.Join(dir, "Dockerfile")); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	if err := os.Symlink(filepath.Join(outside, "nested"), filepath.Join(dir, "nested")); err != nil {
		t.Fatal(err)
	}

	if got := countDockerfiles(dir, defaultDockerfileExcludes); got != 0 {
		t.Errorf("countDockerfiles() = %d, want 0", got)
	}
}

func TestDockerfileExcludeInputIsParsed(t *testing.T) {
	t.Setenv("INPUT_DOCKERFILE_EXCLUDE", "test, docs\nexamples/*")
	cfg := parseFlags(githubactions.New(), false)
	if got := strings.Join(cfg.dockerfileExcludes, "|"); got != "test|docs|examples/*" {
		t.Errorf("dockerfileExcludes = %q, want test|docs|examples/*", got)
	}
}
