// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package java

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lfreleng-actions/build-metadata-action/internal/extractor"
)

// mavenPropertyOpen and mavenPropertyClose delimit a Maven property reference,
// such as the placeholder embedded in a dynamic version string. The brace
// characters are written as unicode escapes so source scanners that track
// brace nesting are not misled by these string literals.
const (
	mavenPropertyOpen  = "$\u007b"
	mavenPropertyClose = "\u007d"
)

// MavenExtractor extracts metadata from Maven projects
type MavenExtractor struct {
	extractor.BaseExtractor
}

// NewMavenExtractor creates a new Maven extractor
func NewMavenExtractor() *MavenExtractor {
	return &MavenExtractor{
		BaseExtractor: extractor.NewBaseExtractor("java-maven", 3),
	}
}

// POM represents a Maven Project Object Model
type POM struct {
	XMLName       xml.Name `xml:"project"`
	ModelVersion  string   `xml:"modelVersion"`
	GroupID       string   `xml:"groupId"`
	ArtifactID    string   `xml:"artifactId"`
	Version       string   `xml:"version"`
	Packaging     string   `xml:"packaging"`
	Name          string   `xml:"name"`
	Description   string   `xml:"description"`
	URL           string   `xml:"url"`
	InceptionYear string   `xml:"inceptionYear"`

	Parent         *Parent         `xml:"parent"`
	Properties     Properties      `xml:"properties"`
	Dependencies   *Dependencies   `xml:"dependencies"`
	DependencyMgmt *DependencyMgmt `xml:"dependencyManagement"`
	Build          *Build          `xml:"build"`
	Modules        *Modules        `xml:"modules"`
	Licenses       *Licenses       `xml:"licenses"`
	Developers     *Developers     `xml:"developers"`
	Contributors   *Contributors   `xml:"contributors"`
	SCM            *SCM            `xml:"scm"`
	Organization   *Organization   `xml:"organization"`
	Profiles       *Profiles       `xml:"profiles"`
}

// Parent represents a parent POM reference
type Parent struct {
	GroupID      string `xml:"groupId"`
	ArtifactID   string `xml:"artifactId"`
	Version      string `xml:"version"`
	RelativePath string `xml:"relativePath"`
}

// Properties represents Maven properties
type Properties struct {
	Entries map[string]string
}

// UnmarshalXML custom unmarshaler for properties
func (p *Properties) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	p.Entries = make(map[string]string)

	for {
		token, err := d.Token()
		if err != nil {
			return err
		}

		switch t := token.(type) {
		case xml.StartElement:
			var value string
			if err := d.DecodeElement(&value, &t); err != nil {
				return err
			}
			p.Entries[t.Name.Local] = value
		case xml.EndElement:
			if t == start.End() {
				return nil
			}
		}
	}
}

// Dependencies represents Maven dependencies
type Dependencies struct {
	Dependency []Dependency `xml:"dependency"`
}

// DependencyMgmt represents dependency management
type DependencyMgmt struct {
	Dependencies *Dependencies `xml:"dependencies"`
}

// Dependency represents a single Maven dependency
type Dependency struct {
	GroupID    string `xml:"groupId"`
	ArtifactID string `xml:"artifactId"`
	Version    string `xml:"version"`
	Scope      string `xml:"scope"`
	Type       string `xml:"type"`
	Classifier string `xml:"classifier"`
	Optional   bool   `xml:"optional"`
}

// Build represents the build configuration
type Build struct {
	SourceDirectory  string            `xml:"sourceDirectory"`
	FinalName        string            `xml:"finalName"`
	Plugins          *Plugins          `xml:"plugins"`
	PluginManagement *PluginManagement `xml:"pluginManagement"`
}

// PluginManagement represents the <build><pluginManagement> block, which
// declares managed plugin defaults that submodules (and the POM itself)
// inherit. Maven projects commonly configure the compiler level here.
type PluginManagement struct {
	Plugins *Plugins `xml:"plugins"`
}

