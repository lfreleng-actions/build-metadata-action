// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package java

import (
	"slices"
	"strings"
)

// compilerArtifactID is the plugin that runs annotation processors and
// writes the sources they generate.
const compilerArtifactID = "maven-compiler-plugin"

// compilerOutput describes where one of the compiler's two goals writes the
// sources annotation processors generate.
type compilerOutput struct {
	// executionID is the execution Maven binds the goal to implicitly.
	executionID string
	goal        string
	tests       bool
	output      pluginParam
	defaultDir  string
	// skip makes the goal return without compiling.
	skip pluginParam
}

var (
	compileOutput = compilerOutput{
		executionID: "default-compile",
		goal:        "compile",
		output:      pluginParam{element: "generatedSourcesDirectory"},
		defaultDir:  buildDirectory + "/generated-sources/annotations",
		skip:        pluginParam{"skipMain", "maven.main.skip"},
	}
	testCompileOutput = compilerOutput{
		executionID: "default-testCompile",
		goal:        "testCompile",
		tests:       true,
		output:      pluginParam{element: "generatedTestSourcesDirectory"},
		defaultDir:  buildDirectory + "/generated-test-sources/test-annotations",
		skip:        pluginParam{"skip", "maven.test.skip"},
	}
)

// procParam switches annotation processing off when set to none.
var procParam = pluginParam{"proc", "maven.compiler.proc"}

// implicitDiscoveryRemoved is the first JDK whose javac stops searching the
// classpath for annotation processors unless processing is configured
// explicitly (JDK-8321314).
const implicitDiscoveryRemoved = 23

// sourceProcessor describes an annotation processor artifact that writes
// Java sources.
type sourceProcessor struct {
	// classPrefixes are the packages of the processor classes it provides,
	// which an annotationProcessors allow-list is matched against.
	classPrefixes []string
	// registeredClassifiers, when set, are the only classifiers whose jar
	// registers a processor for discovery. The plain jar registers none,
	// so javac runs a processor from it only when one is named.
	registeredClassifiers []string
}

// registers reports whether the artifact's jar with a classifier registers
// its processor for discovery.
func (p sourceProcessor) registers(classifier string) bool {
	return p.registeredClassifiers == nil || slices.Contains(p.registeredClassifiers, classifier)
}

// querydslClassifiers are the QueryDSL jars that register a processor; the
// classifier picks the persistence flavour.
var querydslClassifiers = []string{"jpa", "jakarta", "jdo", "hibernate", "morphia", "roo", "general"}

// sourceGeneratingProcessors lists annotation processors that write Java
// sources, keyed groupId:artifactId, with the processor classes each
// registers, as read from the published jars. Lombok and Spring Boot's
// configuration processor are absent on purpose: neither writes a source
// file, so neither leaves anything to exclude.
var sourceGeneratingProcessors = map[string]sourceProcessor{
	"org.mapstruct:mapstruct-processor":       {classPrefixes: []string{"org.mapstruct.ap."}},
	"com.google.dagger:dagger-compiler":       {classPrefixes: []string{"dagger.internal.codegen."}},
	"com.google.auto.value:auto-value":        {classPrefixes: []string{"com.google.auto.value."}},
	"org.hibernate:hibernate-jpamodelgen":     {classPrefixes: []string{"org.hibernate.jpamodelgen."}},
	"org.hibernate.orm:hibernate-jpamodelgen": {classPrefixes: []string{"org.hibernate.jpamodelgen.", "org.hibernate.processor."}},
	"org.hibernate.orm:hibernate-processor":   {classPrefixes: []string{"org.hibernate.processor."}},
	"com.querydsl:querydsl-apt": {
		classPrefixes:         []string{"com.querydsl.apt."},
		registeredClassifiers: querydslClassifiers,
	},
	"com.mysema.querydsl:querydsl-apt": {
		classPrefixes:         []string{"com.mysema.query.apt."},
		registeredClassifiers: querydslClassifiers,
	},
	"io.github.openfeign.querydsl:querydsl-apt": {
		classPrefixes:         []string{"com.querydsl.apt."},
		registeredClassifiers: querydslClassifiers,
	},
	"org.immutables:value": {classPrefixes: []string{"org.immutables."}},
}

// processorRef is a source-generating processor artifact javac can see,
// and whether its jar registers the processor for discovery.
type processorRef struct {
	key          string
	discoverable bool
}

// processorRefFor returns the source-generating processor an artifact
// provides, if it provides one.
func processorRefFor(groupID, artifactID, classifier string) (processorRef, bool) {
	key := processorKey(groupID, artifactID)
	spec, known := sourceGeneratingProcessors[key]
	if !known {
		return processorRef{}, false
	}
	return processorRef{key: key, discoverable: spec.registers(strings.TrimSpace(classifier))}, true
}

