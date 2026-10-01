// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package java

import (
	"slices"
	"strings"
)

// isDeclaredParent reports whether the POM found at a relativePath is the
// parent the child names, as Maven's readParentLocally decides it: the
// candidate's groupId, its own or inherited, its artifactId and its
// version, its own or inherited, must equal the declared ones as raw
// strings. Maven also accepts a declared range that contains the
// candidate's version; without Maven's version ordering the action treats
// a range as a mismatch, and a missing version too, which Maven's
// validation rejects anyway. Both are the safe side, since the chain then
// counts as reaching a parent off disk. On a mismatch Maven falls back to
// the repository: a reactor root sitting at the default ../pom.xml is
// often an aggregator rather than the parent, and a local checkout of
// another version is not the parent either.
func isDeclaredParent(child, parent *POM) bool {
	declared := child.Parent
	groupID, artifactID := strings.TrimSpace(effectiveGroupID(parent)), strings.TrimSpace(parent.ArtifactID)
	if groupID == "" || groupID != strings.TrimSpace(declared.GroupID) ||
		artifactID == "" || artifactID != strings.TrimSpace(declared.ArtifactID) {
		return false
	}
	version := strings.TrimSpace(effectiveVersion(parent))
	return version != "" && version == strings.TrimSpace(declared.Version)
}

// effectiveGroupID returns a POM's groupId, inherited from its own parent
// when it declares none.
func effectiveGroupID(pom *POM) string {
	if pom.GroupID != "" || pom.Parent == nil {
		return pom.GroupID
	}
	return pom.Parent.GroupID
}

// effectiveVersion returns a POM's version, inherited from its own parent
// when it declares none.
func effectiveVersion(pom *POM) string {
	if pom.Version != "" || pom.Parent == nil {
		return pom.Version
	}
	return pom.Parent.Version
}

// isInherited reports whether a plugin or execution reaches child modules,
// read as Maven reads the flag: an absent element means inherited, and a
// present one counts only when it is true in any case, so FALSE, no and an
// empty <inherited/> all withhold it.
func isInherited(flag *string) bool {
	return flag == nil || strings.EqualFold(strings.TrimSpace(*flag), "true")
}

// inheritedDeclarations returns a plugin's declarations in one section of
// the chain, <plugins> or <pluginManagement>, as they reach the module,
// nearest first; none when the plugin does not reach it. Maven applies
// inheritance one level at a time (DefaultInheritanceAssembler), so this
// replays it from the farthest parent down: at each step a plugin passes
// to the child if it is inherited or has executions, its configuration
// only if it is inherited, and each execution by the nearest inherited
// flag set on it, else by the plugin's. A level marking the plugin not
// inherited thereby also stops what it inherited from above, except
// executions explicitly marked inherited.
func inheritedDeclarations(chain []*POM, section func(*POM) []Plugin, key string) []Plugin {
	var layers []Plugin
	for depth := len(chain) - 1; depth >= 0; depth-- {
		if len(layers) > 0 {
			layers = passToChild(layers, declarationsOf(section(chain[depth+1]), key))
		}
		layers = append(declarationsOf(section(chain[depth]), key), layers...)
	}
	return layers
}

// passToChild applies one step of inheritance to the declarations a level
// holds, nearest first, returning none when the plugin stops there. own
// are that level's own declarations, an active profile's first, which
// alone set whether the plugin is inherited: a flag from above is only
// ever passed down when it already allowed inheritance.
func passToChild(layers, own []Plugin) []Plugin {
	inherited := isInherited(pluginFlag(own))
	if !inherited && !hasExecutions(layers) {
		return nil
	}
	flags := executionFlags(layers)
	passed := make([]Plugin, 0, len(layers))
	for _, layer := range layers {
		if !inherited {
			layer.Configuration = nil
		}
		executions := make([]Execution, 0, len(layer.Executions))
		for _, declared := range layer.Executions {
			flag := flags[executionID(declared)]
			if (flag == nil && inherited) || (flag != nil && isInherited(flag)) {
				executions = append(executions, declared)
			}
		}
		layer.Executions = executions
		passed = append(passed, layer)
	}
	return passed
}

// executionFlags returns each execution's inherited flag as Maven merges
// it: the nearest declaration that sets one.
func executionFlags(layers []Plugin) map[string]*string {
	flags := make(map[string]*string)
	for _, layer := range layers {
		for _, declared := range layer.Executions {
			id := executionID(declared)
			if _, set := flags[id]; !set && declared.Inherited != nil {
				flags[id] = declared.Inherited
			}
		}
	}
	return flags
}

// hasExecutions reports whether any declaration carries an execution.
func hasExecutions(layers []Plugin) bool {
	for _, layer := range layers {
		if len(layer.Executions) > 0 {
			return true
		}
	}
	return false
}

// declarationsOf returns copies of the plugins identified by key, in the
// order given: an active profile's declaration ahead of the POM's own.
func declarationsOf(plugins []Plugin, key string) []Plugin {
	var matching []Plugin
	for _, plugin := range plugins {
		if pluginKey(plugin) == key {
			matching = append(matching, plugin)
		}
	}
	return matching
}

// pluginFlag returns the inherited flag of a level's declarations as
// Maven merges them: the first that sets one, or nil.
func pluginFlag(own []Plugin) *string {
	for _, plugin := range own {
		if plugin.Inherited != nil {
			return plugin.Inherited
		}
	}
	return nil
}

