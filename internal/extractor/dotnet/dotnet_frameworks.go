// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package dotnet

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/lfreleng-actions/build-metadata-action/internal/extractor"
)

// dotnetFrameworkPackageMarkers maps lowercased package-name
// substrings to the .NET framework they imply. A package matches when
// its name contains any of the listed substrings.
var dotnetFrameworkPackageMarkers = []struct {
	substrings []string
	framework  string
}{
	{[]string{"microsoft.aspnetcore"}, "ASP.NET Core"},
	{[]string{"microsoft.entityframeworkcore"}, "Entity Framework Core"},
	{[]string{"blazor", "microsoft.aspnetcore.components"}, "Blazor"},
	{[]string{"signalr"}, "SignalR"},
	{[]string{"grpc"}, "gRPC"},
	{[]string{"microsoft.aspnetcore.openapi"}, "Minimal APIs"},
	{[]string{"xamarin"}, "Xamarin"},
	{[]string{"microsoft.maui"}, "MAUI"},
	{[]string{"wpf"}, "WPF"},
	{[]string{"windowsforms", "winforms"}, "WinForms"},
	{[]string{"xunit"}, "xUnit"},
	{[]string{"nunit"}, "NUnit"},
	{[]string{"mstest"}, "MSTest"},
}

// dotnetFrameworkSDKMarkers maps an SDK attribute substring to the
// framework it implies.
var dotnetFrameworkSDKMarkers = []struct {
	substring string
	framework string
}{
	{"Microsoft.NET.Sdk.Web", "ASP.NET Core"},
	{"Microsoft.NET.Sdk.Blazor", "Blazor"},
	{"Microsoft.NET.Sdk.Worker", "Worker Service"},
}

func (e *Extractor) detectFrameworks(metadata *extractor.ProjectMetadata) {
	frameworks := make([]string, 0)

	if pkgs, ok := metadata.LanguageSpecific["dotnet_package_references"].([]map[string]string); ok {
		for _, pkg := range pkgs {
			name := strings.ToLower(pkg["name"])
			for _, marker := range dotnetFrameworkPackageMarkers {
				if containsAny(name, marker.substrings) {
					frameworks = appendUnique(frameworks, marker.framework)
				}
			}
		}
	}

	if sdk, ok := metadata.LanguageSpecific["dotnet_sdk"].(string); ok {
		for _, marker := range dotnetFrameworkSDKMarkers {
			if strings.Contains(sdk, marker.substring) {
				frameworks = appendUnique(frameworks, marker.framework)
			}
		}
	}

	if len(frameworks) > 0 {
		metadata.LanguageSpecific["dotnet_frameworks"] = frameworks
	}
}

// containsAny reports whether s contains any of the given substrings.
func containsAny(s string, substrings []string) bool {
	for _, sub := range substrings {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
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
