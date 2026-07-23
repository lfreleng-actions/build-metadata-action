// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package java

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestMavenDetect tests Maven project detection
func TestMavenDetect(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(dir string) error
		expected bool
	}{
		{
			name: "valid pom.xml",
			setup: func(dir string) error {
				return os.WriteFile(filepath.Join(dir, "pom.xml"), []byte(`<?xml version="1.0"?><project></project>`), 0644)
			},
			expected: true,
		},
		{
			name: "no pom.xml",
			setup: func(dir string) error {
				return nil
			},
			expected: false,
		},
		{
			name: "pom.xml in subdirectory should not detect at root",
			setup: func(dir string) error {
				subdir := filepath.Join(dir, "subproject")
				if err := os.MkdirAll(subdir, 0755); err != nil {
					return err
				}
				return os.WriteFile(filepath.Join(subdir, "pom.xml"), []byte(`<?xml version="1.0"?><project></project>`), 0644)
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

			e := NewMavenExtractor()
			result := e.Detect(tmpDir)

			if result != tt.expected {
				t.Errorf("Detect() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestMavenExtractBasic tests basic Maven metadata extraction
func TestMavenExtractBasic(t *testing.T) {
	pomXML := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0"
         xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
         xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 http://maven.apache.org/xsd/maven-4.0.0.xsd">
    <modelVersion>4.0.0</modelVersion>

    <groupId>com.example</groupId>
    <artifactId>my-app</artifactId>
    <version>1.2.3</version>
    <packaging>jar</packaging>

    <name>My Application</name>
    <description>A sample Maven project</description>
    <url>https://example.com</url>
</project>`

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "pom.xml"), []byte(pomXML), 0644); err != nil {
		t.Fatalf("Failed to write pom.xml: %v", err)
	}

	e := NewMavenExtractor()
	metadata, err := e.Extract(tmpDir)

	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	if metadata.Name != "My Application" {
		t.Errorf("Name = %v, want %v", metadata.Name, "My Application")
	}

	if metadata.Version != "1.2.3" {
		t.Errorf("Version = %v, want %v", metadata.Version, "1.2.3")
	}

	if metadata.Description != "A sample Maven project" {
		t.Errorf("Description = %v, want %v", metadata.Description, "A sample Maven project")
	}

	if metadata.Homepage != "https://example.com" {
		t.Errorf("Homepage = %v, want %v", metadata.Homepage, "https://example.com")
	}

	if groupID, ok := metadata.LanguageSpecific["group_id"].(string); !ok || groupID != "com.example" {
		t.Errorf("group_id = %v, want %v", groupID, "com.example")
	}

	if artifactID, ok := metadata.LanguageSpecific["artifact_id"].(string); !ok || artifactID != "my-app" {
		t.Errorf("artifact_id = %v, want %v", artifactID, "my-app")
	}

	if packaging, ok := metadata.LanguageSpecific["packaging"].(string); !ok || packaging != "jar" {
		t.Errorf("packaging = %v, want %v", packaging, "jar")
	}
}

// TestMavenExtractWithParent tests Maven project with parent POM
func TestMavenExtractWithParent(t *testing.T) {
	pomXML := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>

    <parent>
        <groupId>org.springframework.boot</groupId>
        <artifactId>spring-boot-starter-parent</artifactId>
        <version>3.2.0</version>
    </parent>

    <artifactId>my-spring-app</artifactId>
    <version>0.1.0-SNAPSHOT</version>
    <name>Spring Boot Application</name>
</project>`

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "pom.xml"), []byte(pomXML), 0644); err != nil {
		t.Fatalf("Failed to write pom.xml: %v", err)
	}

	e := NewMavenExtractor()
	metadata, err := e.Extract(tmpDir)

	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	if hasParent, ok := metadata.LanguageSpecific["has_parent"].(bool); !ok || !hasParent {
		t.Errorf("has_parent = %v, want true", hasParent)
	}

	if parentGroupID, ok := metadata.LanguageSpecific["parent_group_id"].(string); !ok || parentGroupID != "org.springframework.boot" {
		t.Errorf("parent_group_id = %v, want org.springframework.boot", parentGroupID)
	}

	if parentArtifactID, ok := metadata.LanguageSpecific["parent_artifact_id"].(string); !ok || parentArtifactID != "spring-boot-starter-parent" {
		t.Errorf("parent_artifact_id = %v, want spring-boot-starter-parent", parentArtifactID)
	}

	// groupId should be inherited from parent
	if groupID, ok := metadata.LanguageSpecific["group_id"].(string); !ok || groupID != "org.springframework.boot" {
		t.Errorf("group_id = %v, want org.springframework.boot (inherited)", groupID)
	}
}

// TestMavenExtractDependencies tests Maven dependency extraction
func TestMavenExtractDependencies(t *testing.T) {
	pomXML := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>

    <groupId>com.example</groupId>
    <artifactId>test-app</artifactId>
    <version>1.0.0</version>

    <dependencies>
        <dependency>
            <groupId>org.springframework.boot</groupId>
            <artifactId>spring-boot-starter-web</artifactId>
            <version>3.2.0</version>
        </dependency>
        <dependency>
            <groupId>junit</groupId>
            <artifactId>junit</artifactId>
            <version>4.13.2</version>
            <scope>test</scope>
        </dependency>
        <dependency>
            <groupId>org.projectlombok</groupId>
            <artifactId>lombok</artifactId>
            <version>1.18.30</version>
            <scope>provided</scope>
        </dependency>
    </dependencies>
</project>`

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "pom.xml"), []byte(pomXML), 0644); err != nil {
		t.Fatalf("Failed to write pom.xml: %v", err)
	}

	e := NewMavenExtractor()
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

	if len(deps) != 3 {
		t.Errorf("len(dependencies) = %v, want 3", len(deps))
	}

	// Check scope categorization
	scopes, ok := metadata.LanguageSpecific["dependency_scopes"].(map[string]int)
	if !ok {
		t.Fatalf("dependency_scopes not found or wrong type")
	}

	if scopes["compile"] != 1 {
		t.Errorf("compile scope count = %v, want 1", scopes["compile"])
	}

	if scopes["test"] != 1 {
		t.Errorf("test scope count = %v, want 1", scopes["test"])
	}

	if scopes["provided"] != 1 {
		t.Errorf("provided scope count = %v, want 1", scopes["provided"])
	}
}

// TestMavenExtractProperties tests Maven properties extraction
func TestMavenExtractProperties(t *testing.T) {
	pomXML := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>

    <groupId>com.example</groupId>
    <artifactId>test-app</artifactId>
    <version>1.0.0</version>

    <properties>
        <maven.compiler.source>17</maven.compiler.source>
        <maven.compiler.target>17</maven.compiler.target>
        <project.build.sourceEncoding>UTF-8</project.build.sourceEncoding>
        <spring.version>6.1.0</spring.version>
    </properties>
</project>`

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "pom.xml"), []byte(pomXML), 0644); err != nil {
		t.Fatalf("Failed to write pom.xml: %v", err)
	}

	e := NewMavenExtractor()
	metadata, err := e.Extract(tmpDir)

	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	props, ok := metadata.LanguageSpecific["properties"].(map[string]string)
	if !ok {
		t.Fatalf("properties not found or wrong type")
	}

	if props["maven.compiler.source"] != "17" {
		t.Errorf("maven.compiler.source = %v, want 17", props["maven.compiler.source"])
	}

	if javaVersion, ok := metadata.LanguageSpecific["version"].(string); !ok || javaVersion != "17" {
		t.Errorf("java_version = %v, want 17", javaVersion)
	}

	if propCount, ok := metadata.LanguageSpecific["property_count"].(int); !ok || propCount != 4 {
		t.Errorf("property_count = %v, want 4", propCount)
	}
}

// TestMavenExtractBuildPlugins tests Maven build plugin extraction
func TestMavenExtractBuildPlugins(t *testing.T) {
	pomXML := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>

    <groupId>com.example</groupId>
    <artifactId>test-app</artifactId>
    <version>1.0.0</version>

    <build>
        <plugins>
            <plugin>
                <groupId>org.apache.maven.plugins</groupId>
                <artifactId>maven-compiler-plugin</artifactId>
                <version>3.11.0</version>
            </plugin>
            <plugin>
                <groupId>org.springframework.boot</groupId>
                <artifactId>spring-boot-maven-plugin</artifactId>
                <version>3.2.0</version>
            </plugin>
            <plugin>
                <groupId>org.apache.maven.plugins</groupId>
                <artifactId>maven-surefire-plugin</artifactId>
                <version>3.0.0</version>
            </plugin>
        </plugins>
    </build>
</project>`

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "pom.xml"), []byte(pomXML), 0644); err != nil {
		t.Fatalf("Failed to write pom.xml: %v", err)
	}

	e := NewMavenExtractor()
	metadata, err := e.Extract(tmpDir)

	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	plugins, ok := metadata.LanguageSpecific["build_plugins"].([]string)
	if !ok {
		t.Fatalf("build_plugins not found or wrong type")
	}

	if len(plugins) != 3 {
		t.Errorf("len(build_plugins) = %v, want 3", len(plugins))
	}

	if pluginCount, ok := metadata.LanguageSpecific["plugin_count"].(int); !ok || pluginCount != 3 {
		t.Errorf("plugin_count = %v, want 3", pluginCount)
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

// TestMavenExtractMultiModule tests multi-module Maven project
func TestMavenExtractMultiModule(t *testing.T) {
	pomXML := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>

    <groupId>com.example</groupId>
    <artifactId>parent-project</artifactId>
    <version>1.0.0</version>
    <packaging>pom</packaging>

    <modules>
        <module>module-a</module>
        <module>module-b</module>
        <module>module-c</module>
    </modules>
</project>`

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "pom.xml"), []byte(pomXML), 0644); err != nil {
		t.Fatalf("Failed to write pom.xml: %v", err)
	}

	e := NewMavenExtractor()
	metadata, err := e.Extract(tmpDir)

	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	if isMultiModule, ok := metadata.LanguageSpecific["is_multi_module"].(bool); !ok || !isMultiModule {
		t.Errorf("is_multi_module = %v, want true", isMultiModule)
	}

	modules, ok := metadata.LanguageSpecific["modules"].([]string)
	if !ok {
		t.Fatalf("modules not found or wrong type")
	}

	if len(modules) != 3 {
		t.Errorf("len(modules) = %v, want 3", len(modules))
	}

	if moduleCount, ok := metadata.LanguageSpecific["module_count"].(int); !ok || moduleCount != 3 {
		t.Errorf("module_count = %v, want 3", moduleCount)
	}

	if packaging, ok := metadata.LanguageSpecific["packaging"].(string); !ok || packaging != "pom" {
		t.Errorf("packaging = %v, want pom", packaging)
	}
}

