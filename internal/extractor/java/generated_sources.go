// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package java

import (
	"path"
	"strings"

	"github.com/lfreleng-actions/build-metadata-action/internal/extractor"
)

// Derivation values record how each generated source directory was found.
// Both are inferences: this action runs before the build, so none of the
// directories it reports exists yet.
const (
	// derivationConfigured marks a directory the project sets in full, in
	// the plugin's <configuration> or through the POM properties backing it.
	derivationConfigured = "configured"
	// derivationPluginDefault marks a directory that rests, in whole or in
	// part, on the plugin's documented default. A configured sourceFolder
	// beneath a defaulted output is still an assumption about the output.
	derivationPluginDefault = "plugin-default"
)

// moduleRoot stands in for the module directory while a path is resolved.
// XML cannot carry a NUL, so no POM value can collide with the marker.
const moduleRoot = "\x00"

// generatedSource is one directory a module's build is expected to generate
// sources into, and how that expectation was reached.
type generatedSource struct {
	Module     string `json:"module"`
	Path       string `json:"path"`
	Plugin     string `json:"plugin"`
	Derivation string `json:"derivation"`
}

// applyPOMGeneratedSources records where the build is expected to generate
// sources, so a consumer can exclude them from analysis.
//
// A scanner walking a built workspace reads target/generated-sources and
// reports findings against code nobody wrote: generated OpenAPI and
// MapStruct output took a SonarCloud analysis of ONAP
// cps-ncmp-dmi-plugin from 0 bugs to 1157.
//
// Unlike the source layout, this is inference rather than fact. The action
// runs before the build, so it reads generator declarations and each
// plugin's documented default, and records per entry which of the two it
// relied on. Paths are module-relative, like java_source_dirs, and nothing
// is recorded when no recognised generator is configured.
func applyPOMGeneratedSources(projectPath string, pom *POM, metadata *extractor.ProjectMetadata) {
	walk := walkReactor(projectPath, pom)
	if len(walk.found) == 0 {
		return
	}
	if dirs := walk.reactorSafeDirs(); len(dirs) > 0 {
		metadata.LanguageSpecific["generated_source_dirs"] = dirs
	}
	metadata.LanguageSpecific["generated_sources"] = walk.found
}

// moduleContext holds what resolving one module's generated sources needs.
type moduleContext struct {
	// module is the module's path from the scanned project, "." for the
	// project itself.
	module string
	// chain is the module's POM followed by its on-disk parents, nearest
	// first, each with the profiles active for this module merged in.
	chain []*POM
	// props are the chain's properties, nearest winning, plus the
	// built-in paths generator outputs are written against.
	props map[string]string
	// handwritten are the module-relative source directories no reported
	// directory may overlap.
	handwritten []string
	// rootsKnown reports that every hand-written source root is known: the
	// chain ends in a POM with no parent, so no unread parent can set one,
	// and every root resolves. Otherwise only build output is reported.
	rootsKnown bool
	// buildDir is the module-relative build directory, or "" when it
	// cannot be resolved.
	buildDir string
	// plugins and managed are the plugins of the module's <plugins> and
	// <pluginManagement> after inheritance and interpolation.
	plugins, managed pluginSet
	// readings caches everyProfileReading.
	readings []reading
}

// pluginSet is one section's plugins after inheritance and interpolation:
// their keys in declaration order, and each one's declarations, nearest
// first.
type pluginSet struct {
	keys   []string
	layers map[string][]Plugin
}

