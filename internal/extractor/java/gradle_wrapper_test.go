// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package java

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lfreleng-actions/build-metadata-action/internal/extractor"
)

func wrapperMetadata(t *testing.T, distributionLine string) *extractor.ProjectMetadata {
	t.Helper()
	dir := t.TempDir()

	if distributionLine != "" {
		wrapperDir := filepath.Join(dir, "gradle", "wrapper")
		if err := os.MkdirAll(wrapperDir, 0o750); err != nil {
			t.Fatalf("creating wrapper directory: %v", err)
		}
		body := "distributionBase=GRADLE_USER_HOME\n" +
			distributionLine + "\n" +
			"zipStoreBase=GRADLE_USER_HOME\n"
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
			metadata := wrapperMetadata(t, tc.line)

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

// Absent beats invented. A consumer comparing this against a plugin's
// minimum has to be able to tell "too old" from "unknown", so anything
// unparsable stays quiet rather than guessing.
func TestWrapperVersionAbsentWhenUnknowable(t *testing.T) {
	cases := []struct {
		name string
		line string
	}{
		{"no wrapper at all", ""},
		{"no distributionUrl", "distributionBase=GRADLE_USER_HOME"},
		{
			name: "url names no version",
			line: `distributionUrl=https\://example.com/gradle/latest.zip`,
		},
		{
			name: "version does not start with a digit",
			line: `distributionUrl=https\://example.com/gradle-snapshot-bin.zip`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			metadata := wrapperMetadata(t, tc.line)

			if got, present := metadata.LanguageSpecific["gradle_version"]; present {
				t.Errorf("gradle_version = %v, want absent", got)
			}
			if _, present := metadata.LanguageSpecific["gradle_version_source"]; present {
				t.Error("gradle_version_source present without a version")
			}
		})
	}
}
