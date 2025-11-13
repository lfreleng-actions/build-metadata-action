// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package java

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/lfreleng-actions/build-metadata-action/internal/extractor"
)

// GradleExtractor extracts metadata from Gradle projects
type GradleExtractor struct {
	extractor.BaseExtractor
}

// NewGradleExtractor creates a new Gradle extractor
func NewGradleExtractor() *GradleExtractor {
	return &GradleExtractor{
		BaseExtractor: extractor.NewBaseExtractor("java-gradle", 4),
	}
}

// GradleProject represents parsed Gradle project information
type GradleProject struct {
	Group       string
	Name        string
	Version     string
	Description string

	// Dependencies
	Dependencies []GradleDependency
	Plugins      []GradlePlugin

	// Build script type
	IsKotlinDSL bool
	BuildFile   string

	// Multi-project
	IsMultiProject bool
	Subprojects    []string

	// Properties
	Properties map[string]string
}

// GradleDependency represents a Gradle dependency
type GradleDependency struct {
	Configuration string
	Group         string
	Name          string
	Version       string
	Notation      string
}

// GradlePlugin represents a Gradle plugin
type GradlePlugin struct {
	ID      string
	Version string
}

// Extract retrieves metadata from a Gradle project
func (e *GradleExtractor) Extract(projectPath string) (*extractor.ProjectMetadata, error) {
	metadata := &extractor.ProjectMetadata{
		LanguageSpecific: make(map[string]interface{}),
	}

	// Determine build file type
	buildFile, isKotlin, err := e.detectBuildFile(projectPath)
	if err != nil {
		return nil, err
	}

	// Parse build file
	gradleProject, err := e.parseGradleBuild(buildFile, isKotlin)
	if err != nil {
		return nil, err
	}

	// Parse settings.gradle if exists
	e.parseSettings(projectPath, gradleProject, isKotlin)

	// Parse gradle.properties if exists
	e.parseProperties(projectPath, gradleProject)

	// Extract common metadata
	metadata.Name = gradleProject.Name
	metadata.Version = gradleProject.Version
	metadata.Description = gradleProject.Description
	metadata.VersionSource = gradleProject.BuildFile

	// Gradle-specific metadata
	metadata.LanguageSpecific["group_id"] = gradleProject.Group
	metadata.LanguageSpecific["artifact_id"] = gradleProject.Name
	metadata.LanguageSpecific["metadata_source"] = gradleProject.BuildFile
	metadata.LanguageSpecific["build_system"] = "gradle"

	if isKotlin {
		metadata.LanguageSpecific["build_dsl"] = "kotlin"
	} else {
		metadata.LanguageSpecific["build_dsl"] = "groovy"
	}

	// Dependencies
	if len(gradleProject.Dependencies) > 0 {
		deps := make([]map[string]string, 0, len(gradleProject.Dependencies))
		configCounts := make(map[string]int)

		for _, dep := range gradleProject.Dependencies {
			depMap := map[string]string{
				"configuration": dep.Configuration,
				"group":         dep.Group,
				"name":          dep.Name,
				"version":       dep.Version,
			}
			deps = append(deps, depMap)
			configCounts[dep.Configuration]++
		}

		metadata.LanguageSpecific["dependencies"] = deps
		metadata.LanguageSpecific["dependency_count"] = len(deps)
		metadata.LanguageSpecific["dependency_configurations"] = configCounts
	}

	// Plugins
	if len(gradleProject.Plugins) > 0 {
		plugins := make([]string, 0, len(gradleProject.Plugins))
		for _, plugin := range gradleProject.Plugins {
			if plugin.Version != "" {
				plugins = append(plugins, fmt.Sprintf("%s:%s", plugin.ID, plugin.Version))
			} else {
				plugins = append(plugins, plugin.ID)
			}
		}
		metadata.LanguageSpecific["plugins"] = plugins
		metadata.LanguageSpecific["plugin_count"] = len(plugins)

		// Detect frameworks from plugins
		frameworks := e.detectGradleFrameworks(gradleProject.Plugins)
		if len(frameworks) > 0 {
			metadata.LanguageSpecific["frameworks"] = frameworks
		}
	}

	// Multi-project
	if gradleProject.IsMultiProject {
		metadata.LanguageSpecific["is_multi_project"] = true
		metadata.LanguageSpecific["subprojects"] = gradleProject.Subprojects
		metadata.LanguageSpecific["subproject_count"] = len(gradleProject.Subprojects)
	}

	// Properties
	if len(gradleProject.Properties) > 0 {
		metadata.LanguageSpecific["properties"] = gradleProject.Properties

		// Extract Java version if specified
		if javaVersion, ok := gradleProject.Properties["java.version"]; ok {
			metadata.LanguageSpecific["java_version"] = javaVersion
		} else if sourceCompat, ok := gradleProject.Properties["sourceCompatibility"]; ok {
			metadata.LanguageSpecific["java_version"] = sourceCompat
		}
	}

	// Check for dynamic version
	if strings.Contains(metadata.Version, "SNAPSHOT") ||
		strings.Contains(metadata.Version, "project.version") ||
		strings.Contains(metadata.Version, "rootProject.version") {
		metadata.LanguageSpecific["versioning_type"] = "dynamic"
	} else {
		metadata.LanguageSpecific["versioning_type"] = "static"
	}

	return metadata, nil
}