// newModuleContext loads a module's on-disk parents and resolves the
// properties and source directories its generators are configured with.
// Each POM in the chain carries the profiles active for this module merged
// in, as Maven merges them before inheritance. Maven sets no limit on how
// deep a parent chain runs, only refusing one that loops, so the chain is
// followed to its end and stopped where a parent repeats.
func newModuleContext(dir, module string, pom *POM) *moduleContext {
	lineage := []*POM{pom}
	seen := map[string]bool{parentKey(dir, pom): true}
	current, currentDir := pom, dir
	for {
		parentDir, parent, ok := loadParentPOM(currentDir, current)
		if !ok || !isDeclaredParent(current, parent) || seen[parentKey(parentDir, parent)] {
			break
		}
		seen[parentKey(parentDir, parent)] = true
		lineage = append(lineage, parent)
		current, currentDir = parent, parentDir
	}
	m := &moduleContext{module: module, chain: make([]*POM, len(lineage))}
	packaging := projectPackaging(pom)
	for i, level := range lineage {
		m.chain[i] = withActiveProfiles(level, activeProfiles(level, dir, packaging))
	}
	m.props = m.properties()
	if buildDir, ok := m.modulePath(m.props["project.build.directory"]); ok && buildDir != "." {
		m.buildDir = buildDir
	}
	roots, resolved := m.sourceRoots()
	m.handwritten = roots
	m.rootsKnown = resolved && current.Parent == nil
	m.plugins = m.pluginSetOf(buildPlugins)
	m.managed = m.pluginSetOf(managedPlugins)
	return m
}

// isBuildOutput reports whether dir is the module's build directory or
// lies inside it. It holds only what the build writes, and mvn clean
// deletes it, so no hand-written source lives there whatever a parent
// declares.
func (m *moduleContext) isBuildOutput(dir string) bool {
	return withinBuildDir(dir, m.buildDir)
}

// withinBuildDir reports whether dir is buildDir or lies inside it, an
// unresolved build directory, "", holding nothing.
func withinBuildDir(dir, buildDir string) bool {
	return buildDir != "" && within(dir, buildDir)
}

// properties returns what a ${...} reference resolves against, in the
// precedence of Maven's model interpolator (AbstractStringBasedModel
// Interpolator): basedir first, prefixed or not; then the model under
// project. or pom., ahead of the POM properties, nearest winning; and the
// model unprefixed last. Model values are the effective model's, with
// coordinates inherited from the parent and the Super POM's defaults for
// the build paths.
func (m *moduleContext) properties() map[string]string {
	return m.propertiesWith(-1, nil)
}

// propertiesWith returns the module's properties as they stand when a
// profile of the POM at one level of the chain is active as well: its
// properties merged over that POM's, as Maven injects them, nearer POMs
// still winning, and likewise the build values it sets. A nil profile adds
// nothing.
func (m *moduleContext) propertiesWith(level int, profile *Profile) map[string]string {
	model := m.modelValues()
	var overlay map[string]string
	if profile != nil {
		overlay = profile.Properties.Entries
		for key, value := range m.profileModel(level, profile) {
			model[key] = value
		}
	}
	props := make(map[string]string)
	for key, value := range model {
		props[key] = value
	}
	for i := len(m.chain) - 1; i >= 0; i-- {
		for key, value := range m.chain[i].Properties.Entries {
			props[key] = value
		}
		if i == level {
			for key, value := range overlay {
				props[key] = value
			}
		}
	}
	for key, value := range model {
		props["project."+key] = value
		props["pom."+key] = value
	}
	for _, key := range []string{"basedir", "project.basedir", "pom.basedir"} {
		props[key] = moduleRoot
	}
	return props
}

// modelValues returns the effective model fields a path is commonly
// written against, keyed as Maven names them below project., omitting any
// the chain leaves unset so a reference to it stays unresolved. groupId
// and version inherit; artifactId, name and packaging, as in Maven, do not.
func (m *moduleContext) modelValues() map[string]string {
	pom := m.chain[0]
	ref := func(name string) string { return mavenPropertyOpen + "project." + name + mavenPropertyClose }
	values := map[string]string{
		"groupId":    inheritedCoordinate(pom.GroupID, pom.Parent, func(p *Parent) string { return p.GroupID }),
		"artifactId": strings.TrimSpace(pom.ArtifactID),
		"name":       strings.TrimSpace(pom.Name),
		"version":    inheritedCoordinate(pom.Version, pom.Parent, func(p *Parent) string { return p.Version }),
		"packaging":  projectPackaging(pom),
		"build.directory": m.buildSetting(
			func(b *Build) string { return b.Directory }, moduleRoot+"/target"),
		"build.outputDirectory": m.buildSetting(
			func(b *Build) string { return b.OutputDirectory }, ref("build.directory")+"/classes"),
		"build.testOutputDirectory": m.buildSetting(
			func(b *Build) string { return b.TestOutputDirectory }, ref("build.directory")+"/test-classes"),
		"build.sourceDirectory": m.buildSetting(
			func(b *Build) string { return b.SourceDirectory }, moduleRoot+"/src/main/java"),
		"build.testSourceDirectory": m.buildSetting(
			func(b *Build) string { return b.TestSourceDirectory }, moduleRoot+"/src/test/java"),
		"build.finalName": m.buildSetting(
			func(b *Build) string { return b.FinalName }, ref("artifactId")+"-"+ref("version")),
	}
	if pom.Parent != nil {
		values["parent.groupId"] = strings.TrimSpace(pom.Parent.GroupID)
		values["parent.artifactId"] = strings.TrimSpace(pom.Parent.ArtifactID)
		values["parent.version"] = strings.TrimSpace(pom.Parent.Version)
	}
	for key, value := range values {
		if value == "" {
			delete(values, key)
		}
	}
	return values
}

