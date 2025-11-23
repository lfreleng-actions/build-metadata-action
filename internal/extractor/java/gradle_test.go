// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package java

import (
	"os"
	"path/filepath"
	"testing"
)

// TestGradleDetect tests Gradle project detection
func TestGradleDetect(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(dir string) error
		expected bool
	}{
		{
			name: "valid build.gradle",
			setup: func(dir string) error {
				return os.WriteFile(filepath.Join(dir, "build.gradle"), []byte("// Gradle build"), 0644)
			},
			expected: true,
		},
		{
			name: "valid build.gradle.kts",
			setup: func(dir string) error {
				return os.WriteFile(filepath.Join(dir, "build.gradle.kts"), []byte("// Kotlin DSL"), 0644)
			},
			expected: true,
		},
		{
			name: "both build files - prefers Kotlin",
			setup: func(dir string) error {
				if err := os.WriteFile(filepath.Join(dir, "build.gradle"), []byte("// Groovy"), 0644); err != nil {
					return err
				}
				return os.WriteFile(filepath.Join(dir, "build.gradle.kts"), []byte("// Kotlin"), 0644)
			},
			expected: true,
		},
		{
			name: "no build file",
			setup: func(dir string) error {
				return nil
			},
			expected: false,
		},
		{
			name: "build.gradle in subdirectory should not detect at root",
			setup: func(dir string) error {
				subdir := filepath.Join(dir, "subproject")
				if err := os.MkdirAll(subdir, 0755); err != nil {
					return err
				}
				return os.WriteFile(filepath.Join(subdir, "build.gradle"), []byte("// Gradle"), 0644)
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			if err := tt.setup(tmpDir); err != nil {
				t.Fatalf("Setup failed: %v", err)
			}

			e := NewGradleExtractor()
			result := e.Detect(tmpDir)

			if result != tt.expected {
				t.Errorf("Detect() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestGradleExtractBasicGroovy tests basic Gradle Groovy DSL extraction
func TestGradleExtractBasicGroovy(t *testing.T) {
	buildGradle := `
group 'com.example'
version '1.2.3'
description 'A sample Gradle project'

plugins {
    id 'java'
    id 'application'
}
`

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "build.gradle"), []byte(buildGradle), 0644); err != nil {
		t.Fatalf("Failed to write build.gradle: %v", err)
	}

	e := NewGradleExtractor()
	metadata, err := e.Extract(tmpDir)

	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	if metadata.Version != "1.2.3" {
		t.Errorf("Version = %v, want 1.2.3", metadata.Version)
	}

	if metadata.Description != "A sample Gradle project" {
		t.Errorf("Description = %v, want 'A sample Gradle project'", metadata.Description)
	}

	if groupID, ok := metadata.LanguageSpecific["group_id"].(string); !ok || groupID != "com.example" {
		t.Errorf("group_id = %v, want com.example", groupID)
	}

	if buildDSL, ok := metadata.LanguageSpecific["build_dsl"].(string); !ok || buildDSL != "groovy" {
		t.Errorf("build_dsl = %v, want groovy", buildDSL)
	}
}

// TestGradleExtractBasicKotlin tests basic Gradle Kotlin DSL extraction
func TestGradleExtractBasicKotlin(t *testing.T) {
	buildGradleKts := `
group = "com.example"
version = "2.0.0"
description = "Kotlin DSL project"

plugins {
    id("java")
    id("application")
}
`

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "build.gradle.kts"), []byte(buildGradleKts), 0644); err != nil {
		t.Fatalf("Failed to write build.gradle.kts: %v", err)
	}

	e := NewGradleExtractor()
	metadata, err := e.Extract(tmpDir)

	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	if metadata.Version != "2.0.0" {
		t.Errorf("Version = %v, want 2.0.0", metadata.Version)
	}

	if metadata.Description != "Kotlin DSL project" {
		t.Errorf("Description = %v, want 'Kotlin DSL project'", metadata.Description)
	}

	if groupID, ok := metadata.LanguageSpecific["group_id"].(string); !ok || groupID != "com.example" {
		t.Errorf("group_id = %v, want com.example", groupID)
	}

	if buildDSL, ok := metadata.LanguageSpecific["build_dsl"].(string); !ok || buildDSL != "kotlin" {
		t.Errorf("build_dsl = %v, want kotlin", buildDSL)
	}
}

// TestGradleExtractDependenciesGroovy tests Gradle dependency extraction (Groovy)
func TestGradleExtractDependenciesGroovy(t *testing.T) {
	buildGradle := `
group 'com.example'
version '1.0.0'

dependencies {
    implementation 'org.springframework.boot:spring-boot-starter-web:3.2.0'
    testImplementation 'org.junit.jupiter:junit-jupiter:5.10.0'
    compileOnly 'org.projectlombok:lombok:1.18.30'
    runtimeOnly 'com.h2database:h2:2.2.224'
}
`

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "build.gradle"), []byte(buildGradle), 0644); err != nil {
		t.Fatalf("Failed to write build.gradle: %v", err)
	}

	e := NewGradleExtractor()
	metadata, err := e.Extract(tmpDir)

	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	if depCount, ok := metadata.LanguageSpecific["dependency_count"].(int); !ok || depCount != 4 {
		t.Errorf("dependency_count = %v, want 4", depCount)
	}

	deps, ok := metadata.LanguageSpecific["dependencies"].([]map[string]string)
	if !ok {
		t.Fatalf("dependencies not found or wrong type")
	}

	if len(deps) != 4 {
		t.Errorf("len(dependencies) = %v, want 4", len(deps))
	}

	// Check configuration categorization
	configCounts, ok := metadata.LanguageSpecific["dependency_configurations"].(map[string]int)
	if !ok {
		t.Fatalf("dependency_configurations not found or wrong type")
	}

	if configCounts["implementation"] != 1 {
		t.Errorf("implementation count = %v, want 1", configCounts["implementation"])
	}

	if configCounts["testImplementation"] != 1 {
		t.Errorf("testImplementation count = %v, want 1", configCounts["testImplementation"])
	}
}

