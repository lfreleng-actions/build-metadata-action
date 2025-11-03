// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package java

import (
	"os"
	"path/filepath"
	"testing"
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

	if javaVersion, ok := metadata.LanguageSpecific["java_version"].(string); !ok || javaVersion != "17" {
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

			if javaVersion, ok := metadata.LanguageSpecific["java_version"].(string); !ok || javaVersion != tt.expectedJava {
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