// TestMavenExtractLicenses tests license extraction
func TestMavenExtractLicenses(t *testing.T) {
	pomXML := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>

    <groupId>com.example</groupId>
    <artifactId>test-app</artifactId>
    <version>1.0.0</version>

    <licenses>
        <license>
            <name>Apache License 2.0</name>
            <url>https://www.apache.org/licenses/LICENSE-2.0</url>
            <distribution>repo</distribution>
        </license>
    </licenses>
</project>`

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "pom.xml"), []byte(pomXML), 0644); err != nil {
		t.Fatalf("Failed to write pom.xml: %v", err)
	}

	e := NewMavenExtractor()
	metadata, err := e.Extract(tmpDir)

	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	if metadata.License != "Apache License 2.0" {
		t.Errorf("License = %v, want Apache License 2.0", metadata.License)
	}
}

// TestMavenExtractDevelopers tests developer/author extraction
func TestMavenExtractDevelopers(t *testing.T) {
	pomXML := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>

    <groupId>com.example</groupId>
    <artifactId>test-app</artifactId>
    <version>1.0.0</version>

    <developers>
        <developer>
            <name>John Doe</name>
            <email>john@example.com</email>
        </developer>
        <developer>
            <name>Jane Smith</name>
            <email>jane@example.com</email>
        </developer>
    </developers>
</project>`

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "pom.xml"), []byte(pomXML), 0644); err != nil {
		t.Fatalf("Failed to write pom.xml: %v", err)
	}

	e := NewMavenExtractor()
	metadata, err := e.Extract(tmpDir)

	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	if len(metadata.Authors) != 2 {
		t.Errorf("len(Authors) = %v, want 2", len(metadata.Authors))
	}

	expectedAuthors := []string{"John Doe <john@example.com>", "Jane Smith <jane@example.com>"}
	for i, expected := range expectedAuthors {
		if i >= len(metadata.Authors) || metadata.Authors[i] != expected {
			t.Errorf("Authors[%d] = %v, want %v", i, metadata.Authors[i], expected)
		}
	}
}

