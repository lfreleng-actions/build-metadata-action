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

	metadataMap := convertToMap(metadata)
	projectType := extractCommonString(metadataMap, "project_type")
	projectPath := extractCommonString(metadataMap, "project_path")

	sb.WriteString("## 🔧 Build Metadata\n\n")

	var repoInfo string
	if projectPath != "" {
		if info, err := repository.DetectRepository(projectPath); err == nil {
			repoInfo = info.FormatForDisplay()
		}
	}

	common, ok := metadataMap["common"].(map[string]interface{})
	if !ok {
		return sb.String()
	}

	writeProjectInfoHeader(&sb, repoInfo)
	writeCommonProjectRows(&sb, common, projectType)

	if langSpecific, ok := metadataMap["language_specific"].(map[string]interface{}); ok && len(langSpecific) > 0 {
		addLanguageSpecificToTable(&sb, projectType, langSpecific)
	}

	writeProjectMatchRepoRow(&sb, common)
	writeRelevantToolRows(&sb, metadataMap, projectType)

	sb.WriteString("\n")
	return sb.String()
}

// extractCommonString returns a string field from the "common" section, or ""
// when the section or field is absent.
func extractCommonString(metadataMap map[string]interface{}, key string) string {
	common, ok := metadataMap["common"].(map[string]interface{})
	if !ok {
		return ""
	}
	if v, ok := common[key].(string); ok {
		return v
	}
	return ""
}

func writeProjectInfoHeader(sb *strings.Builder, repoInfo string) {
	if repoInfo != "" {
		fmt.Fprintf(sb, "### %s\n\n", repoInfo)
	} else {
		sb.WriteString("### Project Information\n\n")
	}
	sb.WriteString("| Key | Value |\n")
	sb.WriteString("|-----|-------|\n")
}

func writeCommonProjectRows(sb *strings.Builder, common map[string]interface{}, projectType string) {
	if projectType != "" {
		fmt.Fprintf(sb, "| Project Type | %s |\n", formatProjectType(projectType))
	}

	writeStringRows(sb, common, []stringRow{
		{"project_name", "Project Name", false},
		{"project_version", "Project Version", false},
		{"version_source", "Version Source", false},
	})

	if versioningType, ok := common["versioning_type"].(string); ok && versioningType != "" {
		fmt.Fprintf(sb, "| Versioning Type | %s |\n", versioningType)
	} else {
		sb.WriteString("| Versioning Type | static |\n")
	}

	writeVersionPropertiesRows(sb, common)

	if snapshotVersion, ok := common["snapshot_version"].(string); ok && snapshotVersion != "" {
		fmt.Fprintf(sb, "| Snapshot Version | %s |\n", snapshotVersion)
	}

	writeBuildTimestampRow(sb, common)

	if gitBranch, ok := common["git_branch"].(string); ok && gitBranch != "" {
		fmt.Fprintf(sb, "| Git Branch | `%s` |\n", gitBranch)
	}
	if gitTag, ok := common["git_tag"].(string); ok && gitTag != "" {
		fmt.Fprintf(sb, "| Git Tag | `%s` |\n", gitTag)
	}
}

// writeVersionPropertiesRows renders version.properties details (LF/ONAP
// release convention) whenever the file yielded a version, so release
// pipelines can see the authoritative value even when a language manifest
// won the version_source selection.
func writeVersionPropertiesRows(sb *strings.Builder, common map[string]interface{}) {
	propsVersion, ok := common["version_properties_version"].(string)
	if !ok || propsVersion == "" {
		return
	}
	fmt.Fprintf(sb, "| version.properties | %s |\n", propsVersion)
	if propsMatch, ok := common["version_properties_match"].(string); ok && propsMatch != "" {
		matchStatus := "true ✅"
		if propsMatch != "true" {
			matchStatus = "false ❌"
		}
		fmt.Fprintf(sb, "| Version Match | %s |\n", matchStatus)
	}
}

// writeBuildTimestampRow renders the build timestamp, which may arrive as a
// time.Time or as an RFC3339 string after JSON round-tripping.
func writeBuildTimestampRow(sb *strings.Builder, common map[string]interface{}) {
	if buildTimestamp, ok := common["build_timestamp"].(time.Time); ok {
		formattedTime := buildTimestamp.UTC().Format("2006-01-02 15:04:05") + " UTC"
		fmt.Fprintf(sb, "| Build Timestamp | %s |\n", formattedTime)
		return
	}

	buildTimestampStr, ok := common["build_timestamp"].(string)
	if !ok || buildTimestampStr == "" {
		return
	}
	if parsedTime, err := time.Parse(time.RFC3339, buildTimestampStr); err == nil {
		formattedTime := parsedTime.UTC().Format("2006-01-02 15:04:05") + " UTC"
		fmt.Fprintf(sb, "| Build Timestamp | %s |\n", formattedTime)
	} else {
		fmt.Fprintf(sb, "| Build Timestamp | %s |\n", buildTimestampStr)
	}
}

