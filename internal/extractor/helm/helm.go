// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package helm

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/lfreleng-actions/build-metadata-action/internal/extractor"
)

// Extractor extracts metadata from Helm charts
type Extractor struct {
	extractor.BaseExtractor
}

// NewExtractor creates a new Helm extractor
func NewExtractor() *Extractor {
	return &Extractor{
		BaseExtractor: extractor.NewBaseExtractor("helm", 1),
	}
}

// ChartYAML represents the structure of a Chart.yaml file
type ChartYAML struct {
	APIVersion   string            `yaml:"apiVersion"`
	Name         string            `yaml:"name"`
	Version      string            `yaml:"version"`
	KubeVersion  string            `yaml:"kubeVersion"`
	Description  string            `yaml:"description"`
	Type         string            `yaml:"type"`
	Keywords     []string          `yaml:"keywords"`
	Home         string            `yaml:"home"`
	Sources      []string          `yaml:"sources"`
	Dependencies []Dependency      `yaml:"dependencies"`
	Maintainers  []Maintainer      `yaml:"maintainers"`
	Icon         string            `yaml:"icon"`
	AppVersion   string            `yaml:"appVersion"`
	Deprecated   bool              `yaml:"deprecated"`
	Annotations  map[string]string `yaml:"annotations"`
}

// Maintainer represents a chart maintainer
type Maintainer struct {
	Name  string `yaml:"name"`
	Email string `yaml:"email"`
	URL   string `yaml:"url"`
}

// Dependency represents a chart dependency
type Dependency struct {
	Name       string   `yaml:"name"`
	Version    string   `yaml:"version"`
	Repository string   `yaml:"repository"`
	Condition  string   `yaml:"condition"`
	Tags       []string `yaml:"tags"`
	Enabled    bool     `yaml:"enabled"`
	Alias      string   `yaml:"alias"`
}

// Extract retrieves metadata from a Helm chart
func (e *Extractor) Extract(projectPath string) (*extractor.ProjectMetadata, error) {
	metadata := &extractor.ProjectMetadata{
		LanguageSpecific: make(map[string]interface{}),
	}

	// Look for Chart.yaml
	chartPath := filepath.Join(projectPath, "Chart.yaml")
	if _, err := os.Stat(chartPath); err != nil {
		return nil, fmt.Errorf("Chart.yaml not found in %s", projectPath)
	}

	if err := e.extractFromChartYAML(chartPath, metadata); err != nil {
		return nil, err
	}

	return metadata, nil
}

// extractFromChartYAML extracts metadata from Chart.yaml
func (e *Extractor) extractFromChartYAML(path string, metadata *extractor.ProjectMetadata) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read Chart.yaml: %w", err)
	}

	var chart ChartYAML
	if err := yaml.Unmarshal(content, &chart); err != nil {
		return fmt.Errorf("failed to parse Chart.yaml: %w", err)
	}

	metadata.Name = chart.Name
	metadata.Version = chart.Version
	metadata.Description = chart.Description
	metadata.Homepage = chart.Home
	metadata.VersionSource = "Chart.yaml"
	metadata.Authors = chartAuthors(chart)

	// Use first source as repository if available
	if len(chart.Sources) > 0 {
		metadata.Repository = chart.Sources[0]
	}

	applyChartLanguageSpecific(chart, metadata)

	return nil
}

// chartAuthors formats maintainers as "Name <email>" (or just "Name").
func chartAuthors(chart ChartYAML) []string {
	authors := make([]string, 0, len(chart.Maintainers))
	for _, maintainer := range chart.Maintainers {
		if maintainer.Name == "" {
			continue
		}
		if maintainer.Email != "" {
			authors = append(authors, fmt.Sprintf("%s <%s>", maintainer.Name, maintainer.Email))
		} else {
			authors = append(authors, maintainer.Name)
		}
	}
	return authors
}

// applyChartLanguageSpecific populates the Helm-specific metadata fields.
func applyChartLanguageSpecific(chart ChartYAML, metadata *extractor.ProjectMetadata) {
	ls := metadata.LanguageSpecific
	ls["chart_name"] = chart.Name
	ls["api_version"] = chart.APIVersion
	ls["app_version"] = chart.AppVersion
	ls["chart_type"] = chart.Type
	ls["kube_version"] = chart.KubeVersion
	ls["deprecated"] = chart.Deprecated
	ls["metadata_source"] = "Chart.yaml"

	if chart.Icon != "" {
		ls["icon"] = chart.Icon
	}
	if len(chart.Keywords) > 0 {
		ls["keywords"] = chart.Keywords
	}
	if len(chart.Sources) > 0 {
		ls["sources"] = chart.Sources
	}
	if len(chart.Annotations) > 0 {
		ls["annotations"] = chart.Annotations
	}

	if deps := chartDependencies(chart); len(deps) > 0 {
		ls["dependencies"] = deps
		ls["dependency_count"] = len(deps)
	}

	applyKubernetesVersionMatrix(chart, metadata)

	chartType := chart.Type
	if chartType == "" {
		chartType = "application"
	}
	ls["is_library_chart"] = (chartType == "library")
}

