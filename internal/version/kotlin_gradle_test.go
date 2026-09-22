// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package version

import (
	"os"
	"path/filepath"
	"testing"
)

const kotlinBuildScript = `plugins { java }
group = "org.example"
version = "4.5.6"
`

// Version extraction dispatches on a project-type prefix, and
// kotlin-gradle does not start with "java" even though the detector
// returns it for a build.gradle.kts root. Without an explicit case it
// falls through to the generic path, which reads a git tag or
// version.properties instead of the build script -- so a project whose
// tag disagrees with its build file reports the wrong version, and the
// source that produced it is wrong too.
//
// The temporary directory is deliberately not a git repository, so the
// fallback has nothing to find and a misroute shows up as an empty or
// absent version rather than a coincidentally correct one.
func TestKotlinGradleReadsTheBuildScript(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "build.gradle.kts")
	if err := os.WriteFile(path, []byte(kotlinBuildScript), 0o600); err != nil {
		t.Fatalf("writing build.gradle.kts: %v", err)
	}

	info, err := extractBasic(dir, "kotlin-gradle")
	if err != nil {
		t.Fatalf("extractBasic: %v", err)
	}
	if info == nil {
		t.Fatal("extractBasic returned no version info")
	}
	if info.Version != "4.5.6" {
		t.Errorf("version = %q, want 4.5.6 from the build script", info.Version)
	}
}

// The same tree routed as java-gradle-kts must agree, since both types
// describe one project and resolve to one extractor.
func TestKotlinGradleAgreesWithJavaGradleKts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "build.gradle.kts")
	if err := os.WriteFile(path, []byte(kotlinBuildScript), 0o600); err != nil {
		t.Fatalf("writing build.gradle.kts: %v", err)
	}

	kotlin, err := extractBasic(dir, "kotlin-gradle")
	if err != nil {
		t.Fatalf("extractBasic(kotlin-gradle): %v", err)
	}
	java, err := extractBasic(dir, "java-gradle-kts")
	if err != nil {
		t.Fatalf("extractBasic(java-gradle-kts): %v", err)
	}

	if kotlin.Version != java.Version {
		t.Errorf("kotlin-gradle version = %q, java-gradle-kts = %q",
			kotlin.Version, java.Version)
	}
}

// A project mid-migration carries both scripts. Version extraction and
// the Gradle extractor have to choose the same one, or the common
// project_version describes a different file from every java_* value.
//
// Kotlin wins, matching GradleExtractor.detectBuildFile and the
// detector, which ranks kotlin-gradle above java-gradle on the same
// tree. The two scripts carry different versions here so a regression
// cannot pass by coincidence.
func TestMixedDSLPrefersTheKotlinScript(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatalf("writing %s: %v", name, err)
		}
	}
	write("build.gradle.kts", kotlinBuildScript)
	write("build.gradle", "group = 'org.example'\nversion = '9.9.9'\n")

	info, err := extractBasic(dir, "kotlin-gradle")
	if err != nil {
		t.Fatalf("extractBasic: %v", err)
	}
	if info.Version != "4.5.6" {
		t.Errorf("version = %q, want 4.5.6 from build.gradle.kts", info.Version)
	}
}
