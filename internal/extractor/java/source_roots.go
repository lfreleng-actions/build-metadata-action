// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package java

import "strings"

// sourceRoots resolves the module's hand-written source roots: its main
// and test source directories, always, read under every profile's
// properties, and every root build-helper-maven-plugin adds outside the
// build directory. An added root inside the build directory is generated
// output registered as a root, the usual pattern for a generator that
// registers none itself. A declared directory always counts, wherever it
// sits. resolved is false when a root cannot be resolved, is the module
// itself, or rests on properties that profiles could set together, so
// that where hand-written code lives is not fully known.
func (m *moduleContext) sourceRoots() (roots []string, resolved bool) {
	declared := []string{
		m.buildSetting(func(b *Build) string { return b.SourceDirectory }, "src/main/java"),
		m.buildSetting(func(b *Build) string { return b.TestSourceDirectory }, "src/test/java"),
	}
	resolved = true
	for _, raw := range declared {
		if m.isAmbiguous(raw, nil) {
			resolved = false
			continue
		}
		for _, read := range m.everyProfileReading() {
			root, ok := m.modulePathIn(raw, read.props)
			if !ok || root == "." {
				resolved = false
				continue
			}
			roots = appendMissing(roots, []string{root})
		}
	}
	for _, ref := range m.addedSourceRoots() {
		if ref.ambiguous {
			resolved = false
			continue
		}
		if ref.optional && isUnresolvedPlaceholder(resolveProperty(ref.raw, ref.props)) {
			continue
		}
		root, ok := m.modulePathIn(ref.raw, ref.props)
		switch {
		case !ok || root == ".":
			resolved = false
		case !m.isBuildOutput(root):
			roots = append(roots, root)
		}
	}
	return roots, resolved
}

// everyProfileReading returns the module's properties, and the properties
// as they stand with each profile anywhere in the chain active as well, so
// a value the POM body declares can be read every way one profile could
// turn it, whether the action knows the profile's state or not. A value
// that profiles could turn together is left to isAmbiguous.
func (m *moduleContext) everyProfileReading() []reading {
	if m.readings != nil {
		return m.readings
	}
	m.readings = []reading{{props: m.props}}
	for level, pom := range m.chain {
		profiles := profilesOf(pom)
		for i := range profiles {
			if len(m.profileKeys(level, &profiles[i])) > 0 {
				m.readings = append(m.readings, reading{props: m.propertiesWith(level, &profiles[i])})
			}
		}
	}
	return m.readings
}

// profileBuildFields are the model values a profile's <build> can set,
// keyed as modelValues keys them.
var profileBuildFields = map[string]func(*Build) string{
	"build.directory": func(b *Build) string { return b.Directory },
	"build.finalName": func(b *Build) string { return b.FinalName },
}

// profileModel returns the model values a profile at a level of the chain
// changes: the build directory and final name its <build> sets, unless a
// nearer POM sets the same, since the profile merges into its own POM
// before inheritance.
func (m *moduleContext) profileModel(level int, profile *Profile) map[string]string {
	values := make(map[string]string)
	if profile.Build == nil {
		return values
	}
	for key, field := range profileBuildFields {
		if value := strings.TrimSpace(field(profile.Build)); value != "" && !m.setNearer(level, field) {
			values[key] = value
		}
	}
	return values
}

// setNearer reports whether a POM nearer than level sets a build field.
func (m *moduleContext) setNearer(level int, field func(*Build) string) bool {
	for _, pom := range m.chain[:level] {
		if pom.Build != nil && strings.TrimSpace(field(pom.Build)) != "" {
			return true
		}
	}
	return false
}

// profileKeys returns the properties a profile sets, with their values:
// its own, and the model properties naming each build value it changes.
func (m *moduleContext) profileKeys(level int, profile *Profile) map[string]string {
	keys := make(map[string]string, len(profile.Properties.Entries))
	for key, value := range profile.Properties.Entries {
		keys[key] = value
	}
	for key, value := range m.profileModel(level, profile) {
		for _, name := range []string{key, "project." + key, "pom." + key} {
			keys[name] = value
		}
	}
	return keys
}

// isAmbiguous reports whether a value rests on properties that profiles
// could set together, a combination no single reading shows: two or more
// profiles anywhere in the chain set a property it refers to, directly or
// through the value another property takes in the POM or any profile. A
// value a profile declares takes effect only with that profile active, so
// the declaring profile counts among them. A reference nested in another,
// whose outer name only substitution settles, leaves every property in
// question.
func (m *moduleContext) isAmbiguous(raw string, declaring *Profile) bool {
	behind, nested := m.propertiesBehind(raw)
	setters := 0
	if declaring != nil {
		setters++
	}
	for level, pom := range m.chain {
		profiles := profilesOf(pom)
		for i := range profiles {
			if &profiles[i] != declaring && setsAny(m.profileKeys(level, &profiles[i]), behind, nested) {
				setters++
			}
		}
	}
	return setters > 1
}