// compilations records which of the two compilations a processor reaches.
type compilations struct {
	main, tests bool
}

// add folds in the compilations another declaration reaches.
func (c *compilations) add(other compilations) {
	c.main = c.main || other.main
	c.tests = c.tests || other.tests
}

// reaches reports whether the compilation a goal performs is among them.
func (c compilations) reaches(out compilerOutput) bool {
	if out.tests {
		return c.tests
	}
	return c.main
}

// compilerRun is one execution of maven-compiler-plugin and the
// compilation it performs.
type compilerRun struct {
	exec   *execution
	output compilerOutput
}

// annotationProcessorSources reports where maven-compiler-plugin writes the
// sources annotation processors generate, for each compiler execution a
// source-generating processor reaches. The compiler runs in every module
// that compiles, whether or not the POM declares it, so only pom packaging
// is skipped.
//
// MapStruct is the usual case, and ONAP adds it as an ordinary dependency
// rather than through annotationProcessorPaths, so both are read.
func (m *moduleContext) annotationProcessorSources() []generatedSource {
	if strings.TrimSpace(m.chain[0].Packaging) == "pom" {
		return nil
	}
	layers := m.pluginLayers(defaultPluginGroupID + ":" + compilerArtifactID)
	deps := m.processorDependencies()
	var found []generatedSource
	for _, run := range compilerRuns(layers) {
		if source, ok := m.compilerRunSource(run, deps, layers); ok {
			found = append(found, source)
		}
	}
	return found
}

// compilerRuns returns every compiler execution in the module: the two
// Maven binds implicitly, merged with whatever the POMs declare under
// their ids, then every further goal an execution binds, including an
// implicit execution given the other compiler goal as well. Each keeps
// its own configuration, so an execution's processor path, proc setting
// and output apply to it alone.
func compilerRuns(layers []Plugin) []compilerRun {
	declared := mergeExecutions(layers)
	runs := []compilerRun{
		{exec: executionWithID(declared, compileOutput.executionID), output: compileOutput},
		{exec: executionWithID(declared, testCompileOutput.executionID), output: testCompileOutput},
	}
	for _, exec := range declared {
		for _, out := range []compilerOutput{compileOutput, testCompileOutput} {
			if exec.id != out.executionID && slices.Contains(exec.goals, out.goal) {
				runs = append(runs, compilerRun{exec: exec, output: out})
			}
		}
	}
	return runs
}

// executionWithID returns the merged execution with an id, or an empty one
// standing for an implicit execution no POM configures.
func executionWithID(executions []*execution, id string) *execution {
	for _, exec := range executions {
		if exec.id == id {
			return exec
		}
	}
	return &execution{id: id}
}

// compilerRunSource reports where one compiler execution writes generated
// sources, when a source-generating processor reaches it and nothing
// switches the execution, its goal or its annotation processing off.
// Binding an execution to the phase none is how a POM disables an
// inherited one.
func (m *moduleContext) compilerRunSource(run compilerRun, deps processorDeps, layers []Plugin) (generatedSource, bool) {
	if !run.exec.bindable() || m.isSkipped(&run.output.skip, run.exec, layers) {
		return generatedSource{}, false
	}
	if proc, set := m.lookup(procParam, run.exec, layers); set && resolveProperty(proc, m.props) == "none" {
		return generatedSource{}, false
	}
	if !m.processorReaches(run, deps, layers) {
		return generatedSource{}, false
	}
	output, derivation := run.output.defaultDir, derivationPluginDefault
	if value, set := m.lookup(run.output.output, run.exec, layers); set {
		output, derivation = value, derivationConfigured
	}
	return m.source(output, compilerArtifactID, derivation)
}

// processorReaches reports whether a source-generating processor runs in a
// compiler execution. javac finds processors on the processor path when
// one is given, and on the classpath only when none is. With an
// annotationProcessors allow-list it runs the classes named there;
// without one, the processors the jars it finds register.
func (m *moduleContext) processorReaches(run compilerRun, deps processorDeps, layers []Plugin) bool {
	allowed := m.allowedProcessors(configured("annotationProcessors", run.exec, layers))
	for _, ref := range m.availableProcessors(run, deps, allowed, layers) {
		if allowed == nil && ref.discoverable {
			return true
		}
		if allowed != nil && allowsProcessor(allowed, ref.key) {
			return true
		}
	}
	return false
}

// availableProcessors returns the source-generating processors javac can
// find for a compiler execution. The processor path is the compiler's
// annotationProcessorPaths together with the dependencies Maven 4 types
// for it; once either supplies a path, javac searches nothing else.
func (m *moduleContext) availableProcessors(run compilerRun, deps processorDeps, allowed []string, layers []Plugin) []processorRef {
	paths := configured("annotationProcessorPaths", run.exec, layers)
	if paths != nil || deps.typedForPath.reaches(run.output) {
		return append(m.pathProcessors(paths), refsReaching(deps.onProcessorPath, run.output)...)
	}
	if !m.searchesClasspath(run, allowed, layers) {
		return nil
	}
	return refsReaching(deps.onClasspath, run.output)
}

