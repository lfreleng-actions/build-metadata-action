// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package rust

import (
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/lfreleng-actions/build-metadata-action/internal/extractor"
)

// extractFromCargoToml extracts metadata from Cargo.toml file
func (e *Extractor) extractFromCargoToml(path string, metadata *extractor.ProjectMetadata) error {
	var cargo CargoToml

	if _, err := toml.DecodeFile(path, &cargo); err != nil {
		return fmt.Errorf("failed to parse Cargo.toml: %w", err)
	}

	applyCoreMetadata(&cargo, metadata)
	edition, rustVersion := applyPackageDetails(&cargo, metadata)
	applyDependencyMetadata(&cargo, metadata)
	applyProjectStructure(&cargo, metadata)
	applyFrameworksAndMatrix(&cargo, metadata, edition, rustVersion)

	return nil
}

// applyCoreMetadata maps the top-level package fields, resolving workspace
// inheritance and rejecting non-semantic version markers so that a git-tag
// fallback can be applied later.
func applyCoreMetadata(cargo *CargoToml, metadata *extractor.ProjectMetadata) {
	metadata.Name = cargo.Package.Name

	if os.Getenv("INPUT_VERBOSE") == "true" {
		log.Printf("[DEBUG] Rust: Package.Version type=%T, value=%#v", cargo.Package.Version, cargo.Package.Version)
		log.Printf("[DEBUG] Rust: Workspace.Package.Version=%s", cargo.Workspace.Package.Version)
	}

	version := getStringValue(cargo.Package.Version, cargo.Workspace.Package.Version)

	if os.Getenv("INPUT_VERBOSE") == "true" {
		log.Printf("[DEBUG] Rust: getStringValue returned: '%s'", version)
	}

	if version != "" && version != "true" && version != "false" {
		metadata.Version = version
		if os.Getenv("INPUT_VERBOSE") == "true" {
			log.Printf("[DEBUG] Rust: Setting metadata.Version to: '%s'", version)
		}
	} else {
		metadata.Version = ""
		if os.Getenv("INPUT_VERBOSE") == "true" {
			log.Printf("[DEBUG] Rust: Version invalid ('%s'), clearing for git tag fallback", version)
		}
	}
	metadata.Description = getStringValue(cargo.Package.Description, cargo.Workspace.Package.Description)
	metadata.License = getStringValue(cargo.Package.License, cargo.Workspace.Package.License)
	metadata.Homepage = getStringValue(cargo.Package.Homepage, cargo.Workspace.Package.Homepage)
	metadata.Repository = getStringValue(cargo.Package.Repository, cargo.Workspace.Package.Repository)
	metadata.Authors = getStringSliceValue(cargo.Package.Authors, cargo.Workspace.Package.Authors)
	metadata.VersionSource = "Cargo.toml"

	metadata.LanguageSpecific["package_name"] = cargo.Package.Name
	metadata.LanguageSpecific["metadata_source"] = "Cargo.toml"
}

// applyPackageDetails records optional package attributes and returns the
// edition and rust-version, which drive the version matrix downstream.
func applyPackageDetails(cargo *CargoToml, metadata *extractor.ProjectMetadata) (edition, rustVersion string) {
	edition = getStringValue(cargo.Package.Edition, cargo.Workspace.Package.Edition)
	if edition != "" {
		metadata.LanguageSpecific["edition"] = edition
	}

	rustVersion = getStringValue(cargo.Package.RustVersion, cargo.Workspace.Package.RustVersion)
	if rustVersion != "" {
		metadata.LanguageSpecific["rust_version"] = rustVersion
		metadata.LanguageSpecific["msrv"] = rustVersion
	}

	if cargo.Package.Documentation != "" {
		metadata.LanguageSpecific["documentation"] = cargo.Package.Documentation
	}

	keywords := getStringSliceValue(cargo.Package.Keywords, cargo.Workspace.Package.Keywords)
	if len(keywords) > 0 {
		metadata.LanguageSpecific["keywords"] = keywords
	}

	categories := getStringSliceValue(cargo.Package.Categories, cargo.Workspace.Package.Categories)
	if len(categories) > 0 {
		metadata.LanguageSpecific["categories"] = categories
	}

	if cargo.Package.Publish != nil {
		metadata.LanguageSpecific["publish"] = cargo.Package.Publish
	}

	if cargo.Package.LicenseFile != "" {
		metadata.LanguageSpecific["license_file"] = cargo.Package.LicenseFile
	}

	readme := getStringValue(cargo.Package.Readme, "")
	if readme != "" {
		metadata.LanguageSpecific["readme"] = readme
	}

	return edition, rustVersion
}

