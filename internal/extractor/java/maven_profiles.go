// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package java

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// profileState is a profile's state in the default build: the build a
// plain mvn runs, with no -P selecting profiles and no -D setting
// properties.
type profileState int

const (
	profileInactive profileState = iota
	// profileUnknown marks a state that turns on what the action cannot
	// see: the JDK, the operating system, or a property the command line or
	// environment may set.
	profileUnknown
	profileActive
)

// activeProfiles returns the profiles of pom known to be active when Maven
// builds the module in projectDir, in declaration order. It replays
// Maven's DefaultProfileSelector: a profile is active when every condition
// it declares holds, and one marked activeByDefault joins only when no
// other profile of the same POM is activated. A profile whose state cannot
// be settled is left out, so its generators go unreported, the safe
// direction. Maven evaluates a parent's profiles for each module it
// builds, against that module's directory and packaging.
func activeProfiles(pom *POM, projectDir, packaging string) []Profile {
	if pom.Profiles == nil {
		return nil
	}
	profiles := pom.Profiles.Profile
	states := make([]profileState, len(profiles))
	for i := range profiles {
		states[i] = conditionState(profiles[i].Activation, pom, projectDir, packaging)
	}
	var active []Profile
	for i, profile := range profiles {
		if withDefault(profile, states, i) == profileActive {
			active = append(active, profile)
		}
	}
	return active
}

// withDefault folds activeByDefault into a profile's state. Such a profile
// is active by its own conditions, or else by default when no other
// profile is activated; another profile whose state is unknown leaves it
// unknown too.
func withDefault(profile Profile, states []profileState, index int) profileState {
	own := states[index]
	if own == profileActive || !isActiveByDefault(profile.Activation) {
		return own
	}
	others := profileInactive
	for i, state := range states {
		if i != index && state > others {
			others = state
		}
	}
	switch others {
	case profileActive:
		return own
	case profileUnknown:
		return profileUnknown
	}
	return profileActive
}

// isActiveByDefault reads activeByDefault as Maven does, Boolean.valueOf:
// true in any case, anything else false.
func isActiveByDefault(activation *Activation) bool {
	return activation != nil && strings.EqualFold(strings.TrimSpace(activation.ActiveByDefault), "true")
}

// conditionState evaluates the conditions a profile's activation declares,
// which Maven requires to hold together. A profile declaring none is not
// activated by them.
func conditionState(activation *Activation, pom *POM, projectDir, packaging string) profileState {
	if activation == nil {
		return profileInactive
	}
	var states []profileState
	if activation.JDK != nil || activation.OS != nil || activation.Condition != nil {
		states = append(states, profileUnknown)
	}
	if activation.Packaging != nil {
		states = append(states, stateOf(strings.TrimSpace(*activation.Packaging) == packaging))
	}
	if activation.Property != nil {
		states = append(states, propertyState(*activation.Property, packaging))
	}
	if activation.File != nil {
		states = append(states, fileState(*activation.File, pom, projectDir))
	}
	if len(states) == 0 {
		return profileInactive
	}
	combined := profileActive
	for _, state := range states {
		if state == profileInactive {
			return profileInactive
		}
		if state == profileUnknown {
			combined = profileUnknown
		}
	}
	return combined
}

// stateOf turns a settled condition into a state.
func stateOf(holds bool) profileState {
	if holds {
		return profileActive
	}
	return profileInactive
}

// propertyState evaluates a property condition as PropertyProfileActivator
// does. The one property the action can read is packaging, which Maven
// sets from the module; any other may come from the command line or the
// environment.
func propertyState(property ActivationProperty, packaging string) profileState {
	name := strings.TrimSpace(property.Name)
	reverseName := strings.HasPrefix(name, "!")
	name = strings.TrimPrefix(name, "!")
	if name == "" {
		return profileInactive
	}
	if name != "packaging" {
		return profileUnknown
	}
	if value := strings.TrimSpace(property.Value); value != "" {
		reverseValue := strings.HasPrefix(value, "!")
		return stateOf((strings.TrimPrefix(value, "!") == packaging) != reverseValue)
	}
	return stateOf((packaging != "") != reverseName)
}

// fileState evaluates a file condition as FileProfileActivator does: the
// path is interpolated with the module directory and the declaring POM's
// own properties, a relative one is taken against the module directory,
// and exists is read before missing. A path naming any other property
// depends on the command line or the environment.
func fileState(file ActivationFile, pom *POM, projectDir string) profileState {
	path, missing := strings.TrimSpace(file.Exists), false
	if path == "" {
		path, missing = strings.TrimSpace(file.Missing), true
	}
	if path == "" {
		return profileInactive
	}
	basedir, err := filepath.Abs(projectDir)
	if err != nil {
		return profileUnknown
	}
	props := make(map[string]string, len(pom.Properties.Entries)+2)
	for key, value := range pom.Properties.Entries {
		props[key] = value
	}
	props["basedir"] = basedir
	props["project.basedir"] = basedir
	path = resolveProperty(path, props)
	if isUnresolvedPlaceholder(path) {
		return profileUnknown
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(basedir, path)
	}
	exists, known := fileExists(path)
	if !known {
		return profileUnknown
	}
	return stateOf(exists != missing)
}

