// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package dotnet

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/lfreleng-actions/build-metadata-action/internal/extractor"
)

// Extractor extracts metadata from .NET projects
type Extractor struct {
	extractor.BaseExtractor
}

// NewExtractor creates a new .NET extractor
func NewExtractor() *Extractor {
	return &Extractor{
		BaseExtractor: extractor.NewBaseExtractor("dotnet", 5),
	}
}

// Project represents a .csproj or similar MSBuild project file
type Project struct {
	XMLName        xml.Name        `xml:"Project"`
	Sdk            string          `xml:"Sdk,attr"`
	PropertyGroups []PropertyGroup `xml:"PropertyGroup"`
	ItemGroups     []ItemGroup     `xml:"ItemGroup"`
}

// PropertyGroup contains MSBuild properties
type PropertyGroup struct {
	Condition                string `xml:"Condition,attr"`
	TargetFramework          string `xml:"TargetFramework"`
	TargetFrameworks         string `xml:"TargetFrameworks"`
	AssemblyName             string `xml:"AssemblyName"`
	RootNamespace            string `xml:"RootNamespace"`
	Version                  string `xml:"Version"`
	AssemblyVersion          string `xml:"AssemblyVersion"`
	FileVersion              string `xml:"FileVersion"`
	PackageId                string `xml:"PackageId"`
	PackageVersion           string `xml:"PackageVersion"`
	Authors                  string `xml:"Authors"`
	Company                  string `xml:"Company"`
	Product                  string `xml:"Product"`
	Description              string `xml:"Description"`
	Copyright                string `xml:"Copyright"`
	PackageLicenseExpression string `xml:"PackageLicenseExpression"`
	PackageProjectUrl        string `xml:"PackageProjectUrl"`
	RepositoryUrl            string `xml:"RepositoryUrl"`
	RepositoryType           string `xml:"RepositoryType"`
	PackageTags              string `xml:"PackageTags"`
	OutputType               string `xml:"OutputType"`
	LangVersion              string `xml:"LangVersion"`
	Nullable                 string `xml:"Nullable"`
	ImplicitUsings           string `xml:"ImplicitUsings"`
	RuntimeIdentifier        string `xml:"RuntimeIdentifier"`
	RuntimeIdentifiers       string `xml:"RuntimeIdentifiers"`
	SelfContained            string `xml:"SelfContained"`
	PublishSingleFile        string `xml:"PublishSingleFile"`
	PublishTrimmed           string `xml:"PublishTrimmed"`
}

// ItemGroup contains MSBuild items (references, packages, etc.)
type ItemGroup struct {
	Condition         string             `xml:"Condition,attr"`
	PackageReferences []PackageReference `xml:"PackageReference"`
	ProjectReferences []ProjectReference `xml:"ProjectReference"`
	References        []Reference        `xml:"Reference"`
}

// PackageReference represents a NuGet package reference
type PackageReference struct {
	Include string `xml:"Include,attr"`
	Version string `xml:"Version,attr"`
}

// ProjectReference represents a project-to-project reference
type ProjectReference struct {
	Include string `xml:"Include,attr"`
}

// Reference represents a framework/assembly reference
type Reference struct {
	Include string `xml:"Include,attr"`
}

// Solution represents a .sln solution file
type Solution struct {
	Projects []SolutionProject
	Version  string
}

// SolutionProject represents a project in a solution
type SolutionProject struct {
	Name string
	Path string
	GUID string
	Type string
}

// init registers the .NET extractor
func init() {
	extractor.RegisterExtractor(NewExtractor())
}

// Detect checks if this extractor can handle the project
func (e *Extractor) Detect(projectPath string) bool {
	// Check for .csproj files
	csprojMatches, _ := filepath.Glob(filepath.Join(projectPath, "*.csproj"))
	if len(csprojMatches) > 0 {
		return true
	}

	// Check for .sln files
	slnMatches, _ := filepath.Glob(filepath.Join(projectPath, "*.sln"))
	if len(slnMatches) > 0 {
		return true
	}

	// Check for .props files
	propsMatches, _ := filepath.Glob(filepath.Join(projectPath, "*.props"))
	if len(propsMatches) > 0 {
		return true
	}

	return false
}

