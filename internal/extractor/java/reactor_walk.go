// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package java

import (
	"path"
	"path/filepath"
	"strings"
)

// walkReactor collects generated sources from the project and every module
// beneath it. Generators usually sit in leaf modules, below an aggregator
// that declares none, so the walk descends through nested reactors rather
// than stopping at direct modules. It reaches every readable module, with
// no depth limit, because the flat list is only safe once every module's
// source directories are known.
func walkReactor(projectPath string, pom *POM) *reactorWalk {
	walk := &reactorWalk{
		visited:          make(map[string]visitMode),
		seen:             make(map[generatedSource]bool),
		handwritten:      make(map[string]bool),
		uncertainOutputs: make(map[string]bool),
		sites:            make(map[string]*moduleSite),
		moduleDirs:       make(map[string]string),
	}
	walk.visit(projectPath, ".", pom, fullVisit)
	walk.found = walk.safeOnDisk(walk.found)
	return walk
}

// visitMode says how much of a module the walk records.
type visitMode int

const (
	// inspectOnly records a module's source roots but not its generators.
	// A module that only a profile not known to be active declares may not
	// join the build, but its sources sit in the workspace either way.
	inspectOnly visitMode = iota + 1
	// fullVisit records the module's generators too.
	fullVisit
)

// reactorWalk is the state of one traversal of a reactor.
type reactorWalk struct {
	visited map[string]visitMode
	seen    map[generatedSource]bool
	found   []generatedSource
	// handwritten holds every module's known source directories,
	// module-relative.
	handwritten map[string]bool
	// uncertainOutputs holds the build directory of every module whose
	// source roots are not all known, "" for one that will not resolve.
	uncertainOutputs map[string]bool
	// incomplete reports a declared module the walk could not read, whose
	// source roots are therefore unknown.
	incomplete bool
	// sites holds where each visited module sits on disk, by directory key,
	// and moduleDirs the directory key of each module.
	sites      map[string]*moduleSite
	moduleDirs map[string]string
}

// reactorSafeDirs returns the distinct generated paths, leaving out any
// that overlaps a source directory of some module. The flat list drops
// the module each path belongs to, so a consumer applies it across the
// reactor: a path that is generated output in one module but holds, or
// sits inside, a source directory in another would exclude real code
// there. A module whose source roots are not all known could have one at
// any path outside its build directory, so a listed path must lie inside
// the build directory of every such module, and a module the walk could
// not read at all leaves no path safe. On disk, a listed path must be safe
// in every module it is applied to. A path left out stays in
// generated_sources, which keeps the module.
func (w *reactorWalk) reactorSafeDirs() []string {
	if w.incomplete {
		return nil
	}
	var dirs []string
	listed := make(map[string]bool)
	for _, source := range w.found {
		if listed[source.Path] || overlapsAny(source.Path, w.handwritten) ||
			!w.outputEverywhereUncertain(source.Path) || !w.safeInEveryModule(source.Path) {
			continue
		}
		listed[source.Path] = true
		dirs = append(dirs, source.Path)
	}
	return dirs
}

// outputEverywhereUncertain reports whether dir is, or lies inside, the
// build directory of every module whose source roots are not all known.
func (w *reactorWalk) outputEverywhereUncertain(dir string) bool {
	for buildDir := range w.uncertainOutputs {
		if !withinBuildDir(dir, buildDir) {
			return false
		}
	}
	return true
}

// overlapsAny reports whether dir overlaps one of the source directories.
func overlapsAny(dir string, handwritten map[string]bool) bool {
	for source := range handwritten {
		if overlaps(dir, source) {
			return true
		}
	}
	return false
}

// overlaps reports whether two module-relative directories are the same or
// one contains the other. Excluding a generated directory that contains a
// source directory hides all of it; excluding one inside a source
// directory hides whatever hand-written files share its packages. Case is
// ignored, since Windows and macOS file systems usually ignore it, and
// over-matching only withholds an entry.
func overlaps(a, b string) bool {
	a, b = strings.ToLower(a), strings.ToLower(b)
	return a == b || strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/")
}

