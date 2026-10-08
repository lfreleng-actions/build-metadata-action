// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package rust

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

// PublishablePackage is a workspace package that cargo publish would
// upload. ManifestPath is relative to the project directory, the form
// rust-crate-publish-action takes as its manifest_path input.
type PublishablePackage struct {
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	ManifestPath string   `json:"manifest_path"`
	Registries   []string `json:"registries,omitempty"`
}

// Values reach outputs and, through them, other workflows, so each
// must match a conservative pattern or the package gets left out.
var (
	// Cargo also requires a letter or "_" first, in package and registry
	// names alike (cargo 1.99 rejects "1demo" and "-demo").
	crateNamePattern    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)
	manifestPathPattern = regexp.MustCompile(`^[A-Za-z0-9._/+-]+$`)

	semverIdentifierPattern = regexp.MustCompile(`^[0-9A-Za-z-]+$`)
	semverNumericPattern    = regexp.MustCompile(`^[0-9]+$`)
)

// parseCrateVersion returns version as cargo reads it, and whether
// cargo accepts it: SemVer MAJOR.MINOR.PATCH, each fitting 64 bits with
// no leading zero, then optional "-" pre-release and "+" build
// identifiers, dot-separated, non-empty and within [0-9A-Za-z-], where a
// numeric pre-release identifier has no leading zero. Cargo trims
// surrounding whitespace (all verified against cargo 1.99).
func parseCrateVersion(version string) (string, bool) {
	version = strings.TrimSpace(version)
	rest, build, hasBuild := strings.Cut(version, "+")
	if hasBuild && !validSemverIdentifiers(build, false) {
		return "", false
	}
	core, pre, hasPre := strings.Cut(rest, "-")
	if hasPre && !validSemverIdentifiers(pre, true) {
		return "", false
	}
	numbers := strings.Split(core, ".")
	if len(numbers) != 3 {
		return "", false
	}
	for _, number := range numbers {
		if !semverNumericPattern.MatchString(number) || (len(number) > 1 && number[0] == '0') {
			return "", false
		}
		if _, err := strconv.ParseUint(number, 10, 64); err != nil {
			return "", false
		}
	}
	return version, true
}

func validSemverIdentifiers(identifiers string, preRelease bool) bool {
	for _, identifier := range strings.Split(identifiers, ".") {
		if !semverIdentifierPattern.MatchString(identifier) {
			return false
		}
		if preRelease && len(identifier) > 1 && identifier[0] == '0' && semverNumericPattern.MatchString(identifier) {
			return false
		}
	}
	return true
}

type dependencyTables struct {
	Dependencies      map[string]interface{} `toml:"dependencies"`
	DevDependencies   map[string]interface{} `toml:"dev-dependencies"`
	BuildDependencies map[string]interface{} `toml:"build-dependencies"`
}

type memberManifest struct {
	dependencyTables
	Package struct {
		Name      string      `toml:"name"`
		Version   interface{} `toml:"version"`
		Publish   interface{} `toml:"publish"`
		Workspace string      `toml:"workspace"`
	} `toml:"package"`
	Target map[string]dependencyTables `toml:"target"`
}

type rootManifest struct {
	memberManifest
	Workspace struct {
		Members      []string               `toml:"members"`
		Exclude      []string               `toml:"exclude"`
		Dependencies map[string]interface{} `toml:"dependencies"`
		Package      struct {
			Version interface{} `toml:"version"`
			Publish interface{} `toml:"publish"`
		} `toml:"package"`
	} `toml:"workspace"`
}

// workspaceWalk collects the members of one workspace the way cargo
// does: the root package, the expanded members globs, and the path
// dependencies of every member that lie inside the root, less anything
// under an exclude entry that no members entry names. A package outside
// the root joins, as in cargo, only when its package.workspace key
// points back at the root, and only inside boundary.
type workspaceWalk struct {
	root     string
	realRoot string
	boundary string
	manifest *rootManifest
	dirs     *dirCache       // directory listings shared by every members entry
	seen     map[string]bool // manifests by path relative to the root
	visited  map[string]bool // manifests by real path, so no alias repeats one
	packages []PublishablePackage
	warnings []string
}