// Extract extracts metadata from a .NET project
func (e *Extractor) Extract(projectPath string) (*extractor.ProjectMetadata, error) {
	metadata := &extractor.ProjectMetadata{
		LanguageSpecific: make(map[string]interface{}),
	}

	// Try to find and parse a .csproj file first
	csprojPath, err := e.findProjectFile(projectPath, "*.csproj")
	if err == nil {
		if err := e.extractFromProjectFile(csprojPath, metadata); err != nil {
			return nil, err
		}
	} else {
		// Try .sln file
		slnPath, err := e.findProjectFile(projectPath, "*.sln")
		if err == nil {
			if err := e.extractFromSolution(projectPath, slnPath, metadata); err != nil {
				return nil, err
			}
		} else {
			// Try .props file
			propsPath, err := e.findProjectFile(projectPath, "*.props")
			if err == nil {
				if err := e.extractFromPropsFile(propsPath, metadata); err != nil {
					return nil, err
				}
			} else {
				return nil, fmt.Errorf("no .NET project files found")
			}
		}
	}

	// Detect frameworks and tools
	e.detectFrameworks(metadata)
	e.generateVersionMatrix(metadata)

	return metadata, nil
}

// extractFromProjectFile extracts metadata from a .csproj file
func (e *Extractor) extractFromProjectFile(csprojPath string, metadata *extractor.ProjectMetadata) error {
	// Parse the project file
	project, err := e.parseProjectFile(csprojPath)
	if err != nil {
		return fmt.Errorf("failed to parse project file: %w", err)
	}

	// Extract metadata from property groups
	e.extractProjectProperties(project, metadata)

	// Extract package references
	e.extractPackageReferences(project, metadata)

	// Extract project references
	e.extractProjectReferences(project, metadata)

	// Store the project file path
	metadata.LanguageSpecific["dotnet_project_file"] = filepath.Base(csprojPath)

	// Detect if it's SDK-style project
	if project.Sdk != "" {
		metadata.LanguageSpecific["dotnet_sdk_style"] = true
		metadata.LanguageSpecific["dotnet_sdk"] = project.Sdk
	} else {
		metadata.LanguageSpecific["dotnet_sdk_style"] = false
	}

	return nil
}

// extractFromSolution extracts metadata from a .sln file
func (e *Extractor) extractFromSolution(projectPath string, slnPath string, metadata *extractor.ProjectMetadata) error {
	// Parse the solution file
	solution, err := e.parseSolutionFile(slnPath)
	if err != nil {
		return fmt.Errorf("failed to parse solution file: %w", err)
	}

	// Store solution metadata
	metadata.Name = strings.TrimSuffix(filepath.Base(slnPath), ".sln")
	metadata.LanguageSpecific["dotnet_solution_file"] = filepath.Base(slnPath)
	metadata.LanguageSpecific["dotnet_solution_version"] = solution.Version

	// Store project list
	projectNames := make([]string, 0, len(solution.Projects))
	for _, proj := range solution.Projects {
		projectNames = append(projectNames, proj.Name)
	}
	metadata.LanguageSpecific["dotnet_projects"] = projectNames
	metadata.LanguageSpecific["dotnet_project_count"] = len(solution.Projects)

	// Try to extract from first project file if available
	if len(solution.Projects) > 0 {
		firstProjectPath := filepath.Join(projectPath, solution.Projects[0].Path)
		if _, err := os.Stat(firstProjectPath); err == nil {
			if project, err := e.parseProjectFile(firstProjectPath); err == nil {
				e.extractProjectProperties(project, metadata)
			}
		}
	}

	return nil
}