// writeProjectMatchRepoRow renders the project/repository match comparison,
// which is common to all project types.
func writeProjectMatchRepoRow(sb *strings.Builder, common map[string]interface{}) {
	if projectMatchRepo, ok := common["project_match_repo"].(bool); ok {
		matchStatus := "true ✅"
		if !projectMatchRepo {
			matchStatus = "false ❌"
		}
		fmt.Fprintf(sb, "| Project Matches Repository | %s |\n", matchStatus)
		return
	}
	if projectMatchRepoStr, ok := common["project_match_repo"].(string); ok {
		switch projectMatchRepoStr {
		case "true":
			sb.WriteString("| Project Matches Repository | true ✅ |\n")
		case "false":
			sb.WriteString("| Project Matches Repository | false ❌ |\n")
		}
	}
}

func writeRelevantToolRows(sb *strings.Builder, metadataMap map[string]interface{}, projectType string) {
	env, ok := metadataMap["environment"].(map[string]interface{})
	if !ok {
		return
	}
	toolsInterface, ok := env["tools"].(map[string]interface{})
	if !ok || len(toolsInterface) == 0 {
		return
	}

	allTools := make(map[string]string)
	for k, v := range toolsInterface {
		if strVal, ok := v.(string); ok {
			allTools[k] = strVal
		}
	}

	relevantTools := filterRelevantTools(projectType, allTools)
	if len(relevantTools) == 0 {
		return
	}
	for _, tool := range sortMapKeys(relevantTools) {
		fmt.Fprintf(sb, "| %s | %s |\n", formatToolName(tool), relevantTools[tool])
	}
}

// stringRow describes a metadata field to render as a table row. When code is
// true the value is wrapped in backticks.
type stringRow struct {
	key   string
	label string
	code  bool
}

func writeStringRows(sb *strings.Builder, metadata map[string]interface{}, rows []stringRow) {
	for _, row := range rows {
		v, ok := metadata[row.key].(string)
		if !ok || v == "" {
			continue
		}
		if row.code {
			fmt.Fprintf(sb, "| %s | `%s` |\n", row.label, v)
		} else {
			fmt.Fprintf(sb, "| %s | %s |\n", row.label, v)
		}
	}
}

// hasAnyPrefix reports whether s starts with any of the given prefixes.
func hasAnyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
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

// languageTableWriters maps project-type prefixes to the writer that renders
// their language-specific rows. The slice is ordered so that more specific
// prefixes (e.g. "javascript") are matched before overlapping shorter ones
// (e.g. "java").
var languageTableWriters = []struct {
	prefixes []string
	write    func(sb *strings.Builder, metadata map[string]interface{})
}{
	{[]string{"python"}, writePythonRows},
	{[]string{"javascript", "typescript"}, writeJSRows},
	{[]string{"java"}, writeJavaRows},
	{[]string{"go"}, writeGoRows},
	{[]string{"rust"}, writeRustRows},
	{[]string{"csharp", "dotnet"}, writeDotnetRows},
	{[]string{"php"}, writePHPRows},
	{[]string{"ruby"}, writeRubyRows},
	{[]string{"swift"}, writeSwiftRows},
	{[]string{"terraform"}, writeTerraformRows},
	{[]string{"helm"}, writeHelmRows},
	{[]string{"dart"}, writeDartRows},
}

// addLanguageSpecificToTable adds key language-specific metadata to the table
func addLanguageSpecificToTable(sb *strings.Builder, projectType string, metadata map[string]interface{}) {
	if metadata == nil {
		return
	}
	for _, w := range languageTableWriters {
		if hasAnyPrefix(projectType, w.prefixes) {
			w.write(sb, metadata)
			return
		}
	}
}

func writePythonRows(sb *strings.Builder, metadata map[string]interface{}) {
	writeStringRows(sb, metadata, []stringRow{
		{"metadata_source", "Metadata Source", false},
		{"package_name", "Package Name", true},
		{"build_version", "Build Python", false},
		{"matrix_json", "Matrix JSON", true},
		{"requires_python", "Requires Python", false},
		{"build_backend", "Build Backend", false},
	})

	if projectMatchPackage, ok := metadata["project_match_package"].(bool); ok {
		matchStatus := "true ✅"
		if !projectMatchPackage {
			matchStatus = "false ⚠️"
		}
		fmt.Fprintf(sb, "| Project/Package Names Match | %s |\n", matchStatus)
	}
}