// chartDependencies converts chart dependencies to serializable maps.
func chartDependencies(chart ChartYAML) []map[string]interface{} {
	if len(chart.Dependencies) == 0 {
		return nil
	}
	deps := make([]map[string]interface{}, 0, len(chart.Dependencies))
	for _, dep := range chart.Dependencies {
		depMap := map[string]interface{}{
			"name":       dep.Name,
			"version":    dep.Version,
			"repository": dep.Repository,
		}
		if dep.Alias != "" {
			depMap["alias"] = dep.Alias
		}
		if dep.Condition != "" {
			depMap["condition"] = dep.Condition
		}
		if len(dep.Tags) > 0 {
			depMap["tags"] = dep.Tags
		}
		deps = append(deps, depMap)
	}
	return deps
}

// applyKubernetesVersionMatrix derives a supported Kubernetes version matrix
// from the chart's kubeVersion constraint.
func applyKubernetesVersionMatrix(chart ChartYAML, metadata *extractor.ProjectMetadata) {
	if chart.KubeVersion == "" {
		return
	}
	matrix := generateKubernetesVersionMatrix(chart.KubeVersion)
	if len(matrix) == 0 {
		return
	}
	metadata.LanguageSpecific["kubernetes_version_matrix"] = matrix
	matrixJSON := fmt.Sprintf(`{"kubernetes-version": [%s]}`,
		strings.Join(quoteStrings(matrix), ", "))
	metadata.LanguageSpecific["matrix_json"] = matrixJSON
}

// Detect checks if this extractor can handle the project
func (e *Extractor) Detect(projectPath string) bool {
	chartPath := filepath.Join(projectPath, "Chart.yaml")
	if _, err := os.Stat(chartPath); err == nil {
		return true
	}

	return false
}

// Helper functions

// generateKubernetesVersionMatrix generates a list of Kubernetes versions from a constraint
func generateKubernetesVersionMatrix(kubeVersion string) []string {
	versions := []string{}

	// Parse common version constraints
	// Examples: ">=1.19.0", ">=1.20.0-0", "^1.20.0", "~1.20.0"

	minVersion := ""
	if strings.Contains(kubeVersion, ">=") {
		// Extract version after >=
		parts := strings.Split(kubeVersion, ">=")
		if len(parts) > 1 {
			version := strings.TrimSpace(parts[1])
			if idx := strings.IndexAny(version, " ,<"); idx != -1 {
				version = version[:idx]
			}
			versionParts := strings.Split(version, ".")
			if len(versionParts) >= 2 {
				minVersion = versionParts[0] + "." + versionParts[1]
			}
		}
	} else if strings.HasPrefix(kubeVersion, "^") || strings.HasPrefix(kubeVersion, "~") {
		// Semver range
		version := strings.TrimPrefix(strings.TrimPrefix(kubeVersion, "^"), "~")
		versionParts := strings.Split(version, ".")
		if len(versionParts) >= 2 {
			minVersion = versionParts[0] + "." + versionParts[1]
		}
	}

	// Map minimum version to supported versions
	supportedVersions := map[string][]string{
		"1.19": {"1.19", "1.20", "1.21", "1.22", "1.23", "1.24", "1.25"},
		"1.20": {"1.20", "1.21", "1.22", "1.23", "1.24", "1.25"},
		"1.21": {"1.21", "1.22", "1.23", "1.24", "1.25"},
		"1.22": {"1.22", "1.23", "1.24", "1.25"},
		"1.23": {"1.23", "1.24", "1.25", "1.26"},
		"1.24": {"1.24", "1.25", "1.26", "1.27"},
		"1.25": {"1.25", "1.26", "1.27", "1.28"},
		"1.26": {"1.26", "1.27", "1.28", "1.29"},
		"1.27": {"1.27", "1.28", "1.29", "1.30"},
		"1.28": {"1.28", "1.29", "1.30"},
		"1.29": {"1.29", "1.30"},
		"1.30": {"1.30"},
	}

	if minVersion != "" {
		if versionList, ok := supportedVersions[minVersion]; ok {
			versions = versionList
		}
	}

	// If we couldn't determine, use current supported versions
	if len(versions) == 0 {
		versions = []string{"1.27", "1.28", "1.29", "1.30"}
	}

	return versions
}

// quoteStrings adds quotes around each string
func quoteStrings(strs []string) []string {
	quoted := make([]string, len(strs))
	for i, s := range strs {
		quoted[i] = fmt.Sprintf(`"%s"`, s)
	}
	return quoted
}

// init registers the Helm extractor
func init() {
	extractor.RegisterExtractor(NewExtractor())
}
