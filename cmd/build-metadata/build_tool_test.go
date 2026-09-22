// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package main

import "testing"

func TestBuildToolForProjectType(t *testing.T) {
	tests := []struct {
		projectType string
		want        string
		why         string
	}{
		{"java-maven", "maven", "the case the Sonar lane needs"},
		{"java-gradle", "gradle", ""},
		{"java-gradle-kts", "gradle", "same tool, different DSL"},
		{"kotlin-gradle", "gradle", "what the detector actually emits for .kts"},
		{"typescript-npm", "npm", "language and tool differ"},
		{"javascript-yarn", "yarn", ""},
		{"javascript-pnpm", "pnpm", ""},
		{"go-module", "go", ""},
		{"rust-cargo", "cargo", ""},
		{"terraform-opentofu", "opentofu", "not terraform"},
		{"c-cmake", "cmake", ""},

		// Absent by design: reported as empty so a consumer can tell
		// "not identified" from a wrong answer.
		{"python-modern", "", "tool is python_build_backend"},
		{"python-legacy", "", "tool is python_build_backend"},
		{"java-library", "", "names a shape, not a tool"},
		{"unknown-thing", "", ""},
		{"", "", ""},
	}

	for _, tt := range tests {
		if got := buildToolForProjectType(tt.projectType); got != tt.want {
			t.Errorf("buildToolForProjectType(%q) = %q, want %q %s",
				tt.projectType, got, tt.want, tt.why)
		}
	}
}

// Every project type that names a build tool must also normalise to a
// language, or the two maps have drifted apart.
func TestBuildToolTypesAreKnownProjectTypes(t *testing.T) {
	for _, projectType := range []string{
		"java-maven", "java-gradle", "java-gradle-kts", "kotlin-gradle",
		"javascript-npm", "javascript-yarn", "javascript-pnpm",
		"typescript-npm", "go-module", "rust-cargo", "ruby-bundler",
		"php-composer", "swift-package", "helm-chart", "docker",
		"terraform", "terraform-module", "terraform-opentofu",
		"c-cmake", "c-autoconf",
	} {
		if normalizeProjectTypeToLanguage(projectType) == "" {
			t.Errorf("%q names a build tool but no language", projectType)
		}
	}
}

// Both maps here must agree with GetExtractor, which routes all three
// Gradle project types to the java-gradle extractor. They did not, and
// nothing caught it: the table above asserted java-gradle-kts, a type
// DetectProjectType can never return, while kotlin-gradle -- the type it
// does return for a build.gradle.kts root -- was absent from both maps.
//
// The consequences were silent rather than loud. buildToolForProjectType
// reported "", and normalizeProjectTypeToLanguage fell through to its
// hyphen-splitting fallback and returned "kotlin", so the Gradle
// extractor's values were emitted as kotlin_* while action.yaml declares
// java_*. Consumers read an empty java_version and fell back to their own
// default, choosing a JDK the project never asked for.
//
// Asserting the whole family together is what makes this stick: a type
// added to one map and forgotten in the other fails here.
func TestGradleProjectTypesResolveConsistently(t *testing.T) {
	for _, projectType := range []string{
		"java-gradle", "java-gradle-kts", "kotlin-gradle",
	} {
		if got := buildToolForProjectType(projectType); got != "gradle" {
			t.Errorf("buildToolForProjectType(%q) = %q, want gradle",
				projectType, got)
		}
		if got := normalizeProjectTypeToLanguage(projectType); got != "java" {
			t.Errorf("normalizeProjectTypeToLanguage(%q) = %q, want java",
				projectType, got)
		}
	}
}