// TestMavenExtractSCM tests SCM/repository extraction
func TestMavenExtractSCM(t *testing.T) {
	pomXML := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>

    <groupId>com.example</groupId>
    <artifactId>test-app</artifactId>
    <version>1.0.0</version>

    <scm>
        <connection>scm:git:https://github.com/example/test-app.git</connection>
        <url>https://github.com/example/test-app</url>
        <tag>HEAD</tag>
    </scm>
</project>`

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "pom.xml"), []byte(pomXML), 0644); err != nil {
		t.Fatalf("Failed to write pom.xml: %v", err)
	}

	e := NewMavenExtractor()
	metadata, err := e.Extract(tmpDir)

	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	if metadata.Repository != "https://github.com/example/test-app" {
		t.Errorf("Repository = %v, want https://github.com/example/test-app", metadata.Repository)
	}
}

// TestMavenExtractOrganization tests organization extraction
func TestMavenExtractOrganization(t *testing.T) {
	pomXML := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>

    <groupId>com.example</groupId>
    <artifactId>test-app</artifactId>
    <version>1.0.0</version>

    <organization>
        <name>Example Corporation</name>
        <url>https://example.com</url>
    </organization>
</project>`

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "pom.xml"), []byte(pomXML), 0644); err != nil {
		t.Fatalf("Failed to write pom.xml: %v", err)
	}

	e := NewMavenExtractor()
	metadata, err := e.Extract(tmpDir)

	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	if org, ok := metadata.LanguageSpecific["organization"].(string); !ok || org != "Example Corporation" {
		t.Errorf("organization = %v, want Example Corporation", org)
	}

	if orgURL, ok := metadata.LanguageSpecific["organization_url"].(string); !ok || orgURL != "https://example.com" {
		t.Errorf("organization_url = %v, want https://example.com", orgURL)
	}
}

// TestMavenExtractProfiles tests Maven profiles extraction
func TestMavenExtractProfiles(t *testing.T) {
	pomXML := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>

    <groupId>com.example</groupId>
    <artifactId>test-app</artifactId>
    <version>1.0.0</version>

    <profiles>
        <profile>
            <id>dev</id>
        </profile>
        <profile>
            <id>prod</id>
        </profile>
        <profile>
            <id>test</id>
        </profile>
    </profiles>
</project>`

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "pom.xml"), []byte(pomXML), 0644); err != nil {
		t.Fatalf("Failed to write pom.xml: %v", err)
	}

	e := NewMavenExtractor()
	metadata, err := e.Extract(tmpDir)

	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	profiles, ok := metadata.LanguageSpecific["profiles"].([]string)
	if !ok {
		t.Fatalf("profiles not found or wrong type")
	}

	if len(profiles) != 3 {
		t.Errorf("len(profiles) = %v, want 3", len(profiles))
	}

	if profileCount, ok := metadata.LanguageSpecific["profile_count"].(int); !ok || profileCount != 3 {
		t.Errorf("profile_count = %v, want 3", profileCount)
	}
}

// TestMavenExtractDynamicVersion tests dynamic version detection
func TestMavenExtractDynamicVersion(t *testing.T) {
	pomXML := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>

    <groupId>com.example</groupId>
    <artifactId>test-app</artifactId>
    <version>${revision}</version>

    <properties>
        <revision>1.0.0-SNAPSHOT</revision>
    </properties>
</project>`

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "pom.xml"), []byte(pomXML), 0644); err != nil {
		t.Fatalf("Failed to write pom.xml: %v", err)
	}

	e := NewMavenExtractor()
	metadata, err := e.Extract(tmpDir)

	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	if versioningType, ok := metadata.LanguageSpecific["versioning_type"].(string); !ok || versioningType != "dynamic" {
		t.Errorf("versioning_type = %v, want 'dynamic'", versioningType)
	}

	if versionProp, ok := metadata.LanguageSpecific["version_property"].(string); !ok || versionProp != "revision" {
		t.Errorf("version_property = %v, want revision", versionProp)
	}

	// Version should be resolved
	if metadata.Version != "1.0.0-SNAPSHOT" {
		t.Errorf("Version = %v, want 1.0.0-SNAPSHOT", metadata.Version)
	}
}

