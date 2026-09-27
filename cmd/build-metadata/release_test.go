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

// TestApplyReleaseFilesContainer covers the container release schema,
// which carries the release version as container_release_tag: global-jjb's
// release-job.sh reads that key for distribution_type container and the
// top-level version key for every other type.
func TestApplyReleaseFilesContainer(t *testing.T) {
	tmpDir := t.TempDir()
	writeReleaseFile(t, tmpDir, "1.7.0-container.yaml", `---
distribution_type: 'container'
container_release_tag: '1.7.0'
container_pull_registry: 'nexus3.onap.org:10003'
container_push_registry: 'nexus3.onap.org:10002'
project: 'sdc-docker-base'
ref: 9dbb2f5ed5b4e0b1a2c3d4e5f60718293a4b5c6d
containers:
  - name: 'sdc-base-jetty'
    version: '1.7.0-20200619T121144Z'
`)

	metadata := newMetadata(tmpDir)
	applyReleaseFiles(metadata, tmpDir)

	if metadata.Common.ReleaseVersion != "1.7.0" {
		t.Errorf("ReleaseVersion = %q, want 1.7.0 from container_release_tag", metadata.Common.ReleaseVersion)
	}
	if metadata.Common.ReleaseRef != "9dbb2f5ed5b4e0b1a2c3d4e5f60718293a4b5c6d" {
		t.Errorf("ReleaseRef = %q, want the declared ref", metadata.Common.ReleaseRef)
	}
	if metadata.Common.ReleaseDistributionType != "container" {
		t.Errorf("ReleaseDistributionType = %q, want container", metadata.Common.ReleaseDistributionType)
	}
}

// TestApplyReleaseFilesContainerIgnoresNestedVersion guards the line scan:
// a container file's only version: keys are per-image and indented, and
// must not stand in for the release version.
func TestApplyReleaseFilesContainerIgnoresNestedVersion(t *testing.T) {
	tmpDir := t.TempDir()
	writeReleaseFile(t, tmpDir, "broken-container.yaml", `distribution_type: container
ref: abc
containers:
  - name: img
    version: 9.9.9-20200101T000000Z
`)

	metadata := newMetadata(tmpDir)
	applyReleaseFiles(metadata, tmpDir)

	if metadata.Common.ReleaseVersion != "" {
		t.Errorf("ReleaseVersion = %q, want empty without container_release_tag", metadata.Common.ReleaseVersion)
	}
}

// TestApplyReleaseFilesContainerDoesNotReadVersionKey pins that a stray
// top-level version: in a container file is not used: the container schema
// does not define it, and release-job.sh never reads it for containers.
func TestApplyReleaseFilesContainerDoesNotReadVersionKey(t *testing.T) {
	tmpDir := t.TempDir()
	writeReleaseFile(t, tmpDir, "c.yaml", "distribution_type: container\nversion: 0.0.1\ncontainer_release_tag: 2.0.0\nref: abc\n")

	metadata := newMetadata(tmpDir)
	applyReleaseFiles(metadata, tmpDir)

	if metadata.Common.ReleaseVersion != "2.0.0" {
		t.Errorf("ReleaseVersion = %q, want 2.0.0", metadata.Common.ReleaseVersion)
	}
}

// TestApplyReleaseFilesDistributionTypes covers each global-jjb schema's
// version key, and a file that declares no type.
func TestApplyReleaseFilesDistributionTypes(t *testing.T) {
	cases := []struct {
		name, content, wantType, wantVersion string
	}{
		{"maven", "distribution_type: maven\nversion: 1.0.0\n", "maven", "1.0.0"},
		{"artifact", "distribution_type: artifact\nversion: 2.0.0\n", "artifact", "2.0.0"},
		{"pypi", "distribution_type: pypi\nversion: 3.0.0\n", "pypi", "3.0.0"},
		{"packagecloud", "package_name: p\nversion: 4.0.0\n", "", "4.0.0"},
		{"upper-case type", "distribution_type: Container\ncontainer_release_tag: 5.0.0\n", "container", "5.0.0"},
		{"quoted type", "distribution_type: \"container\"\ncontainer_release_tag: 6.0.0\n", "container", "6.0.0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			writeReleaseFile(t, tmpDir, "r.yaml", tc.content)
			metadata := newMetadata(tmpDir)
			applyReleaseFiles(metadata, tmpDir)
			if metadata.Common.ReleaseDistributionType != tc.wantType {
				t.Errorf("ReleaseDistributionType = %q, want %q", metadata.Common.ReleaseDistributionType, tc.wantType)
			}
			if metadata.Common.ReleaseVersion != tc.wantVersion {
				t.Errorf("ReleaseVersion = %q, want %q", metadata.Common.ReleaseVersion, tc.wantVersion)
			}
		})
	}
}

// TestApplyReleaseFilesMultipleLeavesTypeEmpty keeps the lone-file rule for
// the new field: with several files, none of them speaks for the release.
func TestApplyReleaseFilesMultipleLeavesTypeEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	writeReleaseFile(t, tmpDir, "a.yaml", "distribution_type: container\ncontainer_release_tag: 1.0.0\n")
	writeReleaseFile(t, tmpDir, "b.yaml", "distribution_type: maven\nversion: 1.0.0\n")

	metadata := newMetadata(tmpDir)
	applyReleaseFiles(metadata, tmpDir)

	if metadata.Common.ReleaseDistributionType != "" {
		t.Errorf("ReleaseDistributionType = %q, want empty with multiple files", metadata.Common.ReleaseDistributionType)
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
