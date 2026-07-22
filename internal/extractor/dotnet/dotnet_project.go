// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package dotnet

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/lfreleng-actions/build-metadata-action/internal/extractor"
)

// extractProjectProperties extracts properties from PropertyGroup elements
func (e *Extractor) extractProjectProperties(project *Project, metadata *extractor.ProjectMetadata) {
	for _, pg := range project.PropertyGroups {
		e.applyFrameworkProperties(pg, metadata)
		e.applyAssemblyAndPackageInfo(pg, metadata)
		e.applyAuthorAndDescription(pg, metadata)
		e.applyURLsAndTags(pg, metadata)
		e.applyBuildOptions(pg, metadata)
	}
}

// applyFrameworkProperties records single- and multi-target framework info.
func (e *Extractor) applyFrameworkProperties(pg PropertyGroup, metadata *extractor.ProjectMetadata) {
	if pg.TargetFramework != "" {
		metadata.LanguageSpecific["dotnet_target_framework"] = pg.TargetFramework
		// Record the framework's numeric version for reference, but do not
		// use it as the project version: a runtime target such as net8.0 is
		// not a release version, and letting it stand would mask the real
		// fallback sources (git tags, version.properties, etc.). This mirrors
		// the Go and Rust extractors, which leave Version empty for fallback.
		if frameworkVersion := e.extractVersionFromFramework(pg.TargetFramework); frameworkVersion != "" {
			metadata.LanguageSpecific["dotnet_framework_version"] = frameworkVersion
		}
	}
	if pg.TargetFrameworks != "" {
		frameworks := strings.Split(pg.TargetFrameworks, ";")
		metadata.LanguageSpecific["dotnet_target_frameworks"] = frameworks
		metadata.LanguageSpecific["dotnet_multi_target"] = true
	}
}

// applyAssemblyAndPackageInfo records assembly and NuGet package identity.
func (e *Extractor) applyAssemblyAndPackageInfo(pg PropertyGroup, metadata *extractor.ProjectMetadata) {
	if pg.AssemblyName != "" {
		metadata.LanguageSpecific["dotnet_assembly_name"] = pg.AssemblyName
		if metadata.Name == "" {
			metadata.Name = pg.AssemblyName
		}
	}
	if pg.Version != "" {
		metadata.Version = pg.Version
	}
	if pg.AssemblyVersion != "" {
		metadata.LanguageSpecific["dotnet_assembly_version"] = pg.AssemblyVersion
	}
	if pg.FileVersion != "" {
		metadata.LanguageSpecific["dotnet_file_version"] = pg.FileVersion
	}
	if pg.PackageId != "" {
		metadata.LanguageSpecific["dotnet_package_id"] = pg.PackageId
	}
	if pg.PackageVersion != "" {
		metadata.LanguageSpecific["dotnet_package_version"] = pg.PackageVersion
	}
}

// applyAuthorAndDescription records authorship, description, and licensing.
func (e *Extractor) applyAuthorAndDescription(pg PropertyGroup, metadata *extractor.ProjectMetadata) {
	if pg.Authors != "" {
		metadata.LanguageSpecific["dotnet_authors"] = pg.Authors
		authors := strings.Split(pg.Authors, ";")
		if len(authors) > 0 {
			metadata.Authors = authors
		}
	}
	if pg.Company != "" {
		metadata.LanguageSpecific["dotnet_company"] = pg.Company
	}
	if pg.Product != "" {
		metadata.LanguageSpecific["dotnet_product"] = pg.Product
	}
	if pg.Description != "" {
		metadata.Description = pg.Description
		metadata.LanguageSpecific["dotnet_description"] = pg.Description
	}
	if pg.Copyright != "" {
		metadata.LanguageSpecific["dotnet_copyright"] = pg.Copyright
	}
	if pg.PackageLicenseExpression != "" {
		metadata.License = pg.PackageLicenseExpression
		metadata.LanguageSpecific["dotnet_license"] = pg.PackageLicenseExpression
	}
}