// publishablePackages lists the packages of the workspace (or single
// package) rooted at manifestPath that cargo publish would upload.
func publishablePackages(manifestPath string) ([]PublishablePackage, []string) {
	walk := &workspaceWalk{
		manifest: &rootManifest{},
		seen:     map[string]bool{},
		visited:  map[string]bool{},
		packages: []PublishablePackage{},
	}
	// The real-path rules (absolute entries, back-pointers, the
	// boundary) compare absolute paths; the action passes a relative one.
	root, err := filepath.Abs(filepath.Dir(manifestPath))
	if err != nil {
		return walk.packages, nil
	}
	walk.root = root
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return walk.packages, nil
	}
	walk.realRoot = realRoot
	// The toolchain search's rule: under GitHub Actions a root whose
	// real path lies outside the workspace, or a workspace that cannot
	// be resolved, ends the scan before any manifest is read.
	boundary, escapes := searchBoundary(realRoot)
	if escapes {
		return walk.packages, []string{"scan skipped: the project directory resolves outside the workspace"}
	}
	if boundary == "" {
		// Outside GitHub Actions members may lie anywhere, as in cargo.
		boundary = filepath.VolumeName(realRoot) + string(filepath.Separator)
	}
	walk.boundary = boundary
	meta, err := toml.DecodeFile(manifestPath, walk.manifest)
	if err != nil {
		return walk.packages, nil
	}

	walk.seen["Cargo.toml"] = true
	if realManifest, err := filepath.EvalSymlinks(filepath.Join(root, filepath.Base(manifestPath))); err == nil {
		walk.visited[realManifest] = true
	}
	walk.consider("Cargo.toml", &walk.manifest.memberManifest)
	if meta.IsDefined("workspace") {
		walk.followPathDependencies(".", &walk.manifest.memberManifest)
		for _, entry := range walk.manifest.Workspace.Members {
			walk.expandMembers(entry)
		}
	}

	sort.Slice(walk.packages, func(i, j int) bool {
		return walk.packages[i].ManifestPath < walk.packages[j].ManifestPath
	})
	return walk.packages, walk.warnings
}

func (w *workspaceWalk) warn(manifest, problem string) {
	if manifestPathPattern.MatchString(manifest) {
		w.warnings = append(w.warnings, manifest+": "+problem)
	} else {
		w.warnings = append(w.warnings, "a workspace member: "+problem)
	}
}

func (w *workspaceWalk) expandMembers(entry string) {
	relativeTo := w.root
	if filepath.IsAbs(entry) {
		// cargo matches an absolute entry against its real root
		relativeTo = w.realRoot
	}
	if w.dirs == nil {
		w.dirs = newDirCache(w.boundary)
	}
	matches, err := expandMemberGlob(w.root, entry, w.dirs)
	if err != nil {
		w.warnings = append(w.warnings, "Cargo.toml: a workspace members entry is not a valid glob")
		return
	}
	for _, match := range matches {
		if rel, err := filepath.Rel(relativeTo, match); err == nil {
			w.visit(rel)
		}
	}
}

// visit adds the package in dir, relative to the root, and follows its
// path dependencies.
func (w *workspaceWalk) visit(dir string) {
	manifest := path.Join(filepath.ToSlash(filepath.Clean(dir)), "Cargo.toml")
	if w.seen[manifest] {
		return
	}
	w.seen[manifest] = true
	outside := strings.HasPrefix(manifest, "../")
	if w.excluded(manifest) {
		return
	}

	resolved, err := filepath.EvalSymlinks(filepath.Join(w.root, filepath.FromSlash(manifest)))
	if err != nil || w.visited[resolved] {
		return
	}
	limit := w.realRoot
	if outside {
		limit = w.boundary
	}
	if !isWithin(limit, resolved) {
		if !outside {
			w.warn(manifest, "links outside the workspace; skipping it")
		}
		return
	}
	var member memberManifest
	if _, err := toml.DecodeFile(resolved, &member); err != nil {
		w.warn(manifest, "cannot be parsed; skipping it")
		return
	}
	if outside && !w.namesRoot(filepath.Dir(resolved), member.Package.Workspace) {
		return
	}
	w.visited[resolved] = true
	w.consider(manifest, &member)
	w.followPathDependencies(path.Dir(manifest), &member)
}

// namesRoot reports whether a package.workspace value, relative to the
// package directory, leads to this workspace root.
func (w *workspaceWalk) namesRoot(packageDir, workspace string) bool {
	if workspace == "" {
		return false
	}
	if !filepath.IsAbs(workspace) {
		workspace = filepath.Join(packageDir, workspace)
	}
	real, err := filepath.EvalSymlinks(workspace)
	return err == nil && real == w.realRoot
}

// excluded applies cargo's rule: a package under an exclude entry is no
// member unless a members entry names it or a parent, literally. An
// absolute entry counts from the real root, as in cargo.
func (w *workspaceWalk) excluded(manifest string) bool {
	under := func(entries []string) bool {
		for _, entry := range entries {
			if filepath.IsAbs(entry) {
				rel, err := filepath.Rel(w.realRoot, entry)
				if err != nil {
					continue
				}
				entry = rel
			}
			prefix := path.Clean(filepath.ToSlash(entry))
			if prefix == "." || strings.HasPrefix(manifest, prefix+"/") {
				return true
			}
		}
		return false
	}
	return under(w.manifest.Workspace.Exclude) && !under(w.manifest.Workspace.Members)
}

