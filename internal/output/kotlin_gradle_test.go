// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package output

import (
	"strings"
	"testing"
)

// gradleMetadata is the shape the java-gradle extractor produces; the
// detector routes both project types below to that same extractor.
func gradleMetadata() map[string]interface{} {
	return map[string]interface{}{
		"version":          "17",
		"version_source":   "build.gradle.kts/toolchain",
		"group_id":         "org.example",
		"artifact_id":      "demo",
		"build_dsl":        "kotlin",
		"gradle_version":   "9.7.1",
		"is_multi_project": true,
	}
}

// Dispatch here matches project-type prefixes, and kotlin-gradle does not
// start with "java" even though it is a Java Gradle project -- it is what
// the detector returns for a build.gradle.kts root. Asserting equivalence
// with java-gradle rather than against fixed text keeps this true as the
// renderers change, and fails if either type is dropped from the Java
// entry.
func TestKotlinGradleRendersTheSameRowsAsJavaGradle(t *testing.T) {
	render := func(projectType string) string {
		var sb strings.Builder
		addLanguageSpecificToTable(&sb, projectType, gradleMetadata())
		return sb.String()
	}

	java := render("java-gradle")
	if java == "" {
		t.Fatal("java-gradle rendered no rows; the fixture or dispatch is wrong")
	}

	if kotlin := render("kotlin-gradle"); kotlin != java {
		t.Errorf("kotlin-gradle rows differ from java-gradle\n got: %q\nwant: %q",
			kotlin, java)
	}
}

func TestKotlinGradleSelectsTheSameToolsAsJavaGradle(t *testing.T) {
	tools := map[string]string{
		"java":   "17.0.13",
		"javac":  "17.0.13",
		"gradle": "9.7.1",
		"mvn":    "3.9.16",
		"node":   "22.11.0",
		"go":     "1.23.3",
	}

	java := filterRelevantTools("java-gradle", tools)
	if len(java) == 0 {
		t.Fatal("java-gradle selected no tools; the fixture or dispatch is wrong")
	}

	kotlin := filterRelevantTools("kotlin-gradle", tools)
	if len(kotlin) != len(java) {
		t.Fatalf("kotlin-gradle selected %d tools, java-gradle %d",
			len(kotlin), len(java))
	}
	for name, version := range java {
		if kotlin[name] != version {
			t.Errorf("tool %q: kotlin-gradle = %q, java-gradle = %q",
				name, kotlin[name], version)
		}
	}
}
