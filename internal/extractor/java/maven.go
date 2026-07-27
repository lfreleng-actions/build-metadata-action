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