// applyURLsAndTags records project URLs, repository info, and package tags.
func (e *Extractor) applyURLsAndTags(pg PropertyGroup, metadata *extractor.ProjectMetadata) {
	if pg.PackageProjectUrl != "" {
		metadata.Homepage = pg.PackageProjectUrl
		metadata.LanguageSpecific["dotnet_project_url"] = pg.PackageProjectUrl
	}
	if pg.RepositoryUrl != "" {
		metadata.Repository = pg.RepositoryUrl
		metadata.LanguageSpecific["dotnet_repository_url"] = pg.RepositoryUrl
	}
	if pg.RepositoryType != "" {
		metadata.LanguageSpecific["dotnet_repository_type"] = pg.RepositoryType
	}
	if pg.PackageTags != "" {
		tags := strings.Split(pg.PackageTags, ";")
		metadata.LanguageSpecific["dotnet_tags"] = tags
	}
}

// applyBuildOptions records compilation, runtime, and publishing settings.
func (e *Extractor) applyBuildOptions(pg PropertyGroup, metadata *extractor.ProjectMetadata) {
	if pg.OutputType != "" {
		metadata.LanguageSpecific["dotnet_output_type"] = pg.OutputType
	}
	if pg.LangVersion != "" {
		metadata.LanguageSpecific["dotnet_lang_version"] = pg.LangVersion
	}
	if pg.Nullable != "" {
		metadata.LanguageSpecific["dotnet_nullable"] = pg.Nullable
	}
	if pg.ImplicitUsings != "" {
		metadata.LanguageSpecific["dotnet_implicit_usings"] = pg.ImplicitUsings
	}
	if pg.RuntimeIdentifier != "" {
		metadata.LanguageSpecific["dotnet_runtime_identifier"] = pg.RuntimeIdentifier
	}
	if pg.RuntimeIdentifiers != "" {
		rids := strings.Split(pg.RuntimeIdentifiers, ";")
		metadata.LanguageSpecific["dotnet_runtime_identifiers"] = rids
	}
	if pg.SelfContained != "" {
		metadata.LanguageSpecific["dotnet_self_contained"] = pg.SelfContained
	}
	if pg.PublishSingleFile != "" {
		metadata.LanguageSpecific["dotnet_publish_single_file"] = pg.PublishSingleFile
	}
	if pg.PublishTrimmed != "" {
		metadata.LanguageSpecific["dotnet_publish_trimmed"] = pg.PublishTrimmed
	}
}

// extractPackageReferences extracts NuGet package references
func (e *Extractor) extractPackageReferences(project *Project, metadata *extractor.ProjectMetadata) {
	packages := make([]map[string]string, 0)
	packageMap := make(map[string]string) // For deduplication

	for _, ig := range project.ItemGroups {
		for _, pkg := range ig.PackageReferences {
			if pkg.Include != "" {
				packageMap[pkg.Include] = pkg.Version
			}
		}
	}

	for name, version := range packageMap {
		packages = append(packages, map[string]string{
			"name":    name,
			"version": version,
		})
	}

	if len(packages) > 0 {
		metadata.LanguageSpecific["dotnet_package_references"] = packages
		metadata.LanguageSpecific["dotnet_package_count"] = len(packages)
	}
}

// extractProjectReferences extracts project-to-project references
func (e *Extractor) extractProjectReferences(project *Project, metadata *extractor.ProjectMetadata) {
	projects := make([]string, 0)
	projectMap := make(map[string]bool) // For deduplication

	for _, ig := range project.ItemGroups {
		for _, proj := range ig.ProjectReferences {
			if proj.Include != "" {
				projectMap[proj.Include] = true
			}
		}
	}

	for path := range projectMap {
		projects = append(projects, path)
	}

	if len(projects) > 0 {
		metadata.LanguageSpecific["dotnet_project_references"] = projects
		metadata.LanguageSpecific["dotnet_project_reference_count"] = len(projects)
	}
}

// extractVersionFromFramework extracts version number from target framework
func (e *Extractor) extractVersionFromFramework(framework string) string {
	// Examples: net8.0, net6.0, net5.0, netcoreapp3.1, netstandard2.1
	versionPattern := regexp.MustCompile(`(\d+)\.(\d+)`)
	matches := versionPattern.FindStringSubmatch(framework)
	if len(matches) >= 3 {
		return fmt.Sprintf("%s.%s.0", matches[1], matches[2])
	}
	return ""
}