// TestMavenExtractDefaultPackaging tests default packaging type
func TestMavenExtractDefaultPackaging(t *testing.T) {
	pomXML := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>

    <groupId>com.example</groupId>
    <artifactId>test-app</artifactId>
    <version>1.0.0</version>
</project>`

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "pom.xml"), []byte(pomXML), 0644); err != nil {
		t.Fatalf("Failed to write pom.xml: %v", err)
	}

	e := NewMavenExtractor()
	metadata, err := e.Extract(tmpDir)

	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	// Default packaging should be jar
	if packaging, ok := metadata.LanguageSpecific["packaging"].(string); !ok || packaging != "jar" {
		t.Errorf("packaging = %v, want jar (default)", packaging)
	}
}

// TestMavenFrameworkDetection tests framework detection from dependencies
func TestMavenFrameworkDetection(t *testing.T) {
	pomXML := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>

    <groupId>com.example</groupId>
    <artifactId>test-app</artifactId>
    <version>1.0.0</version>

    <dependencies>
        <dependency>
            <groupId>org.springframework.boot</groupId>
            <artifactId>spring-boot-starter</artifactId>
            <version>3.2.0</version>
        </dependency>
        <dependency>
            <groupId>org.junit.jupiter</groupId>
            <artifactId>junit-jupiter</artifactId>
            <version>5.10.0</version>
            <scope>test</scope>
        </dependency>
        <dependency>
            <groupId>org.hibernate</groupId>
            <artifactId>hibernate-core</artifactId>
            <version>6.3.0</version>
        </dependency>
    </dependencies>

    <build>
        <plugins>
            <plugin>
                <groupId>org.apache.maven.plugins</groupId>
                <artifactId>maven-surefire-plugin</artifactId>
                <version>3.0.0</version>
            </plugin>
        </plugins>
    </build>
</project>`

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "pom.xml"), []byte(pomXML), 0644); err != nil {
		t.Fatalf("Failed to write pom.xml: %v", err)
	}

	e := NewMavenExtractor()
	metadata, err := e.Extract(tmpDir)

	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	frameworks, ok := metadata.LanguageSpecific["frameworks"].([]string)
	if !ok {
		t.Fatalf("frameworks not found or wrong type")
	}

	expectedFrameworks := map[string]bool{
		"Spring Boot":    false,
		"JUnit":          false,
		"Hibernate":      false,
		"Maven Surefire": false,
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

// TestMavenExtractWithJavaVersion tests Java version extraction from properties
func TestMavenExtractWithJavaVersion(t *testing.T) {
	tests := []struct {
		name         string
		properties   string
		expectedJava string
	}{
		{
			name: "maven.compiler.source",
			properties: `<properties>
                <maven.compiler.source>21</maven.compiler.source>
                <maven.compiler.target>21</maven.compiler.target>
            </properties>`,
			expectedJava: "21",
		},
		{
			name: "maven.compiler.release preferred over source",
			properties: `<properties>
                <maven.compiler.release>21</maven.compiler.release>
                <maven.compiler.source>17</maven.compiler.source>
            </properties>`,
			expectedJava: "21",
		},
		{
			name: "maven.compiler.target preferred over source when differing",
			properties: `<properties>
                <maven.compiler.source>11</maven.compiler.source>
                <maven.compiler.target>17</maven.compiler.target>
            </properties>`,
			expectedJava: "17",
		},
		{
			name: "maven.compiler.release with placeholder",
			properties: `<properties>
                <java.version>21</java.version>
                <maven.compiler.release>${java.version}</maven.compiler.release>
            </properties>`,
			expectedJava: "21",
		},
		{
			name: "java.version",
			properties: `<properties>
                <java.version>17</java.version>
            </properties>`,
			expectedJava: "17",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pomXML := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>
    <groupId>com.example</groupId>
    <artifactId>test-app</artifactId>
    <version>1.0.0</version>
    ` + tt.properties + `
</project>`

			tmpDir := t.TempDir()
			if err := os.WriteFile(filepath.Join(tmpDir, "pom.xml"), []byte(pomXML), 0644); err != nil {
				t.Fatalf("Failed to write pom.xml: %v", err)
			}

			e := NewMavenExtractor()
			metadata, err := e.Extract(tmpDir)

			if err != nil {
				t.Fatalf("Extract() error = %v", err)
			}

			if javaVersion, ok := metadata.LanguageSpecific["version"].(string); !ok || javaVersion != tt.expectedJava {
				t.Errorf("java_version = %v, want %v", javaVersion, tt.expectedJava)
			}
		})
	}
}

// TestMavenExtractorName tests extractor name
func TestMavenExtractorName(t *testing.T) {
	e := NewMavenExtractor()
	if e.Name() != "java-maven" {
		t.Errorf("Name() = %v, want java-maven", e.Name())
	}
}

// TestMavenExtractorPriority tests extractor priority
func TestMavenExtractorPriority(t *testing.T) {
	e := NewMavenExtractor()
	if e.Priority() != 3 {
		t.Errorf("Priority() = %v, want 3", e.Priority())
	}
}

// TestMavenExtractInvalidPOM tests error handling for invalid POM
func TestMavenExtractInvalidPOM(t *testing.T) {
	invalidPOM := `<?xml version="1.0" encoding="UTF-8"?>
<project>
    <invalid>xml</structure>
</project>`

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "pom.xml"), []byte(invalidPOM), 0644); err != nil {
		t.Fatalf("Failed to write pom.xml: %v", err)
	}

	e := NewMavenExtractor()
	_, err := e.Extract(tmpDir)

	// Should not fail on invalid XML, but may have missing data
	// The extractor should be resilient
	if err != nil {
		// Error is acceptable for truly malformed XML
		t.Logf("Extract() returned error (expected): %v", err)
	}
}

// TestMavenExtractNoPOM tests error handling when pom.xml is missing
func TestMavenExtractNoPOM(t *testing.T) {
	tmpDir := t.TempDir()

	e := NewMavenExtractor()
	_, err := e.Extract(tmpDir)

	if err == nil {
		t.Error("Extract() should return error when pom.xml is missing")
	}
}

// mavenJavaVersionOf extracts and returns the resolved java_version and its
// source for the pom.xml written into dir.
func mavenJavaVersionOf(t *testing.T, dir string) (string, string) {
	t.Helper()
	metadata, err := NewMavenExtractor().Extract(dir)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	version, _ := metadata.LanguageSpecific["version"].(string)
	source, _ := metadata.LanguageSpecific["version_source"].(string)
	return version, source
}

// TestMavenExtractJavaVersionFromCompilerPlugin verifies the Java level is
// read from maven-compiler-plugin <configuration> when no property declares
// it.
func TestMavenExtractJavaVersionFromCompilerPlugin(t *testing.T) {
	pomXML := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>
    <groupId>com.example</groupId>
    <artifactId>plugin-configured</artifactId>
    <version>1.0.0</version>
    <build>
        <plugins>
            <plugin>
                <groupId>org.apache.maven.plugins</groupId>
                <artifactId>maven-compiler-plugin</artifactId>
                <configuration>
                    <release>21</release>
                </configuration>
            </plugin>
        </plugins>
    </build>
</project>`

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "pom.xml"), []byte(pomXML), 0644); err != nil {
		t.Fatalf("Failed to write pom.xml: %v", err)
	}

	version, source := mavenJavaVersionOf(t, tmpDir)
	if version != "21" {
		t.Errorf("java_version = %q, want 21", version)
	}
	if source != "maven-compiler-plugin/release" {
		t.Errorf("java_version_source = %q, want maven-compiler-plugin/release", source)
	}
}

// TestMavenExtractJavaVersionPluginTargetPreferred verifies that when the
// maven-compiler-plugin <configuration> declares both <source> and <target>
// with different levels, the stricter <target> (bytecode) level wins.
func TestMavenExtractJavaVersionPluginTargetPreferred(t *testing.T) {
	pomXML := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>
    <groupId>com.example</groupId>
    <artifactId>plugin-source-target</artifactId>
    <version>1.0.0</version>
    <build>
        <plugins>
            <plugin>
                <groupId>org.apache.maven.plugins</groupId>
                <artifactId>maven-compiler-plugin</artifactId>
                <configuration>
                    <source>11</source>
                    <target>17</target>
                </configuration>
            </plugin>
        </plugins>
    </build>
</project>`

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "pom.xml"), []byte(pomXML), 0644); err != nil {
		t.Fatalf("Failed to write pom.xml: %v", err)
	}

	version, source := mavenJavaVersionOf(t, tmpDir)
	if version != "17" {
		t.Errorf("java_version = %q, want 17 (target preferred over source)", version)
	}
	if source != "maven-compiler-plugin/target" {
		t.Errorf("java_version_source = %q, want maven-compiler-plugin/target", source)
	}
}

// TestMavenExtractJavaVersionFromPluginManagement verifies the Java level is
// read from maven-compiler-plugin <configuration> declared under
// <build><pluginManagement>, the common location for managed defaults.
func TestMavenExtractJavaVersionFromPluginManagement(t *testing.T) {
	pomXML := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>
    <groupId>com.example</groupId>
    <artifactId>managed-compiler</artifactId>
    <version>1.0.0</version>
    <build>
        <pluginManagement>
            <plugins>
                <plugin>
                    <groupId>org.apache.maven.plugins</groupId>
                    <artifactId>maven-compiler-plugin</artifactId>
                    <configuration>
                        <release>17</release>
                    </configuration>
                </plugin>
            </plugins>
        </pluginManagement>
    </build>
</project>`

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "pom.xml"), []byte(pomXML), 0644); err != nil {
		t.Fatalf("Failed to write pom.xml: %v", err)
	}

	version, source := mavenJavaVersionOf(t, tmpDir)
	if version != "17" {
		t.Errorf("java_version = %q, want 17", version)
	}
	if source != "maven-compiler-plugin/release" {
		t.Errorf("java_version_source = %q, want maven-compiler-plugin/release", source)
	}
}

