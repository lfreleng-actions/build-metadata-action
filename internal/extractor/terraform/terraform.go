// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package terraform

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/lfreleng-actions/build-metadata-action/internal/extractor"
)

// Extractor extracts metadata from Terraform projects
type Extractor struct {
	extractor.BaseExtractor
}

// NewExtractor creates a new Terraform extractor
func NewExtractor() *Extractor {
	return &Extractor{
		BaseExtractor: extractor.NewBaseExtractor("terraform", 1),
	}
}

// TerraformConfig represents parsed Terraform/OpenTofu configuration
type TerraformConfig struct {
	RequiredVersion   string
	RequiredProviders map[string]ProviderRequirement
	Backend           string
	CloudOrganization string
	Modules           []ModuleCall
	Resources         []Resource
	IsOpenTofu        bool // Detected if using OpenTofu
}

// ProviderRequirement represents a required provider
type ProviderRequirement struct {
	Source  string
	Version string
}

// ModuleCall represents a module being called
type ModuleCall struct {
	Name    string
	Source  string
	Version string
}

// Resource represents a Terraform resource
type Resource struct {
	Type string
	Name string
}

// Extract retrieves metadata from a Terraform project
func (e *Extractor) Extract(projectPath string) (*extractor.ProjectMetadata, error) {
	metadata := &extractor.ProjectMetadata{
		LanguageSpecific: make(map[string]interface{}),
	}

	config := &TerraformConfig{
		RequiredProviders: make(map[string]ProviderRequirement),
		Modules:           make([]ModuleCall, 0),
		Resources:         make([]Resource, 0),
	}

	// Parse all .tf files
	files, err := filepath.Glob(filepath.Join(projectPath, "*.tf"))
	if err != nil || len(files) == 0 {
		return nil, fmt.Errorf("no Terraform files found in %s", projectPath)
	}

	for _, file := range files {
		if err := e.parseFile(file, config); err != nil {
			// Continue on error, we'll gather what we can
			continue
		}
	}

	// Check for OpenTofu-specific files
	openTofuFiles := []string{
		filepath.Join(projectPath, ".opentofu"),
		filepath.Join(projectPath, ".terraform-version"),
	}
	for _, ofFile := range openTofuFiles {
		if content, err := os.ReadFile(ofFile); err == nil {
			contentStr := string(content)
			if strings.Contains(contentStr, "opentofu") || strings.Contains(contentStr, "tofu") {
				config.IsOpenTofu = true
				break
			}
		}
	}

	// Extract metadata
	e.populateMetadata(config, metadata, projectPath)

	return metadata, nil
}

// parseFile parses a single Terraform file
func (e *Extractor) parseFile(path string, config *TerraformConfig) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", path, err)
	}

	// Try HCL parsing first
	parser := hclparse.NewParser()
	file, diags := parser.ParseHCL(content, filepath.Base(path))
	if diags.HasErrors() {
		// Fall back to regex-based parsing
		return e.parseWithRegex(string(content), config)
	}

	// Extract terraform block
	if file != nil && file.Body != nil {
		schema := &hcl.BodySchema{
			Blocks: []hcl.BlockHeaderSchema{
				{Type: "terraform"},
				{Type: "provider", LabelNames: []string{"name"}},
				{Type: "module", LabelNames: []string{"name"}},
				{Type: "resource", LabelNames: []string{"type", "name"}},
			},
		}

		bodyContent, _, _ := file.Body.PartialContent(schema)
		if bodyContent != nil {
			for _, block := range bodyContent.Blocks {
				switch block.Type {
				case "terraform":
					e.parseTerraformBlock(block, config)
				case "module":
					e.parseModuleBlock(block, config)
				case "resource":
					e.parseResourceBlock(block, config)
				}
			}
		}
	}

	return nil
}

// parseTerraformBlock extracts data from the terraform {} block
func (e *Extractor) parseTerraformBlock(block *hcl.Block, config *TerraformConfig) {
	schema := &hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "required_version"},
		},
		Blocks: []hcl.BlockHeaderSchema{
			{Type: "required_providers"},
			{Type: "backend", LabelNames: []string{"type"}},
			{Type: "cloud"},
		},
	}

	content, _, _ := block.Body.PartialContent(schema)
	if content != nil {
		// Extract required_version
		if attr, exists := content.Attributes["required_version"]; exists {
			val, _ := attr.Expr.Value(nil)
			config.RequiredVersion = strings.Trim(val.AsString(), `"`)
		}

		// Extract required_providers
		for _, innerBlock := range content.Blocks {
			if innerBlock.Type == "required_providers" {
				attrs, _ := innerBlock.Body.JustAttributes()
				for name, attr := range attrs {
					val, _ := attr.Expr.Value(nil)
					// Handle both string and object syntax
					if val.Type().IsObjectType() {
						// Parse object attributes
						config.RequiredProviders[name] = ProviderRequirement{}
					} else {
						config.RequiredProviders[name] = ProviderRequirement{
							Version: strings.Trim(val.AsString(), `"`),
						}
					}
				}
			} else if innerBlock.Type == "backend" {
				if len(innerBlock.Labels) > 0 {
					config.Backend = innerBlock.Labels[0]
				}
			}
		}
	}
}