// Plugins represents Maven plugins
type Plugins struct {
	Plugin []Plugin `xml:"plugin"`
}

// Plugin represents a single Maven plugin
type Plugin struct {
	GroupID       string               `xml:"groupId"`
	ArtifactID    string               `xml:"artifactId"`
	Version       string               `xml:"version"`
	Configuration *PluginConfiguration `xml:"configuration"`
}

// PluginConfiguration captures the subset of maven-compiler-plugin
// <configuration> that declares the Java language level. Projects that do
// not set the compiler level via properties often set it here instead.
type PluginConfiguration struct {
	Release string `xml:"release"`
	Source  string `xml:"source"`
	Target  string `xml:"target"`
}

// Modules represents Maven modules
type Modules struct {
	Module []string `xml:"module"`
}

// Licenses represents project licenses
type Licenses struct {
	License []License `xml:"license"`
}

// License represents a single license
type License struct {
	Name         string `xml:"name"`
	URL          string `xml:"url"`
	Distribution string `xml:"distribution"`
	Comments     string `xml:"comments"`
}

// Developers represents project developers
type Developers struct {
	Developer []Developer `xml:"developer"`
}

// Developer represents a single developer
type Developer struct {
	ID              string   `xml:"id"`
	Name            string   `xml:"name"`
	Email           string   `xml:"email"`
	URL             string   `xml:"url"`
	Organization    string   `xml:"organization"`
	OrganizationURL string   `xml:"organizationUrl"`
	Roles           []string `xml:"roles>role"`
}

// Contributors represents project contributors
type Contributors struct {
	Contributor []Developer `xml:"contributor"`
}

// SCM represents source control management
type SCM struct {
	Connection          string `xml:"connection"`
	DeveloperConnection string `xml:"developerConnection"`
	URL                 string `xml:"url"`
	Tag                 string `xml:"tag"`
}

// Organization represents the project organization
type Organization struct {
	Name string `xml:"name"`
	URL  string `xml:"url"`
}

// Profiles represents Maven profiles
type Profiles struct {
	Profile []Profile `xml:"profile"`
}

// Profile represents a single Maven profile
type Profile struct {
	ID         string      `xml:"id"`
	Activation *Activation `xml:"activation"`
}

// Activation represents profile activation conditions
type Activation struct {
	ActiveByDefault bool   `xml:"activeByDefault"`
	JDK             string `xml:"jdk"`
}

// Extract retrieves metadata from a Maven project
func (e *MavenExtractor) Extract(projectPath string) (*extractor.ProjectMetadata, error) {
	metadata := &extractor.ProjectMetadata{
		LanguageSpecific: make(map[string]interface{}),
	}

	pomPath := filepath.Join(projectPath, "pom.xml")
	if _, err := os.Stat(pomPath); err != nil {
		return nil, fmt.Errorf("pom.xml not found in %s", projectPath)
	}

	if err := e.extractFromPOM(pomPath, projectPath, metadata); err != nil {
		return nil, err
	}

	return metadata, nil
}

// extractFromPOM parses pom.xml and populates metadata from the resolved POM.
func (e *MavenExtractor) extractFromPOM(pomPath, projectPath string, metadata *extractor.ProjectMetadata) error {
	content, err := os.ReadFile(pomPath)
	if err != nil {
		return fmt.Errorf("failed to read pom.xml: %w", err)
	}

	var pom POM
	if err := xml.Unmarshal(content, &pom); err != nil {
		return fmt.Errorf("failed to parse pom.xml: %w", err)
	}

	resolvedPOM := e.resolveProperties(&pom)

	applyPOMCoreMetadata(resolvedPOM, metadata)
	applyPOMIdentifiers(resolvedPOM, metadata)
	applyPOMProperties(resolvedPOM, metadata)
	applyPOMDependencies(resolvedPOM, metadata)
	applyPOMBuildPlugins(resolvedPOM, metadata)
	applyPOMStructure(resolvedPOM, metadata)
	e.applyPOMJavaVersion(projectPath, resolvedPOM, metadata)
	applyPOMVersioningType(metadata)

	return nil
}

