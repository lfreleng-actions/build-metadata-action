// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package java

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lfreleng-actions/build-metadata-action/internal/extractor"
)

// wrapperMetadata runs the extractor against a wrapper properties file
// containing body. An empty body writes no wrapper at all.
func wrapperMetadata(t *testing.T, body string) *extractor.ProjectMetadata {
	t.Helper()
	dir := t.TempDir()

	if body != "" {
		wrapperDir := filepath.Join(dir, "gradle", "wrapper")
		if err := os.MkdirAll(wrapperDir, 0o750); err != nil {
			t.Fatalf("creating wrapper directory: %v", err)
		}
		path := filepath.Join(wrapperDir, "gradle-wrapper.properties")
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("writing wrapper properties: %v", err)
		}
	}

	metadata := &extractor.ProjectMetadata{
		LanguageSpecific: make(map[string]interface{}),
	}
	applyGradleWrapper(dir, metadata)
	return metadata
}

// properties wraps a distributionUrl in the surrounding keys Gradle
// writes, so the parser is exercised against a realistic file.
func properties(distributionLine string) string {
	return "distributionBase=GRADLE_USER_HOME\n" +
		"distributionPath=wrapper/dists\n" +
		distributionLine + "\n" +
		"zipStoreBase=GRADLE_USER_HOME\n"
}

func TestWrapperVersionIsReported(t *testing.T) {
	cases := []struct {
		name string
		line string
		want string
	}{
		{
			// Properties files escape the scheme colon; the version sits
			// past it either way.
			name: "escaped colon, bin distribution",
			line: `distributionUrl=https\://services.gradle.org/distributions/gradle-9.7.1-bin.zip`,
			want: "9.7.1",
		},
		{
			name: "all distribution",
			line: `distributionUrl=https\://services.gradle.org/distributions/gradle-8.14.2-all.zip`,
			want: "8.14.2",
		},
		{
			// Pre-release qualifiers carry hyphens of their own, so the
			// -bin/-all suffix is what bounds the version.
			name: "release candidate",
			line: `distributionUrl=https\://services.gradle.org/distributions/gradle-8.5-rc-3-bin.zip`,
			want: "8.5-rc-3",
		},
		{
			name: "milestone",
			line: `distributionUrl=https\://services.gradle.org/distributions/gradle-9.0-milestone-1-bin.zip`,
			want: "9.0-milestone-1",
		},
		{
			name: "two-part version",
			line: `distributionUrl=https\://services.gradle.org/distributions/gradle-8.4-bin.zip`,
			want: "8.4",
		},
		{
			// A mirror is still a distribution URL.
			name: "self-hosted mirror",
			line: `distributionUrl=https\://mirror.example.com/gradle/gradle-9.0-bin.zip`,
			want: "9.0",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			metadata := wrapperMetadata(t, properties(tc.line))

			got, ok := metadata.LanguageSpecific["gradle_version"].(string)
			if !ok {
				t.Fatalf("gradle_version absent for %q", tc.line)
			}
			if got != tc.want {
				t.Errorf("gradle_version = %q, want %q", got, tc.want)
			}
			if metadata.LanguageSpecific["gradle_version_source"] == nil {
				t.Error("gradle_version reported with no source")
			}
		})
	}
}

// Absent beats invented. A consumer comparing this against a tool's
// floor has to tell "too old" from "unknown", so anything that is not a
// version stays quiet rather than passing malformed text through.
func TestWrapperVersionAbsentWhenUnknowable(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"no wrapper at all", ""},
		{"no distributionUrl", properties("distributionSha256Sum=abc123")},
		{
			name: "url names no version",
			body: properties(`distributionUrl=https\://example.com/gradle/latest.zip`),
		},
		{
			// A digit followed by letters is not a version component.
			name: "digits run into letters",
			body: properties(`distributionUrl=https\://example.com/gradle-9foo-bin.zip`),
		},
		{
			// An empty component between dots is malformed.
			name: "doubled separator",
			body: properties(`distributionUrl=https\://example.com/gradle-8..1-bin.zip`),
		},
		{
			name: "qualifier without a version",
			body: properties(`distributionUrl=https\://example.com/gradle-snapshot-bin.zip`),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			metadata := wrapperMetadata(t, tc.body)

			if got, present := metadata.LanguageSpecific["gradle_version"]; present {
				t.Errorf("gradle_version = %v, want absent", got)
			}
			if _, present := metadata.LanguageSpecific["gradle_version_source"]; present {
				t.Error("gradle_version_source present without a version")
			}
		})
	}
}