func (w *workspaceWalk) followPathDependencies(dir string, member *memberManifest) {
	tables := []dependencyTables{member.dependencyTables}
	targets := make([]string, 0, len(member.Target))
	for target := range member.Target {
		targets = append(targets, target)
	}
	sort.Strings(targets)
	for _, target := range targets {
		tables = append(tables, member.Target[target])
	}

	for _, table := range tables {
		for _, deps := range []map[string]interface{}{table.Dependencies, table.DevDependencies, table.BuildDependencies} {
			names := make([]string, 0, len(deps))
			for name := range deps {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				if depDir, ok := w.pathDependency(dir, name, deps[name]); ok {
					w.visit(depDir)
				}
			}
		}
	}
}

// pathDependency returns the directory, relative to the root, of a path
// dependency, resolving { workspace = true } through
// [workspace.dependencies], whose paths are relative to the root.
func (w *workspaceWalk) pathDependency(dir, name string, spec interface{}) (string, bool) {
	table, ok := spec.(map[string]interface{})
	if !ok {
		return "", false
	}
	if inherit, ok := table["workspace"].(bool); ok && inherit {
		inherited, ok := w.manifest.Workspace.Dependencies[name].(map[string]interface{})
		if !ok {
			return "", false
		}
		table, dir = inherited, "."
	}
	depPath, ok := table["path"].(string)
	if !ok || depPath == "" {
		return "", false
	}
	if filepath.IsAbs(depPath) {
		// Cargo normalises an absolute path and counts it as a member
		// when it lies under the real workspace root; visit rejects
		// the rest.
		rel, err := filepath.Rel(w.realRoot, filepath.Clean(depPath))
		if err != nil {
			return "", false
		}
		return filepath.ToSlash(rel), true
	}
	return path.Join(dir, filepath.ToSlash(depPath)), true
}

// consider records the package in manifest if cargo publish would
// upload it: it has a version, publish is not false or [], and every
// reported value is well formed.
func (w *workspaceWalk) consider(manifest string, member *memberManifest) {
	pkg := member.Package
	if pkg.Name == "" {
		return
	}
	if !crateNamePattern.MatchString(pkg.Name) {
		w.warn(manifest, "has an invalid package name; skipping it")
		return
	}
	if !manifestPathPattern.MatchString(manifest) {
		w.warn(manifest, "has a path outside [A-Za-z0-9._/+-]; skipping it")
		return
	}

	// The action reads [workspace.package] from the selected manifest
	// alone. A field inheriting from a workspace it does not define
	// belongs to a root above path_prefix, whose value is unknown here.
	workspacePackage := w.manifest.Workspace.Package
	if (inheritsFromWorkspace(pkg.Version) && workspacePackage.Version == nil) ||
		(inheritsFromWorkspace(pkg.Publish) && workspacePackage.Publish == nil) {
		w.warn(manifest, "inherits version or publish from a workspace root above the selected manifest; skipping it")
		return
	}

	// Without a version, cargo (1.75 and later) treats the package as
	// publish = false.
	version, ok := inheritedValue(pkg.Version, w.manifest.Workspace.Package.Version).(string)
	if !ok {
		return
	}
	version, ok = parseCrateVersion(version)
	if !ok {
		w.warn(manifest, "has a version cargo rejects; skipping it")
		return
	}

	entry := PublishablePackage{Name: pkg.Name, Version: version, ManifestPath: manifest}
	switch publish := inheritedValue(pkg.Publish, w.manifest.Workspace.Package.Publish).(type) {
	case nil:
	case bool:
		if !publish {
			return
		}
	case []interface{}:
		for _, registry := range publish {
			if name, ok := registry.(string); ok && crateNamePattern.MatchString(name) {
				entry.Registries = append(entry.Registries, name)
			}
		}
		if len(entry.Registries) < len(publish) {
			w.warn(manifest, fmt.Sprintf("ignoring %d invalid registry name(s)", len(publish)-len(entry.Registries)))
		}
		if len(entry.Registries) == 0 {
			return
		}
	default:
		w.warn(manifest, "has an invalid publish setting; skipping it")
		return
	}
	w.packages = append(w.packages, entry)
}

// inheritsFromWorkspace reports whether a field is the
// `{ workspace = true }` marker.
func inheritsFromWorkspace(value interface{}) bool {
	m, ok := value.(map[string]interface{})
	if !ok {
		return false
	}
	inherit, ok := m["workspace"].(bool)
	return ok && inherit
}

// applyPublishable records the publishable packages and reports any
// warnings as workflow annotations.
func applyPublishable(manifestPath string, languageSpecific map[string]interface{}) {
	packages, warnings := publishablePackages(manifestPath)
	for _, warning := range warnings {
		fmt.Fprintf(os.Stderr, "::warning::Rust workspace %s\n", warning)
	}
	languageSpecific["publishable_packages"] = packages
	languageSpecific["publishable_package_count"] = len(packages)
}