func writeJSRows(sb *strings.Builder, metadata map[string]interface{}) {
	writeStringRows(sb, metadata, []stringRow{
		{"package_manager", "Package Manager", false},
		{"module_type", "Module Type", false},
		{"requires_node", "Requires Node", false},
	})
}

func writeJavaRows(sb *strings.Builder, metadata map[string]interface{}) {
	writeStringRows(sb, metadata, []stringRow{
		{"group_id", "Group ID", true},
		{"artifact_id", "Artifact ID", true},
		{"packaging", "Packaging", false},
	})
}

func writeGoRows(sb *strings.Builder, metadata map[string]interface{}) {
	writeStringRows(sb, metadata, []stringRow{
		{"module", "Go Module", true},
		{"go_version", "Go Version", false},
	})
}

func writeRustRows(sb *strings.Builder, metadata map[string]interface{}) {
	writeStringRows(sb, metadata, []stringRow{
		{"edition", "Rust Edition", false},
		{"msrv", "MSRV", false},
	})
}

func writeDotnetRows(sb *strings.Builder, metadata map[string]interface{}) {
	writeStringRows(sb, metadata, []stringRow{
		{"framework", "Target Framework", false},
	})
}

func writePHPRows(sb *strings.Builder, metadata map[string]interface{}) {
	writeStringRows(sb, metadata, []stringRow{
		{"requires_php", "Requires PHP", false},
	})
}

func writeRubyRows(sb *strings.Builder, metadata map[string]interface{}) {
	writeStringRows(sb, metadata, []stringRow{
		{"ruby_version", "Ruby Version", false},
	})
}

func writeSwiftRows(sb *strings.Builder, metadata map[string]interface{}) {
	writeStringRows(sb, metadata, []stringRow{
		{"swift_tools_version", "Swift Tools Version", false},
	})
}

func writeTerraformRows(sb *strings.Builder, metadata map[string]interface{}) {
	writeStringRows(sb, metadata, []stringRow{
		{"terraform_version", "Terraform Version", false},
	})
	if isOpenTofu, ok := metadata["is_opentofu"].(bool); ok && isOpenTofu {
		sb.WriteString("| Engine | OpenTofu |\n")
	}
}

func writeHelmRows(sb *strings.Builder, metadata map[string]interface{}) {
	writeStringRows(sb, metadata, []stringRow{
		{"api_version", "Chart API Version", false},
		{"app_version", "App Version", false},
	})
}

func writeDartRows(sb *strings.Builder, metadata map[string]interface{}) {
	writeStringRows(sb, metadata, []stringRow{
		{"sdk_constraint", "Dart SDK", false},
	})
	if isFlutter, ok := metadata["is_flutter"].(bool); ok && isFlutter {
		sb.WriteString("| Framework | Flutter |\n")
	}
}

// toolsByProjectPrefix maps project-type prefixes to the tools worth showing
// for that ecosystem. The slice is ordered so overlapping prefixes (e.g.
// "javascript" before "java") resolve to the intended entry.
var toolsByProjectPrefix = []struct {
	prefixes []string
	tools    []string
}{
	// python3 is intentionally omitted: the "Build Python" field already shows
	// the recommended version from project metadata, and the detected python3
	// is the system Python (build-metadata-action runs before setup-python),
	// which would be misleading. Only pip is relevant for dependency install.
	{[]string{"python"}, []string{"pip"}},
	{[]string{"javascript", "typescript"}, []string{"node", "npm", "yarn"}},
	{[]string{"java"}, []string{"java", "javac", "mvn", "gradle"}},
	{[]string{"go"}, []string{"go"}},
	{[]string{"rust"}, []string{"rustc", "cargo"}},
	{[]string{"csharp", "dotnet"}, []string{"dotnet"}},
	{[]string{"php"}, []string{"php", "composer"}},
	{[]string{"ruby"}, []string{"ruby", "gem"}},
	{[]string{"swift"}, []string{"swift"}},
	{[]string{"terraform"}, []string{"terraform", "tofu"}},
	{[]string{"docker"}, []string{"docker", "kubectl"}},
	{[]string{"helm"}, []string{"helm", "kubectl"}},
	{[]string{"dart"}, []string{"dart", "flutter"}},
	{[]string{"c-"}, []string{"gcc", "clang", "cmake", "make"}},
}

// filterRelevantTools filters tools to only those relevant to the project type
func filterRelevantTools(projectType string, allTools map[string]string) map[string]string {
	relevant := make(map[string]string)
	if projectType == "" || len(allTools) == 0 {
		return relevant
	}

	for _, entry := range toolsByProjectPrefix {
		if !hasAnyPrefix(projectType, entry.prefixes) {
			continue
		}
		for _, tool := range entry.tools {
			if version, ok := allTools[tool]; ok {
				relevant[tool] = version
			}
		}
		break
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