// TestGradleExtractDependenciesKotlin tests Gradle dependency extraction (Kotlin)
func TestGradleExtractDependenciesKotlin(t *testing.T) {
	buildGradleKts := `
group = "com.example"
version = "1.0.0"

dependencies {
    implementation("org.springframework.boot:spring-boot-starter-web:3.2.0")
    testImplementation("org.junit.jupiter:junit-jupiter:5.10.0")
    api("com.google.guava:guava:32.1.3-jre")
}
`

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "build.gradle.kts"), []byte(buildGradleKts), 0644); err != nil {
		t.Fatalf("Failed to write build.gradle.kts: %v", err)
	}

	e := NewGradleExtractor()
	metadata, err := e.Extract(tmpDir)

	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	if depCount, ok := metadata.LanguageSpecific["dependency_count"].(int); !ok || depCount != 3 {
		t.Errorf("dependency_count = %v, want 3", depCount)
	}

	deps, ok := metadata.LanguageSpecific["dependencies"].([]map[string]string)
	if !ok {
		t.Fatalf("dependencies not found or wrong type")
	}

	// Verify specific dependency
	foundGuava := false
	for _, dep := range deps {
		if dep["name"] == "guava" && dep["group"] == "com.google.guava" {
			foundGuava = true
			if dep["configuration"] != "api" {
				t.Errorf("Guava configuration = %v, want api", dep["configuration"])
			}
		}
	}

	if !foundGuava {
		t.Error("Guava dependency not found")
	}
}

// TestGradleExtractPluginsGroovy tests plugin extraction (Groovy)
func TestGradleExtractPluginsGroovy(t *testing.T) {
	buildGradle := `
plugins {
    id 'java'
    id 'org.springframework.boot' version '3.2.0'
    id 'io.spring.dependency-management' version '1.1.4'
}
`

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "build.gradle"), []byte(buildGradle), 0644); err != nil {
		t.Fatalf("Failed to write build.gradle: %v", err)
	}

	e := NewGradleExtractor()
	metadata, err := e.Extract(tmpDir)

	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	plugins, ok := metadata.LanguageSpecific["plugins"].([]string)
	if !ok {
		t.Fatalf("plugins not found or wrong type")
	}

	// Plugin extraction may include all plugins found
	if len(plugins) < 3 {
		t.Errorf("len(plugins) = %v, want at least 3", len(plugins))
	}

	if pluginCount, ok := metadata.LanguageSpecific["plugin_count"].(int); !ok || pluginCount < 3 {
		t.Errorf("plugin_count = %v, want at least 3", pluginCount)
	}

	// Check framework detection
	frameworks, ok := metadata.LanguageSpecific["frameworks"].([]string)
	if !ok {
		t.Fatalf("frameworks not found or wrong type")
	}

	hasSpringBoot := false
	for _, fw := range frameworks {
		if fw == "Spring Boot" {
			hasSpringBoot = true
			break
		}
	}

	if !hasSpringBoot {
		t.Errorf("Spring Boot framework not detected in %v", frameworks)
	}
}