// detectBuildFile determines which build file to use
func (e *GradleExtractor) detectBuildFile(projectPath string) (string, bool, error) {
	// Try build.gradle.kts first (Kotlin DSL)
	ktsPath := filepath.Join(projectPath, "build.gradle.kts")
	if _, err := os.Stat(ktsPath); err == nil {
		return ktsPath, true, nil
	}

	// Try build.gradle (Groovy DSL)
	groovyPath := filepath.Join(projectPath, "build.gradle")
	if _, err := os.Stat(groovyPath); err == nil {
		return groovyPath, false, nil
	}

	return "", false, fmt.Errorf("no Gradle build file found in %s", projectPath)
}

// parseGradleBuild parses a Gradle build file
func (e *GradleExtractor) parseGradleBuild(buildFile string, isKotlin bool) (*GradleProject, error) {
	content, err := os.ReadFile(buildFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read build file: %w", err)
	}

	project := &GradleProject{
		BuildFile:    filepath.Base(buildFile),
		IsKotlinDSL:  isKotlin,
		Dependencies: make([]GradleDependency, 0),
		Plugins:      make([]GradlePlugin, 0),
		Properties:   make(map[string]string),
	}

	text := string(content)

	// Extract group
	project.Group = e.extractGradleProperty(text, "group", isKotlin)

	// Extract version
	project.Version = e.extractGradleProperty(text, "version", isKotlin)

	// Extract description
	project.Description = e.extractGradleProperty(text, "description", isKotlin)

	// Extract plugins
	project.Plugins = e.extractPlugins(text, isKotlin)

	// Extract dependencies
	project.Dependencies = e.extractDependencies(text, isKotlin)

	return project, nil
}

// extractGradleProperty extracts a property value from Gradle build file
func (e *GradleExtractor) extractGradleProperty(content, property string, isKotlin bool) string {
	if isKotlin {
		// Kotlin DSL patterns:
		// group = "com.example"
		// version = "1.0.0"
		patterns := []string{
			fmt.Sprintf(`%s\s*=\s*"([^"]+)"`, property),
			fmt.Sprintf(`%s\s*=\s*'([^']+)'`, property),
		}

		for _, pattern := range patterns {
			re := regexp.MustCompile(pattern)
			if matches := re.FindStringSubmatch(content); len(matches) > 1 {
				return matches[1]
			}
		}
	} else {
		// Groovy DSL patterns:
		// group 'com.example'
		// group = 'com.example'
		// version "1.0.0"
		patterns := []string{
			fmt.Sprintf(`%s\s+['"]([^'"]+)['"]`, property),
			fmt.Sprintf(`%s\s*=\s*['"]([^'"]+)['"]`, property),
		}

		for _, pattern := range patterns {
			re := regexp.MustCompile(pattern)
			if matches := re.FindStringSubmatch(content); len(matches) > 1 {
				return matches[1]
			}
		}
	}

	return ""
}