// TestMavenExtractJavaVersionFromParent verifies a module inherits the Java
// level from an on-disk parent POM referenced via relativePath.
func TestMavenExtractJavaVersionFromParent(t *testing.T) {
	tmpDir := t.TempDir()

	parentPOM := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>
    <groupId>com.example</groupId>
    <artifactId>parent</artifactId>
    <version>1.0.0</version>
    <packaging>pom</packaging>
    <properties>
        <maven.compiler.release>21</maven.compiler.release>
    </properties>
</project>`
	if err := os.WriteFile(filepath.Join(tmpDir, "pom.xml"), []byte(parentPOM), 0644); err != nil {
		t.Fatalf("Failed to write parent pom.xml: %v", err)
	}

	childDir := filepath.Join(tmpDir, "child")
	if err := os.Mkdir(childDir, 0755); err != nil {
		t.Fatalf("Failed to create child dir: %v", err)
	}
	childPOM := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>
    <parent>
        <groupId>com.example</groupId>
        <artifactId>parent</artifactId>
        <version>1.0.0</version>
        <relativePath>../pom.xml</relativePath>
    </parent>
    <artifactId>child</artifactId>
</project>`
	if err := os.WriteFile(filepath.Join(childDir, "pom.xml"), []byte(childPOM), 0644); err != nil {
		t.Fatalf("Failed to write child pom.xml: %v", err)
	}

	version, source := mavenJavaVersionOf(t, childDir)
	if version != "21" {
		t.Errorf("java_version = %q, want 21 (inherited from parent)", version)
	}
	if source != "maven.compiler.release" {
		t.Errorf("java_version_source = %q, want maven.compiler.release", source)
	}
}

