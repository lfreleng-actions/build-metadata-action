// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package output

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/lfreleng-actions/build-metadata-action/internal/repository"
)

// Metadata interface represents the metadata structure
// This is a simplified interface - actual implementation should match main.Metadata
type Metadata interface{}

// GenerateSummary creates a GitHub Step Summary formatted output
func GenerateSummary(metadata interface{}) string {
	var sb strings.Builder

	// Try to extract metadata fields using type assertion
	// In real implementation, this would work with the actual Metadata struct
	metadataMap := convertToMap(metadata)

	// Extract project type early as we need it for filtering
	var projectType string
	if common, ok := metadataMap["common"].(map[string]interface{}); ok {
		if pt, ok := common["project_type"].(string); ok {
			projectType = pt
		}
	}

	// Extract project path for repository detection
	var projectPath string
	if common, ok := metadataMap["common"].(map[string]interface{}); ok {
		if pp, ok := common["project_path"].(string); ok {
			projectPath = pp
		}
	}

	// Header
	sb.WriteString("## 🔧 Build Metadata\n\n")

	// Detect repository information
	var repoInfo string
	if projectPath != "" {
		if info, err := repository.DetectRepository(projectPath); err == nil {
			repoInfo = info.FormatForDisplay()
		}
	}

	// Project Information Section (consolidated)
	if common, ok := metadataMap["common"].(map[string]interface{}); ok {
		// Include repository info in header if available
		if repoInfo != "" {
			sb.WriteString(fmt.Sprintf("### %s\n\n", repoInfo))
		} else {
			sb.WriteString("### Project Information\n\n")
		}
		sb.WriteString("| Key | Value |\n")
		sb.WriteString("|-----|-------|\n")

		// Basic project info
		if projectType != "" {
			sb.WriteString(fmt.Sprintf("| Project Type | %s |\n", formatProjectType(projectType)))
		}

		if projectName, ok := common["project_name"].(string); ok && projectName != "" {
			sb.WriteString(fmt.Sprintf("| Project Name | %s |\n", projectName))
		}

		if projectVersion, ok := common["project_version"].(string); ok && projectVersion != "" {
			sb.WriteString(fmt.Sprintf("| Project Version | %s |\n", projectVersion))
		}

		if versionSource, ok := common["version_source"].(string); ok && versionSource != "" {
			sb.WriteString(fmt.Sprintf("| Version Source | %s |\n", versionSource))
		}

		if versioningType, ok := common["versioning_type"].(string); ok && versioningType != "" {
			sb.WriteString(fmt.Sprintf("| Versioning Type | %s |\n", versioningType))
		} else {
			// Default to "static" if not specified
			sb.WriteString("| Versioning Type | static |\n")
		}

		// Handle timestamp - could be time.Time or string after JSON conversion
		if buildTimestamp, ok := common["build_timestamp"].(time.Time); ok {
			// Format as: 2025-11-03 11:37:48 UTC
			formattedTime := buildTimestamp.UTC().Format("2006-01-02 15:04:05") + " UTC"
			sb.WriteString(fmt.Sprintf("| Build Timestamp | %s |\n", formattedTime))
		} else if buildTimestampStr, ok := common["build_timestamp"].(string); ok && buildTimestampStr != "" {
			// Already in string format from JSON marshaling, try to parse and reformat
			if parsedTime, err := time.Parse(time.RFC3339, buildTimestampStr); err == nil {
				formattedTime := parsedTime.UTC().Format("2006-01-02 15:04:05") + " UTC"
				sb.WriteString(fmt.Sprintf("| Build Timestamp | %s |\n", formattedTime))
			} else {
				// If parsing fails, use original string
				sb.WriteString(fmt.Sprintf("| Build Timestamp | %s |\n", buildTimestampStr))
			}
		}

		if gitBranch, ok := common["git_branch"].(string); ok && gitBranch != "" {
			sb.WriteString(fmt.Sprintf("| Git Branch | `%s` |\n", gitBranch))
		}

		if gitTag, ok := common["git_tag"].(string); ok && gitTag != "" {
			sb.WriteString(fmt.Sprintf("| Git Tag | `%s` |\n", gitTag))
		}

		// Add language-specific metadata to the same table
		if langSpecific, ok := metadataMap["language_specific"].(map[string]interface{}); ok && len(langSpecific) > 0 {
			addLanguageSpecificToTable(&sb, projectType, langSpecific)
		}

		// Add project_match_repo comparison (common to all project types)
		if projectMatchRepo, ok := common["project_match_repo"].(bool); ok {
			matchStatus := "true ✅"
			if !projectMatchRepo {
				matchStatus = "false ❌"
			}
			sb.WriteString(fmt.Sprintf("| Project Matches Repository | %s |\n", matchStatus))
		} else if projectMatchRepoStr, ok := common["project_match_repo"].(string); ok {
			if projectMatchRepoStr == "true" {
				sb.WriteString("| Project Matches Repository | true ✅ |\n")
			} else if projectMatchRepoStr == "false" {
				sb.WriteString("| Project Matches Repository | false ❌ |\n")
			}
		}

		// Add relevant tool versions to the same table
		if env, ok := metadataMap["environment"].(map[string]interface{}); ok {
			if toolsInterface, ok := env["tools"].(map[string]interface{}); ok && len(toolsInterface) > 0 {
				// Convert map[string]interface{} to map[string]string
				allTools := make(map[string]string)
				for k, v := range toolsInterface {
					if strVal, ok := v.(string); ok {
						allTools[k] = strVal
					}
				}

				// Filter to only relevant tools based on project type
				relevantTools := filterRelevantTools(projectType, allTools)
				if len(relevantTools) > 0 {
					// Sort tools alphabetically for consistent output
					sortedTools := sortMapKeys(relevantTools)
					for _, tool := range sortedTools {
						sb.WriteString(fmt.Sprintf("| %s | %s |\n", formatToolName(tool), relevantTools[tool]))
					}
				}
			}
		}

		sb.WriteString("\n")
	}

	return sb.String()
}