// visit records a module's source directories and, on a full visit, its
// generated sources, then descends into the modules it declares: fully
// into its own and those its active profiles add, and for source roots
// only into those its other profiles declare or its own entries name under
// another profile's properties. Module entries are interpolated as Maven
// interpolates them, then read through loadModulePOM, which keeps every
// path in the workspace. Each directory is visited once per mode, by its
// resolved path, so a module graph that loops, through symlinks or
// otherwise, ends; a module first reached through a profile is visited
// again in full if the build declares it too.
func (w *reactorWalk) visit(dir, module string, pom *POM, mode visitMode) {
	key := directoryKey(dir)
	if w.visited[key] >= mode {
		return
	}
	w.visited[key] = mode

	scope := newModuleContext(dir, module, pom)
	if w.sites[key] == nil {
		w.sites[key] = siteOf(key, scope)
	}
	w.moduleDirs[module] = key
	if !scope.rootsKnown {
		w.uncertainOutputs[scope.buildDir] = true
	}
	for _, source := range scope.handwritten {
		w.handwritten[source] = true
	}
	if mode == fullVisit {
		for _, source := range scope.sources() {
			if !w.seen[source] {
				w.seen[source] = true
				w.found = append(w.found, source)
			}
		}
	}

	w.visitModules(dir, module, scope.modulePaths(declaredModules(scope.chain[0])), mode)
	w.visitModules(dir, module, scope.alternateModulePaths(declaredModules(scope.chain[0])), inspectOnly)
	w.visitModules(dir, module, scope.profileModulePaths(scope.chain[0]), inspectOnly)
}

// modulePaths interpolates module entries against the declaring module's
// properties.
func (m *moduleContext) modulePaths(entries []string) []string {
	paths := make([]string, len(entries))
	for i, entry := range entries {
		paths[i] = modulePathOf(entry, m.props)
	}
	return paths
}

// alternateModulePaths returns the paths module entries take with any one
// profile's properties merged in, for source roots only: under a profile
// the action does not see active, an entry may name another module of the
// reactor. An entry resting on properties profiles could set together
// comes back empty, which never loads, so the walk counts it unread.
func (m *moduleContext) alternateModulePaths(entries []string) []string {
	var paths []string
	for _, entry := range entries {
		if m.isAmbiguous(entry, nil) {
			paths = appendMissing(paths, []string{""})
			continue
		}
		for _, read := range m.everyProfileReading()[1:] {
			paths = appendMissing(paths, []string{modulePathOf(entry, read.props)})
		}
	}
	return paths
}

// profileModulePaths returns the module paths the POM's profiles declare,
// each read in every context the profile can take effect in: with its own
// properties merged in, as when it activates, and alongside the profiles
// already active where that reading resolves. An entry resting on
// properties another profile could set as well comes back empty.
func (m *moduleContext) profileModulePaths(pom *POM) []string {
	var paths []string
	profiles := profilesOf(pom)
	for i := range profiles {
		profile := &profiles[i]
		if profile.Modules == nil {
			continue
		}
		for _, entry := range profile.Modules.Module {
			if m.isAmbiguous(entry, profile) {
				paths = appendMissing(paths, []string{""})
				continue
			}
			for _, read := range m.profileReadings(0, *profile) {
				if read.optional && isUnresolvedPlaceholder(resolveProperty(entry, read.props)) {
					continue
				}
				paths = appendMissing(paths, []string{modulePathOf(entry, read.props)})
			}
		}
	}
	return paths
}

// modulePathOf interpolates one module entry. One that leads with
// ${basedir} and a separator becomes relative to the module, so the
// workspace guard still judges it. With no separator Maven's string join
// names a sibling of the module rather than a directory inside it, which
// the action does not follow: the entry comes back empty, which never
// loads, so the walk counts it unread.
func modulePathOf(entry string, props map[string]string) string {
	resolved := resolvedIn(entry, props)
	rest, rooted := strings.CutPrefix(resolved, moduleRoot)
	if !rooted {
		return resolved
	}
	if rest != "" && rest[0] != '/' && rest[0] != '\\' {
		return ""
	}
	return "." + rest
}

// visitModules visits a module's declared children. One that cannot be
// read leaves the walk incomplete.
func (w *reactorWalk) visitModules(dir, module string, children []string, mode visitMode) {
	for _, child := range children {
		childDir, childPOM, ok := loadModulePOM(dir, child)
		if !ok {
			w.incomplete = true
			continue
		}
		w.visit(childDir, path.Join(module, filepath.ToSlash(child)), childPOM, mode)
	}
}

// declaredModules returns the modules a POM declares for every build.
func declaredModules(pom *POM) []string {
	if pom.Modules == nil {
		return nil
	}
	return pom.Modules.Module
}

// directoryKey identifies a directory by its resolved path, falling back to
// the cleaned path when it cannot be resolved.
func directoryKey(dir string) string {
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		return resolved
	}
	return filepath.Clean(dir)
}

// parentKey identifies a POM in a parent chain by its directory and
// artifactId, enough to tell when a chain comes back to a POM it passed.
func parentKey(dir string, pom *POM) string {
	return directoryKey(dir) + "\x00" + strings.TrimSpace(pom.ArtifactID)
}
