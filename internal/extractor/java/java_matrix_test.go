// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package java

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lfreleng-actions/build-metadata-action/internal/extractor"
	"github.com/lfreleng-actions/build-metadata-action/internal/supported"
	"github.com/lfreleng-actions/build-metadata-action/internal/supported/supportedtest"
)

// fixedNow is the clock the matrix tests run at: Java 17, 21 and 25
// are the supported LTS lines from the catalog floor of 17.
func fixedNow() time.Time {
	return time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
}

// fullJavaMatrix is java_matrix_json with no constraint, from the
// recorded upstream data and the embedded snapshot alike.
const fullJavaMatrix = `{"java-version": ["17", "21", "25"]}`

func captureWarnings(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	saved := warnings
	warnings = &buf
	t.Cleanup(func() { warnings = saved })
	return &buf
}

// useSnapshotFallback makes the snapshot fallback run at the fixed
// clock until the test ends.
func useSnapshotFallback(t *testing.T) {
	t.Helper()
	saved := newSnapshotResolver
	newSnapshotResolver = func() (supported.ProjectMatrixResolver, error) {
		return NewResolver(supported.Settings{Offline: true, NoCache: true, Now: fixedNow})
	}
	t.Cleanup(func() { newSnapshotResolver = saved })
}

// installResolver installs r and the fixed-clock snapshot fallback,
// with the cache in a fresh directory, until the test ends.
func installResolver(t *testing.T, r supported.ProjectMatrixResolver) {
	t.Helper()
	t.Setenv("SUPPORTED_VERSIONS_CACHE_DIR", t.TempDir())
	useSnapshotFallback(t)
	SetResolver(r)
	t.Cleanup(func() { SetResolver(nil) })
}

// useRecordedUpstream installs a resolver reading up at the fixed
// clock.
func useRecordedUpstream(t *testing.T, up *supportedtest.Upstream) {
	t.Helper()
	t.Setenv("SUPPORTED_VERSIONS_CACHE_DIR", t.TempDir())
	r, err := NewResolver(supported.Settings{
		Timeout: time.Second, Now: fixedNow, EndOfLifeBaseURL: up.URL, AdoptiumBaseURL: up.URL,
	})
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })
	installResolver(t, r)
}

type failingResolver struct{}

func (failingResolver) MatrixFor(context.Context, string, supported.MatrixRequest) (supported.Report, error) {
	return supported.Report{}, errors.New("upstream unavailable")
}

type emptyResolver struct{}

func (emptyResolver) MatrixFor(context.Context, string, supported.MatrixRequest) (supported.Report, error) {
	return supported.Report{}, nil
}

// writeFiles creates a project directory holding files.
func writeFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// mavenProject returns a Maven project declaring maven.compiler.release
// as release, or no Java level when release is empty.
func mavenProject(t *testing.T, release string) string {
	t.Helper()
	props := ""
	if release != "" {
		props = "<properties><maven.compiler.release>" + release + "</maven.compiler.release></properties>"
	}
	return writeFiles(t, map[string]string{"pom.xml": `<?xml version="1.0"?>
<project><modelVersion>4.0.0</modelVersion><groupId>org.example</groupId>
<artifactId>demo</artifactId><version>1.0.0</version>` + props + `</project>`})
}

// javaOutputs returns the values behind java_version, java_matrix_json
// and java_latest; a key left unset reads as "".
func javaOutputs(t *testing.T, e extractor.Extractor, dir string) (level, matrix, latest string) {
	t.Helper()
	metadata, err := e.Extract(dir)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	level, _ = metadata.LanguageSpecific["version"].(string)
	matrix, _ = metadata.LanguageSpecific["matrix_json"].(string)
	latest, _ = metadata.LanguageSpecific["latest"].(string)
	return level, matrix, latest
}