// extractPlugins extracts plugin declarations
func (e *GradleExtractor) extractPlugins(content string, isKotlin bool) []GradlePlugin {
	plugins := make([]GradlePlugin, 0)

	if isKotlin {
		// Kotlin DSL patterns:
		// id("com.example.plugin") version "1.0"
		// kotlin("jvm") version "1.9.0"
		patterns := []*regexp.Regexp{
			regexp.MustCompile(`id\("([^"]+)"\)\s+version\s+"([^"]+)"`),
			regexp.MustCompile(`kotlin\("([^"]+)"\)\s+version\s+"([^"]+)"`),
			regexp.MustCompile(`id\("([^"]+)"\)`),
		}

		for _, pattern := range patterns {
			matches := pattern.FindAllStringSubmatch(content, -1)
			for _, match := range matches {
				plugin := GradlePlugin{}
				if len(match) > 1 {
					plugin.ID = match[1]
					if pattern.String() == `kotlin\("([^"]+)"\)\s+version\s+"([^"]+)"` {
						plugin.ID = "org.jetbrains.kotlin." + match[1]
					}
				}
				if len(match) > 2 {
					plugin.Version = match[2]
				}
				plugins = append(plugins, plugin)
			}
		}
	} else {
		// Groovy DSL patterns:
		// id 'com.example.plugin' version '1.0'
		// id 'com.example.plugin'
		patterns := []*regexp.Regexp{
			regexp.MustCompile(`id\s+['"]([^'"]+)['"]\s+version\s+['"]([^'"]+)['"]`),
			regexp.MustCompile(`id\s+['"]([^'"]+)['"]`),
		}

		for _, pattern := range patterns {
			matches := pattern.FindAllStringSubmatch(content, -1)
			for _, match := range matches {
				plugin := GradlePlugin{}
				if len(match) > 1 {
					plugin.ID = match[1]
				}
				if len(match) > 2 {
					plugin.Version = match[2]
				}
				plugins = append(plugins, plugin)
			}
		}
	}

	return plugins
}

// extractDependencies extracts dependency declarations
func (e *GradleExtractor) extractDependencies(content string, isKotlin bool) []GradleDependency {
	dependencies := make([]GradleDependency, 0)

	// Common configurations
	configurations := []string{
		"implementation", "api", "compileOnly", "runtimeOnly",
		"testImplementation", "testCompileOnly", "testRuntimeOnly",
		"annotationProcessor", "kapt",
	}

	for _, config := range configurations {
		if isKotlin {
			// Kotlin DSL: implementation("group:artifact:version")
			pattern := regexp.MustCompile(fmt.Sprintf(`%s\("([^:]+):([^:]+):([^"]+)"\)`, config))
			matches := pattern.FindAllStringSubmatch(content, -1)
			for _, match := range matches {
				if len(match) > 3 {
					dep := GradleDependency{
						Configuration: config,
						Group:         match[1],
						Name:          match[2],
						Version:       match[3],
						Notation:      fmt.Sprintf("%s:%s:%s", match[1], match[2], match[3]),
					}
					dependencies = append(dependencies, dep)
				}
			}
		} else {
			// Groovy DSL: implementation 'group:artifact:version'
			pattern := regexp.MustCompile(fmt.Sprintf(`%s\s+['"]([^:]+):([^:]+):([^'"]+)['"]`, config))
			matches := pattern.FindAllStringSubmatch(content, -1)
			for _, match := range matches {
				if len(match) > 3 {
					dep := GradleDependency{
						Configuration: config,
						Group:         match[1],
						Name:          match[2],
						Version:       match[3],
						Notation:      fmt.Sprintf("%s:%s:%s", match[1], match[2], match[3]),
					}
					dependencies = append(dependencies, dep)
				}
			}
		}
	}

	return dependencies
}