// applyPOMCoreMetadata maps top-level project fields, the first declared
// license, developer authors, and the SCM URL onto the shared metadata.
func applyPOMCoreMetadata(pom *POM, metadata *extractor.ProjectMetadata) {
	metadata.Name = pom.ArtifactID
	if pom.Name != "" {
		metadata.Name = pom.Name
	}
	metadata.Version = pom.Version
	metadata.Description = pom.Description
	metadata.Homepage = pom.URL
	metadata.VersionSource = "pom.xml"

	if pom.Licenses != nil && len(pom.Licenses.License) > 0 {
		metadata.License = pom.Licenses.License[0].Name
	}

	authors := make([]string, 0)
	if pom.Developers != nil {
		for _, dev := range pom.Developers.Developer {
			if dev.Name != "" {
				if dev.Email != "" {
					authors = append(authors, fmt.Sprintf("%s <%s>", dev.Name, dev.Email))
				} else {
					authors = append(authors, dev.Name)
				}
			}
		}
	}
	metadata.Authors = authors

	if pom.SCM != nil && pom.SCM.URL != "" {
		metadata.Repository = pom.SCM.URL
	}
}

// applyPOMIdentifiers records Maven coordinates and packaging, falling back to
// parent-inherited group and version when the child POM omits them.
func applyPOMIdentifiers(pom *POM, metadata *extractor.ProjectMetadata) {
	metadata.LanguageSpecific["group_id"] = pom.GroupID
	metadata.LanguageSpecific["artifact_id"] = pom.ArtifactID
	metadata.LanguageSpecific["packaging"] = pom.Packaging
	if metadata.LanguageSpecific["packaging"] == "" {
		metadata.LanguageSpecific["packaging"] = "jar"
	}
	metadata.LanguageSpecific["metadata_source"] = "pom.xml"
	metadata.LanguageSpecific["model_version"] = pom.ModelVersion

	if pom.Parent == nil {
		return
	}
	metadata.LanguageSpecific["has_parent"] = true
	metadata.LanguageSpecific["parent_group_id"] = pom.Parent.GroupID
	metadata.LanguageSpecific["parent_artifact_id"] = pom.Parent.ArtifactID
	metadata.LanguageSpecific["parent_version"] = pom.Parent.Version

	if metadata.Version == "" && pom.Parent.Version != "" {
		metadata.Version = pom.Parent.Version
		metadata.LanguageSpecific["version_from_parent"] = true
	}
	if pom.GroupID == "" && pom.Parent.GroupID != "" {
		metadata.LanguageSpecific["group_id"] = pom.Parent.GroupID
		metadata.LanguageSpecific["group_id_from_parent"] = true
	}
}

// applyPOMProperties records declared properties and derives the
// revision-based dynamic versioning hint from the well-known key. The
// Java language level is resolved separately by applyPOMJavaVersion so it
// can also consult compiler-plugin configuration and inherited POMs.
func applyPOMProperties(pom *POM, metadata *extractor.ProjectMetadata) {
	if len(pom.Properties.Entries) == 0 {
		return
	}
	metadata.LanguageSpecific["properties"] = pom.Properties.Entries
	metadata.LanguageSpecific["property_count"] = len(pom.Properties.Entries)

	if projVersion, ok := pom.Properties.Entries["revision"]; ok {
		metadata.LanguageSpecific["versioning_type"] = "dynamic"
		metadata.LanguageSpecific["version_property"] = "revision"
		if metadata.Version == mavenPropertyOpen+"revision"+mavenPropertyClose {
			metadata.Version = projVersion
		}
	}
}

