// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sethvargo/go-githubactions"
)

const minimalPOM = `<?xml version="1.0"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
  <modelVersion>4.0.0</modelVersion>
  <groupId>org.example</groupId>
  <artifactId>demo</artifactId>
  <version>1.0.0</version>
  <properties>
    <maven.compiler.release>17</maven.compiler.release>
  </properties>
</project>
`

// polyglotProject writes a Maven project that also carries a
// package.json, the shape frontend-maven-plugin produces.
func polyglotProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatalf("writing %s: %v", name, err)
		}
	}
	write("pom.xml", minimalPOM)
	write("package.json", `{"name":"ui","version":"1.0.0"}`)
	return dir
}

func resolve(t *testing.T, dir, override string) (string, string) {
	t.Helper()
	ctx := &appContext{action: githubactions.New(), isCI: false}
	metadata := newMetadata(dir)
	cfg := runConfig{absPath: dir, projectTypeOverride: override}
	projectType := detectProjectType(ctx, metadata, cfg)
	return projectType, metadata.Common.BuildTool
}

// Detection resolves the first rule that matches in priority order, and
// javascript-npm outranks java-maven. A Maven project carrying a
// package.json therefore resolves to JavaScript and reports no Java
// metadata, which is the case project_type exists to answer.
func TestDetectionPrefersHigherPriorityMarker(t *testing.T) {
	projectType, buildTool := resolve(t, polyglotProject(t), "")

	if projectType != "javascript-npm" {
		t.Errorf("project type = %q, want javascript-npm", projectType)
	}
	if buildTool != "npm" {
		t.Errorf("build tool = %q, want npm", buildTool)
	}
}

// A caller that already knows what it is overrides that.
func TestSuppliedProjectTypeWins(t *testing.T) {
	projectType, buildTool := resolve(t, polyglotProject(t), "java-maven")

	if projectType != "java-maven" {
		t.Errorf("project type = %q, want java-maven", projectType)
	}
	if buildTool != "maven" {
		t.Errorf("build tool = %q, want maven", buildTool)
	}
}

// An unrecognised value is discarded rather than honoured: detection
// still yields a real answer, whereas an unknown type resolves to no
// extractor and would yield none at all.
func TestUnknownSuppliedProjectTypeFallsBackToDetection(t *testing.T) {
	projectType, _ := resolve(t, polyglotProject(t), "not-a-real-type")

	if projectType != "javascript-npm" {
		t.Errorf("project type = %q, want javascript-npm", projectType)
	}
}

// Every type a Java lane would supply has to resolve, or the override is
// useless to the callers it exists for.
func TestSuppliedJavaProjectTypesAllResolve(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pom.xml"), []byte(minimalPOM), 0o600); err != nil {
		t.Fatalf("writing pom.xml: %v", err)
	}

	for _, supplied := range []string{
		"java-maven", "java-gradle", "java-gradle-kts", "kotlin-gradle",
	} {
		projectType, buildTool := resolve(t, dir, supplied)
		if projectType != supplied {
			t.Errorf("supplied %q, resolved %q", supplied, projectType)
		}
		if buildTool == "" {
			t.Errorf("supplied %q named no build tool", supplied)
		}
	}
}