// parseModuleBlock extracts module call information
func (e *Extractor) parseModuleBlock(block *hcl.Block, config *TerraformConfig) {
	if len(block.Labels) == 0 {
		return
	}

	module := ModuleCall{
		Name: block.Labels[0],
	}

	attrs, _ := block.Body.JustAttributes()
	if sourceAttr, exists := attrs["source"]; exists {
		val, _ := sourceAttr.Expr.Value(nil)
		module.Source = strings.Trim(val.AsString(), `"`)
	}
	if versionAttr, exists := attrs["version"]; exists {
		val, _ := versionAttr.Expr.Value(nil)
		module.Version = strings.Trim(val.AsString(), `"`)
	}

	config.Modules = append(config.Modules, module)
}

// parseResourceBlock extracts resource information
func (e *Extractor) parseResourceBlock(block *hcl.Block, config *TerraformConfig) {
	if len(block.Labels) < 2 {
		return
	}

	resource := Resource{
		Type: block.Labels[0],
		Name: block.Labels[1],
	}

	config.Resources = append(config.Resources, resource)
}

// parseWithRegex uses regex patterns as a fallback parser
func (e *Extractor) parseWithRegex(content string, config *TerraformConfig) error {
	// Check for OpenTofu in comments
	if strings.Contains(content, "OpenTofu") || strings.Contains(content, "opentofu") {
		config.IsOpenTofu = true
	}

	// Extract required_version
	versionRe := regexp.MustCompile(`required_version\s*=\s*"([^"]+)"`)
	if matches := versionRe.FindStringSubmatch(content); len(matches) > 1 {
		config.RequiredVersion = matches[1]
	}

	// Extract required_providers
	providerBlockRe := regexp.MustCompile(`required_providers\s*{([^}]+)}`)
	if matches := providerBlockRe.FindStringSubmatch(content); len(matches) > 1 {
		providerContent := matches[1]
		providerRe := regexp.MustCompile(`(\w+)\s*=\s*{[^}]*version\s*=\s*"([^"]+)"`)
		for _, match := range providerRe.FindAllStringSubmatch(providerContent, -1) {
			if len(match) > 2 {
				config.RequiredProviders[match[1]] = ProviderRequirement{
					Version: match[2],
				}
			}
		}
		// Also handle simple string syntax
		simpleProviderRe := regexp.MustCompile(`(\w+)\s*=\s*"([^"]+)"`)
		for _, match := range simpleProviderRe.FindAllStringSubmatch(providerContent, -1) {
			if len(match) > 2 {
				if _, exists := config.RequiredProviders[match[1]]; !exists {
					config.RequiredProviders[match[1]] = ProviderRequirement{
						Version: match[2],
					}
				}
			}
		}
	}

	// Extract backend
	backendRe := regexp.MustCompile(`backend\s+"(\w+)"\s*{`)
	if matches := backendRe.FindStringSubmatch(content); len(matches) > 1 {
		config.Backend = matches[1]
	}

	// Extract modules
	moduleRe := regexp.MustCompile(`module\s+"([^"]+)"\s*{([^}]+)}`)
	for _, match := range moduleRe.FindAllStringSubmatch(content, -1) {
		if len(match) > 2 {
			module := ModuleCall{Name: match[1]}
			moduleContent := match[2]

			sourceRe := regexp.MustCompile(`source\s*=\s*"([^"]+)"`)
			if sourceMatch := sourceRe.FindStringSubmatch(moduleContent); len(sourceMatch) > 1 {
				module.Source = sourceMatch[1]
			}

			versionRe := regexp.MustCompile(`version\s*=\s*"([^"]+)"`)
			if versionMatch := versionRe.FindStringSubmatch(moduleContent); len(versionMatch) > 1 {
				module.Version = versionMatch[1]
			}

			config.Modules = append(config.Modules, module)
		}
	}

	return nil
}