// extractFromPropsFile extracts metadata from a .props file
func (e *Extractor) extractFromPropsFile(propsPath string, metadata *extractor.ProjectMetadata) error {
	// Parse as project file (same XML structure)
	project, err := e.parseProjectFile(propsPath)
	if err != nil {
		return fmt.Errorf("failed to parse props file: %w", err)
	}

	metadata.Name = strings.TrimSuffix(filepath.Base(propsPath), ".props")
	metadata.LanguageSpecific["dotnet_props_file"] = filepath.Base(propsPath)

	e.extractProjectProperties(project, metadata)

	return nil
}

// findProjectFile finds a project file matching the pattern
func (e *Extractor) findProjectFile(basePath string, pattern string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(basePath, pattern))
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no files matching %s found", pattern)
	}
	return matches[0], nil
}

// parseProjectFile parses a .csproj, .vbproj, .fsproj, or .props file
func (e *Extractor) parseProjectFile(path string) (*Project, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var project Project
	if err := xml.Unmarshal(data, &project); err != nil {
		return nil, err
	}

	return &project, nil
}

// parseSolutionFile parses a .sln file
func (e *Extractor) parseSolutionFile(path string) (*Solution, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	solution := &Solution{
		Projects: make([]SolutionProject, 0),
	}

	lines := strings.Split(string(data), "\n")

	// Parse solution version
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Microsoft Visual Studio Solution File, Format Version") {
			parts := strings.Fields(line)
			if len(parts) > 0 {
				solution.Version = parts[len(parts)-1]
			}
		}

		// Parse project entries
		// Format: Project("{GUID}") = "Name", "Path", "{PROJECT-GUID}"
		if strings.HasPrefix(line, "Project(") {
			project := e.parseSolutionProject(line)
			if project.Name != "" {
				solution.Projects = append(solution.Projects, project)
			}
		}
	}

	return solution, nil
}

// parseSolutionProject parses a single project line from a solution file
func (e *Extractor) parseSolutionProject(line string) SolutionProject {
	project := SolutionProject{}

	// Extract project type GUID
	typeGUIDMatch := regexp.MustCompile(`Project\("([^"]+)"\)`).FindStringSubmatch(line)
	if len(typeGUIDMatch) > 1 {
		project.Type = typeGUIDMatch[1]
	}

	// Extract name, path, and GUID
	// Format: = "Name", "Path", "{GUID}"
	parts := strings.Split(line, "=")
	if len(parts) < 2 {
		return project
	}

	// Parse the quoted values
	quotedPattern := regexp.MustCompile(`"([^"]+)"`)
	matches := quotedPattern.FindAllStringSubmatch(parts[1], -1)

	if len(matches) >= 3 {
		project.Name = matches[0][1]
		project.Path = matches[1][1]
		project.GUID = matches[2][1]
	}

	return project
}