// inheritedCoordinate returns a coordinate as the effective model holds
// it: the POM's own, else its parent's, which the parent declaration names.
func inheritedCoordinate(own string, parent *Parent, declared func(*Parent) string) string {
	if own = strings.TrimSpace(own); own != "" || parent == nil {
		return own
	}
	return strings.TrimSpace(declared(parent))
}

// buildSetting returns the nearest non-empty <build> value in the chain,
// since <build> settings are inherited, or fallback when none sets it.
func (m *moduleContext) buildSetting(field func(*Build) string, fallback string) string {
	for _, pom := range m.chain {
		if pom.Build == nil {
			continue
		}
		if value := strings.TrimSpace(field(pom.Build)); value != "" {
			return value
		}
	}
	return fallback
}

// resolvedIn expands properties in a POM value from props and trims it.
func resolvedIn(value string, props map[string]string) string {
	return strings.TrimSpace(resolveProperty(strings.TrimSpace(value), props))
}

// sources returns every generated source directory the module declares.
func (m *moduleContext) sources() []generatedSource {
	var found []generatedSource
	for _, key := range m.plugins.keys {
		if generator := generatorFor(m.plugins.layers[key][0]); generator != nil {
			found = append(found, m.pluginSources(generator, key)...)
		}
	}
	return append(found, m.annotationProcessorSources()...)
}

// pluginSetOf returns the plugins of one section, <plugins> or
// <pluginManagement>, that reach the module, in Maven's order of work.
// DefaultModelBuilder assembles inheritance on the POMs as written,
// matching plugins and executions by their raw coordinates and reading
// <inherited> uninterpolated, then interpolates the result; management is
// joined to the build plugins after that, by the interpolated key.
func (m *moduleContext) pluginSetOf(section func(*POM) []Plugin) pluginSet {
	set := pluginSet{layers: make(map[string][]Plugin)}
	seen := make(map[string]bool)
	for _, pom := range m.chain {
		for _, plugin := range section(pom) {
			raw := pluginKey(plugin)
			if seen[raw] {
				continue
			}
			seen[raw] = true
			for _, layer := range inheritedDeclarations(m.chain, section, raw) {
				resolved := m.resolvedPlugin(layer)
				key := pluginKey(resolved)
				if _, listed := set.layers[key]; !listed {
					set.keys = append(set.keys, key)
				}
				set.layers[key] = append(set.layers[key], resolved)
			}
		}
	}
	return set
}

// resolvedPlugin returns a declaration with the fields Maven interpolates
// before deciding what runs: its coordinates and its executions' ids,
// phases and goals. Configuration values resolve as they are read.
func (m *moduleContext) resolvedPlugin(plugin Plugin) Plugin {
	plugin.GroupID = m.resolved(plugin.GroupID)
	plugin.ArtifactID = m.resolved(plugin.ArtifactID)
	plugin.Version = m.resolved(plugin.Version)
	executions := make([]Execution, len(plugin.Executions))
	for i, declared := range plugin.Executions {
		declared.ID = m.resolved(declared.ID)
		declared.Phase = m.resolved(declared.Phase)
		goals := make([]string, len(declared.Goals))
		for j, goal := range declared.Goals {
			goals[j] = m.resolved(goal)
		}
		declared.Goals = goals
		executions[i] = declared
	}
	plugin.Executions = executions
	return plugin
}

