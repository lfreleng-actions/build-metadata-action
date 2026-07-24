// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// writeReleaseFile creates releases/<name> under dir with the given content.
func writeReleaseFile(t *testing.T, dir, name, content string) {
	t.Helper()
	releasesDir := filepath.Join(dir, "releases")
	if err := os.MkdirAll(releasesDir, 0755); err != nil {
		t.Fatalf("Failed to create releases dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(releasesDir, name), []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write release file: %v", err)
	}
}

func TestApplyReleaseFilesNone(t *testing.T) {
	metadata := newMetadata(t.TempDir())
	applyReleaseFiles(metadata, metadata.Common.ProjectPath)

	if metadata.Common.IsReleaseReady {
		t.Error("IsReleaseReady = true, want false when no releases/ dir exists")
	}
	if metadata.Common.ReleaseFileCount != 0 {
		t.Errorf("ReleaseFileCount = %d, want 0", metadata.Common.ReleaseFileCount)
	}
}

func TestApplyReleaseFilesSingle(t *testing.T) {
	tmpDir := t.TempDir()
	writeReleaseFile(t, tmpDir, "3.8.2.yaml", `---
distribution_type: maven
version: 3.8.2
project: cps
ref: abcdef1234567890abcdef1234567890abcdef12
`)

	metadata := newMetadata(tmpDir)
	applyReleaseFiles(metadata, tmpDir)

	if !metadata.Common.IsReleaseReady {
		t.Fatal("IsReleaseReady = false, want true")
	}
	if metadata.Common.ReleaseFileCount != 1 {
		t.Errorf("ReleaseFileCount = %d, want 1", metadata.Common.ReleaseFileCount)
	}
	if got := metadata.Common.ReleaseFiles; len(got) != 1 || got[0] != "releases/3.8.2.yaml" {
		t.Errorf("ReleaseFiles = %v, want [releases/3.8.2.yaml]", got)
	}
	if metadata.Common.ReleaseVersion != "3.8.2" {
		t.Errorf("ReleaseVersion = %q, want 3.8.2", metadata.Common.ReleaseVersion)
	}
	if metadata.Common.ReleaseRef != "abcdef1234567890abcdef1234567890abcdef12" {
		t.Errorf("ReleaseRef = %q, want the declared ref", metadata.Common.ReleaseRef)
	}
}

func TestApplyReleaseFilesMultipleLeavesVersionEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	writeReleaseFile(t, tmpDir, "3.8.1.yaml", "version: 3.8.1\nref: aaa\n")
	writeReleaseFile(t, tmpDir, "3.8.2.yml", "version: 3.8.2\nref: bbb\n")

	metadata := newMetadata(tmpDir)
	applyReleaseFiles(metadata, tmpDir)

	if metadata.Common.ReleaseFileCount != 2 {
		t.Errorf("ReleaseFileCount = %d, want 2", metadata.Common.ReleaseFileCount)
	}
	if metadata.Common.ReleaseVersion != "" || metadata.Common.ReleaseRef != "" {
		t.Errorf("version/ref should be empty with multiple release files, got %q/%q",
			metadata.Common.ReleaseVersion, metadata.Common.ReleaseRef)
	}
	// Sorted, deterministic order.
	want := []string{"releases/3.8.1.yaml", "releases/3.8.2.yml"}
	for i, file := range metadata.Common.ReleaseFiles {
		if file != want[i] {
			t.Errorf("ReleaseFiles[%d] = %q, want %q", i, file, want[i])
		}
	}
}

func TestApplyReleaseFilesStripsQuotesFromScalars(t *testing.T) {
	tmpDir := t.TempDir()
	writeReleaseFile(t, tmpDir, "release.yaml", "version: \"1.2.3\"\nref: 'deadbeef'\n")

	metadata := newMetadata(tmpDir)
	applyReleaseFiles(metadata, tmpDir)

	if metadata.Common.ReleaseVersion != "1.2.3" {
		t.Errorf("ReleaseVersion = %q, want 1.2.3", metadata.Common.ReleaseVersion)
	}
	if metadata.Common.ReleaseRef != "deadbeef" {
		t.Errorf("ReleaseRef = %q, want deadbeef", metadata.Common.ReleaseRef)
	}
}

// TestFindReleaseFilesSkipsSymlinkedFile verifies a symlinked release file is
// skipped rather than followed, so a crafted symlink cannot make the scan
// read a YAML file outside the workspace.
func TestFindReleaseFilesSkipsSymlinkedFile(t *testing.T) {
	tmpDir := t.TempDir()
	writeReleaseFile(t, tmpDir, "real.yaml", "version: 1.0.0\n")

	// Place a target outside the releases/ directory and symlink to it from
	// within releases/ using a matching *.yaml name.
	outside := filepath.Join(tmpDir, "outside.yaml")
	if err := os.WriteFile(outside, []byte("version: 9.9.9\n"), 0644); err != nil {
		t.Fatalf("Failed to write outside target: %v", err)
	}
	link := filepath.Join(tmpDir, "releases", "link.yaml")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unsupported on this platform: %v", err)
	}

	files := findReleaseFiles(tmpDir)
	for _, f := range files {
		if f == "releases/link.yaml" {
			t.Errorf("symlinked release file was included: %v", files)
		}
	}
	if len(files) != 1 || files[0] != "releases/real.yaml" {
		t.Errorf("findReleaseFiles = %v, want [releases/real.yaml]", files)
	}
}

// TestFindReleaseFilesSkipsSymlinkedDir verifies a symlinked releases/
// directory is not traversed.
func TestFindReleaseFilesSkipsSymlinkedDir(t *testing.T) {
	tmpDir := t.TempDir()

	// A real directory of release files living outside the project.
	target := filepath.Join(tmpDir, "elsewhere")
	if err := os.MkdirAll(target, 0755); err != nil {
		t.Fatalf("Failed to create target dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(target, "escape.yaml"), []byte("version: 9.9.9\n"), 0644); err != nil {
		t.Fatalf("Failed to write target file: %v", err)
	}

	project := filepath.Join(tmpDir, "project")
	if err := os.MkdirAll(project, 0755); err != nil {
		t.Fatalf("Failed to create project dir: %v", err)
	}
	if err := os.Symlink(target, filepath.Join(project, "releases")); err != nil {
		t.Skipf("symlinks unsupported on this platform: %v", err)
	}

	if files := findReleaseFiles(project); len(files) != 0 {
		t.Errorf("findReleaseFiles via symlinked dir = %v, want none", files)
	}
}