// TestMavenExtractJavaVersionFromModule verifies an aggregator root that
// declares no compiler level itself resolves it from a reactor module (the
// ONAP layout, where a shared *-parent module carries the level).
func TestMavenExtractJavaVersionFromModule(t *testing.T) {
	tmpDir := t.TempDir()

	rootPOM := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>
    <groupId>org.onap.cps</groupId>
    <artifactId>cps</artifactId>
    <version>3.8.2-SNAPSHOT</version>
    <packaging>pom</packaging>
    <modules>
        <module>cps-parent</module>
    </modules>
</project>`
	if err := os.WriteFile(filepath.Join(tmpDir, "pom.xml"), []byte(rootPOM), 0644); err != nil {
		t.Fatalf("Failed to write root pom.xml: %v", err)
	}

	moduleDir := filepath.Join(tmpDir, "cps-parent")
	if err := os.Mkdir(moduleDir, 0755); err != nil {
		t.Fatalf("Failed to create module dir: %v", err)
	}
	modulePOM := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>
    <groupId>org.onap.cps</groupId>
    <artifactId>cps-parent</artifactId>
    <version>3.8.2-SNAPSHOT</version>
    <packaging>pom</packaging>
    <properties>
        <java.version>21</java.version>
        <maven.compiler.release>21</maven.compiler.release>
    </properties>
</project>`
	if err := os.WriteFile(filepath.Join(moduleDir, "pom.xml"), []byte(modulePOM), 0644); err != nil {
		t.Fatalf("Failed to write module pom.xml: %v", err)
	}

	version, source := mavenJavaVersionOf(t, tmpDir)
	if version != "21" {
		t.Errorf("java_version = %q, want 21 (from reactor module)", version)
	}
	if source != "module:cps-parent" {
		t.Errorf("java_version_source = %q, want module:cps-parent", source)
	}
}

// TestWithinWorkspace verifies the workspace boundary check: it confines
// paths to GITHUB_WORKSPACE when set and disables bounding when unset.
func TestWithinWorkspace(t *testing.T) {
	root := filepath.Join(string(filepath.Separator)+"srv", "work", "repo")

	t.Run("unset disables bounding", func(t *testing.T) {
		t.Setenv("GITHUB_WORKSPACE", "")
		if !withinWorkspace(filepath.Join(root, "..", "escape")) {
			t.Error("withinWorkspace should return true when GITHUB_WORKSPACE is unset")
		}
	})

	t.Run("bounded to workspace", func(t *testing.T) {
		t.Setenv("GITHUB_WORKSPACE", root)
		cases := []struct {
			name string
			path string
			want bool
		}{
			{"root itself", root, true},
			{"descendant", filepath.Join(root, "cps-parent"), true},
			{"nested descendant", filepath.Join(root, "a", "b", "pom.xml"), true},
			{"parent traversal", filepath.Join(root, "..", "escape"), false},
			{"double traversal", filepath.Join(root, "..", "..", "tmp"), false},
			{"prefix sibling", root + "-other", false},
		}
		for _, tc := range cases {
			if got := withinWorkspace(tc.path); got != tc.want {
				t.Errorf("withinWorkspace(%q) = %v, want %v", tc.path, got, tc.want)
			}
		}
	})
}

// TestMavenParentTraversalBoundedToWorkspace verifies a parent relativePath
// whose "../" segments escape GITHUB_WORKSPACE is rejected, so a crafted POM
// cannot read files outside the checkout on a CI runner.
func TestMavenParentTraversalBoundedToWorkspace(t *testing.T) {
	base := t.TempDir()
	workspace := filepath.Join(base, "workspace")
	outside := filepath.Join(base, "outside")
	childDir := filepath.Join(workspace, "child")
	for _, dir := range []string{workspace, outside, childDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create %s: %v", dir, err)
		}
	}

	outsidePOM := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>
    <groupId>com.example</groupId>
    <artifactId>parent</artifactId>
    <version>1.0.0</version>
    <packaging>pom</packaging>
    <properties>
        <maven.compiler.release>21</maven.compiler.release>
    </properties>