// pluginLayers returns every declaration that shapes a plugin's
// configuration in the module, in Maven's precedence: <plugins> entries
// from the module up through its parents, then <pluginManagement> entries
// the same way.
func (m *moduleContext) pluginLayers(key string) []Plugin {
	return append(append([]Plugin{}, m.plugins.layers[key]...), m.managed.layers[key]...)
}

// pluginSources reports where each execution of a generator writes. An
// execution bound to the phase none is disabled, the way a module switches
// off one it inherits.
func (m *moduleContext) pluginSources(generator *sourceGenerator, key string) []generatedSource {
	layers := m.pluginLayers(key)
	var found []generatedSource
	for _, exec := range mergeExecutions(layers) {
		if !exec.bindable() || m.isOpaque(generator, exec, layers) {
			continue
		}
		for _, goal := range exec.goals {
			spec, known := generator.goals[goal]
			if !known || m.isSkipped(spec.skip, exec, layers) {
				continue
			}
			if source, ok := m.goalSource(generator, spec, exec, layers); ok {
				found = append(found, source)
			}
		}
	}
	return found
}

// isOpaque reports whether an execution sets a parameter that moves its
// output out of this action's sight.
func (m *moduleContext) isOpaque(generator *sourceGenerator, exec *execution, layers []Plugin) bool {
	for _, param := range generator.opaqueWhen {
		if _, set := m.lookup(param, exec, layers); set {
			return true
		}
	}
	return false
}

// isSkipped reports whether a goal's skip parameter is true for an
// execution, in its configuration or through the property backing it. A
// skipped goal returns without generating anything.
func (m *moduleContext) isSkipped(skip *pluginParam, exec *execution, layers []Plugin) bool {
	if skip == nil {
		return false
	}
	value, set := m.lookup(*skip, exec, layers)
	return set && strings.EqualFold(resolveProperty(value, m.props), "true")
}

// goalSource resolves where one goal of an execution writes. The entry
// counts as configured only when nothing in its path was assumed.
//
// For openapi-generator and swagger-codegen, a configured sourceFolder
// names exactly where Java lands beneath the output. Without one, each
// generator picks its own (src/main/java for most Java generators,
// src/gen/java for JAX-RS, src/main/kotlin for Kotlin), so the whole
// output is reported instead, which holds it whatever the generator. That
// is only safe inside the build directory: elsewhere the output may share
// a directory with hand-written files, and the entry is left out.
func (m *moduleContext) goalSource(generator *sourceGenerator, spec generatorGoal, exec *execution, layers []Plugin) (generatedSource, bool) {
	output, derivation, ok := m.goalOutput(generator, spec, exec, layers)
	if !ok {
		return generatedSource{}, false
	}
	if generator.sourceFolder == nil {
		return m.source(output, generator.artifactID, derivation)
	}
	if folder, set := m.lookup(*generator.sourceFolder, exec, layers); set {
		return m.source(output+"/"+folder, generator.artifactID, derivation)
	}
	if dir, resolved := m.modulePath(output); !resolved || !m.isBuildOutput(dir) {
		return generatedSource{}, false
	}
	return m.source(output, generator.artifactID, derivation)
}

// goalOutput returns the directory a goal writes to and how it was found:
// a configured value first, then the plugin's default.
func (m *moduleContext) goalOutput(generator *sourceGenerator, spec generatorGoal, exec *execution, layers []Plugin) (string, string, bool) {
	if value, set := m.lookup(spec.output, exec, layers); set {
		return value, derivationConfigured, true
	}
	dir, ok := m.defaultOutput(generator, spec, exec, layers)
	if !ok {
		return "", "", false
	}
	if spec.defaultSuffix == nil {
		return dir, derivationPluginDefault, true
	}
	// The plugin cannot run without the suffix, so an unset one means a
	// declaration this action cannot complete; report nothing for it.
	suffix, set := m.lookup(*spec.defaultSuffix, exec, layers)
	if !set {
		return "", "", false
	}
	return dir + "/" + suffix, derivationPluginDefault, true
}