// executionID returns an execution's id, applying Maven's default.
func executionID(declared Execution) string {
	if id := strings.TrimSpace(declared.ID); id != "" {
		return id
	}
	return "default"
}

// execution is one effective execution of a plugin after inheritance: the
// goals of every declaration, as Maven combines them, its phase from the
// nearest declaration naming one, and its configuration from every
// declaration, nearest first.
type execution struct {
	id      string
	goals   []string
	phase   string
	configs []*PluginConfiguration
}

// bindable reports whether Maven can schedule the execution in a build.
// Bound to the phase none, it is switched off, the way a POM disables one
// it inherits; bound to a phase that keeps a property unresolved, it names
// no lifecycle phase that exists. An execution with no phase runs at its
// goal's default.
func (e *execution) bindable() bool {
	return e.phase != "none" && !isUnresolvedPlaceholder(e.phase)
}

// absorb folds a declaration into the execution. Goals accumulate, so a
// module adding a goal to an inherited execution keeps the inherited one;
// a phase a nearer declaration set is kept.
func (e *execution) absorb(declared Execution) {
	for _, goal := range declared.Goals {
		if goal = strings.TrimSpace(goal); goal != "" && !slices.Contains(e.goals, goal) {
			e.goals = append(e.goals, goal)
		}
	}
	if e.phase == "" {
		e.phase = strings.TrimSpace(declared.Phase)
	}
	if declared.Configuration != nil {
		e.configs = append(e.configs, declared.Configuration)
	}
}

// mergeExecutions folds a plugin's executions across its declarations by
// id, as Maven does. A generator goal runs only when an execution binds
// it, so an execution naming no goal in any declaration this action can
// read contributes nothing, and none is invented for it.
func mergeExecutions(layers []Plugin) []*execution {
	byID := make(map[string]*execution)
	var ordered []*execution
	for _, layer := range layers {
		for _, declared := range layer.Executions {
			id := executionID(declared)
			merged, seen := byID[id]
			if !seen {
				merged = &execution{id: id}
				byID[id] = merged
				ordered = append(ordered, merged)
			}
			merged.absorb(declared)
		}
	}
	return ordered
}

// configured returns the nearest declaration of a configuration element
// for one execution, searching the execution's configuration and then the
// plugin-level configuration, each nearest first, as Maven merges them. A
// combine.self="override" cuts the search off: what lies beyond it does
// not reach the execution.
func configured(elementPath string, exec *execution, layers []Plugin) *ConfigElement {
	for _, config := range configChain(exec, layers) {
		element, overridden := config.find(elementPath)
		if element != nil && element.isSet() {
			return element
		}
		if overridden {
			return nil
		}
	}
	return nil
}

// configChain returns the configurations that apply to one execution in
// the order Maven merges them: the execution's own, then the plugin-level
// ones, each nearest first.
func configChain(exec *execution, layers []Plugin) []*PluginConfiguration {
	chain := append([]*PluginConfiguration{}, exec.configs...)
	for _, layer := range layers {
		chain = append(chain, layer.Configuration)
	}
	return chain
}

// pluginVersion returns the version a plugin resolves to in the module:
// the nearest declaration naming one, with properties expanded.
func pluginVersion(layers []Plugin, props map[string]string) string {
	for _, layer := range layers {
		if version := strings.TrimSpace(layer.Version); version != "" {
			return resolveProperty(version, props)
		}
	}
	return ""
}

// buildPlugins returns a POM's <build><plugins>.
func buildPlugins(pom *POM) []Plugin {
	if pom.Build == nil || pom.Build.Plugins == nil {
		return nil
	}
	return pom.Build.Plugins.Plugin
}

// managedPlugins returns a POM's <build><pluginManagement><plugins>.
func managedPlugins(pom *POM) []Plugin {
	if pom.Build == nil || pom.Build.PluginManagement == nil || pom.Build.PluginManagement.Plugins == nil {
		return nil
	}
	return pom.Build.PluginManagement.Plugins.Plugin
}

// buildDeclaredPlugins returns the plugins and managed plugins of one
// <build> section.
func buildDeclaredPlugins(build *Build) []Plugin {
	if build == nil {
		return nil
	}
	var plugins []Plugin
	if build.Plugins != nil {
		plugins = append(plugins, build.Plugins.Plugin...)
	}
	if build.PluginManagement != nil && build.PluginManagement.Plugins != nil {
		plugins = append(plugins, build.PluginManagement.Plugins.Plugin...)
	}
	return plugins
}

// declaredDependencies returns a POM's <dependencies>.
func declaredDependencies(pom *POM) []Dependency {
	if pom.Dependencies == nil {
		return nil
	}
	return pom.Dependencies.Dependency
}

// managedDependencies returns a POM's <dependencyManagement> entries.
func managedDependencies(pom *POM) []Dependency {
	if pom.DependencyMgmt == nil || pom.DependencyMgmt.Dependencies == nil {
		return nil
	}
	return pom.DependencyMgmt.Dependencies.Dependency
}

// dependencyKey identifies a dependency as Maven merges it across
// inheritance: by groupId, artifactId, type and classifier.
func dependencyKey(dep Dependency) string {
	depType := strings.TrimSpace(dep.Type)
	if depType == "" {
		depType = "jar"
	}
	return strings.Join([]string{
		strings.TrimSpace(dep.GroupID), strings.TrimSpace(dep.ArtifactID),
		depType, strings.TrimSpace(dep.Classifier),
	}, ":")
}
