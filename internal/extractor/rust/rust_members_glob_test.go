// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package rust

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Repeated "**" components must not revisit directories
// combinatorially: a repository could stall the action with one members
// entry. Every directory is listed at most once.
func TestMemberGlobListsEachDirectoryOnce(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	deepest := filepath.Join(append([]string{root}, strings.Split("d/d/d/d/d/d/d/d/d/d", "/")...)...)
	if err := os.MkdirAll(filepath.Join(deepest, "x"), 0o755); err != nil {
		t.Fatal(err)
	}

	listed := map[string]int{}
	t.Cleanup(func() { listDir = os.ReadDir })
	listDir = func(dir string) ([]os.DirEntry, error) {
		listed[dir]++
		return os.ReadDir(dir)
	}

	got, err := expandMemberGlob(root, strings.Repeat("**/", 10)+"x", newDirCache(root))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{filepath.Join(deepest, "x")}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	for dir, count := range listed {
		if count > 1 {
			t.Errorf("listed %s %d times", dir, count)
		}
	}
}

// Symlinked spellings of one directory share its listing and its walk,
// whether they come from several members entries or from links back to
// an ancestor, so aliases cannot multiply the work.
func TestMemberGlobListsEachRealDirectoryOnce(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "tree", "a", "b", "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, link := range []string{"alias1", "alias2", "alias3"} {
		if err := os.Symlink("tree", filepath.Join(root, link)); err != nil {
			t.Fatal(err)
		}
	}
	for _, link := range []string{"l1", "l2", "l3", "l4", "l5", "l6"} {
		if err := os.Symlink(".", filepath.Join(root, "tree", link)); err != nil {
			t.Fatal(err)
		}
	}

	listed := map[string]int{}
	t.Cleanup(func() { listDir = os.ReadDir })
	listDir = func(dir string) ([]os.DirEntry, error) {
		real, err := filepath.EvalSymlinks(dir)
		if err != nil {
			t.Errorf("listed %s, which does not resolve", dir)
		}
		listed[real]++
		return os.ReadDir(dir)
	}

	cache := newDirCache(root)
	for _, pattern := range []string{"alias1/**", "alias2/**", "alias3/**", "tree/*/*/*/*/*/*/x"} {
		got, err := expandMemberGlob(root, pattern, cache)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) == 0 {
			t.Errorf("%s matched nothing", pattern)
		}
	}
	for dir, count := range listed {
		if count > 1 {
			t.Errorf("listed %s %d times", dir, count)
		}
	}
	// Each spelling resolved stands for one step of the walk: 69 when
	// aliases share their walks, over 11000 when every spelling walks
	// the tree again.
	if n := len(cache.real); n > 100 {
		t.Errorf("walked %d spellings of %d real directories", n, len(listed))
	}
}

// Cargo applies ".." lexically (verified with cargo 1.99), so two
// spellings of one directory followed by ".." lead to different parents
// and both must be walked.
func TestMemberGlobParentAfterSymlinkIsLexical(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"p/q", "p/x", "z/x"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join("..", "p", "q"), filepath.Join(root, "z", "l")); err != nil {
		t.Fatal(err)
	}
	got, err := expandMemberGlob(root, "*/[ql]/../x", newDirCache(root))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{filepath.Join(root, "p", "x"), filepath.Join(root, "z", "x")}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