func TestJavaMatrixFromProjectLevel(t *testing.T) {
	groovy := "plugins { id 'java' }\njava { toolchain { languageVersion = JavaLanguageVersion.of(21) } }\n"
	kotlin := "plugins { java }\njava { toolchain { languageVersion.set(JavaLanguageVersion.of(21)) } }\n"
	tests := []struct {
		name        string
		extractor   extractor.Extractor
		dir         func(t *testing.T) string
		level       string
		want        string
		latest      string
		wantWarning string
	}{
		{"Maven release 17", NewMavenExtractor(), func(t *testing.T) string { return mavenProject(t, "17") },
			"17", fullJavaMatrix, "25", ""},
		{"Maven release 21", NewMavenExtractor(), func(t *testing.T) string { return mavenProject(t, "21") },
			"21", `{"java-version": ["21", "25"]}`, "25", ""},
		{"Maven release 1.8", NewMavenExtractor(), func(t *testing.T) string { return mavenProject(t, "1.8") },
			"1.8", fullJavaMatrix, "25", ""},
		{"Maven release 25", NewMavenExtractor(), func(t *testing.T) string { return mavenProject(t, "25") },
			"25", `{"java-version": ["25"]}`, "25", ""},
		{"Maven without a level", NewMavenExtractor(), func(t *testing.T) string { return mavenProject(t, "") },
			"", fullJavaMatrix, "25", ""},
		{"Maven out-of-range level", NewMavenExtractor(), func(t *testing.T) string { return mavenProject(t, "40") },
			"40", fullJavaMatrix, "25", `allows no line of the matrix`},
		{"Groovy Gradle", NewGradleExtractor(),
			func(t *testing.T) string { return writeFiles(t, map[string]string{"build.gradle": groovy}) },
			"21", `{"java-version": ["21", "25"]}`, "25", ""},
		{"Kotlin DSL Gradle", NewGradleExtractor(),
			func(t *testing.T) string { return writeFiles(t, map[string]string{"build.gradle.kts": kotlin}) },
			"21", `{"java-version": ["21", "25"]}`, "25", ""},
		{"Gradle without a level", NewGradleExtractor(),
			func(t *testing.T) string {
				return writeFiles(t, map[string]string{"build.gradle": "plugins { id 'java' }\n"})
			},
			"", fullJavaMatrix, "25", ""},
		{"Gradle level that does not parse", NewGradleExtractor(),
			func(t *testing.T) string {
				return writeFiles(t, map[string]string{
					"build.gradle":      "plugins { id 'java' }\n",
					"gradle.properties": "sourceCompatibility=VERSION_17\n",
				})
			},
			"VERSION_17", fullJavaMatrix, "25", `Could not parse the Java version "VERSION_17"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warned := captureWarnings(t)
			up := supportedtest.NewJavaUpstream(t)
			useRecordedUpstream(t, up)
			level, matrix, latest := javaOutputs(t, tt.extractor, tt.dir(t))
			if level != tt.level {
				t.Errorf("java_version = %q, want %q", level, tt.level)
			}
			if matrix != tt.want {
				t.Errorf("java_matrix_json = %s, want %s", matrix, tt.want)
			}
			if latest != tt.latest {
				t.Errorf("java_latest = %q, want %q", latest, tt.latest)
			}
			if tt.wantWarning == "" && warned.Len() != 0 {
				t.Errorf("warnings = %q, want none", warned.String())
			}
			if tt.wantWarning != "" && !strings.Contains(warned.String(), tt.wantWarning) {
				t.Errorf("warnings = %q, want %q", warned.String(), tt.wantWarning)
			}
			if up.Requests() == 0 {
				t.Error("resolver made no request to the upstream")
			}
		})
	}
}

// TestKotlinGradleProjectType checks that the kotlin-gradle project
// type, which the detector reports for a build.gradle.kts root, routes
// to the Gradle extractor and emits the Java matrix.
func TestKotlinGradleProjectType(t *testing.T) {
	captureWarnings(t)
	useRecordedUpstream(t, supportedtest.NewJavaUpstream(t))
	e, err := extractor.GetExtractor("kotlin-gradle")
	if err != nil {
		t.Fatal(err)
	}
	dir := writeFiles(t, map[string]string{
		"build.gradle.kts": "plugins { kotlin(\"jvm\") }\nkotlin { jvmToolchain(17) }\n" +
			"java { toolchain { languageVersion.set(JavaLanguageVersion.of(17)) } }\n",
	})
	if _, matrix, _ := javaOutputs(t, e, dir); matrix != fullJavaMatrix {
		t.Errorf("java_matrix_json = %s, want %s", matrix, fullJavaMatrix)
	}
}

// TestJavaMatrixNewerUpstream checks that a line newer than the
// embedded snapshot reaches java_matrix_json from live data.
func TestJavaMatrixNewerUpstream(t *testing.T) {
	captureWarnings(t)
	up := supportedtest.NewJavaUpstream(t)
	up.ServeReleases(supportedtest.JavaReleasesNewer)
	up.ServeManifest(supportedtest.AdoptiumReleasesNewer)
	useRecordedUpstream(t, up)

	_, matrix, latest := javaOutputs(t, NewMavenExtractor(), mavenProject(t, "21"))
	if want := `{"java-version": ["21", "25", "29"]}`; matrix != want {
		t.Errorf("java_matrix_json = %s, want %s", matrix, want)
	}
	if latest != "29" {
		t.Errorf("java_latest = %q, want 29", latest)
	}
}

// TestJavaMatrixOfflineIgnoresCache checks that offline mode, as the
// action configures it, reads the embedded snapshot even when the
// runner cache holds newer release facts.
func TestJavaMatrixOfflineIgnoresCache(t *testing.T) {
	captureWarnings(t)
	t.Setenv("SUPPORTED_VERSIONS_CACHE_DIR", t.TempDir())
	fetched := fixedNow().Add(-time.Hour)
	supportedtest.CacheResponse(t, supportedtest.DefaultJavaReleasesURL, supportedtest.JavaReleasesNewer, fetched)
	supportedtest.CacheResponse(t, supportedtest.DefaultAdoptiumURL, supportedtest.AdoptiumReleasesNewer, fetched)

	// A resolver that reads the cache sees the newer line 29.
	cached, err := NewResolver(supported.Settings{Offline: true, Now: fixedNow})
	if err != nil {
		t.Fatal(err)
	}
	rep, err := cached.MatrixFor(context.Background(), javaTool, supported.MatrixRequest{Constraint: "21"})
	_ = cached.Close()
	if err != nil {
		t.Fatal(err)
	}
	if rep.Latest != "29" {
		t.Fatalf("cache-reading resolver latest = %q, want 29: the seeded cache was not read", rep.Latest)
	}

	r, err := NewResolver(supported.Settings{Offline: true, NoCache: true, Now: fixedNow})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	useSnapshotFallback(t)
	SetResolver(r)
	t.Cleanup(func() { SetResolver(nil) })

	_, matrix, latest := javaOutputs(t, NewMavenExtractor(), mavenProject(t, "21"))
	if want := `{"java-version": ["21", "25"]}`; matrix != want {
		t.Errorf("java_matrix_json = %s, want %s", matrix, want)
	}
	if latest != "25" {
		t.Errorf("java_latest = %q, want 25", latest)
	}
}

// TestJavaMatrixRecoversFromTheSnapshot checks that a resolver error
// or an empty report falls back to the embedded snapshot.
func TestJavaMatrixRecoversFromTheSnapshot(t *testing.T) {
	for name, r := range map[string]supported.ProjectMatrixResolver{
		"resolver error": failingResolver{},
		"empty report":   emptyResolver{},
	} {
		t.Run(name, func(t *testing.T) {
			warned := captureWarnings(t)
			installResolver(t, r)
			_, matrix, latest := javaOutputs(t, NewMavenExtractor(), mavenProject(t, "21"))
			if want := `{"java-version": ["21", "25"]}`; matrix != want {
				t.Errorf("java_matrix_json = %s, want %s", matrix, want)
			}
			if latest != "25" {
				t.Errorf("java_latest = %q, want 25", latest)
			}
			if !strings.Contains(warned.String(), "retrying from the embedded snapshot") {
				t.Errorf("warnings = %q, want a snapshot retry", warned.String())
			}
		})
	}
}

// TestJavaMatrixUnsetWhenNothingResolves checks that extraction
// succeeds, with the new keys unset and java_version untouched, when
// the snapshot fails too.
func TestJavaMatrixUnsetWhenNothingResolves(t *testing.T) {
	warned := captureWarnings(t)
	SetResolver(failingResolver{})
	saved := newSnapshotResolver
	newSnapshotResolver = func() (supported.ProjectMatrixResolver, error) { return emptyResolver{}, nil }
	t.Cleanup(func() {
		SetResolver(nil)
		newSnapshotResolver = saved
	})

	metadata, err := NewMavenExtractor().Extract(mavenProject(t, "21"))
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	for _, key := range []string{"matrix_json", "latest"} {
		if v, ok := metadata.LanguageSpecific[key]; ok {
			t.Errorf("%s = %v, want unset", key, v)
		}
	}
	if got := metadata.LanguageSpecific["version"]; got != "21" {
		t.Errorf("version = %v, want 21", got)
	}
	if got := metadata.LanguageSpecific["version_source"]; got != "maven.compiler.release" {
		t.Errorf("version_source = %v, want maven.compiler.release", got)
	}
	if !strings.Contains(warned.String(), "leaving java_matrix_json and java_latest unset") {
		t.Errorf("warnings = %q, want the unset warning", warned.String())
	}
}