// Java properties match an exact key, so a longer key that merely starts
// with distributionUrl is a different property and must not be read as
// this one.
func TestWrapperIgnoresSimilarlyNamedKeys(t *testing.T) {
	body := "distributionUrlBackup=https\\://example.com/gradle-7.6-bin.zip\n" +
		"distributionUrl=https\\://services.gradle.org/distributions/gradle-9.7.1-bin.zip\n"

	metadata := wrapperMetadata(t, body)

	got, _ := metadata.LanguageSpecific["gradle_version"].(string)
	if got != "9.7.1" {
		t.Errorf("gradle_version = %q, want 9.7.1 from distributionUrl", got)
	}
}

// A later entry replaces an earlier one, so the last occurrence is the
// effective value.
func TestWrapperTakesTheLastDistributionUrl(t *testing.T) {
	body := "distributionUrl=https\\://services.gradle.org/distributions/gradle-7.6-bin.zip\n" +
		"distributionUrl=https\\://services.gradle.org/distributions/gradle-9.7.1-bin.zip\n"

	metadata := wrapperMetadata(t, body)

	got, _ := metadata.LanguageSpecific["gradle_version"].(string)
	if got != "9.7.1" {
		t.Errorf("gradle_version = %q, want 9.7.1 from the last entry", got)
	}
}

// A commented-out entry is not a value.
func TestWrapperIgnoresComments(t *testing.T) {
	body := "# distributionUrl=https\\://example.com/gradle-7.6-bin.zip\n" +
		"! distributionUrl=https\\://example.com/gradle-7.5-bin.zip\n" +
		"distributionUrl=https\\://services.gradle.org/distributions/gradle-9.7.1-bin.zip\n"

	metadata := wrapperMetadata(t, body)

	got, _ := metadata.LanguageSpecific["gradle_version"].(string)
	if got != "9.7.1" {
		t.Errorf("gradle_version = %q, want 9.7.1", got)
	}
}

// Java properties accept '=', ':' or whitespace as the separator, and
// Gradle's wrapper task only happens to write '='. A hand-edited file
// using another form loads fine for Gradle, so it has to be read here
// too. Each value escapes its scheme colon, which must not be taken for
// the separator.
func TestWrapperAcceptsEverySeparator(t *testing.T) {
	cases := []struct {
		name string
		line string
		want string
	}{
		{
			name: "equals",
			line: `distributionUrl=https\://services.gradle.org/distributions/gradle-9.7.1-bin.zip`,
			want: "9.7.1",
		},
		{
			name: "colon",
			line: `distributionUrl: https\://services.gradle.org/distributions/gradle-8.5-bin.zip`,
			want: "8.5",
		},
		{
			name: "whitespace",
			line: `distributionUrl https\://services.gradle.org/distributions/gradle-8.4-bin.zip`,
			want: "8.4",
		},
		{
			name: "padded equals",
			line: `distributionUrl = https\://services.gradle.org/distributions/gradle-8.14.2-all.zip`,
			want: "8.14.2",
		},
		{
			name: "padded colon",
			line: `distributionUrl : https\://services.gradle.org/distributions/gradle-9.0-bin.zip`,
			want: "9.0",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			metadata := wrapperMetadata(t, properties(tc.line))

			got, _ := metadata.LanguageSpecific["gradle_version"].(string)
			if got != tc.want {
				t.Errorf("gradle_version = %q, want %q", got, tc.want)
			}
		})
	}
}

// The exact-key rule has to hold for every separator, not only '='.
func TestWrapperIgnoresSimilarKeysWithAnySeparator(t *testing.T) {
	body := "distributionUrlBackup: https\\://example.com/gradle-7.6-bin.zip\n" +
		"distributionUrl: https\\://services.gradle.org/distributions/gradle-9.7.1-bin.zip\n"

	metadata := wrapperMetadata(t, body)

	got, _ := metadata.LanguageSpecific["gradle_version"].(string)
	if got != "9.7.1" {
		t.Errorf("gradle_version = %q, want 9.7.1 from distributionUrl", got)
	}
}
