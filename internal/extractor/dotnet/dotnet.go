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
	csprojMatches, _ := filepath.Glob(filepath.Join(projectPath, "*.csproj"))
	if len(csprojMatches) > 0 {
		return true
	}

	slnMatches, _ := filepath.Glob(filepath.Join(projectPath, "*.sln"))
	if len(slnMatches) > 0 {
		return true
	}

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
	project, err := e.parseProjectFile(csprojPath)
	if err != nil {
		return fmt.Errorf("failed to parse project file: %w", err)
	}

	e.extractProjectProperties(project, metadata)

	e.extractPackageReferences(project, metadata)

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

	quotedPattern := regexp.MustCompile(`"([^"]+)"`)
	matches := quotedPattern.FindAllStringSubmatch(parts[1], -1)

	if len(matches) >= 3 {
		project.Name = matches[0][1]
		project.Path = matches[1][1]
		project.GUID = matches[2][1]
	}

	return project
}
