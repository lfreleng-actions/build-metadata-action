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
	SourceDirectory string   `xml:"sourceDirectory"`
	FinalName       string   `xml:"finalName"`
	Plugins         *Plugins `xml:"plugins"`
}

// Plugins represents Maven plugins
type Plugins struct {
	Plugin []Plugin `xml:"plugin"`
}

// Plugin represents a single Maven plugin
type Plugin struct {
	GroupID    string `xml:"groupId"`
	ArtifactID string `xml:"artifactId"`
	Version    string `xml:"version"`
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

// applyPOMProperties records declared properties and derives the Java version
// and revision-based dynamic versioning hints from well-known keys.
func applyPOMProperties(pom *POM, metadata *extractor.ProjectMetadata) {
	if len(pom.Properties.Entries) == 0 {
		return
	}
	metadata.LanguageSpecific["properties"] = pom.Properties.Entries
	metadata.LanguageSpecific["property_count"] = len(pom.Properties.Entries)

	if javaVersion, ok := pom.Properties.Entries["maven.compiler.source"]; ok {
		metadata.LanguageSpecific["java_version"] = javaVersion
	} else if javaVersion, ok := pom.Properties.Entries["java.version"]; ok {
		metadata.LanguageSpecific["java_version"] = javaVersion
	}

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

// resolveProperty resolves a single property value
func resolveProperty(value string, props map[string]string) string {
	if !strings.Contains(value, mavenPropertyOpen) {
		return value
	}

	// Simple property resolution
	for key, val := range props {
		placeholder := mavenPropertyOpen + key + mavenPropertyClose
		if strings.Contains(value, placeholder) {
			value = strings.ReplaceAll(value, placeholder, val)
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