// parseSettings parses settings.gradle or settings.gradle.kts
func (e *GradleExtractor) parseSettings(projectPath string, project *GradleProject, isKotlin bool) {
	var settingsFile string
	if isKotlin {
		settingsFile = filepath.Join(projectPath, "settings.gradle.kts")
	} else {
		settingsFile = filepath.Join(projectPath, "settings.gradle")
	}

	content, err := os.ReadFile(settingsFile)
	if err != nil {
		return // Settings file is optional
	}

	text := string(content)

	// Extract root project name
	if project.Name == "" {
		project.Name = e.extractGradleProperty(text, "rootProject.name", isKotlin)
	}

	// Extract subprojects
	subprojects := e.extractSubprojects(text, isKotlin)
	if len(subprojects) > 0 {
		project.IsMultiProject = true
		project.Subprojects = subprojects
	}
}

// extractSubprojects extracts subproject names from settings file
func (e *GradleExtractor) extractSubprojects(content string, isKotlin bool) []string {
	subprojects := make([]string, 0)

	// Pattern: include("project1", "project2") or include 'project1', 'project2'
	var pattern *regexp.Regexp
	if isKotlin {
		pattern = regexp.MustCompile(`include\("([^"]+)"\)`)
	} else {
		pattern = regexp.MustCompile(`include\s+['"]([^'"]+)['"]`)
	}

	matches := pattern.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		if len(match) > 1 {
			subprojects = append(subprojects, match[1])
		}
	}

	return subprojects
}

// parseProperties parses gradle.properties file
func (e *GradleExtractor) parseProperties(projectPath string, project *GradleProject) {
	propsFile := filepath.Join(projectPath, "gradle.properties")

	file, err := os.Open(propsFile)
	if err != nil {
		return // Properties file is optional
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse key=value
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			project.Properties[key] = value

			// Override project properties if defined in gradle.properties
			if key == "version" && project.Version == "" {
				project.Version = value
			}
			if key == "group" && project.Group == "" {
				project.Group = value
			}
		}
	}
}

// detectGradleFrameworks detects common frameworks from Gradle plugins
func (e *GradleExtractor) detectGradleFrameworks(plugins []GradlePlugin) []string {
	frameworks := make([]string, 0)
	seen := make(map[string]bool)

	for _, plugin := range plugins {
		framework := ""

		switch {
		case plugin.ID == "org.springframework.boot":
			framework = "Spring Boot"
		case plugin.ID == "io.quarkus":
			framework = "Quarkus"
		case plugin.ID == "io.micronaut.application":
			framework = "Micronaut"
		case strings.Contains(plugin.ID, "kotlin"):
			framework = "Kotlin"
		case plugin.ID == "java":
			framework = "Java"
		case plugin.ID == "application":
			framework = "Java Application"
		case plugin.ID == "java-library":
			framework = "Java Library"
		case plugin.ID == "com.android.application":
			framework = "Android"
		case plugin.ID == "com.android.library":
			framework = "Android Library"
		}

		if framework != "" && !seen[framework] {
			frameworks = append(frameworks, framework)
			seen[framework] = true
		}
	}

	return frameworks
}

// Detect checks if this extractor can handle the project
func (e *GradleExtractor) Detect(projectPath string) bool {
	// Check for build.gradle.kts
	ktsPath := filepath.Join(projectPath, "build.gradle.kts")
	if _, err := os.Stat(ktsPath); err == nil {
		return true
	}

	// Check for build.gradle
	groovyPath := filepath.Join(projectPath, "build.gradle")
	if _, err := os.Stat(groovyPath); err == nil {
		return true
	}

	return false
}

// init registers the Gradle extractor
func init() {
	extractor.RegisterExtractor(NewGradleExtractor())
}