// applyPOMDependencies records the dependency list and a per-scope tally,
// treating an unspecified scope as Maven's implicit "compile" scope.
func applyPOMDependencies(pom *POM, metadata *extractor.ProjectMetadata) {
	if pom.Dependencies == nil || len(pom.Dependencies.Dependency) == 0 {
		return
	}
	deps := make([]map[string]string, 0, len(pom.Dependencies.Dependency))
	scopeCounts := make(map[string]int)
	for _, dep := range pom.Dependencies.Dependency {
		depMap := map[string]string{
			"group_id":    dep.GroupID,
			"artifact_id": dep.ArtifactID,
			"version":     dep.Version,
		}
		if dep.Scope != "" {
			depMap["scope"] = dep.Scope
		}
		deps = append(deps, depMap)

		scope := dep.Scope
		if scope == "" {
			scope = "compile"
		}
		scopeCounts[scope]++
	}
	metadata.LanguageSpecific["dependencies"] = deps
	metadata.LanguageSpecific["dependency_count"] = len(deps)
	metadata.LanguageSpecific["dependency_scopes"] = scopeCounts
}

// applyPOMBuildPlugins records build plugin coordinates and any frameworks
// inferred from the plugin and dependency sets.
func applyPOMBuildPlugins(pom *POM, metadata *extractor.ProjectMetadata) {
	if pom.Build == nil || pom.Build.Plugins == nil {
		return
	}
	plugins := make([]string, 0, len(pom.Build.Plugins.Plugin))
	for _, plugin := range pom.Build.Plugins.Plugin {
		pluginID := fmt.Sprintf("%s:%s", plugin.GroupID, plugin.ArtifactID)
		if plugin.Version != "" {
			pluginID += ":" + plugin.Version
		}
		plugins = append(plugins, pluginID)
	}
	metadata.LanguageSpecific["build_plugins"] = plugins
	metadata.LanguageSpecific["plugin_count"] = len(plugins)

	frameworks := detectMavenFrameworks(pom.Build.Plugins.Plugin, pom.Dependencies)
	if len(frameworks) > 0 {
		metadata.LanguageSpecific["frameworks"] = frameworks
	}
}

// applyPOMStructure records project shape: modules, profiles, and organization.
func applyPOMStructure(pom *POM, metadata *extractor.ProjectMetadata) {
	if pom.Modules != nil && len(pom.Modules.Module) > 0 {
		metadata.LanguageSpecific["is_multi_module"] = true
		metadata.LanguageSpecific["modules"] = pom.Modules.Module
		metadata.LanguageSpecific["module_count"] = len(pom.Modules.Module)
	}

	if pom.Profiles != nil && len(pom.Profiles.Profile) > 0 {
		profileIDs := make([]string, 0, len(pom.Profiles.Profile))
		for _, profile := range pom.Profiles.Profile {
			profileIDs = append(profileIDs, profile.ID)
		}
		metadata.LanguageSpecific["profiles"] = profileIDs
		metadata.LanguageSpecific["profile_count"] = len(profileIDs)
	}

	if pom.Organization != nil && pom.Organization.Name != "" {
		metadata.LanguageSpecific["organization"] = pom.Organization.Name
		if pom.Organization.URL != "" {
			metadata.LanguageSpecific["organization_url"] = pom.Organization.URL
		}
	}
}

// applyPOMVersioningType classifies versioning by placeholder presence, but
// defers to an earlier classification (e.g. the revision property) if set.
func applyPOMVersioningType(metadata *extractor.ProjectMetadata) {
	if _, alreadySet := metadata.LanguageSpecific["versioning_type"]; alreadySet {
		return
	}
	if strings.Contains(metadata.Version, mavenPropertyOpen) {
		metadata.LanguageSpecific["versioning_type"] = "dynamic"
	} else {
		metadata.LanguageSpecific["versioning_type"] = "static"
	}
}

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
	return strings.Contains(value, "${")
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
		if version, _ := javaVersionFromProperties(props); version != "" {
			return version, "module:" + module
		}
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

