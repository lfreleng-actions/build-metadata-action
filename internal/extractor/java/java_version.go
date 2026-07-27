// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package java

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"

	"github.com/lfreleng-actions/build-metadata-action/internal/extractor"
)

// maxParentDepth bounds how far up the on-disk parent chain the extractor
// walks when resolving inherited properties, guarding against unexpectedly
// deep or cyclic relativePath links. It is set generously so large
// multi-module builds with several on-disk parent levels still resolve their
// Java level, while still terminating on pathological or cyclic chains.
const maxParentDepth = 10

// javaVersionPropertyKeys lists the POM property keys that declare the Java
// language level, in precedence order. maven.compiler.release is the modern
// authoritative form (Java 9+). Of the older source/target pair, target is
// preferred: the bytecode target dictates the minimum JDK the artifact needs,
// so it is the stricter constraint when source and target differ. java.version
// is the Spring Boot parent convention.
var javaVersionPropertyKeys = []string{
	"maven.compiler.release",
	"maven.compiler.target",
	"maven.compiler.source",
	"java.version",
}

// applyPOMJavaVersion resolves the project's Java language level and records
// both the value and the source it came from. Resolution mirrors how Maven
// itself settles the level: first the POM's effective properties, which
// already fold in values inherited from on-disk parent POMs (resolved
// up-front via effectiveProperties), then the POM's own
// maven-compiler-plugin configuration, and finally (for aggregator POMs
// that declare no level themselves) the reactor modules. ONAP-style roots
// inherit the level from a shared *-parent module, so the reactor fallback
// is what makes java_version resolvable at repository root.
func (e *MavenExtractor) applyPOMJavaVersion(projectPath string, pom *POM, metadata *extractor.ProjectMetadata) {
	props := effectiveProperties(projectPath, pom, 0)

	if version, source := javaVersionFromProperties(props); version != "" {
		setJavaVersion(metadata, version, source)
		return
	}
	if version, source := javaVersionFromCompilerPlugin(pom, props); version != "" {
		setJavaVersion(metadata, version, source)
		return
	}
	if version, source := javaVersionFromModules(projectPath, pom); version != "" {
		setJavaVersion(metadata, version, source)
		return
	}
}

// setJavaVersion records the resolved Java level and the key or location it
// was derived from (e.g. "maven.compiler.release", "module:cps-parent").
func setJavaVersion(metadata *extractor.ProjectMetadata, version, source string) {
	metadata.LanguageSpecific["version"] = version
	metadata.LanguageSpecific["version_source"] = source
}

// javaVersionFromProperties returns the first declared Java version from the
// supplied property map, resolving ${...} placeholders against the same map,
// plus the property key it came from. A value that still contains an
// unresolved ${...} placeholder after resolution (an undefined or cyclic
// reference) is skipped, so detection falls through to the next candidate
// instead of emitting an invalid version.
func javaVersionFromProperties(props map[string]string) (string, string) {
	for _, key := range javaVersionPropertyKeys {
		if value, ok := props[key]; ok && value != "" {
			resolved := resolveProperty(value, props)
			if isUnresolvedPlaceholder(resolved) {
				continue
			}
			return resolved, key
		}
	}
	return "", ""
}

// isUnresolvedPlaceholder reports whether a resolved value still contains a
// ${...} placeholder (for example an undefined or cyclic property reference).
// Such a value is not a usable Java version, so callers skip it and fall back
// to the next candidate rather than emitting an invalid version.
func isUnresolvedPlaceholder(value string) bool {
	return strings.Contains(value, mavenPropertyOpen)
}

// javaVersionFromCompilerPlugin returns the Java version declared in the
// maven-compiler-plugin <configuration> (release, then target, then source),
// resolving placeholders against props. Direct <build><plugins>
// configuration takes precedence over managed defaults declared under
// <build><pluginManagement><plugins>.
func javaVersionFromCompilerPlugin(pom *POM, props map[string]string) (string, string) {
	if pom.Build == nil {
		return "", ""
	}
	if version, source := compilerPluginJavaVersion(pom.Build.Plugins, props); version != "" {
		return version, source
	}
	if pom.Build.PluginManagement != nil {
		if version, source := compilerPluginJavaVersion(pom.Build.PluginManagement.Plugins, props); version != "" {
			return version, source
		}
	}
	return "", ""
}