// fileExists reports whether a path exists, following links as Java's
// File.exists does. Nothing outside the workspace is probed: a link out of
// it leaves the answer unknown, so a crafted repository cannot learn about
// the runner's file system from which profiles the action reports.
func fileExists(path string) (exists, known bool) {
	path = filepath.Clean(path)
	if !withinWorkspace(path) {
		return false, false
	}
	below := ""
	for probe := path; ; probe = filepath.Dir(probe) {
		resolved, err := filepath.EvalSymlinks(probe)
		if err == nil {
			if !withinWorkspace(resolved) {
				return false, false
			}
			return probe == path, probe == path || isAbsent(below)
		}
		if !errors.Is(err, fs.ErrNotExist) || filepath.Dir(probe) == probe {
			return false, false
		}
		below = probe
	}
}

// isAbsent reports whether nothing, not even a dangling link, sits at a
// path whose parent resolves inside the workspace. A dangling link points
// somewhere unknown, so it settles nothing.
func isAbsent(path string) bool {
	_, err := os.Lstat(path)
	return errors.Is(err, fs.ErrNotExist)
}

// withActiveProfiles returns pom with its active profiles merged in, as
// Maven's DefaultProfileInjector does before inheritance: each profile
// wins over the POM and over the profiles before it. Lists keep the
// winning declarations first, the order every lookup here reads them in;
// modules a profile adds join the POM's.
func withActiveProfiles(pom *POM, profiles []Profile) *POM {
	if len(profiles) == 0 {
		return pom
	}
	effective := *pom
	build := Build{}
	if pom.Build != nil {
		build = *pom.Build
	}
	entries := make(map[string]string, len(pom.Properties.Entries))
	for key, value := range pom.Properties.Entries {
		entries[key] = value
	}
	modules := append([]string{}, declaredModules(pom)...)
	for _, profile := range profiles {
		for key, value := range profile.Properties.Entries {
			entries[key] = value
		}
		if profile.Build != nil && strings.TrimSpace(profile.Build.Directory) != "" {
			build.Directory = profile.Build.Directory
		}
		if profile.Build != nil && strings.TrimSpace(profile.Build.FinalName) != "" {
			build.FinalName = profile.Build.FinalName
		}
		if profile.Modules != nil {
			modules = appendMissing(modules, profile.Modules.Module)
		}
	}

	var dependencies, managed []Dependency
	var plugins, managedPluginList []Plugin
	for i := len(profiles) - 1; i >= 0; i-- {
		profile := &profiles[i]
		dependencies = append(dependencies, dependencyList(profile.Dependencies)...)
		if profile.DependencyMgmt != nil {
			managed = append(managed, dependencyList(profile.DependencyMgmt.Dependencies)...)
		}
		plugins = append(plugins, sectionPlugins(profile.Build)...)
		managedPluginList = append(managedPluginList, sectionManagedPlugins(profile.Build)...)
	}
	effective.Properties = Properties{Entries: entries}
	effective.Dependencies = &Dependencies{Dependency: append(dependencies, declaredDependencies(pom)...)}
	effective.DependencyMgmt = &DependencyMgmt{Dependencies: &Dependencies{
		Dependency: append(managed, managedDependencies(pom)...),
	}}
	build.Plugins = &Plugins{Plugin: append(plugins, buildPlugins(pom)...)}
	build.PluginManagement = &PluginManagement{Plugins: &Plugins{
		Plugin: append(managedPluginList, managedPlugins(pom)...),
	}}
	effective.Build = &build
	effective.Modules = &Modules{Module: modules}
	return &effective
}

// appendMissing appends the entries not already present.
func appendMissing(list, entries []string) []string {
	for _, entry := range entries {
		if !slices.Contains(list, entry) {
			list = append(list, entry)
		}
	}
	return list
}

// dependencyList returns the entries of a <dependencies> element.
func dependencyList(dependencies *Dependencies) []Dependency {
	if dependencies == nil {
		return nil
	}
	return dependencies.Dependency
}

// sectionPlugins returns the <plugins> of one <build> section.
func sectionPlugins(build *Build) []Plugin {
	if build == nil || build.Plugins == nil {
		return nil
	}
	return build.Plugins.Plugin
}

// sectionManagedPlugins returns the <pluginManagement> plugins of one
// <build> section.
func sectionManagedPlugins(build *Build) []Plugin {
	if build == nil || build.PluginManagement == nil || build.PluginManagement.Plugins == nil {
		return nil
	}
	return build.PluginManagement.Plugins.Plugin
}

// projectPackaging returns a POM's packaging with Maven's default.
func projectPackaging(pom *POM) string {
	if packaging := strings.TrimSpace(pom.Packaging); packaging != "" {
		return packaging
	}
	return "jar"
}