// refsReaching returns the processors that reach a compilation.
func refsReaching(processors map[processorRef]compilations, out compilerOutput) []processorRef {
	var refs []processorRef
	for ref, reached := range processors {
		if reached.reaches(out) {
			refs = append(refs, ref)
		}
	}
	return refs
}

// searchesClasspath reports whether javac looks for processors on the
// classpath in a compiler execution without a processor path. From JDK 23
// it does so only when processing is requested explicitly, by naming
// processors or by proc full or only. A module declaring Java 23 or later
// is built by such a JDK. Below that the action cannot see which JDK
// builds the module, and keeps the discovery older ones perform: dropping
// it would miss the processors most builds still find this way.
func (m *moduleContext) searchesClasspath(run compilerRun, allowed []string, layers []Plugin) bool {
	if allowed != nil {
		return true
	}
	if proc, set := m.lookup(procParam, run.exec, layers); set {
		if mode := resolveProperty(proc, m.props); mode == "full" || mode == "only" {
			return true
		}
	}
	release, known := javaFeatureRelease(m.compilerLevel(run.exec, layers))
	return !known || release < implicitDiscoveryRemoved
}

// compilerLevels are the compiler parameters that set the Java level, in
// the order the level is taken from them: release wins over target, and
// target, the bytecode the JDK must emit, over source.
var compilerLevels = []struct {
	field    func(*PluginConfiguration) string
	property string
}{
	{func(c *PluginConfiguration) string { return c.Release }, "maven.compiler.release"},
	{func(c *PluginConfiguration) string { return c.Target }, "maven.compiler.target"},
	{func(c *PluginConfiguration) string { return c.Source }, "maven.compiler.source"},
}

// compilerLevel returns the Java level one compiler execution compiles
// for, or "" when none is declared. Each parameter resolves as Maven
// resolves it, from the nearest configuration, the execution's own first,
// and then its property, so an execution compiling for its own level is
// judged by that level. java.version, the Spring Boot convention, is the
// last resort.
func (m *moduleContext) compilerLevel(exec *execution, layers []Plugin) string {
	chain := configChain(exec, layers)
	for _, level := range compilerLevels {
		if value := m.resolvedLevel(configuredField(chain, level.field), level.property); value != "" {
			return value
		}
	}
	return m.resolvedLevel("", "java.version")
}

// configuredField returns the nearest non-empty value of a typed
// configuration field, stopping where combine.self="override" cuts off
// what lies beyond.
func configuredField(chain []*PluginConfiguration, field func(*PluginConfiguration) string) string {
	for _, config := range chain {
		if config == nil {
			continue
		}
		if value := strings.TrimSpace(field(config)); value != "" {
			return value
		}
		if overridesInherited(config.Attrs) {
			return ""
		}
	}
	return ""
}

// resolvedLevel resolves a configured level, or the property standing in
// for it, returning "" when neither yields a usable value.
func (m *moduleContext) resolvedLevel(value, property string) string {
	if value == "" {
		value = strings.TrimSpace(m.props[property])
	}
	value = resolveProperty(value, m.props)
	if isUnresolvedPlaceholder(value) {
		return ""
	}
	return value
}

// javaFeatureRelease reads the feature release from a Java level in either
// form, 17 or 1.8 for Java 8.
func javaFeatureRelease(level string) (int, bool) {
	parts, ok := majorMinor(strings.TrimPrefix(strings.TrimSpace(level), "1."))
	return parts[0], ok && parts[0] > 0
}

// pathProcessors returns the source-generating processors on
// annotationProcessorPaths, with their coordinates interpolated.
func (m *moduleContext) pathProcessors(paths *ConfigElement) []processorRef {
	if paths == nil {
		return nil
	}
	var refs []processorRef
	for _, entry := range paths.Children {
		ref, known := processorRefFor(m.resolved(entry.childValue("groupId")),
			m.resolved(entry.childValue("artifactId")), m.resolved(entry.childValue("classifier")))
		if known {
			refs = append(refs, ref)
		}
	}
	return refs
}

// allowedProcessors returns the class names an annotationProcessors
// allow-list gives, interpolated, as list entries or comma-separated
// text, or nil when there is no list.
func (m *moduleContext) allowedProcessors(list *ConfigElement) []string {
	if list == nil {
		return nil
	}
	var classes []string
	if len(list.Children) == 0 {
		classes = strings.Split(m.resolved(list.Value), ",")
	}
	for _, entry := range list.Children {
		classes = append(classes, m.resolved(entry.Value))
	}
	for i := range classes {
		classes[i] = strings.TrimSpace(classes[i])
	}
	return classes
}

