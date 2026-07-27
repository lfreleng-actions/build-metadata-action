// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package java

import "encoding/xml"

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
