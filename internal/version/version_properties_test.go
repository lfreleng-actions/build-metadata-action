// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package version

import (
	"os"
	"path/filepath"
	"testing"
)

// The checked-in fixture follows the layout ONAP repositories carry: the
// trio plus base_version, release_version and
// snapshot_version derived through ${...}, which Jenkins resolves by
// sourcing the file. The parser never sources it, so the version must
// come from the trio and the derived keys must not leak through.
func TestExtractVersionPropertiesONAPFixture(t *testing.T) {
	info, ok := ExtractVersionProperties(filepath.Join("testdata", "onap"))
	if !ok {
		t.Fatal("ExtractVersionProperties() ok = false, want true")
	}
	if info.Version != "1.9.1" {
		t.Errorf("Version = %q, want 1.9.1", info.Version)
	}
	if info.Source != "version.properties" {
		t.Errorf("Source = %q, want version.properties", info.Source)
	}
}

// When path_prefix names a sub-project, only that directory's
// version.properties counts. The parent's file is neither read nor
// used as a fallback, so each sub-project reports its own version.
func TestExtractVersionPropertiesPerDirectory(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "version.properties", "major=1\nminor=0\npatch=0\n")

	withOwn := filepath.Join(root, "service-a")
	withoutOwn := filepath.Join(root, "service-b")
	mkdirAll(t, withOwn)
	mkdirAll(t, withoutOwn)
	writeFile(t, withOwn, "version.properties", "major=2\nminor=3\npatch=4\n")

	info, ok := ExtractVersionProperties(withOwn)
	if !ok || info.Version != "2.3.4" {
		t.Errorf("sub-project with its own file: got (%v, %v), want 2.3.4", info, ok)
	}

	if info, ok := ExtractVersionProperties(withoutOwn); ok {
		t.Errorf("sub-project without a file: got %q, want no version (no parent fallback)", info.Version)
	}

	info, ok = ExtractVersionProperties(root)
	if !ok || info.Version != "1.0.0" {
		t.Errorf("repository root: got (%v, %v), want 1.0.0", info, ok)
	}
}

func mkdirAll(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("creating %s: %v", dir, err)
	}
}
