// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package extractor

import (
	"fmt"
)

// ProjectMetadata contains metadata extracted from a project
type ProjectMetadata struct {
	// Common fields
	Name          string
	Version       string
	VersionSource string
	Description   string
	License       string
	Authors       []string
	Homepage      string
	Repository    string

	// Language-specific metadata stored as key-value pairs
	// Keys should be namespaced by language (e.g., "python_requires_python")
	LanguageSpecific map[string]interface{}
}

// Extractor is the interface that all language-specific extractors must implement
type Extractor interface {
	// Extract retrieves metadata from the project at the given path
	Extract(projectPath string) (*ProjectMetadata, error)

	// Detect checks if this extractor can handle the project at the given path
	Detect(projectPath string) bool

	// Name returns the name of this extractor (e.g., "python", "java-maven")
	Name() string

	// Priority returns the priority of this extractor (higher = checked first)
	Priority() int
}

// Registry maintains a collection of available extractors
type Registry struct {
	extractors map[string]Extractor
}

// NewRegistry creates a new extractor registry
func NewRegistry() *Registry {
	return &Registry{
		extractors: make(map[string]Extractor),
	}
}

// Register adds an extractor to the registry
func (r *Registry) Register(extractor Extractor) {
	r.extractors[extractor.Name()] = extractor
}

// Get retrieves an extractor by name
func (r *Registry) Get(name string) (Extractor, error) {
	extractor, ok := r.extractors[name]
	if !ok {
		return nil, fmt.Errorf("no extractor found for type: %s", name)
	}
	return extractor, nil
}

// GetAll returns all registered extractors
func (r *Registry) GetAll() []Extractor {
	extractors := make([]Extractor, 0, len(r.extractors))
	for _, e := range r.extractors {
		extractors = append(extractors, e)
	}
	return extractors
}

// Global registry instance
var globalRegistry = NewRegistry()

// RegisterExtractor adds an extractor to the global registry
func RegisterExtractor(extractor Extractor) {
	globalRegistry.Register(extractor)
}

// GetExtractor retrieves an extractor by name from the global registry
// It handles mapping from detector project types (e.g., "python-modern") to
// extractor names (e.g., "python")
func GetExtractor(name string) (Extractor, error) {
	// Map detector project types to extractor names
	extractorName := mapProjectTypeToExtractor(name)
	return globalRegistry.Get(extractorName)
}

// mapProjectTypeToExtractor converts detector project types to extractor names
func mapProjectTypeToExtractor(projectType string) string {
	// Handle Python variants
	if projectType == "python-modern" || projectType == "python-legacy" || projectType == "python-setup-cfg" {
		return "python"
	}

	// Handle JavaScript/TypeScript variants
	if projectType == "javascript-npm" || projectType == "javascript-yarn" ||
		projectType == "javascript-pnpm" || projectType == "typescript-npm" {
		return "javascript"
	}

	// Handle Java variants
	if projectType == "java-maven" {
		return "java-maven"
	}
	if projectType == "java-gradle" || projectType == "java-gradle-kts" || projectType == "kotlin-gradle" {
		return "java-gradle"
	}

	// Handle .NET variants
	if projectType == "csharp-project" || projectType == "csharp-solution" ||
		projectType == "csharp-props" || projectType == "dotnet-project" {
		return "dotnet"
	}

	// Handle Go variants
	if projectType == "go-module" {
		return "go-module"
	}

	// Handle Rust variants
	if projectType == "rust-cargo" {
		return "rust-cargo"
	}

	// Handle Ruby variants
	if projectType == "ruby-gemspec" || projectType == "ruby-bundler" {
		return "ruby"
	}

	// Handle PHP variants
	if projectType == "php-composer" {
		return "php"
	}

	// Handle Swift variants
	if projectType == "swift-package" {
		return "swift"
	}

	// Handle Dart/Flutter variants
	if projectType == "dart-flutter" || projectType == "dart-package" {
		return "dart"
	}

	// Handle Elixir variants
	if projectType == "elixir-mix" {
		return "elixir"
	}

	// Handle Scala variants
	if projectType == "scala-sbt" {
		return "scala"
	}

	// Handle Haskell variants
	if projectType == "haskell-cabal" {
		return "haskell"
	}

	// Handle Julia variants
	if projectType == "julia-project" {
		return "julia"
	}

	// Handle C/C++ variants
	if projectType == "c-cmake" || projectType == "c-qmake" || projectType == "c-autoconf" || projectType == "c-autoconf-legacy" || projectType == "c-meson" {
		return "cpp"
	}

	// Handle Docker
	if projectType == "docker" {
		return "docker"
	}

	// Handle Helm variants
	if projectType == "helm" || projectType == "helm-chart" {
		return "helm"
	}

	// Handle Terraform variants
	if projectType == "terraform" || projectType == "terraform-module" {
		return "terraform"
	}

	// Return original if no mapping found
	return projectType
}

// GetAllExtractors returns all registered extractors
func GetAllExtractors() []Extractor {
	return globalRegistry.GetAll()
}

// BaseExtractor provides common functionality for all extractors
type BaseExtractor struct {
	name     string
	priority int
}

// NewBaseExtractor creates a new base extractor
func NewBaseExtractor(name string, priority int) BaseExtractor {
	return BaseExtractor{
		name:     name,
		priority: priority,
	}
}

// Name returns the extractor name
func (b *BaseExtractor) Name() string {
	return b.name
}

// Priority returns the extractor priority
func (b *BaseExtractor) Priority() int {
	return b.priority
}
