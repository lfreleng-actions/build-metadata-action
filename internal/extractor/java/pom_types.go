// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package java

import (
	"encoding/xml"
	"strings"
)

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
	Directory           string            `xml:"directory"`
	SourceDirectory     string            `xml:"sourceDirectory"`
	TestSourceDirectory string            `xml:"testSourceDirectory"`
	OutputDirectory     string            `xml:"outputDirectory"`
	TestOutputDirectory string            `xml:"testOutputDirectory"`
	FinalName           string            `xml:"finalName"`
	Plugins             *Plugins          `xml:"plugins"`
	PluginManagement    *PluginManagement `xml:"pluginManagement"`
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
	Inherited     *string              `xml:"inherited"`
	Configuration *PluginConfiguration `xml:"configuration"`
	Executions    []Execution          `xml:"executions>execution"`
}

// Execution represents one <execution> of a plugin. Code generators are
// usually configured here rather than at plugin level, one execution per
// generated API or schema.
type Execution struct {
	ID            string               `xml:"id"`
	Phase         string               `xml:"phase"`
	Inherited     *string              `xml:"inherited"`
	Goals         []string             `xml:"goals>goal"`
	Configuration *PluginConfiguration `xml:"configuration"`
}

// PluginConfiguration captures a plugin's <configuration>. The
// maven-compiler-plugin language level is decoded into named fields;
// every other element is kept as a generic tree in Elements, because each
// plugin defines its own parameters and some nest them (openapi-generator
// reads configOptions/sourceFolder, jOOQ generator/target/directory).
// Attributes are kept for Maven's combine.self merge control.
type PluginConfiguration struct {
	Release  string          `xml:"release"`
	Source   string          `xml:"source"`
	Target   string          `xml:"target"`
	Attrs    []xml.Attr      `xml:",any,attr"`
	Elements []ConfigElement `xml:",any"`
}

// ConfigElement is one element of a plugin configuration, with its
// attributes, its text and any nested elements.
type ConfigElement struct {
	XMLName  xml.Name
	Attrs    []xml.Attr      `xml:",any,attr"`
	Value    string          `xml:",chardata"`
	Children []ConfigElement `xml:",any"`
}

// find returns the element at a slash-separated path, such as
// "configOptions/sourceFolder", and whether the configuration or an
// element on the way to it carries combine.self="override". Maven merges
// nothing from farther up the inheritance chain into an overriding
// element, so a parameter it omits stays unset rather than inherited.
func (c *PluginConfiguration) find(elementPath string) (*ConfigElement, bool) {
	if c == nil {
		return nil, false
	}
	overridden := overridesInherited(c.Attrs)
	elements := c.Elements
	var element *ConfigElement
	for _, name := range strings.Split(elementPath, "/") {
		if element = findElement(elements, name); element == nil {
			return nil, overridden
		}
		overridden = overridden || overridesInherited(element.Attrs)
		elements = element.Children
	}
	return element, overridden
}

// isSet reports whether an element carries a value or nested elements.
// Maven fills an empty element from farther up the inheritance chain.
func (e *ConfigElement) isSet() bool {
	return strings.TrimSpace(e.Value) != "" || len(e.Children) > 0
}

// overridesInherited reports a combine.self="override" attribute.
func overridesInherited(attrs []xml.Attr) bool {
	for _, attr := range attrs {
		if attr.Name.Local == "combine.self" && strings.TrimSpace(attr.Value) == "override" {
			return true
		}
	}
	return false
}

// childValue returns the trimmed text of a direct child element.
func (e ConfigElement) childValue(name string) string {
	if child := findElement(e.Children, name); child != nil {
		return strings.TrimSpace(child.Value)
	}
	return ""
}

// findElement returns the first element with the given local name.
func findElement(elements []ConfigElement, name string) *ConfigElement {
	for i := range elements {
		if elements[i].XMLName.Local == name {
			return &elements[i]
		}
	}
	return nil
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

// Profile represents a single Maven profile. When active, Maven merges it
// into its POM before inheritance, the profile winning: it can add
// properties, dependencies, plugins and modules and move the build
// directory, but cannot set the source directories.
type Profile struct {
	ID             string          `xml:"id"`
	Activation     *Activation     `xml:"activation"`
	Properties     Properties      `xml:"properties"`
	Dependencies   *Dependencies   `xml:"dependencies"`
	DependencyMgmt *DependencyMgmt `xml:"dependencyManagement"`
	Build          *Build          `xml:"build"`
	Modules        *Modules        `xml:"modules"`
}

// Activation represents profile activation conditions. ActiveByDefault is
// kept as text and read as Maven reads it, Boolean.valueOf, so a value
// such as a property reference does not stop the POM parsing.
type Activation struct {
	ActiveByDefault string              `xml:"activeByDefault"`
	JDK             *string             `xml:"jdk"`
	OS              *struct{}           `xml:"os"`
	Property        *ActivationProperty `xml:"property"`
	File            *ActivationFile     `xml:"file"`
	Packaging       *string             `xml:"packaging"`
	Condition       *string             `xml:"condition"`
}

// ActivationProperty is a property condition on a profile.
type ActivationProperty struct {
	Name  string `xml:"name"`
	Value string `xml:"value"`
}

// ActivationFile is a file condition on a profile.
type ActivationFile struct {
	Exists  string `xml:"exists"`
	Missing string `xml:"missing"`
}