// GenerateMarkdown creates a markdown formatted output
func GenerateMarkdown(metadata interface{}) string {
	// Similar to GenerateSummary but with different formatting
	return GenerateSummary(metadata)
}

// formatProjectType converts internal project type to display name
func formatProjectType(projectType string) string {
	typeMap := map[string]string{
		"python-modern":      "Python (Modern)",
		"python-legacy":      "Python (Legacy)",
		"javascript-npm":     "JavaScript (npm)",
		"javascript-yarn":    "JavaScript (Yarn)",
		"javascript-pnpm":    "JavaScript (pnpm)",
		"typescript-npm":     "TypeScript (npm)",
		"java-maven":         "Java (Maven)",
		"java-gradle":        "Java (Gradle)",
		"java-gradle-kts":    "Java (Gradle Kotlin DSL)",
		"csharp-project":     "C# (.NET Project)",
		"csharp-solution":    "C# (.NET Solution)",
		"dotnet-project":     ".NET Project",
		"go-module":          "Go (Module)",
		"rust-cargo":         "Rust (Cargo)",
		"ruby-gemspec":       "Ruby (Gem)",
		"ruby-bundler":       "Ruby (Bundler)",
		"php-composer":       "PHP (Composer)",
		"swift-package":      "Swift (Package)",
		"dart-flutter":       "Dart/Flutter",
		"terraform":          "Terraform",
		"terraform-opentofu": "OpenTofu",
		"docker":             "Docker",
		"helm":               "Helm Chart",
		"c-cmake":            "C/C++ (CMake)",
		"c-qmake":            "C/C++ (Qt qmake)",
		"c-autoconf":         "C/C++ (Autoconf)",
	}

	if display, ok := typeMap[projectType]; ok {
		return display
	}

	// Capitalize first letter and replace hyphens with spaces
	parts := strings.Split(projectType, "-")
	for i, part := range parts {
		if len(part) > 0 {
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return strings.Join(parts, " ")
}

// addLanguageSpecificToTable adds key language-specific metadata to the table
func addLanguageSpecificToTable(sb *strings.Builder, projectType string, metadata map[string]interface{}) {
	if metadata == nil {
		return
	}

	switch {
	case strings.HasPrefix(projectType, "python"):
		// Metadata source
		if metadataSource, ok := metadata["metadata_source"].(string); ok && metadataSource != "" {
			sb.WriteString(fmt.Sprintf("| Metadata Source | %s |\n", metadataSource))
		}

		// Package name
		if packageName, ok := metadata["package_name"].(string); ok && packageName != "" {
			sb.WriteString(fmt.Sprintf("| Package Name | `%s` |\n", packageName))
		}

		// Build Python version
		if buildVersion, ok := metadata["build_version"].(string); ok && buildVersion != "" {
			sb.WriteString(fmt.Sprintf("| Build Python | %s |\n", buildVersion))
		}

		// Matrix JSON
		if matrixJSON, ok := metadata["matrix_json"].(string); ok && matrixJSON != "" {
			sb.WriteString(fmt.Sprintf("| Matrix JSON | `%s` |\n", matrixJSON))
		}

		// Requires Python
		if requiresPython, ok := metadata["requires_python"].(string); ok && requiresPython != "" {
			sb.WriteString(fmt.Sprintf("| Requires Python | %s |\n", requiresPython))
		}

		// Build Backend
		if buildBackend, ok := metadata["build_backend"].(string); ok && buildBackend != "" {
			sb.WriteString(fmt.Sprintf("| Build Backend | %s |\n", buildBackend))
		}

		// Project/Package match
		if projectMatchPackage, ok := metadata["project_match_package"].(bool); ok {
			matchStatus := "true ✅"
			if !projectMatchPackage {
				matchStatus = "false ⚠️"
			}
			sb.WriteString(fmt.Sprintf("| Project/Package Names Match | %s |\n", matchStatus))
		}

	case strings.HasPrefix(projectType, "javascript") || strings.HasPrefix(projectType, "typescript"):
		if packageManager, ok := metadata["package_manager"].(string); ok && packageManager != "" {
			sb.WriteString(fmt.Sprintf("| Package Manager | %s |\n", packageManager))
		}
		if moduleType, ok := metadata["module_type"].(string); ok && moduleType != "" {
			sb.WriteString(fmt.Sprintf("| Module Type | %s |\n", moduleType))
		}
		if requiresNode, ok := metadata["requires_node"].(string); ok && requiresNode != "" {
			sb.WriteString(fmt.Sprintf("| Requires Node | %s |\n", requiresNode))
		}

	case strings.HasPrefix(projectType, "java"):
		if groupID, ok := metadata["group_id"].(string); ok && groupID != "" {
			sb.WriteString(fmt.Sprintf("| Group ID | `%s` |\n", groupID))
		}
		if artifactID, ok := metadata["artifact_id"].(string); ok && artifactID != "" {
			sb.WriteString(fmt.Sprintf("| Artifact ID | `%s` |\n", artifactID))
		}
		if packaging, ok := metadata["packaging"].(string); ok && packaging != "" {
			sb.WriteString(fmt.Sprintf("| Packaging | %s |\n", packaging))
		}

	case strings.HasPrefix(projectType, "go"):
		if module, ok := metadata["module"].(string); ok && module != "" {
			sb.WriteString(fmt.Sprintf("| Go Module | `%s` |\n", module))
		}
		if goVersion, ok := metadata["go_version"].(string); ok && goVersion != "" {
			sb.WriteString(fmt.Sprintf("| Go Version | %s |\n", goVersion))
		}

	case strings.HasPrefix(projectType, "rust"):
		if edition, ok := metadata["edition"].(string); ok && edition != "" {
			sb.WriteString(fmt.Sprintf("| Rust Edition | %s |\n", edition))
		}
		if msrv, ok := metadata["msrv"].(string); ok && msrv != "" {
			sb.WriteString(fmt.Sprintf("| MSRV | %s |\n", msrv))
		}

	case strings.HasPrefix(projectType, "csharp") || strings.HasPrefix(projectType, "dotnet"):
		if framework, ok := metadata["framework"].(string); ok && framework != "" {
			sb.WriteString(fmt.Sprintf("| Target Framework | %s |\n", framework))
		}

	case strings.HasPrefix(projectType, "php"):
		if requiresPhp, ok := metadata["requires_php"].(string); ok && requiresPhp != "" {
			sb.WriteString(fmt.Sprintf("| Requires PHP | %s |\n", requiresPhp))
		}

	case strings.HasPrefix(projectType, "ruby"):
		if rubyVersion, ok := metadata["ruby_version"].(string); ok && rubyVersion != "" {
			sb.WriteString(fmt.Sprintf("| Ruby Version | %s |\n", rubyVersion))
		}

	case strings.HasPrefix(projectType, "swift"):
		if swiftVersion, ok := metadata["swift_tools_version"].(string); ok && swiftVersion != "" {
			sb.WriteString(fmt.Sprintf("| Swift Tools Version | %s |\n", swiftVersion))
		}

	case strings.HasPrefix(projectType, "terraform"):
		if terraformVersion, ok := metadata["terraform_version"].(string); ok && terraformVersion != "" {
			sb.WriteString(fmt.Sprintf("| Terraform Version | %s |\n", terraformVersion))
		}
		if isOpenTofu, ok := metadata["is_opentofu"].(bool); ok && isOpenTofu {
			sb.WriteString("| Engine | OpenTofu |\n")
		}

	case strings.HasPrefix(projectType, "helm"):
		if apiVersion, ok := metadata["api_version"].(string); ok && apiVersion != "" {
			sb.WriteString(fmt.Sprintf("| Chart API Version | %s |\n", apiVersion))
		}
		if appVersion, ok := metadata["app_version"].(string); ok && appVersion != "" {
			sb.WriteString(fmt.Sprintf("| App Version | %s |\n", appVersion))
		}

	case strings.HasPrefix(projectType, "dart"):
		if sdkConstraint, ok := metadata["sdk_constraint"].(string); ok && sdkConstraint != "" {
			sb.WriteString(fmt.Sprintf("| Dart SDK | %s |\n", sdkConstraint))
		}
		if isFlutter, ok := metadata["is_flutter"].(bool); ok && isFlutter {
			sb.WriteString("| Framework | Flutter |\n")
		}
	}
}

// filterRelevantTools filters tools to only those relevant to the project type
func filterRelevantTools(projectType string, allTools map[string]string) map[string]string {
	if projectType == "" || len(allTools) == 0 {
		return make(map[string]string)
	}

	relevant := make(map[string]string)

	// Filter based on project type
	switch {
	case strings.HasPrefix(projectType, "python"):
		for _, tool := range []string{"python3", "pip"} {
			if version, ok := allTools[tool]; ok {
				relevant[tool] = version
			}
		}

	case strings.HasPrefix(projectType, "javascript") || strings.HasPrefix(projectType, "typescript"):
		for _, tool := range []string{"node", "npm", "yarn"} {
			if version, ok := allTools[tool]; ok {
				relevant[tool] = version
			}
		}

	case strings.HasPrefix(projectType, "java"):
		for _, tool := range []string{"java", "javac", "mvn", "gradle"} {
			if version, ok := allTools[tool]; ok {
				relevant[tool] = version
			}
		}

	case strings.HasPrefix(projectType, "go"):
		if version, ok := allTools["go"]; ok {
			relevant["go"] = version
		}

	case strings.HasPrefix(projectType, "rust"):
		for _, tool := range []string{"rustc", "cargo"} {
			if version, ok := allTools[tool]; ok {
				relevant[tool] = version
			}
		}

	case strings.HasPrefix(projectType, "csharp") || strings.HasPrefix(projectType, "dotnet"):
		if version, ok := allTools["dotnet"]; ok {
			relevant["dotnet"] = version
		}

	case strings.HasPrefix(projectType, "php"):
		for _, tool := range []string{"php", "composer"} {
			if version, ok := allTools[tool]; ok {
				relevant[tool] = version
			}
		}

	case strings.HasPrefix(projectType, "ruby"):
		for _, tool := range []string{"ruby", "gem"} {
			if version, ok := allTools[tool]; ok {
				relevant[tool] = version
			}
		}

	case strings.HasPrefix(projectType, "swift"):
		if version, ok := allTools["swift"]; ok {
			relevant["swift"] = version
		}

	case strings.HasPrefix(projectType, "terraform"):
		for _, tool := range []string{"terraform", "tofu"} {
			if version, ok := allTools[tool]; ok {
				relevant[tool] = version
			}
		}

	case strings.HasPrefix(projectType, "docker"):
		for _, tool := range []string{"docker", "kubectl"} {
			if version, ok := allTools[tool]; ok {
				relevant[tool] = version
			}
		}

	case strings.HasPrefix(projectType, "helm"):
		for _, tool := range []string{"helm", "kubectl"} {
			if version, ok := allTools[tool]; ok {
				relevant[tool] = version
			}
		}

	case strings.HasPrefix(projectType, "dart"):
		for _, tool := range []string{"dart", "flutter"} {
			if version, ok := allTools[tool]; ok {
				relevant[tool] = version
			}
		}

	case strings.HasPrefix(projectType, "c-"):
		for _, tool := range []string{"gcc", "clang", "cmake", "make"} {
			if version, ok := allTools[tool]; ok {
				relevant[tool] = version
			}
		}
	}

	return relevant
}

// formatToolName formats tool names for display
func formatToolName(tool string) string {
	nameMap := map[string]string{
		"python3":   "Python 3 Version",
		"python":    "Python Version",
		"pip":       "pip Version",
		"node":      "Node.js Version",
		"npm":       "npm Version",
		"yarn":      "Yarn Version",
		"go":        "Go Version",
		"rustc":     "Rust Version",
		"cargo":     "Cargo Version",
		"java":      "Java Version",
		"javac":     "Java Compiler Version",
		"mvn":       "Maven Version",
		"gradle":    "Gradle Version",
		"dotnet":    ".NET Version",
		"php":       "PHP Version",
		"composer":  "Composer Version",
		"ruby":      "Ruby Version",
		"gem":       "RubyGems Version",
		"swift":     "Swift Version",
		"git":       "Git Version",
		"terraform": "Terraform Version",
		"tofu":      "OpenTofu Version",
		"docker":    "Docker Version",
		"kubectl":   "kubectl Version",
		"helm":      "Helm Version",
		"dart":      "Dart Version",
		"flutter":   "Flutter Version",
		"gcc":       "GCC Version",
		"clang":     "Clang Version",
		"cmake":     "CMake Version",
		"make":      "Make Version",
	}

	if display, ok := nameMap[tool]; ok {
		return display
	}

	// Capitalize first letter
	if len(tool) > 0 {
		return strings.ToUpper(tool[:1]) + tool[1:] + " Version"
	}
	return tool
}

// convertToMap converts metadata to a map using JSON marshaling
func convertToMap(metadata interface{}) map[string]interface{} {
	// Marshal to JSON and back to get a map
	jsonBytes, err := json.Marshal(metadata)
	if err != nil {
		return make(map[string]interface{})
	}

	var result map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		return make(map[string]interface{})
	}

	return result
}

// sortMapKeys returns sorted keys from a map
func sortMapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	// Simple alphabetical sort
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[i] > keys[j] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}

	return keys
}