// compilerPluginJavaVersion scans a <plugins> list for the
// maven-compiler-plugin and returns the first declared Java level from its
// <configuration> (release, then target, then source). target is preferred
// over source because the bytecode target is the stricter constraint on the
// JDK when the two differ.
func compilerPluginJavaVersion(plugins *Plugins, props map[string]string) (string, string) {
	if plugins == nil {
		return "", ""
	}
	for _, plugin := range plugins.Plugin {
		if plugin.ArtifactID != "maven-compiler-plugin" || plugin.Configuration == nil {
			continue
		}
		cfg := plugin.Configuration
		candidates := []struct{ value, source string }{
			{cfg.Release, "maven-compiler-plugin/release"},
			{cfg.Target, "maven-compiler-plugin/target"},
			{cfg.Source, "maven-compiler-plugin/source"},
		}
		for _, candidate := range candidates {
			if candidate.value != "" {
				resolved := resolveProperty(candidate.value, props)
				if isUnresolvedPlaceholder(resolved) {
					continue
				}
				return resolved, candidate.source
			}
		}
	}
	return "", ""
}

// javaVersionFromModules scans an aggregator POM's direct reactor modules
// for a declared Java level, returning the first found together with a
// "module:<name>" source tag. This resolves the common ONAP layout where
// the root aggregator declares no compiler level and a shared *-parent
// module carries it.
func javaVersionFromModules(projectPath string, pom *POM) (string, string) {
	if pom.Modules == nil {
		return "", ""
	}
	for _, module := range pom.Modules.Module {
		// Reject absolute module paths: filepath.Join would discard
		// projectPath and read a POM from outside the workspace.
		if module == "" || filepath.IsAbs(module) {
			continue
		}
		moduleDir := filepath.Join(projectPath, module)
		// Reject modules whose "../" segments resolve outside the trusted
		// workspace root, so a crafted module cannot read a POM elsewhere
		// on the runner.
		if !withinWorkspace(moduleDir) {
			continue
		}
		modulePOM, ok := readPOM(filepath.Join(moduleDir, "pom.xml"))
		if !ok {
			continue
		}
		props := effectiveProperties(moduleDir, modulePOM, 0)
		// The blank return is the detection source (a property key or
		// plugin location), not an error; module scans deliberately report
		// "module:<name>" as the source instead.
		// aislop-ignore-next-line ai-slop/swallowed-exception -- second return is a source label, not an error
		if version, _ := javaVersionFromProperties(props); version != "" {
			return version, "module:" + module
		}
		// aislop-ignore-next-line ai-slop/swallowed-exception -- second return is a source label, not an error
		if version, _ := javaVersionFromCompilerPlugin(modulePOM, props); version != "" {
			return version, "module:" + module
		}
	}
	return "", ""
}

// effectiveProperties returns the property map visible to a POM after
// Maven-style inheritance: properties from on-disk parent POMs (resolved via
// relativePath, up to maxParentDepth) overlaid by the POM's own properties,
// which take precedence. Parents that are not present locally are skipped so
// the function degrades gracefully outside a full reactor checkout.
func effectiveProperties(projectPath string, pom *POM, depth int) map[string]string {
	merged := make(map[string]string)
	if pom.Parent != nil && depth < maxParentDepth {
		if parentDir, parentPOM, ok := loadParentPOM(projectPath, pom); ok {
			for key, value := range effectiveProperties(parentDir, parentPOM, depth+1) {
				merged[key] = value
			}
		}
	}
	for key, value := range pom.Properties.Entries {
		merged[key] = value
	}
	return merged
}

