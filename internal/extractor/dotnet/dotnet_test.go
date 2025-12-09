// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package dotnet

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetect(t *testing.T) {
	tests := []struct {
		name  string
		files []string
		want  bool
	}{
		{"with .csproj", []string{"MyApp.csproj"}, true},
		{"with .sln", []string{"MySolution.sln"}, true},
		{"with .props", []string{"Directory.Build.props"}, true},
		{"no .NET files", []string{"package.json"}, false},
		{"empty directory", []string{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			for _, file := range tt.files {
				path := filepath.Join(tmpDir, file)
				if err := os.WriteFile(path, []byte("test"), 0644); err != nil {
					t.Fatalf("Failed to create test file: %v", err)
				}
			}

			e := NewExtractor()
			if got := e.Detect(tmpDir); got != tt.want {
				t.Errorf("Detect() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractorName(t *testing.T) {
	e := NewExtractor()
	if got := e.Name(); got != "dotnet" {
		t.Errorf("Name() = %q, want %q", got, "dotnet")
	}
}

func TestGetNetVersion(t *testing.T) {
	tests := []struct {
		name      string
		framework string
		want      string
	}{
		// Modern .NET (5.0+) - should extract version
		{name: "net8.0", framework: "net8.0", want: "8.0"},
		{name: "net7.0", framework: "net7.0", want: "7.0"},
		{name: "net6.0", framework: "net6.0", want: "6.0"},
		{name: "net5.0", framework: "net5.0", want: "5.0"},
		{name: "NET8.0 uppercase", framework: "NET8.0", want: "8.0"},
		{name: "net9.0 future", framework: "net9.0", want: "9.0"},
		{name: "net10.0 future", framework: "net10.0", want: "10.0"},
		{name: "net8.0-windows", framework: "net8.0-windows", want: "8.0"},
		{name: "net8.0-android", framework: "net8.0-android", want: "8.0"},

		// Legacy .NET Core - should extract version
		{name: "netcoreapp3.1", framework: "netcoreapp3.1", want: "3.1"},
		{name: "netcoreapp2.1", framework: "netcoreapp2.1", want: "2.1"},
		{name: "netcoreapp2.0", framework: "netcoreapp2.0", want: "2.0"},

		// .NET Standard - should NOT extract (return empty)
		{name: "netstandard2.1", framework: "netstandard2.1", want: ""},
		{name: "netstandard2.0", framework: "netstandard2.0", want: ""},
		{name: "netstandard1.6", framework: "netstandard1.6", want: ""},

		// Legacy .NET Framework (no decimal) - should NOT extract
		{name: "net48", framework: "net48", want: ""},
		{name: "net472", framework: "net472", want: ""},
		{name: "net471", framework: "net471", want: ""},
		{name: "net47", framework: "net47", want: ""},
		{name: "net462", framework: "net462", want: ""},
		{name: "net461", framework: "net461", want: ""},
		{name: "net46", framework: "net46", want: ""},
		{name: "net452", framework: "net452", want: ""},
		{name: "net451", framework: "net451", want: ""},
		{name: "net45", framework: "net45", want: ""},
		{name: "net40", framework: "net40", want: ""},
		{name: "net35", framework: "net35", want: ""},

		// Invalid or edge cases
		{name: "empty", framework: "", want: ""},
		{name: "just net", framework: "net", want: ""},
		{name: "invalid format", framework: "netx.y", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewExtractor()
			got := e.getNetVersion(tt.framework)
			if got != tt.want {
				t.Errorf("getNetVersion(%q) = %q, want %q", tt.framework, got, tt.want)
			}
		})
	}
}

func TestIsModernDotNet(t *testing.T) {
	tests := []struct {
		name      string
		framework string
		want      bool
	}{
		// Modern .NET (5.0+) - should return true
		{name: "net8.0", framework: "net8.0", want: true},
		{name: "net7.0", framework: "net7.0", want: true},
		{name: "net6.0", framework: "net6.0", want: true},
		{name: "net5.0", framework: "net5.0", want: true},
		{name: "net9.0", framework: "net9.0", want: true},
		{name: "net10.0", framework: "net10.0", want: true},
		{name: "net8.0-windows", framework: "net8.0-windows", want: true},
		{name: "net8.0-android", framework: "net8.0-android", want: true},

		// NOT modern .NET - should return false
		{name: "netcoreapp3.1", framework: "netcoreapp3.1", want: false},
		{name: "netcoreapp2.1", framework: "netcoreapp2.1", want: false},
		{name: "netstandard2.1", framework: "netstandard2.1", want: false},
		{name: "netstandard2.0", framework: "netstandard2.0", want: false},
		{name: "net48", framework: "net48", want: false},
		{name: "net472", framework: "net472", want: false},
		{name: "net45", framework: "net45", want: false},
		{name: "net40", framework: "net40", want: false},
		{name: "empty", framework: "", want: false},
		{name: "just net", framework: "net", want: false},
		{name: "no decimal", framework: "net8", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isModernDotNet(tt.framework)
			if got != tt.want {
				t.Errorf("isModernDotNet(%q) = %v, want %v", tt.framework, got, tt.want)
			}
		})
	}
}

func TestExtractorPriority(t *testing.T) {
	e := NewExtractor()
	if got := e.Priority(); got != 5 {
		t.Errorf("Priority() = %d, want %d", got, 5)
	}
}

func TestExtractSDKStyleProject(t *testing.T) {
	tmpDir := t.TempDir()

	csprojContent := `<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup>
    <TargetFramework>net8.0</TargetFramework>
    <AssemblyName>MyApp</AssemblyName>
    <Version>1.2.3</Version>
    <Authors>John Doe</Authors>
    <Company>Acme Corp</Company>
    <Description>A sample application</Description>
    <PackageLicenseExpression>MIT</PackageLicenseExpression>
    <OutputType>Exe</OutputType>
    <LangVersion>latest</LangVersion>
    <Nullable>enable</Nullable>
  </PropertyGroup>

  <ItemGroup>
    <PackageReference Include="Newtonsoft.Json" Version="13.0.3" />
    <PackageReference Include="Serilog" Version="3.1.1" />
  </ItemGroup>
</Project>`

	csprojPath := filepath.Join(tmpDir, "MyApp.csproj")
	if err := os.WriteFile(csprojPath, []byte(csprojContent), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	e := NewExtractor()
	metadata, err := e.Extract(tmpDir)
	if err != nil {
		t.Fatalf("Extract() failed: %v", err)
	}

	// Validate common metadata
	if got := metadata.Name; got != "MyApp" {
		t.Errorf("Name = %q, want %q", got, "MyApp")
	}

	if got := metadata.Version; got != "1.2.3" {
		t.Errorf("Version = %q, want %q", got, "1.2.3")
	}

	if got := metadata.Description; got != "A sample application" {
		t.Errorf("Description = %q, want %q", got, "A sample application")
	}

	if got := metadata.License; got != "MIT" {
		t.Errorf("License = %q, want %q", got, "MIT")
	}

	// Validate language-specific metadata
	if got := metadata.LanguageSpecific["dotnet_target_framework"]; got != "net8.0" {
		t.Errorf("dotnet_target_framework = %q, want %q", got, "net8.0")
	}

	if got := metadata.LanguageSpecific["dotnet_assembly_name"]; got != "MyApp" {
		t.Errorf("dotnet_assembly_name = %q, want %q", got, "MyApp")
	}

	if got := metadata.LanguageSpecific["dotnet_sdk_style"]; got != true {
		t.Errorf("dotnet_sdk_style = %v, want true", got)
	}

	if got := metadata.LanguageSpecific["dotnet_sdk"]; got != "Microsoft.NET.Sdk" {
		t.Errorf("dotnet_sdk = %q, want %q", got, "Microsoft.NET.Sdk")
	}

	if got := metadata.LanguageSpecific["dotnet_output_type"]; got != "Exe" {
		t.Errorf("dotnet_output_type = %q, want %q", got, "Exe")
	}

	if got := metadata.LanguageSpecific["dotnet_nullable"]; got != "enable" {
		t.Errorf("dotnet_nullable = %q, want %q", got, "enable")
	}

	// Validate package references
	pkgs, ok := metadata.LanguageSpecific["dotnet_package_references"].([]map[string]string)
	if !ok {
		t.Fatal("dotnet_package_references is not []map[string]string")
	}

	if len(pkgs) != 2 {
		t.Errorf("Expected 2 package references, got %d", len(pkgs))
	}

	if got := metadata.LanguageSpecific["dotnet_package_count"]; got != 2 {
		t.Errorf("dotnet_package_count = %v, want 2", got)
	}
}

func TestExtractWebProject(t *testing.T) {
	tmpDir := t.TempDir()

	csprojContent := `<Project Sdk="Microsoft.NET.Sdk.Web">
  <PropertyGroup>
    <TargetFramework>net8.0</TargetFramework>
    <AssemblyName>WebApp</AssemblyName>
    <Version>2.0.0</Version>
    <ImplicitUsings>enable</ImplicitUsings>
  </PropertyGroup>

  <ItemGroup>
    <PackageReference Include="Microsoft.AspNetCore.OpenApi" Version="8.0.0" />
    <PackageReference Include="Swashbuckle.AspNetCore" Version="6.5.0" />
  </ItemGroup>
</Project>`

	csprojPath := filepath.Join(tmpDir, "WebApp.csproj")
	if err := os.WriteFile(csprojPath, []byte(csprojContent), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	e := NewExtractor()
	metadata, err := e.Extract(tmpDir)
	if err != nil {
		t.Fatalf("Extract() failed: %v", err)
	}

	// Check SDK
	if got := metadata.LanguageSpecific["dotnet_sdk"]; got != "Microsoft.NET.Sdk.Web" {
		t.Errorf("dotnet_sdk = %q, want %q", got, "Microsoft.NET.Sdk.Web")
	}

	// Check frameworks detection
	frameworks, ok := metadata.LanguageSpecific["dotnet_frameworks"].([]string)
	if !ok {
		t.Fatal("dotnet_frameworks is not []string")
	}

	foundAspNetCore := false
	foundMinimalAPIs := false
	for _, fw := range frameworks {
		if fw == "ASP.NET Core" {
			foundAspNetCore = true
		}
		if fw == "Minimal APIs" {
			foundMinimalAPIs = true
		}
	}

	if !foundAspNetCore {
		t.Error("Expected ASP.NET Core framework to be detected")
	}

	if !foundMinimalAPIs {
		t.Error("Expected Minimal APIs framework to be detected")
	}
}

func TestExtractMultiTargetProject(t *testing.T) {
	tmpDir := t.TempDir()

	csprojContent := `<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup>
    <TargetFrameworks>net8.0;net7.0;net6.0</TargetFrameworks>
    <AssemblyName>MultiTarget</AssemblyName>
    <Version>1.0.0</Version>
  </PropertyGroup>
</Project>`

	csprojPath := filepath.Join(tmpDir, "MultiTarget.csproj")
	if err := os.WriteFile(csprojPath, []byte(csprojContent), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	e := NewExtractor()
	metadata, err := e.Extract(tmpDir)
	if err != nil {
		t.Fatalf("Extract() failed: %v", err)
	}

	// Check multi-target flag
	if got := metadata.LanguageSpecific["dotnet_multi_target"]; got != true {
		t.Errorf("dotnet_multi_target = %v, want true", got)
	}

	// Check target frameworks
	frameworks, ok := metadata.LanguageSpecific["dotnet_target_frameworks"].([]string)
	if !ok {
		t.Fatal("dotnet_target_frameworks is not []string")
	}

	if len(frameworks) != 3 {
		t.Errorf("Expected 3 target frameworks, got %d", len(frameworks))
	}

	// Check version matrix
	matrix, ok := metadata.LanguageSpecific["dotnet_version_matrix"].([]string)
	if !ok {
		t.Fatal("dotnet_version_matrix is not []string")
	}

	expectedVersions := map[string]bool{"8.0": true, "7.0": true, "6.0": true}
	for _, version := range matrix {
		if !expectedVersions[version] {
			t.Errorf("Unexpected version in matrix: %s", version)
		}
	}
}

func TestExtractProjectWithReferences(t *testing.T) {
	tmpDir := t.TempDir()

	csprojContent := `<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup>
    <TargetFramework>net8.0</TargetFramework>
    <AssemblyName>MainApp</AssemblyName>
  </PropertyGroup>

  <ItemGroup>
    <ProjectReference Include="..\LibraryA\LibraryA.csproj" />
    <ProjectReference Include="..\LibraryB\LibraryB.csproj" />
  </ItemGroup>

  <ItemGroup>
    <PackageReference Include="Newtonsoft.Json" Version="13.0.3" />
  </ItemGroup>
</Project>`

	csprojPath := filepath.Join(tmpDir, "MainApp.csproj")
	if err := os.WriteFile(csprojPath, []byte(csprojContent), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	e := NewExtractor()
	metadata, err := e.Extract(tmpDir)
	if err != nil {
		t.Fatalf("Extract() failed: %v", err)
	}

	// Check project references
	projRefs, ok := metadata.LanguageSpecific["dotnet_project_references"].([]string)
	if !ok {
		t.Fatal("dotnet_project_references is not []string")
	}

	if len(projRefs) != 2 {
		t.Errorf("Expected 2 project references, got %d", len(projRefs))
	}

	if got := metadata.LanguageSpecific["dotnet_project_reference_count"]; got != 2 {
		t.Errorf("dotnet_project_reference_count = %v, want 2", got)
	}
}

func TestExtractSolutionFile(t *testing.T) {
	tmpDir := t.TempDir()

	slnContent := `
Microsoft Visual Studio Solution File, Format Version 12.00
# Visual Studio Version 17
VisualStudioVersion = 17.0.31903.59
MinimumVisualStudioVersion = 10.0.40219.1
Project("{FAE04EC0-301F-11D3-BF4B-00C04F79EFBC}") = "WebApp", "src\WebApp\WebApp.csproj", "{12345678-1234-1234-1234-123456789012}"
EndProject
Project("{FAE04EC0-301F-11D3-BF4B-00C04F79EFBC}") = "Library", "src\Library\Library.csproj", "{87654321-4321-4321-4321-210987654321}"
EndProject
Global
	GlobalSection(SolutionConfigurationPlatforms) = preSolution
		Debug|Any CPU = Debug|Any CPU
		Release|Any CPU = Release|Any CPU
	EndGlobalSection
EndGlobal`

	slnPath := filepath.Join(tmpDir, "MySolution.sln")
	if err := os.WriteFile(slnPath, []byte(slnContent), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	e := NewExtractor()
	metadata, err := e.Extract(tmpDir)
	if err != nil {
		t.Fatalf("Extract() failed: %v", err)
	}

	// Check solution name
	if got := metadata.Name; got != "MySolution" {
		t.Errorf("Name = %q, want %q", got, "MySolution")
	}

	// Check solution version
	if got := metadata.LanguageSpecific["dotnet_solution_version"]; got != "12.00" {
		t.Errorf("dotnet_solution_version = %q, want %q", got, "12.00")
	}

	// Check project count
	if got := metadata.LanguageSpecific["dotnet_project_count"]; got != 2 {
		t.Errorf("dotnet_project_count = %v, want 2", got)
	}

	// Check projects list
	projects, ok := metadata.LanguageSpecific["dotnet_projects"].([]string)
	if !ok {
		t.Fatal("dotnet_projects is not []string")
	}

	if len(projects) != 2 {
		t.Errorf("Expected 2 projects, got %d", len(projects))
	}
}

func TestExtractBlazorProject(t *testing.T) {
	tmpDir := t.TempDir()

	csprojContent := `<Project Sdk="Microsoft.NET.Sdk.BlazorWebAssembly">
  <PropertyGroup>
    <TargetFramework>net8.0</TargetFramework>
    <AssemblyName>BlazorApp</AssemblyName>
  </PropertyGroup>

  <ItemGroup>
    <PackageReference Include="Microsoft.AspNetCore.Components.WebAssembly" Version="8.0.0" />
    <PackageReference Include="Microsoft.AspNetCore.Components.WebAssembly.DevServer" Version="8.0.0" />
  </ItemGroup>
</Project>`

	csprojPath := filepath.Join(tmpDir, "BlazorApp.csproj")
	if err := os.WriteFile(csprojPath, []byte(csprojContent), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	e := NewExtractor()
	metadata, err := e.Extract(tmpDir)
	if err != nil {
		t.Fatalf("Extract() failed: %v", err)
	}

	// Check frameworks detection
	frameworks, ok := metadata.LanguageSpecific["dotnet_frameworks"].([]string)
	if !ok {
		t.Fatal("dotnet_frameworks is not []string")
	}

	foundBlazor := false
	for _, fw := range frameworks {
		if fw == "Blazor" {
			foundBlazor = true
		}
	}

	if !foundBlazor {
		t.Error("Expected Blazor framework to be detected")
	}
}

func TestExtractTestProject(t *testing.T) {
	tmpDir := t.TempDir()

	csprojContent := `<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup>
    <TargetFramework>net8.0</TargetFramework>
    <IsPackable>false</IsPackable>
    <IsTestProject>true</IsTestProject>
  </PropertyGroup>

  <ItemGroup>
    <PackageReference Include="Microsoft.NET.Test.Sdk" Version="17.8.0" />
    <PackageReference Include="xunit" Version="2.6.2" />
    <PackageReference Include="xunit.runner.visualstudio" Version="2.5.4" />
    <PackageReference Include="coverlet.collector" Version="6.0.0" />
  </ItemGroup>
</Project>`

	csprojPath := filepath.Join(tmpDir, "Tests.csproj")
	if err := os.WriteFile(csprojPath, []byte(csprojContent), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	e := NewExtractor()
	metadata, err := e.Extract(tmpDir)
	if err != nil {
		t.Fatalf("Extract() failed: %v", err)
	}

	// Check frameworks detection
	frameworks, ok := metadata.LanguageSpecific["dotnet_frameworks"].([]string)
	if !ok {
		t.Fatal("dotnet_frameworks is not []string")
	}

	foundXUnit := false
	for _, fw := range frameworks {
		if fw == "xUnit" {
			foundXUnit = true
		}
	}

	if !foundXUnit {
		t.Error("Expected xUnit framework to be detected")
	}
}

func TestExtractEntityFrameworkProject(t *testing.T) {
	tmpDir := t.TempDir()

	csprojContent := `<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup>
    <TargetFramework>net8.0</TargetFramework>
  </PropertyGroup>

  <ItemGroup>
    <PackageReference Include="Microsoft.EntityFrameworkCore" Version="8.0.0" />
    <PackageReference Include="Microsoft.EntityFrameworkCore.SqlServer" Version="8.0.0" />
    <PackageReference Include="Microsoft.EntityFrameworkCore.Tools" Version="8.0.0" />
  </ItemGroup>
</Project>`

	csprojPath := filepath.Join(tmpDir, "DataAccess.csproj")
	if err := os.WriteFile(csprojPath, []byte(csprojContent), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	e := NewExtractor()
	metadata, err := e.Extract(tmpDir)
	if err != nil {
		t.Fatalf("Extract() failed: %v", err)
	}

	// Check frameworks detection
	frameworks, ok := metadata.LanguageSpecific["dotnet_frameworks"].([]string)
	if !ok {
		t.Fatal("dotnet_frameworks is not []string")
	}

	foundEF := false
	for _, fw := range frameworks {
		if fw == "Entity Framework Core" {
			foundEF = true
		}
	}

	if !foundEF {
		t.Error("Expected Entity Framework Core to be detected")
	}
}

func TestExtractLegacyProject(t *testing.T) {
	tmpDir := t.TempDir()

	csprojContent := `<?xml version="1.0" encoding="utf-8"?>
<Project ToolsVersion="15.0" xmlns="http://schemas.microsoft.com/developer/msbuild/2003">
  <PropertyGroup>
    <Configuration Condition=" '$(Configuration)' == '' ">Debug</Configuration>
    <Platform Condition=" '$(Platform)' == '' ">AnyCPU</Platform>
    <ProjectGuid>{12345678-1234-1234-1234-123456789012}</ProjectGuid>
    <OutputType>Library</OutputType>
    <AssemblyName>LegacyLibrary</AssemblyName>
    <TargetFrameworkVersion>v4.7.2</TargetFrameworkVersion>
  </PropertyGroup>
</Project>`

	csprojPath := filepath.Join(tmpDir, "LegacyLibrary.csproj")
	if err := os.WriteFile(csprojPath, []byte(csprojContent), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	e := NewExtractor()
	metadata, err := e.Extract(tmpDir)
	if err != nil {
		t.Fatalf("Extract() failed: %v", err)
	}

	// Check that it's not SDK-style
	if got := metadata.LanguageSpecific["dotnet_sdk_style"]; got != false {
		t.Errorf("dotnet_sdk_style = %v, want false", got)
	}

	// Should still extract assembly name
	if got := metadata.LanguageSpecific["dotnet_assembly_name"]; got != "LegacyLibrary" {
		t.Errorf("dotnet_assembly_name = %q, want %q", got, "LegacyLibrary")
	}
}

func TestExtractWithPackageMetadata(t *testing.T) {
	tmpDir := t.TempDir()

	csprojContent := `<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup>
    <TargetFramework>net8.0</TargetFramework>
    <PackageId>MyNuGetPackage</PackageId>
    <Version>1.2.3</Version>
    <Authors>Jane Doe;John Smith</Authors>
    <Company>Example Inc</Company>
    <Product>My Product</Product>
    <Description>A comprehensive library for doing things</Description>
    <Copyright>Copyright (c) 2025 Example Inc</Copyright>
    <PackageLicenseExpression>Apache-2.0</PackageLicenseExpression>
    <PackageProjectUrl>https://github.com/example/mypackage</PackageProjectUrl>
    <RepositoryUrl>https://github.com/example/mypackage.git</RepositoryUrl>
    <RepositoryType>git</RepositoryType>
    <PackageTags>library;utility;helper</PackageTags>
  </PropertyGroup>
</Project>`

	csprojPath := filepath.Join(tmpDir, "MyPackage.csproj")
	if err := os.WriteFile(csprojPath, []byte(csprojContent), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	e := NewExtractor()
	metadata, err := e.Extract(tmpDir)
	if err != nil {
		t.Fatalf("Extract() failed: %v", err)
	}

	// Validate common metadata
	if len(metadata.Authors) != 2 {
		t.Errorf("Expected 2 authors, got %d", len(metadata.Authors))
	}

	if got := metadata.Homepage; got != "https://github.com/example/mypackage" {
		t.Errorf("Homepage = %q, want expected URL", got)
	}

	if got := metadata.Repository; got != "https://github.com/example/mypackage.git" {
		t.Errorf("Repository = %q, want expected URL", got)
	}

	// Validate package metadata
	if got := metadata.LanguageSpecific["dotnet_package_id"]; got != "MyNuGetPackage" {
		t.Errorf("dotnet_package_id = %q, want %q", got, "MyNuGetPackage")
	}

	if got := metadata.LanguageSpecific["dotnet_company"]; got != "Example Inc" {
		t.Errorf("dotnet_company = %q, want %q", got, "Example Inc")
	}

	// Check tags
	tags, ok := metadata.LanguageSpecific["dotnet_tags"].([]string)
	if !ok {
		t.Fatal("dotnet_tags is not []string")
	}

	if len(tags) != 3 {
		t.Errorf("Expected 3 tags, got %d", len(tags))
	}
}

func TestExtractNoProjectFile(t *testing.T) {
	tmpDir := t.TempDir()

	e := NewExtractor()
	_, err := e.Extract(tmpDir)
	if err == nil {
		t.Error("Expected error when no project files exist, got nil")
	}
}

func TestExtractInvalidXML(t *testing.T) {
	tmpDir := t.TempDir()

	csprojContent := `<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup>
    <TargetFramework>net8.0
  </PropertyGroup>
</Project>`

	csprojPath := filepath.Join(tmpDir, "Invalid.csproj")
	if err := os.WriteFile(csprojPath, []byte(csprojContent), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	e := NewExtractor()
	_, err := e.Extract(tmpDir)
	if err == nil {
		t.Error("Expected error when parsing invalid XML, got nil")
	}
}

func TestVersionMatrixGeneration(t *testing.T) {
	tmpDir := t.TempDir()

	csprojContent := `<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup>
    <TargetFrameworks>net8.0;net7.0;netcoreapp3.1</TargetFrameworks>
  </PropertyGroup>
</Project>`

	csprojPath := filepath.Join(tmpDir, "MultiTarget.csproj")
	if err := os.WriteFile(csprojPath, []byte(csprojContent), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	e := NewExtractor()
	metadata, err := e.Extract(tmpDir)
	if err != nil {
		t.Fatalf("Extract() failed: %v", err)
	}

	// Check matrix JSON
	matrixJSON, ok := metadata.LanguageSpecific["matrix_json"].(string)
	if !ok {
		t.Fatal("matrix_json is not string")
	}

	if matrixJSON == "" {
		t.Error("matrix_json is empty")
	}

	// Should contain version numbers
	if !contains(matrixJSON, "8.0") {
		t.Error("matrix_json missing 8.0")
	}
	if !contains(matrixJSON, "7.0") {
		t.Error("matrix_json missing 7.0")
	}
	if !contains(matrixJSON, "3.1") {
		t.Error("matrix_json missing 3.1")
	}
}

func TestExtractWithRuntimeIdentifiers(t *testing.T) {
	tmpDir := t.TempDir()

	csprojContent := `<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup>
    <TargetFramework>net8.0</TargetFramework>
    <RuntimeIdentifiers>win-x64;linux-x64;osx-x64</RuntimeIdentifiers>
    <SelfContained>true</SelfContained>
    <PublishSingleFile>true</PublishSingleFile>
    <PublishTrimmed>true</PublishTrimmed>
  </PropertyGroup>
</Project>`

	csprojPath := filepath.Join(tmpDir, "NativeApp.csproj")
	if err := os.WriteFile(csprojPath, []byte(csprojContent), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	e := NewExtractor()
	metadata, err := e.Extract(tmpDir)
	if err != nil {
		t.Fatalf("Extract() failed: %v", err)
	}

	// Check runtime identifiers
	rids, ok := metadata.LanguageSpecific["dotnet_runtime_identifiers"].([]string)
	if !ok {
		t.Fatal("dotnet_runtime_identifiers is not []string")
	}

	if len(rids) != 3 {
		t.Errorf("Expected 3 runtime identifiers, got %d", len(rids))
	}

	// Check publishing options
	if got := metadata.LanguageSpecific["dotnet_self_contained"]; got != "true" {
		t.Errorf("dotnet_self_contained = %q, want %q", got, "true")
	}

	if got := metadata.LanguageSpecific["dotnet_publish_single_file"]; got != "true" {
		t.Errorf("dotnet_publish_single_file = %q, want %q", got, "true")
	}
}

// Helper function
func contains(s, substr string) bool {
	if len(s) == 0 || len(substr) == 0 {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