// propertiesBehind returns the properties a value refers to, directly or
// through any value one of them takes in the module's properties or in a
// profile's, and whether any of those values nests one reference in
// another.
func (m *moduleContext) propertiesBehind(raw string) (map[string]bool, bool) {
	values := make(map[string][]string)
	for key, value := range m.props {
		values[key] = append(values[key], value)
	}
	for level, pom := range m.chain {
		profiles := profilesOf(pom)
		for i := range profiles {
			for key, value := range m.profileKeys(level, &profiles[i]) {
				values[key] = append(values[key], value)
			}
		}
	}
	behind, nested := make(map[string]bool), false
	pending := []string{raw}
	for len(pending) > 0 {
		value := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		names, inner := propertyNames(value)
		nested = nested || inner
		for _, name := range names {
			if !behind[name] {
				behind[name] = true
				pending = append(pending, values[name]...)
			}
		}
	}
	return behind, nested
}

// propertyNames returns the names of the properties a value refers to,
// the innermost reference wherever one nests inside another, and whether
// one does.
func propertyNames(value string) (names []string, nested bool) {
	for {
		end := strings.Index(value, mavenPropertyClose)
		if end < 0 {
			return names, nested
		}
		if start := strings.LastIndex(value[:end], mavenPropertyOpen); start >= 0 {
			names = append(names, value[start+len(mavenPropertyOpen):end])
			nested = nested || strings.Contains(value[:start], mavenPropertyOpen)
		}
		value = value[end+len(mavenPropertyClose):]
	}
}

// setsAny reports whether properties a profile sets include one of the
// named ones, or, with every name in question, any property at all.
func setsAny(properties map[string]string, names map[string]bool, every bool) bool {
	if every {
		return len(properties) > 0
	}
	for key := range properties {
		if names[key] {
			return true
		}
	}
	return false
}

// profilesOf returns the profiles a POM declares.
func profilesOf(pom *POM) []Profile {
	if pom.Profiles == nil {
		return nil
	}
	return pom.Profiles.Profile
}

// buildHelperArtifactID is the plugin whose add-source and add-test-source
// goals add source roots during the build, listed under <sources>.
const buildHelperArtifactID = "build-helper-maven-plugin"

// reading is a set of properties to resolve a declared value with. An
// optional reading is one context among several, and one that leaves a
// property unresolved there is not a value the build can take.
type reading struct {
	props    map[string]string
	optional bool
}

// valueRef is a value a POM declares and the reading to resolve it with.
// An ambiguous one rests on properties profiles could set together, so no
// reading settles it.
type valueRef struct {
	raw string
	reading
	ambiguous bool
}

// addedSourceRoots returns every source root build-helper-maven-plugin
// declares anywhere in the module's ancestry: in <plugins>,
// <pluginManagement> and every profile, whether or not it runs. Counting
// a root that never gets added can only withhold an entry. The POM body's
// declarations are read under every profile's properties, and a profile's,
// the plugin's identity included, in each context the profile can take
// effect in.
func (m *moduleContext) addedSourceRoots() []valueRef {
	var roots []valueRef
	for level, pom := range m.chain {
		roots = append(roots, m.buildHelperSources(buildDeclaredPlugins(pom.Build), m.everyProfileReading(), nil)...)
		profiles := profilesOf(pom)
		for i := range profiles {
			profile := &profiles[i]
			roots = append(roots, m.buildHelperSources(buildDeclaredPlugins(profile.Build), m.profileReadings(level, *profile), profile)...)
		}
	}
	return roots
}

// buildHelperSources returns, for each reading, the <sources> entries of
// the plugins that reading identifies as build-helper-maven-plugin, at
// plugin level and in executions, to resolve with that same reading.
// declaring is the profile the plugins sit in, nil for the POM body.
func (m *moduleContext) buildHelperSources(plugins []Plugin, readings []reading, declaring *Profile) []valueRef {
	var sources []valueRef
	for _, read := range readings {
		for _, plugin := range plugins {
			if resolvedIn(plugin.ArtifactID, read.props) != buildHelperArtifactID {
				continue
			}
			entries := sourcesIn(plugin.Configuration)
			for _, declared := range plugin.Executions {
				entries = append(entries, sourcesIn(declared.Configuration)...)
			}
			for _, raw := range entries {
				sources = append(sources, valueRef{raw: raw, reading: read, ambiguous: m.isAmbiguous(raw, declaring)})
			}
		}
	}
	return sources
}

// profileReadings returns the contexts a profile's declarations can take
// effect in. When the profile activates, its own properties are merged
// over its POM's, so that reading must resolve. Alongside the profiles
// already active, a later one of which may win, it reads with the module's
// properties, a context that counts only where it resolves.
func (m *moduleContext) profileReadings(level int, profile Profile) []reading {
	return []reading{
		{props: m.propertiesWith(level, &profile)},
		{props: m.props, optional: true},
	}
}

// sourcesIn returns the entries of a configuration's <sources> list.
func sourcesIn(config *PluginConfiguration) []string {
	list, _ := config.find("sources")
	if list == nil {
		return nil
	}
	var sources []string
	for _, entry := range list.Children {
		if source := strings.TrimSpace(entry.Value); source != "" {
			sources = append(sources, source)
		}
	}
	return sources
}