</project>`
	if err := os.WriteFile(filepath.Join(outside, "pom.xml"), []byte(outsidePOM), 0644); err != nil {
		t.Fatalf("Failed to write outside pom.xml: %v", err)
	}

	childPOM := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>
    <parent>
        <groupId>com.example</groupId>
        <artifactId>parent</artifactId>
        <version>1.0.0</version>
        <relativePath>../../outside/pom.xml</relativePath>
    </parent>
    <artifactId>child</artifactId>
</project>`
	if err := os.WriteFile(filepath.Join(childDir, "pom.xml"), []byte(childPOM), 0644); err != nil {
		t.Fatalf("Failed to write child pom.xml: %v", err)
	}

	t.Setenv("GITHUB_WORKSPACE", workspace)
	version, _ := mavenJavaVersionOf(t, childDir)
	if version != "" {
		t.Errorf("java_version = %q, want empty (parent outside workspace must be rejected)", version)
	}
}

// TestMavenModuleTraversalBoundedToWorkspace verifies a reactor <module>
// whose "../" segments escape GITHUB_WORKSPACE is skipped rather than read.
func TestMavenModuleTraversalBoundedToWorkspace(t *testing.T) {
	base := t.TempDir()
	workspace := filepath.Join(base, "workspace")
	outside := filepath.Join(base, "outside")
	for _, dir := range []string{workspace, outside} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create %s: %v", dir, err)
		}
	}

	rootPOM := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>
    <groupId>com.example</groupId>
    <artifactId>aggregator</artifactId>
    <version>1.0.0</version>
    <packaging>pom</packaging>
    <modules>
        <module>../outside</module>
    </modules>
</project>`
	if err := os.WriteFile(filepath.Join(workspace, "pom.xml"), []byte(rootPOM), 0644); err != nil {
		t.Fatalf("Failed to write root pom.xml: %v", err)
	}

	outsidePOM := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>
    <groupId>com.example</groupId>
    <artifactId>outside</artifactId>
    <version>1.0.0</version>
    <properties>
        <maven.compiler.release>21</maven.compiler.release>
    </properties>
</project>`
	if err := os.WriteFile(filepath.Join(outside, "pom.xml"), []byte(outsidePOM), 0644); err != nil {
		t.Fatalf("Failed to write outside pom.xml: %v", err)
	}

	t.Setenv("GITHUB_WORKSPACE", workspace)
	version, _ := mavenJavaVersionOf(t, workspace)
	if version != "" {
		t.Errorf("java_version = %q, want empty (module outside workspace must be skipped)", version)
	}
}

// TestReadPOMRejectsSymlink verifies that, when a workspace boundary is
// active, readPOM refuses a symlinked pom.xml that resolves outside the
// workspace (and a symlinked parent directory), while still reading a regular
// in-workspace POM. This guards against symlink escapes that the callers'
// lexical withinWorkspace prefix check cannot detect.
func TestReadPOMRejectsSymlink(t *testing.T) {
	base := t.TempDir()
	workspace := filepath.Join(base, "workspace")
	outside := filepath.Join(base, "outside")
	for _, dir := range []string{workspace, outside} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create %s: %v", dir, err)
		}
	}

	pomXML := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>
    <groupId>com.example</groupId>
    <artifactId>secret</artifactId>
    <version>1.0.0</version>
</project>`
	outsidePOM := filepath.Join(outside, "pom.xml")
	if err := os.WriteFile(outsidePOM, []byte(pomXML), 0644); err != nil {
		t.Fatalf("Failed to write outside pom.xml: %v", err)
	}

	t.Setenv("GITHUB_WORKSPACE", workspace)

	t.Run("symlinked file resolving outside is rejected", func(t *testing.T) {
		link := filepath.Join(workspace, "pom.xml")
		if err := os.Symlink(outsidePOM, link); err != nil {
			t.Fatalf("Failed to create symlink: %v", err)
		}
		t.Cleanup(func() { _ = os.Remove(link) })
		if _, ok := readPOM(link); ok {
			t.Error("readPOM should reject a symlinked pom.xml resolving outside the workspace")
		}
	})

	t.Run("symlinked parent directory is rejected", func(t *testing.T) {
		linkDir := filepath.Join(workspace, "linkdir")
		if err := os.Symlink(outside, linkDir); err != nil {
			t.Fatalf("Failed to create dir symlink: %v", err)
		}
		t.Cleanup(func() { _ = os.Remove(linkDir) })
		if _, ok := readPOM(filepath.Join(linkDir, "pom.xml")); ok {
			t.Error("readPOM should reject a pom.xml reached through a symlinked parent directory")
		}
	})

	t.Run("regular in-workspace file is read", func(t *testing.T) {
		reg := filepath.Join(workspace, "real-pom.xml")
		if err := os.WriteFile(reg, []byte(pomXML), 0644); err != nil {
			t.Fatalf("Failed to write regular pom.xml: %v", err)
		}
		t.Cleanup(func() { _ = os.Remove(reg) })
		if _, ok := readPOM(reg); !ok {
			t.Error("readPOM should read a regular pom.xml inside the workspace")
		}
	})
}

// TestLoadParentPOMRejectsSymlinkedParent verifies that a <parent>
// <relativePath> resolving through an in-workspace symlink that points
// outside GITHUB_WORKSPACE is rejected, so os.Stat never follows the link to
// leak filesystem structure outside the checkout on a CI runner.
func TestLoadParentPOMRejectsSymlinkedParent(t *testing.T) {
	base := t.TempDir()
	workspace := filepath.Join(base, "workspace")
	outside := filepath.Join(base, "outside")
	childDir := filepath.Join(workspace, "child")
	for _, dir := range []string{workspace, outside, childDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create %s: %v", dir, err)
		}
	}

	outsidePOM := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>
    <groupId>com.example</groupId>
    <artifactId>parent</artifactId>
    <version>1.0.0</version>
    <packaging>pom</packaging>
    <properties>
        <maven.compiler.release>21</maven.compiler.release>
    </properties>
</project>`
	if err := os.WriteFile(filepath.Join(outside, "pom.xml"), []byte(outsidePOM), 0644); err != nil {
		t.Fatalf("Failed to write outside pom.xml: %v", err)
	}

	// A symlink inside the workspace that points at the out-of-workspace
	// directory holding the parent POM.
	linkDir := filepath.Join(workspace, "link-parent")
	if err := os.Symlink(outside, linkDir); err != nil {
		t.Fatalf("Failed to create dir symlink: %v", err)
	}

	childPOM := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>
    <parent>
        <groupId>com.example</groupId>
        <artifactId>parent</artifactId>
        <version>1.0.0</version>
        <relativePath>../link-parent/pom.xml</relativePath>
    </parent>
    <artifactId>child</artifactId>
