// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package java

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
)

// moduleSite is where a visited module sits on disk, as slash paths with
// links resolved. Module-relative checks see neither a link, which can
// carry a generator into hand-written sources under another name, nor a
// generator writing into another module's directory; these checks do.
type moduleSite struct {
	dir string
	// roots holds each source root as written and, where a link moves it,
	// where it resolves to.
	roots []string
	// buildDir is the build directory, "" when it cannot be located.
	buildDir   string
	rootsKnown bool
}

// siteOf locates a module's directories, given its resolved directory. A
// root behind a dangling link holds nothing yet, so its path as written
// stands for it.
func siteOf(dir string, scope *moduleContext) *moduleSite {
	site := &moduleSite{dir: filepath.ToSlash(dir), rootsKnown: scope.rootsKnown}
	for _, root := range scope.handwritten {
		written := filepath.Join(dir, filepath.FromSlash(root))
		site.roots = appendMissing(site.roots, []string{filepath.ToSlash(written)})
		if located, ok := locate(written); ok {
			site.roots = appendMissing(site.roots, []string{filepath.ToSlash(located)})
		}
	}
	if scope.buildDir != "" {
		if located, ok := locate(filepath.Join(dir, filepath.FromSlash(scope.buildDir))); ok {
			site.buildDir = filepath.ToSlash(located)
		}
	}
	return site
}

// locate returns where a path lies on disk: its deepest existing ancestor
// with links resolved, joined to the rest of the path. ok is false when
// that cannot be settled: an error other than absence, a dangling link on
// the way, or a location outside the workspace.
func locate(path string) (string, bool) {
	rest := ""
	for probe := filepath.Clean(path); ; probe = filepath.Dir(probe) {
		resolved, err := filepath.EvalSymlinks(probe)
		if err == nil {
			located := filepath.Join(resolved, rest)
			return located, withinWorkspace(located)
		}
		if !errors.Is(err, fs.ErrNotExist) || filepath.Dir(probe) == probe || !isAbsent(probe) {
			return "", false
		}
		rest = filepath.Join(filepath.Base(probe), rest)
	}
}

// safeOnDisk returns the generated sources that stay safe once links are
// followed and every module of the reactor is known.
func (w *reactorWalk) safeOnDisk(found []generatedSource) []generatedSource {
	var kept []generatedSource
	for _, source := range found {
		if dir, ok := w.moduleDirs[source.Module]; ok && w.safeAt(dir, source.Path) {
			kept = append(kept, source)
		}
	}
	return kept
}

// safeInEveryModule reports whether a flat-list path is safe on disk in
// every module, since a consumer applies it in each.
func (w *reactorWalk) safeInEveryModule(path string) bool {
	for dir := range w.sites {
		if !w.safeAt(dir, path) {
			return false
		}
	}
	return true
}

// safeAt reports whether a directory, relative to the module at dir, is
// safe to report on disk. It must lie where it reads, through no link,
// since past a link even the build directory may hold sources; overlap no
// module's source root, links resolved; and, inside any module whose
// source roots are not all known, the generating module and its ancestors
// included, lie in a build directory. An unseen root may sit anywhere else
// in such a module, a reactor root's included, while mvn clean empties a
// build directory, so none holds hand-written code. Containment in a
// module is compared without regard to case, which only withholds more.
func (w *reactorWalk) safeAt(dir, rel string) bool {
	written := filepath.Join(dir, filepath.FromSlash(rel))
	located, ok := locate(written)
	if !ok || located != written {
		return false
	}
	at := filepath.ToSlash(located)
	uncertain, buildOutput := false, false
	for _, site := range w.sites {
		for _, root := range site.roots {
			if overlaps(at, root) {
				return false
			}
		}
		if !site.rootsKnown && within(strings.ToLower(at), strings.ToLower(site.dir)) {
			uncertain = true
		}
		if withinBuildDir(at, site.buildDir) {
			buildOutput = true
		}
	}
	return !uncertain || buildOutput
}

// within reports whether dir is base or lies inside it.
func within(dir, base string) bool {
	return dir == base || strings.HasPrefix(dir, base+"/")
}
