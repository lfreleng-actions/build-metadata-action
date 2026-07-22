// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package rust

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/lfreleng-actions/build-metadata-action/internal/extractor"
)

// Extractor extracts metadata from Rust projects
type Extractor struct {
	extractor.BaseExtractor
}

// NewExtractor creates a new Rust extractor
func NewExtractor() *Extractor {
	return &Extractor{
		BaseExtractor: extractor.NewBaseExtractor("rust-cargo", 1),
	}
}

// CargoToml represents the structure of a Cargo.toml file
type CargoToml struct {
	Package           Package                           `toml:"package"`
	Dependencies      map[string]interface{}            `toml:"dependencies"`
	DevDependencies   map[string]interface{}            `toml:"dev-dependencies"`
	BuildDependencies map[string]interface{}            `toml:"build-dependencies"`
	Features          map[string][]string               `toml:"features"`
	Workspace         Workspace                         `toml:"workspace"`
	Bin               []Bin                             `toml:"bin"`
	Lib               Lib                               `toml:"lib"`
	Profile           map[string]map[string]interface{} `toml:"profile"`
}

// Package represents the [package] section of Cargo.toml
type Package struct {
	Name          string                 `toml:"name"`
	Version       interface{}            `toml:"version"`      // Can be string or map (workspace inheritance)
	Authors       interface{}            `toml:"authors"`      // Can be []string or map (workspace inheritance)
	Edition       interface{}            `toml:"edition"`      // Can be string or map (workspace inheritance)
	RustVersion   interface{}            `toml:"rust-version"` // Can be string or map (workspace inheritance)
	Description   interface{}            `toml:"description"`  // Can be string or map (workspace inheritance)
	Documentation string                 `toml:"documentation"`
	Homepage      interface{}            `toml:"homepage"`   // Can be string or map (workspace inheritance)
	Repository    interface{}            `toml:"repository"` // Can be string or map (workspace inheritance)
	License       interface{}            `toml:"license"`    // Can be string or map (workspace inheritance)
	LicenseFile   string                 `toml:"license-file"`
	Keywords      interface{}            `toml:"keywords"`   // Can be []string or map (workspace inheritance)
	Categories    interface{}            `toml:"categories"` // Can be []string or map (workspace inheritance)
	Readme        interface{}            `toml:"readme"`     // Can be string or map (workspace inheritance)
	Publish       interface{}            `toml:"publish"`
	Metadata      map[string]interface{} `toml:"metadata"`
	DefaultRun    string                 `toml:"default-run"`
	AutoBenches   bool                   `toml:"autobins"`
	AutoExamples  bool                   `toml:"autoexamples"`
	AutoTests     bool                   `toml:"autotests"`
	Build         string                 `toml:"build"`
}

// Workspace represents the [workspace] section of Cargo.toml
type Workspace struct {
	Members  []string         `toml:"members"`
	Exclude  []string         `toml:"exclude"`
	Resolver string           `toml:"resolver"`
	Package  WorkspacePackage `toml:"package"`
}

// WorkspacePackage represents workspace-level package metadata
type WorkspacePackage struct {
	Version     string   `toml:"version"`
	Authors     []string `toml:"authors"`
	Edition     string   `toml:"edition"`
	RustVersion string   `toml:"rust-version"`
	Description string   `toml:"description"`
	Homepage    string   `toml:"homepage"`
	Repository  string   `toml:"repository"`
	License     string   `toml:"license"`
	Keywords    []string `toml:"keywords"`
	Categories  []string `toml:"categories"`
}

// Bin represents a [[bin]] section
type Bin struct {
	Name string `toml:"name"`
	Path string `toml:"path"`
}

// Lib represents the [lib] section
type Lib struct {
	Name      string   `toml:"name"`
	Path      string   `toml:"path"`
	CrateType []string `toml:"crate-type"`
}

// Dependency represents a parsed dependency
type Dependency struct {
	Name     string
	Version  string
	Optional bool
	Features []string
	Source   string // "crates.io", "git", "path", etc.
}

// Extract retrieves metadata from a Rust project
func (e *Extractor) Extract(projectPath string) (*extractor.ProjectMetadata, error) {
	metadata := &extractor.ProjectMetadata{
		LanguageSpecific: make(map[string]interface{}),
	}

	// Try Cargo.toml file
	cargoTomlPath := filepath.Join(projectPath, "Cargo.toml")
	if _, err := os.Stat(cargoTomlPath); err == nil {
		if err := e.extractFromCargoToml(cargoTomlPath, metadata); err != nil {
			return nil, err
		}
		return metadata, nil
	}

	return nil, fmt.Errorf("no Cargo.toml file found in %s", projectPath)
}

// Detect checks if this extractor can handle the project
func (e *Extractor) Detect(projectPath string) bool {
	if _, err := os.Stat(filepath.Join(projectPath, "Cargo.toml")); err == nil {
		return true
	}

	return false
}

// quoteStrings adds quotes around each string
func quoteStrings(strs []string) []string {
	quoted := make([]string, len(strs))
	for i, s := range strs {
		quoted[i] = fmt.Sprintf(`"%s"`, s)
	}
	return quoted
}

// init registers the Rust extractor
func init() {
	extractor.RegisterExtractor(NewExtractor())
}