// loadParentPOM resolves and parses a POM's parent from its relativePath
// (defaulting to "../pom.xml"), returning the parent's directory (for
// further relativePath resolution) and the parsed POM. It returns ok=false
// when no local parent file exists, which is the normal case for a bare
// module checkout or a repository-root aggregator.
func loadParentPOM(projectPath string, pom *POM) (string, *POM, bool) {
	if pom.Parent == nil {
		return "", nil, false
	}
	relativePath := pom.Parent.RelativePath
	if relativePath == "" {
		relativePath = "../pom.xml"
	}
	// Reject absolute relativePath values: filepath.Join would discard
	// projectPath and could read an arbitrary file on the runner.
	if filepath.IsAbs(relativePath) {
		return "", nil, false
	}
	candidate := filepath.Join(projectPath, relativePath)
	// Reject relativePath values whose "../" segments resolve outside the
	// trusted workspace root. Parent POMs legitimately live above the
	// current module, so the boundary is the workspace rather than
	// projectPath.
	if !withinWorkspace(candidate) {
		return "", nil, false
	}
	// When a workspace boundary is active, resolve symlinks and confirm the
	// candidate still stays inside the workspace before os.Stat follows any
	// link. A symlink under GITHUB_WORKSPACE that points outside would
	// otherwise be followed by the stat below, leaking filesystem structure
	// even though readPOM later rejects the out-of-workspace read.
	if _, bounded := workspaceRoot(); bounded {
		resolved, err := filepath.EvalSymlinks(candidate)
		if err != nil || !withinWorkspace(resolved) {
			return "", nil, false
		}
	}
	info, err := os.Stat(candidate)
	if err != nil {
		return "", nil, false
	}
	if info.IsDir() {
		candidate = filepath.Join(candidate, "pom.xml")
	}
	parentPOM, ok := readPOM(candidate)
	if !ok {
		return "", nil, false
	}
	return filepath.Dir(candidate), parentPOM, true
}

// workspaceRoot returns the trusted workspace boundary (GITHUB_WORKSPACE)
// and whether it is set. On CI runners a repository's POM values, such as
// <module> and <relativePath>, may be attacker-controlled through a pull
// request, so resolved POM paths are confined to this root. Outside CI (no
// GITHUB_WORKSPACE) the checkout is developer-owned and no boundary applies.
func workspaceRoot() (string, bool) {
	ws := os.Getenv("GITHUB_WORKSPACE")
	if ws == "" {
		return "", false
	}
	return filepath.Clean(ws), true
}

// withinWorkspace reports whether candidate is the workspace root or a
// descendant of it. When no workspace boundary is configured it returns true
// so local extraction and unit tests operate on paths outside any workspace.
func withinWorkspace(candidate string) bool {
	root, ok := workspaceRoot()
	if !ok {
		return true
	}
	candidate = filepath.Clean(candidate)
	if candidate == root {
		return true
	}
	return strings.HasPrefix(candidate, root+string(os.PathSeparator))
}

// readPOM reads and unmarshals a pom.xml, returning ok=false on any error so
// callers can skip missing or malformed POMs without failing extraction.
//
// When a workspace boundary is active (CI), the POM path can be derived from
// attacker-controlled <module>/<relativePath> values, so a symlinked pom.xml
// or a symlinked parent directory could still escape the lexical
// withinWorkspace prefix check applied by callers. readPOM therefore rejects
// non-regular files and confirms the fully symlink-resolved path stays inside
// the workspace root before reading. Outside CI (no GITHUB_WORKSPACE) the
// checkout is developer-owned and no boundary applies.
func readPOM(pomPath string) (*POM, bool) {
	if _, bounded := workspaceRoot(); bounded {
		info, err := os.Lstat(pomPath)
		if err != nil || !info.Mode().IsRegular() {
			return nil, false
		}
		resolved, err := filepath.EvalSymlinks(pomPath)
		if err != nil || !withinWorkspace(resolved) {
			return nil, false
		}
	}
	content, err := os.ReadFile(pomPath)
	if err != nil {
		return nil, false
	}
	var pom POM
	if err := xml.Unmarshal(content, &pom); err != nil {
		return nil, false
	}
	return &pom, true
}