// TestGradleExtractPluginsKotlin tests plugin extraction (Kotlin)
func TestGradleExtractPluginsKotlin(t *testing.T) {
	buildGradleKts := `
plugins {
    id("java")
    id("org.springframework.boot") version "3.2.0"
    kotlin("jvm") version "1.9.21"
}
`

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "build.gradle.kts"), []byte(buildGradleKts), 0644); err != nil {
		t.Fatalf("Failed to write build.gradle.kts: %v", err)
	}

	e := NewGradleExtractor()
	metadata, err := e.Extract(tmpDir)

	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	plugins, ok := metadata.LanguageSpecific["plugins"].([]string)
	if !ok {
		t.Fatalf("plugins not found or wrong type")
	}

	// Check for Kotlin plugin
	hasKotlin := false
	for _, plugin := range plugins {
		if plugin == "org.jetbrains.kotlin.jvm:1.9.21" {
			hasKotlin = true
			break
		}
	}

	if !hasKotlin {
		t.Errorf("Kotlin plugin not found in %v", plugins)
	}

	// Check framework detection
	frameworks, ok := metadata.LanguageSpecific["frameworks"].([]string)
	if !ok {
		t.Fatalf("frameworks not found or wrong type")
	}

	hasKotlinFramework := false
	for _, fw := range frameworks {
		if fw == "Kotlin" {
			hasKotlinFramework = true
			break
		}
	}

	if !hasKotlinFramework {
		t.Errorf("Kotlin framework not detected in %v", frameworks)
	}
}

// TestGradleExtractWithSettings tests settings.gradle parsing
func TestGradleExtractWithSettings(t *testing.T) {
	buildGradle := `
group 'com.example'
version '1.0.0'
`

	settingsGradle := `
rootProject.name = 'my-awesome-project'
`

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "build.gradle"), []byte(buildGradle), 0644); err != nil {
		t.Fatalf("Failed to write build.gradle: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "settings.gradle"), []byte(settingsGradle), 0644); err != nil {
		t.Fatalf("Failed to write settings.gradle: %v", err)
	}

	e := NewGradleExtractor()
	metadata, err := e.Extract(tmpDir)

	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	if metadata.Name != "my-awesome-project" {
		t.Errorf("Name = %v, want my-awesome-project", metadata.Name)
	}
}

// TestGradleExtractWithSettingsKotlin tests settings.gradle.kts parsing
func TestGradleExtractWithSettingsKotlin(t *testing.T) {
	buildGradleKts := `
group = "com.example"
version = "1.0.0"
`

	settingsGradleKts := `
rootProject.name = "kotlin-project"
`

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "build.gradle.kts"), []byte(buildGradleKts), 0644); err != nil {
		t.Fatalf("Failed to write build.gradle.kts: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "settings.gradle.kts"), []byte(settingsGradleKts), 0644); err != nil {
		t.Fatalf("Failed to write settings.gradle.kts: %v", err)
	}

	e := NewGradleExtractor()
	metadata, err := e.Extract(tmpDir)

	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	if metadata.Name != "kotlin-project" {
		t.Errorf("Name = %v, want kotlin-project", metadata.Name)
	}
}

// TestGradleExtractMultiProject tests multi-project Gradle build
func TestGradleExtractMultiProject(t *testing.T) {
	buildGradle := `
group 'com.example'
version '1.0.0'
`

	settingsGradle := `
rootProject.name = 'parent-project'

include 'module-a'
include 'module-b'
include 'module-c'
`

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "build.gradle"), []byte(buildGradle), 0644); err != nil {
		t.Fatalf("Failed to write build.gradle: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "settings.gradle"), []byte(settingsGradle), 0644); err != nil {
		t.Fatalf("Failed to write settings.gradle: %v", err)
	}

	e := NewGradleExtractor()
	metadata, err := e.Extract(tmpDir)

	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	if isMultiProject, ok := metadata.LanguageSpecific["is_multi_project"].(bool); !ok || !isMultiProject {
		t.Errorf("is_multi_project = %v, want true", isMultiProject)
	}

	subprojects, ok := metadata.LanguageSpecific["subprojects"].([]string)
	if !ok {
		t.Fatalf("subprojects not found or wrong type")
	}

	if len(subprojects) != 3 {
		t.Errorf("len(subprojects) = %v, want 3", len(subprojects))
	}

	if subprojectCount, ok := metadata.LanguageSpecific["subproject_count"].(int); !ok || subprojectCount != 3 {
		t.Errorf("subproject_count = %v, want 3", subprojectCount)
	}
}

