// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package java

import (
	"os"
	"path/filepath"
	"testing"
)

// extractPOM writes a POM to a temporary directory and extracts it.
func extractPOM(t *testing.T, pomXML string) map[string]interface{} {
	t.Helper()

	tmpDir := t.TempDir()
	pomPath := filepath.Join(tmpDir, "pom.xml")
	if err := os.WriteFile(pomPath, []byte(pomXML), 0644); err != nil {
		t.Fatalf("failed to write pom.xml: %v", err)
	}

	metadata, err := NewMavenExtractor().Extract(tmpDir)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	return metadata.LanguageSpecific
}

// assertStringSlice checks a language-specific value holds want.
func assertStringSlice(t *testing.T, ls map[string]interface{}, key string, want []string) {
	t.Helper()

	raw, present := ls[key]
	if !present {
		t.Fatalf("%s absent, want %v", key, want)
	}
	got, ok := raw.([]string)
	if !ok {
		t.Fatalf("%s is %T, want []string", key, raw)
	}
	if len(got) != len(want) {
		t.Fatalf("%s = %v, want %v", key, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s[%d] = %q, want %q", key, i, got[i], want[i])
		}
	}
}

// A POM that says nothing about layout should report Maven's conventions
// rather than nothing, since that is what the build will use.
func TestLayoutDefaultsToMavenConvention(t *testing.T) {
	ls := extractPOM(t, `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>
    <groupId>com.example</groupId>
    <artifactId>plain</artifactId>
    <version>1.0.0</version>
</project>`)

	assertStringSlice(t, ls, "source_dirs", []string{"src/main/java"})
	assertStringSlice(t, ls, "test_source_dirs", []string{"src/test/java"})
}

// An explicit <build> entry overrides the convention.
func TestLayoutHonoursExplicitDirectories(t *testing.T) {
	ls := extractPOM(t, `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>
    <groupId>com.example</groupId>
    <artifactId>custom</artifactId>
    <version>1.0.0</version>
    <build>
        <sourceDirectory>src/java</sourceDirectory>
        <testSourceDirectory>src/tests</testSourceDirectory>
    </build>
</project>`)

	assertStringSlice(t, ls, "source_dirs", []string{"src/java"})
	assertStringSlice(t, ls, "test_source_dirs", []string{"src/tests"})
}

// Without JaCoCo there is no report to point at, and inventing a path
// would send a scanner looking for a file that never appears.
func TestLayoutOmitsCoverageWithoutJacoco(t *testing.T) {
	ls := extractPOM(t, `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>
    <groupId>com.example</groupId>
    <artifactId>nocoverage</artifactId>
    <version>1.0.0</version>
    <build>
        <plugins>
            <plugin>
                <groupId>org.apache.maven.plugins</groupId>
                <artifactId>maven-compiler-plugin</artifactId>
            </plugin>
        </plugins>
    </build>
</project>`)

	if _, present := ls["coverage_report_paths"]; present {
		t.Errorf("coverage_report_paths present without JaCoCo: %v",
			ls["coverage_report_paths"])
	}
	if _, present := ls["coverage_tool"]; present {
		t.Errorf("coverage_tool present without JaCoCo: %v", ls["coverage_tool"])
	}
}

// JaCoCo in the active plugin list yields its default report location.
func TestLayoutDetectsJacocoInPlugins(t *testing.T) {
	ls := extractPOM(t, `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>
    <groupId>com.example</groupId>
    <artifactId>covered</artifactId>
    <version>1.0.0</version>
    <build>
        <plugins>
            <plugin>
                <groupId>org.jacoco</groupId>
                <artifactId>jacoco-maven-plugin</artifactId>
            </plugin>
        </plugins>
    </build>
</project>`)

	if got, _ := ls["coverage_tool"].(string); got != "jacoco" {
		t.Errorf("coverage_tool = %q, want %q", got, "jacoco")
	}
	assertStringSlice(t, ls, "coverage_report_paths",
		[]string{"target/site/jacoco/jacoco.xml"})
}