</project>`
	if err := os.WriteFile(filepath.Join(childDir, "pom.xml"), []byte(childPOM), 0644); err != nil {
		t.Fatalf("Failed to write child pom.xml: %v", err)
	}

	t.Setenv("GITHUB_WORKSPACE", workspace)
	version, _ := mavenJavaVersionOf(t, childDir)
	if version != "" {
		t.Errorf("java_version = %q, want empty (symlinked parent escaping workspace must be rejected)", version)
	}
}

// TestResolvePropertyNestedPlaceholders verifies that a value which expands
// to another placeholder resolves fully and deterministically, regardless of
// Go's randomised map iteration order. A single-pass resolver would
// intermittently leave ${...} unresolved depending on iteration order.
func TestResolvePropertyNestedPlaceholders(t *testing.T) {
	props := map[string]string{
		"maven.compiler.release": "${java.level}",
		"java.level":             "${java.version}",
		"java.version":           "21",
	}
	for i := 0; i < 100; i++ {
		if got := resolveProperty(props["maven.compiler.release"], props); got != "21" {
			t.Fatalf("resolveProperty nested = %q, want 21", got)
		}
	}
}

// TestResolvePropertyStopsOnCycle verifies the resolver terminates on a
// cyclic reference rather than looping forever.
func TestResolvePropertyStopsOnCycle(t *testing.T) {
	props := map[string]string{"a": "${b}", "b": "${a}"}
	done := make(chan struct{})
	go func() {
		_ = resolveProperty("${a}", props)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("resolveProperty did not terminate on a cyclic reference")
	}
}

// TestMavenExtractJavaVersionNestedProperty verifies end-to-end that a
// maven.compiler.release declared as a nested placeholder resolves to the
// underlying literal Java level.
func TestMavenExtractJavaVersionNestedProperty(t *testing.T) {
	tmpDir := t.TempDir()
	pomXML := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>
    <groupId>com.example</groupId>
    <artifactId>nested</artifactId>
    <version>1.0.0</version>
    <properties>
        <maven.compiler.release>${java.level}</maven.compiler.release>
        <java.level>${java.version}</java.level>
        <java.version>21</java.version>
    </properties>
</project>`
	if err := os.WriteFile(filepath.Join(tmpDir, "pom.xml"), []byte(pomXML), 0644); err != nil {
		t.Fatalf("Failed to write pom.xml: %v", err)
	}
	version, _ := mavenJavaVersionOf(t, tmpDir)
	if version != "21" {
		t.Errorf("java_version = %q, want 21 (nested placeholder resolution)", version)
	}
}

// TestMavenExtractJavaVersionSkipsUnresolvedPlaceholder verifies that a
// Java-version property whose value stays an unresolved ${...} placeholder
// (an undefined reference) is skipped, so detection falls through to the next
// candidate instead of emitting an invalid version like "${missing}".
func TestMavenExtractJavaVersionSkipsUnresolvedPlaceholder(t *testing.T) {
	tmpDir := t.TempDir()
	pomXML := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>
    <groupId>com.example</groupId>
    <artifactId>unresolved</artifactId>
    <version>1.0.0</version>
    <properties>
        <maven.compiler.release>${missing.property}</maven.compiler.release>
        <maven.compiler.target>17</maven.compiler.target>
    </properties>
</project>`
	if err := os.WriteFile(filepath.Join(tmpDir, "pom.xml"), []byte(pomXML), 0644); err != nil {
		t.Fatalf("Failed to write pom.xml: %v", err)
	}
	version, source := mavenJavaVersionOf(t, tmpDir)
	if version != "17" {
		t.Errorf("java_version = %q, want 17 (unresolved release placeholder must be skipped)", version)
	}
	if source != "maven.compiler.target" {
		t.Errorf("java_version_source = %q, want maven.compiler.target", source)
	}
}