// TestGradleExtractProperties tests gradle.properties parsing
func TestGradleExtractProperties(t *testing.T) {
	buildGradle := `
group 'com.example'
version '1.0.0'
`

	gradleProperties := `
# Project properties
version=2.0.0
group=org.example.overridden

# Build properties
java.version=17
sourceCompatibility=17

# Custom properties
app.name=MyApp
`

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "build.gradle"), []byte(buildGradle), 0644); err != nil {
		t.Fatalf("Failed to write build.gradle: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "gradle.properties"), []byte(gradleProperties), 0644); err != nil {
		t.Fatalf("Failed to write gradle.properties: %v", err)
	}

	e := NewGradleExtractor()
	metadata, err := e.Extract(tmpDir)

	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	props, ok := metadata.LanguageSpecific["properties"].(map[string]string)
	if !ok {
		t.Fatalf("properties not found or wrong type")
	}

	// Properties file values are loaded but may not override build.gradle
	// Check that properties were parsed
	if len(props) == 0 {
		t.Error("Properties should be loaded from gradle.properties")
	}

	if props["java.version"] != "17" {
		t.Errorf("java.version property = %v, want 17", props["java.version"])
	}

	if javaVersion, ok := metadata.LanguageSpecific["java_version"].(string); !ok || javaVersion != "17" {
		t.Errorf("java_version = %v, want 17", javaVersion)
	}
}

// TestGradleExtractDynamicVersion tests dynamic version detection
func TestGradleExtractDynamicVersion(t *testing.T) {
	tests := []struct {
		name            string
		buildContent    string
		expectedDynamic bool
	}{
		{
			name: "SNAPSHOT version",
			buildContent: `
group 'com.example'
version '1.0.0-SNAPSHOT'
`,
			expectedDynamic: true,
		},
		{
			name: "project.version reference",
			buildContent: `
group 'com.example'
version '${project.version}'
`,
			expectedDynamic: true, // project.version is a dynamic reference
		},
		{
			name: "static version",
			buildContent: `
group 'com.example'
version '1.2.3'
`,
			expectedDynamic: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			if err := os.WriteFile(filepath.Join(tmpDir, "build.gradle"), []byte(tt.buildContent), 0644); err != nil {
				t.Fatalf("Failed to write build.gradle: %v", err)
			}

			e := NewGradleExtractor()
			metadata, err := e.Extract(tmpDir)

			if err != nil {
				t.Fatalf("Extract() error = %v", err)
			}

			versioningType, ok := metadata.LanguageSpecific["versioning_type"].(string)
			if tt.expectedDynamic {
				if !ok || versioningType != "dynamic" {
					t.Errorf("versioning_type = %v, want 'dynamic'", versioningType)
				}
			} else {
				if ok && versioningType == "dynamic" {
					t.Errorf("versioning_type = %v, want 'static' or unset", versioningType)
				}
			}
		})
	}
}

// TestGradleFrameworkDetection tests framework detection from plugins
func TestGradleFrameworkDetection(t *testing.T) {
	buildGradle := `
plugins {
    id 'java'
    id 'org.springframework.boot' version '3.2.0'
    id 'io.quarkus' version '3.6.0'
    id 'application'
    id 'com.android.application' version '8.2.0'
}
`

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "build.gradle"), []byte(buildGradle), 0644); err != nil {
		t.Fatalf("Failed to write build.gradle: %v", err)
	}

	e := NewGradleExtractor()
	metadata, err := e.Extract(tmpDir)

	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	frameworks, ok := metadata.LanguageSpecific["frameworks"].([]string)
	if !ok {
		t.Fatalf("frameworks not found or wrong type")
	}

	expectedFrameworks := map[string]bool{
		"Spring Boot":      false,
		"Quarkus":          false,
		"Java":             false,
		"Java Application": false,
		"Android":          false,
	}

	for _, fw := range frameworks {
		if _, exists := expectedFrameworks[fw]; exists {
			expectedFrameworks[fw] = true
		}
	}

	for fw, found := range expectedFrameworks {
		if !found {
			t.Errorf("Framework %v not detected", fw)
		}
	}
}