// populateMetadata converts TerraformConfig to ProjectMetadata
func (e *Extractor) populateMetadata(config *TerraformConfig, metadata *extractor.ProjectMetadata, projectPath string) {
	// Try to extract project name from directory
	metadata.Name = filepath.Base(projectPath)
	metadata.Version = config.RequiredVersion
	metadata.VersionSource = "terraform.required_version"

	// Terraform/OpenTofu-specific metadata
	metadata.LanguageSpecific["terraform_version"] = config.RequiredVersion
	metadata.LanguageSpecific["metadata_source"] = "versions.tf"
	metadata.LanguageSpecific["is_opentofu"] = config.IsOpenTofu

	if config.IsOpenTofu {
		metadata.LanguageSpecific["engine"] = "opentofu"
	} else {
		metadata.LanguageSpecific["engine"] = "terraform"
	}

	if config.Backend != "" {
		metadata.LanguageSpecific["backend"] = config.Backend
	}

	// Providers
	if len(config.RequiredProviders) > 0 {
		providers := make([]map[string]string, 0, len(config.RequiredProviders))
		for name, req := range config.RequiredProviders {
			provider := map[string]string{
				"name": name,
			}
			if req.Version != "" {
				provider["version"] = req.Version
			}
			if req.Source != "" {
				provider["source"] = req.Source
			}
			providers = append(providers, provider)
		}
		metadata.LanguageSpecific["providers"] = providers
		metadata.LanguageSpecific["provider_count"] = len(providers)
	}

	// Modules
	if len(config.Modules) > 0 {
		modules := make([]map[string]string, 0, len(config.Modules))
		for _, mod := range config.Modules {
			module := map[string]string{
				"name":   mod.Name,
				"source": mod.Source,
			}
			if mod.Version != "" {
				module["version"] = mod.Version
			}
			modules = append(modules, module)
		}
		metadata.LanguageSpecific["modules"] = modules
		metadata.LanguageSpecific["module_count"] = len(modules)
	}

	// Resources
	if len(config.Resources) > 0 {
		resourceTypes := make(map[string]int)
		for _, res := range config.Resources {
			resourceTypes[res.Type]++
		}
		metadata.LanguageSpecific["resource_types"] = resourceTypes
		metadata.LanguageSpecific["resource_count"] = len(config.Resources)
	}

	// Generate Terraform/OpenTofu version matrix
	if config.RequiredVersion != "" {
		matrix := generateTerraformVersionMatrix(config.RequiredVersion)
		if len(matrix) > 0 {
			metadata.LanguageSpecific["terraform_version_matrix"] = matrix

			// Generate matrix for both Terraform and OpenTofu if applicable
			engine := "terraform"
			if config.IsOpenTofu {
				engine = "opentofu"
			}
			matrixJSON := fmt.Sprintf(`{"%s-version": [%s]}`,
				engine, strings.Join(quoteStrings(matrix), ", "))
			metadata.LanguageSpecific["matrix_json"] = matrixJSON
		}
	}
}

// Detect checks if this extractor can handle the project
func (e *Extractor) Detect(projectPath string) bool {
	// Check for any .tf files
	files, err := filepath.Glob(filepath.Join(projectPath, "*.tf"))
	return err == nil && len(files) > 0
}

// Helper functions

// generateTerraformVersionMatrix generates a list of Terraform/OpenTofu versions from a constraint
func generateTerraformVersionMatrix(requiredVersion string) []string {
	versions := []string{}

	// Parse common version constraints
	minVersion := ""
	if strings.Contains(requiredVersion, ">=") {
		re := regexp.MustCompile(`>=\s*(\d+\.\d+)`)
		if matches := re.FindStringSubmatch(requiredVersion); len(matches) > 1 {
			minVersion = matches[1]
		}
	} else if strings.HasPrefix(requiredVersion, "~>") {
		// Pessimistic constraint
		version := strings.TrimPrefix(requiredVersion, "~>")
		version = strings.TrimSpace(version)
		parts := strings.Split(version, ".")
		if len(parts) >= 2 {
			minVersion = parts[0] + "." + parts[1]
		}
	}

	// Map minimum version to supported versions
	// Includes latest Terraform and OpenTofu versions
	supportedVersions := map[string][]string{
		"1.5":  {"1.5", "1.6", "1.7", "1.8", "1.9", "1.10"},
		"1.6":  {"1.6", "1.7", "1.8", "1.9", "1.10"},
		"1.7":  {"1.7", "1.8", "1.9", "1.10"},
		"1.8":  {"1.8", "1.9", "1.10"},
		"1.9":  {"1.9", "1.10"},
		"1.10": {"1.10"},
	}

	if minVersion != "" {
		if versionList, ok := supportedVersions[minVersion]; ok {
			versions = versionList
		} else {
			// Map legacy versions to supported versions
			if minVersion < "1.5" {
				versions = []string{"1.5", "1.6", "1.7", "1.8", "1.9", "1.10"}
			} else {
				versions = []string{"1.8", "1.9", "1.10"}
			}
		}
	}

	// Default to current stable versions
	if len(versions) == 0 {
		versions = []string{"1.8", "1.9", "1.10"}
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

// init registers the Terraform extractor
func init() {
	extractor.RegisterExtractor(NewExtractor())
}