// A reactor parent commonly declares JaCoCo under pluginManagement for
// submodules to inherit. Checking only the active plugin list would miss
// exactly the aggregator POM a scanner is pointed at.
func TestLayoutDetectsJacocoInPluginManagement(t *testing.T) {
	ls := extractPOM(t, `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>
    <groupId>com.example</groupId>
    <artifactId>reactor</artifactId>
    <version>1.0.0</version>
    <packaging>pom</packaging>
    <build>
        <pluginManagement>
            <plugins>
                <plugin>
                    <groupId>org.jacoco</groupId>
                    <artifactId>jacoco-maven-plugin</artifactId>
                </plugin>
            </plugins>
        </pluginManagement>
    </build>
</project>`)

	if got, _ := ls["coverage_tool"].(string); got != "jacoco" {
		t.Errorf("coverage_tool = %q, want %q", got, "jacoco")
	}
	assertStringSlice(t, ls, "coverage_report_paths",
		[]string{"target/site/jacoco/jacoco.xml"})
}

// A reactor root that declares nothing itself and delegates build
// configuration to a parent module is the shape ONAP cps uses: JaCoCo
// lives in cps-parent/pom.xml, so inspecting only the aggregator would
// report no coverage on a project that has 99% of it.
func TestLayoutDetectsJacocoInAModule(t *testing.T) {
	tmpDir := t.TempDir()

	root := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>
    <groupId>com.example</groupId>
    <artifactId>aggregator</artifactId>
    <version>1.0.0</version>
    <packaging>pom</packaging>
    <modules>
        <module>project-parent</module>
    </modules>
</project>`
	if err := os.WriteFile(filepath.Join(tmpDir, "pom.xml"), []byte(root), 0644); err != nil {
		t.Fatalf("failed to write aggregator pom.xml: %v", err)
	}

	moduleDir := filepath.Join(tmpDir, "project-parent")
	if err := os.MkdirAll(moduleDir, 0755); err != nil {
		t.Fatalf("failed to create module dir: %v", err)
	}
	module := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>
    <groupId>com.example</groupId>
    <artifactId>project-parent</artifactId>
    <version>1.0.0</version>
    <packaging>pom</packaging>
    <build>
        <pluginManagement>
            <plugins>
                <plugin>
                    <groupId>org.jacoco</groupId>
                    <artifactId>jacoco-maven-plugin</artifactId>
                </plugin>
            </plugins>
        </pluginManagement>
    </build>
</project>`
	if err := os.WriteFile(filepath.Join(moduleDir, "pom.xml"), []byte(module), 0644); err != nil {
		t.Fatalf("failed to write module pom.xml: %v", err)
	}

	metadata, err := NewMavenExtractor().Extract(tmpDir)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	if got, _ := metadata.LanguageSpecific["coverage_tool"].(string); got != "jacoco" {
		t.Errorf("coverage_tool = %q, want %q", got, "jacoco")
	}
}

// A module path escaping the workspace must not be read, matching the
// guard javaVersionFromModules applies.
func TestLayoutIgnoresModulesOutsideWorkspace(t *testing.T) {
	tmpDir := t.TempDir()

	root := `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>
    <groupId>com.example</groupId>
    <artifactId>escaper</artifactId>
    <version>1.0.0</version>
    <packaging>pom</packaging>
    <modules>
        <module>/etc</module>
        <module>../../../elsewhere</module>
    </modules>
</project>`
	if err := os.WriteFile(filepath.Join(tmpDir, "pom.xml"), []byte(root), 0644); err != nil {
		t.Fatalf("failed to write pom.xml: %v", err)
	}

	metadata, err := NewMavenExtractor().Extract(tmpDir)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	if _, present := metadata.LanguageSpecific["coverage_tool"]; present {
		t.Errorf("coverage_tool present from an out-of-workspace module")
	}
}