// applyDependencyMetadata records normal, dev and build dependencies together
// with optional-dependency names and the aggregate count.
func applyDependencyMetadata(cargo *CargoToml, metadata *extractor.ProjectMetadata) {
	if len(cargo.Dependencies) > 0 {
		deps := parseDependencies(cargo.Dependencies)
		metadata.LanguageSpecific["dependencies"] = formatDependencies(deps)
		metadata.LanguageSpecific["dependency_count"] = len(deps)

		optionalDeps := []string{}
		for _, dep := range deps {
			if dep.Optional {
				optionalDeps = append(optionalDeps, dep.Name)
			}
		}
		if len(optionalDeps) > 0 {
			metadata.LanguageSpecific["optional_dependencies"] = optionalDeps
		}
	}

	if len(cargo.DevDependencies) > 0 {
		devDeps := parseDependencies(cargo.DevDependencies)
		metadata.LanguageSpecific["dev_dependencies"] = formatDependencies(devDeps)
		metadata.LanguageSpecific["dev_dependency_count"] = len(devDeps)
	}

	if len(cargo.BuildDependencies) > 0 {
		buildDeps := parseDependencies(cargo.BuildDependencies)
		metadata.LanguageSpecific["build_dependencies"] = formatDependencies(buildDeps)
		metadata.LanguageSpecific["build_dependency_count"] = len(buildDeps)
	}

	totalDeps := len(cargo.Dependencies) + len(cargo.DevDependencies) + len(cargo.BuildDependencies)
	if totalDeps > 0 {
		metadata.LanguageSpecific["total_dependency_count"] = totalDeps
	}
}

// applyProjectStructure records features, workspace layout, binary/library
// targets and the presence of a build script.
func applyProjectStructure(cargo *CargoToml, metadata *extractor.ProjectMetadata) {
	if len(cargo.Features) > 0 {
		metadata.LanguageSpecific["features"] = cargo.Features
		metadata.LanguageSpecific["feature_count"] = len(cargo.Features)

		featureNames := make([]string, 0, len(cargo.Features))
		for name := range cargo.Features {
			featureNames = append(featureNames, name)
		}
		sort.Strings(featureNames)
		metadata.LanguageSpecific["feature_names"] = featureNames
	}

	if len(cargo.Workspace.Members) > 0 {
		metadata.LanguageSpecific["is_workspace"] = true
		metadata.LanguageSpecific["workspace_members"] = cargo.Workspace.Members
		metadata.LanguageSpecific["workspace_member_count"] = len(cargo.Workspace.Members)

		if cargo.Workspace.Resolver != "" {
			metadata.LanguageSpecific["workspace_resolver"] = cargo.Workspace.Resolver
		}
	}

	if len(cargo.Bin) > 0 {
		binNames := make([]string, 0, len(cargo.Bin))
		for _, bin := range cargo.Bin {
			binNames = append(binNames, bin.Name)
		}
		metadata.LanguageSpecific["binary_targets"] = binNames
		metadata.LanguageSpecific["binary_count"] = len(cargo.Bin)
	}

	if cargo.Lib.Name != "" {
		metadata.LanguageSpecific["lib_name"] = cargo.Lib.Name
		if len(cargo.Lib.CrateType) > 0 {
			metadata.LanguageSpecific["crate_types"] = cargo.Lib.CrateType
		}
	}

	if cargo.Package.Build != "" {
		metadata.LanguageSpecific["has_build_script"] = true
		metadata.LanguageSpecific["build_script"] = cargo.Package.Build
	}
}

// applyFrameworksAndMatrix records detected frameworks and derives the Rust
// version matrix from the MSRV, falling back to the edition when unset.
func applyFrameworksAndMatrix(cargo *CargoToml, metadata *extractor.ProjectMetadata, edition, rustVersion string) {
	frameworks := detectRustFrameworks(cargo.Dependencies)
	if len(frameworks) > 0 {
		metadata.LanguageSpecific["frameworks"] = frameworks
	}

	if rustVersion != "" {
		matrix := generateRustVersionMatrix(rustVersion)
		if len(matrix) > 0 {
			metadata.LanguageSpecific["rust_version_matrix"] = matrix
			matrixJSON := fmt.Sprintf(`{"rust-version": [%s]}`,
				strings.Join(quoteStrings(matrix), ", "))
			metadata.LanguageSpecific["matrix_json"] = matrixJSON
		}
	} else if edition != "" {
		matrix := generateRustVersionMatrixFromEdition(edition)
		if len(matrix) > 0 {
			metadata.LanguageSpecific["rust_version_matrix"] = matrix
			matrixJSON := fmt.Sprintf(`{"rust-version": [%s]}`,
				strings.Join(quoteStrings(matrix), ", "))
			metadata.LanguageSpecific["matrix_json"] = matrixJSON
		}
	}
}

// getStringValue extracts a string from an interface{} that could be a string or workspace reference
func getStringValue(value interface{}, workspaceDefault string) string {
	if value == nil {
		return workspaceDefault
	}

	switch v := value.(type) {
	case string:
		return v
	case map[string]interface{}:
		// Check if it's a workspace reference
		if workspace, ok := v["workspace"].(bool); ok && workspace {
			return workspaceDefault
		}
	case bool:
		// A bare boolean indicates malformed TOML; treat as absent.
		return ""
	}

	return ""
}

// getStringSliceValue extracts a []string from an interface{} that could be []string or workspace reference
func getStringSliceValue(value interface{}, workspaceDefault []string) []string {
	if value == nil {
		return workspaceDefault
	}

	switch v := value.(type) {
	case []interface{}:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if str, ok := item.(string); ok {
				result = append(result, str)
			}
		}
		return result
	case []string:
		return v
	case map[string]interface{}:
		// Check if it's a workspace reference
		if workspace, ok := v["workspace"].(bool); ok && workspace {
			return workspaceDefault
		}
	}

	return nil
}