// resolveProperties resolves property placeholders in POM values
func (e *MavenExtractor) resolveProperties(pom *POM) *POM {
	// Create a copy to avoid modifying the original
	resolved := *pom

	props := make(map[string]string)
	if pom.Properties.Entries != nil {
		for k, v := range pom.Properties.Entries {
			props[k] = v
		}
	}

	// Add implicit properties
	if pom.GroupID != "" {
		props["project.groupId"] = pom.GroupID
	}
	if pom.ArtifactID != "" {
		props["project.artifactId"] = pom.ArtifactID
	}
	if pom.Version != "" {
		props["project.version"] = pom.Version
	}

	// Resolve version
	resolved.Version = resolveProperty(pom.Version, props)
	resolved.GroupID = resolveProperty(pom.GroupID, props)

	return &resolved
}

// maxPropertyResolutionPasses bounds nested placeholder expansion so a
// cyclic reference (${a} -> ${b} -> ${a}) cannot loop forever. Real POMs
// nest at most a couple of levels, so the limit is never reached in practice.
const maxPropertyResolutionPasses = 10

// resolveProperty expands ${...} placeholders in value against props. It
// iterates to a fixpoint (bounded by maxPropertyResolutionPasses) so nested
// placeholders, such as ${a} -> ${b} -> literal, resolve fully. Iterating
// until the value stops changing makes the result deterministic regardless
// of Go's random map iteration order: a single pass could otherwise leave a
// placeholder unresolved when the referenced key happened to be visited
// before the key that produced it.
func resolveProperty(value string, props map[string]string) string {
	for pass := 0; pass < maxPropertyResolutionPasses; pass++ {
		if !strings.Contains(value, mavenPropertyOpen) {
			break
		}
		previous := value
		for key, val := range props {
			placeholder := mavenPropertyOpen + key + mavenPropertyClose
			if strings.Contains(value, placeholder) {
				value = strings.ReplaceAll(value, placeholder, val)
			}
		}
		if value == previous {
			// No known placeholder resolved this pass; stop rather than
			// spin on an unresolved or cyclic reference.
			break
		}
	}

	return value
}

// detectMavenFrameworks detects common Java frameworks and tools
func detectMavenFrameworks(plugins []Plugin, deps *Dependencies) []string {
	frameworks := make([]string, 0)
	seen := make(map[string]bool)

	for _, plugin := range plugins {
		framework := ""
		switch {
		case plugin.ArtifactID == "spring-boot-maven-plugin":
			framework = "Spring Boot"
		case plugin.ArtifactID == "quarkus-maven-plugin":
			framework = "Quarkus"
		case plugin.ArtifactID == "micronaut-maven-plugin":
			framework = "Micronaut"
		case plugin.ArtifactID == "maven-compiler-plugin":
			framework = "Maven Compiler"
		case plugin.ArtifactID == "maven-surefire-plugin":
			framework = "Maven Surefire"
		}

		if framework != "" && !seen[framework] {
			frameworks = append(frameworks, framework)
			seen[framework] = true
		}
	}

	if deps != nil {
		for _, dep := range deps.Dependency {
			framework := ""
			switch {
			case strings.HasPrefix(dep.GroupID, "org.springframework.boot"):
				framework = "Spring Boot"
			case strings.HasPrefix(dep.GroupID, "io.quarkus"):
				framework = "Quarkus"
			case strings.HasPrefix(dep.GroupID, "io.micronaut"):
				framework = "Micronaut"
			case dep.GroupID == "junit" || strings.HasPrefix(dep.GroupID, "org.junit"):
				framework = "JUnit"
			case dep.GroupID == "org.testng":
				framework = "TestNG"
			case strings.HasPrefix(dep.GroupID, "io.vertx"):
				framework = "Vert.x"
			case strings.HasPrefix(dep.GroupID, "org.hibernate"):
				framework = "Hibernate"
			}

			if framework != "" && !seen[framework] {
				frameworks = append(frameworks, framework)
				seen[framework] = true
			}
		}
	}

	return frameworks
}

// Detect checks if this extractor can handle the project
func (e *MavenExtractor) Detect(projectPath string) bool {
	pomPath := filepath.Join(projectPath, "pom.xml")
	_, err := os.Stat(pomPath)
	return err == nil
}

// init registers the Maven extractor
func init() {
	extractor.RegisterExtractor(NewMavenExtractor())
}