// TestGradleExtractAndroidProject tests Android project detection
func TestGradleExtractAndroidProject(t *testing.T) {
	buildGradle := `
plugins {
    id 'com.android.library'
}

android {
    compileSdk 34
}
`

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "build.gradle"), []byte(buildGradle), 0644); err != nil {
		t.Fatalf("Failed to write build.gradle: %v", err)
	}

	e := NewGradleExtractor()
	metadata, err := e.Extract(tmpDir)

	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	frameworks, ok := metadata.LanguageSpecific["frameworks"].([]string)
	if !ok {
		t.Fatalf("frameworks not found or wrong type")
	}

	hasAndroidLibrary := false
	for _, fw := range frameworks {
		if fw == "Android Library" {
			hasAndroidLibrary = true
			break
		}
	}

	if !hasAndroidLibrary {
		t.Errorf("Android Library framework not detected in %v", frameworks)
	}
}

// TestGradleExtractMicronautProject tests Micronaut project detection
func TestGradleExtractMicronautProject(t *testing.T) {
	buildGradleKts := `
plugins {
    id("io.micronaut.application") version "4.2.0"
}
`

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "build.gradle.kts"), []byte(buildGradleKts), 0644); err != nil {
		t.Fatalf("Failed to write build.gradle.kts: %v", err)
	}

	e := NewGradleExtractor()
	metadata, err := e.Extract(tmpDir)

	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	frameworks, ok := metadata.LanguageSpecific["frameworks"].([]string)
	if !ok {
		t.Fatalf("frameworks not found or wrong type")
	}

	hasMicronaut := false
	for _, fw := range frameworks {
		if fw == "Micronaut" {
			hasMicronaut = true
			break
		}
	}

	if !hasMicronaut {
		t.Errorf("Micronaut framework not detected in %v", frameworks)
	}
}

// TestGradleExtractorName tests extractor name
func TestGradleExtractorName(t *testing.T) {
	e := NewGradleExtractor()
	if e.Name() != "java-gradle" {
		t.Errorf("Name() = %v, want java-gradle", e.Name())
	}
}

// TestGradleExtractorPriority tests extractor priority
func TestGradleExtractorPriority(t *testing.T) {
	e := NewGradleExtractor()
	if e.Priority() != 4 {
		t.Errorf("Priority() = %v, want 4", e.Priority())
	}
}

// TestGradleExtractNoBuildFile tests error handling when build file is missing
func TestGradleExtractNoBuildFile(t *testing.T) {
	tmpDir := t.TempDir()

	e := NewGradleExtractor()
	_, err := e.Extract(tmpDir)

	if err == nil {
		t.Error("Extract() should return error when build file is missing")
	}
}

// TestGradleExtractEmptyBuildFile tests extraction with minimal build file
func TestGradleExtractEmptyBuildFile(t *testing.T) {
	buildGradle := `
// Minimal build file
plugins {
    id 'java'
}
`

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "build.gradle"), []byte(buildGradle), 0644); err != nil {
		t.Fatalf("Failed to write build.gradle: %v", err)
	}

	e := NewGradleExtractor()
	metadata, err := e.Extract(tmpDir)

	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	// Should not fail, just have minimal data
	if metadata == nil {
		t.Error("metadata should not be nil")
	}
}

// TestGradleExtractJavaLibrary tests Java library project
func TestGradleExtractJavaLibrary(t *testing.T) {
	buildGradle := `
plugins {
    id 'java-library'
}

group = 'com.example.lib'
version = '1.0.0'
`

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "build.gradle"), []byte(buildGradle), 0644); err != nil {
		t.Fatalf("Failed to write build.gradle: %v", err)
	}

	e := NewGradleExtractor()
	metadata, err := e.Extract(tmpDir)

	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	frameworks, ok := metadata.LanguageSpecific["frameworks"].([]string)
	if !ok {
		t.Fatalf("frameworks not found or wrong type")
	}

	hasJavaLibrary := false
	for _, fw := range frameworks {
		if fw == "Java Library" {
			hasJavaLibrary = true
			break
		}
	}

	if !hasJavaLibrary {
		t.Errorf("Java Library framework not detected in %v", frameworks)
	}
}
