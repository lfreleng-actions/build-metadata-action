// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package main

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/lfreleng-actions/build-metadata-action/internal/extractor/java"
	"github.com/lfreleng-actions/build-metadata-action/internal/supported"
	"github.com/lfreleng-actions/build-metadata-action/internal/supported/supportedtest"
)

// javaFixtures writes one project file, name, under a fresh
// fixtures directory and returns that directory.
func javaFixtures(t *testing.T, name, body string) string {
	t.Helper()
	fixtures := t.TempDir()
	dir := filepath.Join(fixtures, "project")
	if err := os.Mkdir(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return fixtures
}

// TestJavaMatrixOutputsAreDeclared checks that java_matrix_json and
// java_latest are written for Maven, Gradle and Kotlin Gradle projects
// and declared in action.yaml, and that java_version keeps its name.
func TestJavaMatrixOutputsAreDeclared(t *testing.T) {
	t.Setenv("GITHUB_WORKSPACE", "")
	declared := declaredOutputs(t)
	pom := `<project><modelVersion>4.0.0</modelVersion><groupId>g</groupId><artifactId>a</artifactId>` +
		`<version>1</version><properties><maven.compiler.release>21</maven.compiler.release></properties></project>`
	kts := "java { toolchain { languageVersion.set(JavaLanguageVersion.of(21)) } }\n"
	for projectType, fixtures := range map[string]string{
		"java-maven":    javaFixtures(t, "pom.xml", pom),
		"java-gradle":   javaFixtures(t, "build.gradle", "sourceCompatibility = '21'\n"),
		"kotlin-gradle": javaFixtures(t, "build.gradle.kts", kts),
	} {
		written := writtenOutputs(t, fixtures, projectType)
		for _, name := range []string{"java_version", "java_matrix_json", "java_latest"} {
			if !written[name] {
				t.Errorf("%s: %s is not written", projectType, name)
			}
			if want := "${{ steps.extract.outputs." + name + " }}"; declared[name] != want {
				t.Errorf("output %s maps to %q, want %q", name, declared[name], want)
			}
		}
	}
}

// javaResolverNow is a clock after the hypothetical Java 29 in the
// newer recorded data shipped.
func javaResolverNow() time.Time {
	return time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
}

// javaResolverMatrix resolves the unconstrained Java matrix through a
// resolver built from cfg, with the clock fixed.
func javaResolverMatrix(t *testing.T, cfg runConfig, noCache bool) []string {
	t.Helper()
	s := resolverSettings(cfg)
	s.NoCache = noCache
	s.Now = javaResolverNow
	r, err := java.NewResolver(s)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	defer func() { _ = r.Close() }()
	rep, err := r.MatrixFor(context.Background(), "java", supported.MatrixRequest{})
	if err != nil {
		t.Fatalf("MatrixFor: %v", err)
	}
	return rep.Matrix.Versions
}

// TestJavaResolverSettingsOfflineIgnoresCache checks that offline mode
// resolves from the embedded snapshot even when an earlier online step
// cached newer data: the cached data give 17, 21, 25 and 29, the
// snapshot 17, 21 and 25.
func TestJavaResolverSettingsOfflineIgnoresCache(t *testing.T) {
	t.Setenv("SUPPORTED_VERSIONS_CACHE_DIR", t.TempDir())
	supportedtest.CacheResponse(t, supportedtest.DefaultJavaReleasesURL,
		supportedtest.JavaReleasesNewer, javaResolverNow())
	supportedtest.CacheResponse(t, supportedtest.DefaultAdoptiumURL,
		supportedtest.AdoptiumReleasesNewer, javaResolverNow())
	cfg := runConfig{pythonOffline: true, pythonTimeout: time.Second}

	s := resolverSettings(cfg)
	if !s.Offline || !s.NoCache {
		t.Fatalf("offline settings = %+v, want Offline and NoCache", s)
	}
	if got, want := javaResolverMatrix(t, cfg, s.NoCache), []string{"17", "21", "25"}; !reflect.DeepEqual(got, want) {
		t.Errorf("offline matrix = %v, want %v", got, want)
	}
	if got, want := javaResolverMatrix(t, cfg, false), []string{"17", "21", "25", "29"}; !reflect.DeepEqual(got, want) {
		t.Errorf("cache-reading offline matrix = %v, want %v", got, want)
	}
}