// defaultOutput returns the plugin's default for an execution. A default
// chosen by phase depends on the plugin version that introduced it, so
// when that version cannot be read the entry is left out rather than
// guessed between the two.
func (m *moduleContext) defaultOutput(generator *sourceGenerator, spec generatorGoal, exec *execution, layers []Plugin) (string, bool) {
	if generator.testPhase == nil || exec.phase != "generate-test-sources" {
		return spec.defaultDir, true
	}
	byPhase, known := versionAtLeast(pluginVersion(layers, m.props), generator.testPhase.since)
	if !known {
		return "", false
	}
	if byPhase {
		return generator.testPhase.dir, true
	}
	return spec.defaultDir, true
}

// lookup resolves a parameter for one execution in Maven's order: the
// nearest configuration that sets it, unless combine.self="override" cuts
// the search off, then a POM property backing its user property.
func (m *moduleContext) lookup(param pluginParam, exec *execution, layers []Plugin) (string, bool) {
	if param.element != "" {
		if element := configured(param.element, exec, layers); element != nil {
			if value := strings.TrimSpace(element.Value); value != "" {
				return value, true
			}
		}
	}
	if param.property == "" {
		return "", false
	}
	value := strings.TrimSpace(m.props[param.property])
	return value, value != ""
}

// source builds the record for a generated directory, or reports false
// when the directory cannot be reported safely.
func (m *moduleContext) source(output, plugin, derivation string) (generatedSource, bool) {
	dir, ok := m.generatedDir(output)
	if !ok {
		return generatedSource{}, false
	}
	return generatedSource{Module: m.module, Path: dir, Plugin: plugin, Derivation: derivation}, true
}

// generatedDir resolves a generator output to a module-relative directory,
// and refuses one that would hide hand-written code: the module root, or a
// directory that overlaps a source directory. A generator pointed at
// ${project.basedir} does write into src/main/java, but reporting that
// would exclude every hand-written file along with the generated ones, and
// one writing into a package inside src/main/java may share it with
// hand-written classes. When the module's source roots are not all known,
// because it inherits from a parent off disk or a root will not resolve,
// only directories inside the build output are reported.
func (m *moduleContext) generatedDir(output string) (string, bool) {
	dir, ok := m.modulePath(output)
	if !ok || dir == "." {
		return "", false
	}
	for _, handwritten := range m.handwritten {
		if overlaps(dir, handwritten) {
			return "", false
		}
	}
	if !m.rootsKnown && !m.isBuildOutput(dir) {
		return "", false
	}
	return dir, true
}

// modulePath resolves a configured or default path to one relative to the
// module, reading properties from the module's own context.
func (m *moduleContext) modulePath(raw string) (string, bool) {
	return m.modulePathIn(raw, m.props)
}

// modulePathIn resolves a path to one relative to the module, reading
// properties from props. It refuses anything that cannot be expressed
// that way: a property left unresolved, an absolute path, or one that
// leaves the module. A backslash counts as a separator on every platform:
// a POM written on Windows may be read on any runner, and treating it as
// one can only expose a .. or an overlap, never hide one.
func (m *moduleContext) modulePathIn(raw string, props map[string]string) (string, bool) {
	value := resolveProperty(strings.TrimSpace(raw), props)
	if isUnresolvedPlaceholder(value) {
		return "", false
	}
	if strings.HasPrefix(value, moduleRoot) {
		// Maven joins strings, so ${project.basedir}generated names a sibling
		// of the module rather than a directory inside it.
		rest := strings.TrimPrefix(value, moduleRoot)
		if rest != "" && rest[0] != '/' && rest[0] != '\\' {
			return "", false
		}
		value = "." + rest
	}
	value = strings.ReplaceAll(value, `\`, "/")
	if strings.Contains(value, moduleRoot) || isAbsolutePath(value) {
		return "", false
	}
	cleaned := path.Clean(value)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", false
	}
	return cleaned, true
}

// isAbsolutePath reports an absolute path once backslashes are normalised
// to slashes: a rooted or UNC path, or one with a Windows drive letter,
// since the POM may have been written on a different platform.
func isAbsolutePath(value string) bool {
	return strings.HasPrefix(value, "/") || (len(value) > 1 && value[1] == ':')
}