// allowsProcessor reports whether an allow-list names a class from one of a
// processor's packages.
func allowsProcessor(allowed []string, processor string) bool {
	for _, class := range allowed {
		for _, prefix := range sourceGeneratingProcessors[processor].classPrefixes {
			if strings.HasPrefix(class, prefix) {
				return true
			}
		}
	}
	return false
}

// processorKey identifies a processor artifact as groupId:artifactId.
func processorKey(groupID, artifactID string) string {
	return strings.TrimSpace(groupID) + ":" + strings.TrimSpace(artifactID)
}

// processorPathTypes are the Maven 4 dependency types that place a jar on
// the annotation processor path rather than the classpath.
var processorPathTypes = []string{"processor", "classpath-processor", "modular-processor"}

// processorDeps sorts the module's dependencies by the path Maven puts
// them on, as far as processors are concerned.
type processorDeps struct {
	// onClasspath are recognised processors on the classpath, which javac
	// searches only without a processor path.
	onClasspath map[processorRef]compilations
	// onProcessorPath are recognised processors typed for the processor
	// path.
	onProcessorPath map[processorRef]compilations
	// typedForPath are the compilations any dependency typed for the
	// processor path reaches, recognised or not: one is enough to give
	// javac a processor path and stop it searching the classpath.
	typedForPath compilations
}

// processorDependencies sorts the module's dependencies, after
// inheritance, by the path Maven puts them on. Maven merges inherited
// dependencies by key, the nearest declaration winning, so a parent's
// declaration of a dependency the module redeclares does not count.
func (m *moduleContext) processorDependencies() processorDeps {
	deps := processorDeps{
		onClasspath:     make(map[processorRef]compilations),
		onProcessorPath: make(map[processorRef]compilations),
	}
	seen := make(map[string]bool)
	for _, pom := range m.chain {
		for _, declared := range declaredDependencies(pom) {
			dep := m.resolvedDependency(declared)
			key := dependencyKey(dep)
			if seen[key] {
				continue
			}
			seen[key] = true
			deps.add(dep, scopeCompilations(m.dependencyScope(dep)))
		}
	}
	return deps
}

// add files one dependency under the path it goes on.
func (d *processorDeps) add(dep Dependency, reached compilations) {
	onPath := slices.Contains(processorPathTypes, dep.Type)
	if onPath {
		d.typedForPath.add(reached)
	}
	ref, known := processorRefFor(dep.GroupID, dep.ArtifactID, dep.Classifier)
	if !known {
		return
	}
	target := d.onClasspath
	if onPath {
		target = d.onProcessorPath
	}
	scopes := target[ref]
	scopes.add(reached)
	target[ref] = scopes
}

// resolvedDependency returns a dependency with its coordinates and scope
// interpolated, as Maven matches and manages it.
func (m *moduleContext) resolvedDependency(dep Dependency) Dependency {
	dep.GroupID = m.resolved(dep.GroupID)
	dep.ArtifactID = m.resolved(dep.ArtifactID)
	dep.Type = m.resolved(dep.Type)
	dep.Classifier = m.resolved(dep.Classifier)
	dep.Scope = m.resolved(dep.Scope)
	return dep
}

// resolved expands properties in a POM value and trims it.
func (m *moduleContext) resolved(value string) string {
	return resolvedIn(value, m.props)
}

// dependencyScope returns a resolved dependency's effective scope: its
// own, else the nearest dependencyManagement entry's, else Maven's
// default of compile. A scope left unresolved matches no scope, as Maven
// would reject it.
func (m *moduleContext) dependencyScope(dep Dependency) string {
	if dep.Scope != "" {
		return dep.Scope
	}
	key := dependencyKey(dep)
	for _, pom := range m.chain {
		if scope := m.managedScope(pom, key); scope != "" {
			return scope
		}
	}
	return "compile"
}

// managedScope returns the resolved scope a POM's dependencyManagement
// gives a dependency, or "" when it gives none.
func (m *moduleContext) managedScope(pom *POM, key string) string {
	for _, declared := range managedDependencies(pom) {
		if managed := m.resolvedDependency(declared); dependencyKey(managed) == key {
			return managed.Scope
		}
	}
	return ""
}

// scopeCompilations maps a dependency scope onto the compilations whose
// classpath carries it: Maven's main compilation sees compile, provided
// and system dependencies, and its test compilation sees every scope.
func scopeCompilations(scope string) compilations {
	switch scope {
	case "compile", "provided", "system":
		return compilations{main: true, tests: true}
	case "runtime", "test":
		return compilations{tests: true}
	}
	return compilations{}
}