// extractProjectProperties extracts properties from PropertyGroup elements
func (e *Extractor) extractProjectProperties(project *Project, metadata *extractor.ProjectMetadata) {
	// Merge all property groups
	for _, pg := range project.PropertyGroups {
		// Target framework(s)
		if pg.TargetFramework != "" {
			metadata.LanguageSpecific["dotnet_target_framework"] = pg.TargetFramework
			if metadata.Version == "" {
				metadata.Version = e.extractVersionFromFramework(pg.TargetFramework)
			}
		}
		if pg.TargetFrameworks != "" {
			frameworks := strings.Split(pg.TargetFrameworks, ";")
			metadata.LanguageSpecific["dotnet_target_frameworks"] = frameworks
			metadata.LanguageSpecific["dotnet_multi_target"] = true
		}

		// Assembly and package info
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

		// Author and company info
		if pg.Authors != "" {
			metadata.LanguageSpecific["dotnet_authors"] = pg.Authors
			// Split authors into array
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

		// Description and documentation
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

		// URLs
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

		// Tags
		if pg.PackageTags != "" {
			tags := strings.Split(pg.PackageTags, ";")
			metadata.LanguageSpecific["dotnet_tags"] = tags
		}

		// Output type
		if pg.OutputType != "" {
			metadata.LanguageSpecific["dotnet_output_type"] = pg.OutputType
		}

		// Language version
		if pg.LangVersion != "" {
			metadata.LanguageSpecific["dotnet_lang_version"] = pg.LangVersion
		}

		// Nullable reference types
		if pg.Nullable != "" {
			metadata.LanguageSpecific["dotnet_nullable"] = pg.Nullable
		}

		// Implicit usings (C# 10+)
		if pg.ImplicitUsings != "" {
			metadata.LanguageSpecific["dotnet_implicit_usings"] = pg.ImplicitUsings
		}

		// Runtime identifiers
		if pg.RuntimeIdentifier != "" {
			metadata.LanguageSpecific["dotnet_runtime_identifier"] = pg.RuntimeIdentifier
		}
		if pg.RuntimeIdentifiers != "" {
			rids := strings.Split(pg.RuntimeIdentifiers, ";")
			metadata.LanguageSpecific["dotnet_runtime_identifiers"] = rids
		}

		// Publishing options
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

// detectFrameworks detects common .NET frameworks and libraries
func (e *Extractor) detectFrameworks(metadata *extractor.ProjectMetadata) {
	frameworks := make([]string, 0)

	// Check package references
	if pkgs, ok := metadata.LanguageSpecific["dotnet_package_references"].([]map[string]string); ok {
		for _, pkg := range pkgs {
			name := strings.ToLower(pkg["name"])

			// ASP.NET Core
			if strings.Contains(name, "microsoft.aspnetcore") {
				frameworks = appendUnique(frameworks, "ASP.NET Core")
			}

			// Entity Framework Core
			if strings.Contains(name, "microsoft.entityframeworkcore") {
				frameworks = appendUnique(frameworks, "Entity Framework Core")
			}

			// Blazor
			if strings.Contains(name, "blazor") || strings.Contains(name, "microsoft.aspnetcore.components") {
				frameworks = appendUnique(frameworks, "Blazor")
			}

			// SignalR
			if strings.Contains(name, "signalr") {
				frameworks = appendUnique(frameworks, "SignalR")
			}

			// gRPC
			if strings.Contains(name, "grpc") {
				frameworks = appendUnique(frameworks, "gRPC")
			}

			// Minimal APIs
			if strings.Contains(name, "microsoft.aspnetcore.openapi") {
				frameworks = appendUnique(frameworks, "Minimal APIs")
			}

			// Xamarin
			if strings.Contains(name, "xamarin") {
				frameworks = appendUnique(frameworks, "Xamarin")
			}

			// MAUI
			if strings.Contains(name, "microsoft.maui") {
				frameworks = appendUnique(frameworks, "MAUI")
			}

			// WPF
			if strings.Contains(name, "wpf") {
				frameworks = appendUnique(frameworks, "WPF")
			}

			// WinForms
			if strings.Contains(name, "windowsforms") || strings.Contains(name, "winforms") {
				frameworks = appendUnique(frameworks, "WinForms")
			}

			// xUnit
			if strings.Contains(name, "xunit") {
				frameworks = appendUnique(frameworks, "xUnit")
			}

			// NUnit
			if strings.Contains(name, "nunit") {
				frameworks = appendUnique(frameworks, "NUnit")
			}

			// MSTest
			if strings.Contains(name, "mstest") {
				frameworks = appendUnique(frameworks, "MSTest")
			}
		}
	}

	// Check SDK type
	if sdk, ok := metadata.LanguageSpecific["dotnet_sdk"].(string); ok {
		if strings.Contains(sdk, "Microsoft.NET.Sdk.Web") {
			frameworks = appendUnique(frameworks, "ASP.NET Core")
		}
		if strings.Contains(sdk, "Microsoft.NET.Sdk.Blazor") {
			frameworks = appendUnique(frameworks, "Blazor")
		}
		if strings.Contains(sdk, "Microsoft.NET.Sdk.Worker") {
			frameworks = appendUnique(frameworks, "Worker Service")
		}
	}

	if len(frameworks) > 0 {
		metadata.LanguageSpecific["dotnet_frameworks"] = frameworks
	}
}

// generateVersionMatrix generates a version matrix for CI/CD
func (e *Extractor) generateVersionMatrix(metadata *extractor.ProjectMetadata) {
	versions := make([]string, 0)

	// Get target framework(s)
	if fw, ok := metadata.LanguageSpecific["dotnet_target_framework"].(string); ok {
		version := e.getNetVersion(fw)
		if version != "" {
			versions = append(versions, version)
		}
	}

	if fws, ok := metadata.LanguageSpecific["dotnet_target_frameworks"].([]string); ok {
		for _, fw := range fws {
			version := e.getNetVersion(fw)
			if version != "" {
				versions = appendUnique(versions, version)
			}
		}
	}

	// If no specific versions, use modern defaults
	if len(versions) == 0 {
		versions = []string{"8.0", "7.0", "6.0"}
	}

	metadata.LanguageSpecific["dotnet_version_matrix"] = versions

	// Create matrix JSON for GitHub Actions
	matrixJSON := fmt.Sprintf(`{"dotnet-version":["%s"]}`, strings.Join(versions, `","`))
	metadata.LanguageSpecific["matrix_json"] = matrixJSON
}

// getNetVersion extracts .NET version from target framework
func (e *Extractor) getNetVersion(framework string) string {
	framework = strings.ToLower(framework)

	// Modern .NET (5.0+): net8.0, net7.0, net6.0, etc. -> 8.0, 7.0, 6.0
	// Pattern: net followed by digits, dot, and more digits
	if isModernDotNet(framework) {
		versionPattern := regexp.MustCompile(`net(\d+)\.(\d+)`)
		matches := versionPattern.FindStringSubmatch(framework)
		if len(matches) >= 3 {
			return fmt.Sprintf("%s.%s", matches[1], matches[2])
		}
	}

	// Legacy .NET Core: netcoreapp3.1, netcoreapp2.1, etc. -> 3.1, 2.1
	if strings.HasPrefix(framework, "netcoreapp") {
		versionPattern := regexp.MustCompile(`netcoreapp(\d+)\.(\d+)`)
		matches := versionPattern.FindStringSubmatch(framework)
		if len(matches) >= 3 {
			return fmt.Sprintf("%s.%s", matches[1], matches[2])
		}
	}

	return ""
}

// isModernDotNet checks if the framework is modern .NET (5.0+) vs legacy .NET Framework or other variants
// Modern .NET: net5.0, net6.0, net7.0, net8.0, net9.0 (format: net + digits + . + digits)
// NOT: netstandard2.1, netcoreapp3.1, net48 (legacy .NET Framework without decimal)
func isModernDotNet(framework string) bool {
	// Must start with "net" and contain a decimal point
	if !strings.HasPrefix(framework, "net") || !strings.Contains(framework, ".") {
		return false
	}

	// Extract what comes after "net"
	afterNet := strings.TrimPrefix(framework, "net")

	// Check if it starts with a digit (this excludes netstandard, netcoreapp, etc.)
	if len(afterNet) == 0 || afterNet[0] < '0' || afterNet[0] > '9' {
		return false
	}

	// Verify it matches the pattern: digits.digits (optionally followed by platform like -windows)
	matched, _ := regexp.MatchString(`^\d+\.\d+`, afterNet)
	return matched
}

// appendUnique appends a string to a slice if not already present
func appendUnique(slice []string, item string) []string {
	for _, existing := range slice {
		if existing == item {
			return slice
		}
	}
	return append(slice, item)
}
