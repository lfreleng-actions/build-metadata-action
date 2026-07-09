// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package version

import (
	"os"
	"path/filepath"
	"testing"
)

// writeFile is a test helper that writes content to a file in dir
func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write %s: %v", name, err)
	}
}

const onapVersionProperties = `# Versioning variables
# Note that these variables cannot be structured
# because they are used in Jenkins

major=0
minor=2
patch=0

base_version=${major}.${minor}.${patch}

release_version=${base_version}
snapshot_version=${base_version}-SNAPSHOT
`

func TestExtractVersionProperties(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantVersion string
		wantOK      bool
	}{
		{
			name:        "ONAP major/minor/patch convention",
			content:     onapVersionProperties,
			wantVersion: "0.2.0",
			wantOK:      true,
		},
		{
			name:        "literal version key",
			content:     "version=1.4.2\n",
			wantVersion: "1.4.2",
			wantOK:      true,
		},
		{
			name:        "literal release_version key",
			content:     "release_version=2.7.1\n",
			wantVersion: "2.7.1",
			wantOK:      true,
		},
		{
			name: "major/minor/patch trio wins over release_version",
			content: "major=1\nminor=1\npatch=0\n" +
				"release_version=9.9.9\n",
			wantVersion: "1.1.0",
			wantOK:      true,
		},
		{
			name:    "interpolated values are ignored",
			content: "version=${base_version}\n",
			wantOK:  false,
		},
		{
			name:    "incomplete major/minor/patch",
			content: "major=1\nminor=2\n",
			wantOK:  false,
		},
		{
			name:    "comments and blank lines only",
			content: "# nothing here\n\n",
			wantOK:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "version.properties", tt.content)

			info, ok := ExtractVersionProperties(dir)
			if ok != tt.wantOK {
				t.Fatalf("ExtractVersionProperties() ok = %v, want %v", ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if info.Version != tt.wantVersion {
				t.Errorf("Version = %q, want %q", info.Version, tt.wantVersion)
			}
			if info.Source != "version.properties" {
				t.Errorf("Source = %q, want version.properties", info.Source)
			}
			if info.IsDynamic {
				t.Errorf("IsDynamic = true, want false")
			}
		})
	}
}

func TestExtractVersionPropertiesMissingFile(t *testing.T) {
	dir := t.TempDir()
	if _, ok := ExtractVersionProperties(dir); ok {
		t.Fatal("ExtractVersionProperties() ok = true for missing file, want false")
	}
}

func TestExtractJavaScriptVersion(t *testing.T) {
	tests := []struct {
		name          string
		packageJSON   string
		versionProps  string
		wantVersion   string
		wantSource    string
		wantIsDynamic bool
	}{
		{
			name: "maintained package.json version",
			packageJSON: `{
  "name": "app",
  "version": "3.1.4"
}`,
			wantVersion: "3.1.4",
			wantSource:  "package.json",
		},
		{
			name: "placeholder 0.0.0 falls through to version.properties",
			packageJSON: `{
  "name": "frontend",
  "version": "0.0.0",
  "private": true
}`,
			versionProps: onapVersionProperties,
			wantVersion:  "0.2.0",
			wantSource:   "version.properties",
		},
		{
			name:        "minified package.json",
			packageJSON: `{"name":"app","version":"2.7.1","private":true}`,
			wantVersion: "2.7.1",
			wantSource:  "package.json",
		},
		{
			name:         "malformed package.json falls through",
			packageJSON:  `{"name": "app", "version": "1.0.0"`,
			versionProps: "major=4\nminor=5\npatch=6\n",
			wantVersion:  "4.5.6",
			wantSource:   "version.properties",
		},
		{
			name: "semantic-release placeholder stays dynamic",
			packageJSON: `{
  "name": "app",
  "version": "0.0.0-development"
}`,
			wantVersion:   "0.0.0-development",
			wantSource:    "package.json",
			wantIsDynamic: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "package.json", tt.packageJSON)
			if tt.versionProps != "" {
				writeFile(t, dir, "version.properties", tt.versionProps)
			}

			info, err := extractJavaScriptVersion(dir)
			if err != nil {
				t.Fatalf("extractJavaScriptVersion() error = %v", err)
			}
			if info.Version != tt.wantVersion {
				t.Errorf("Version = %q, want %q", info.Version, tt.wantVersion)
			}
			if info.Source != tt.wantSource {
				t.Errorf("Source = %q, want %q", info.Source, tt.wantSource)
			}
			if info.IsDynamic != tt.wantIsDynamic {
				t.Errorf("IsDynamic = %v, want %v", info.IsDynamic, tt.wantIsDynamic)
			}
		})
	}
}

func TestExtractBasicTypeScriptProject(t *testing.T) {
	// Regression test: typescript-npm projects must use the
	// JavaScript extraction path (package.json + fallback chain),
	// not fall straight through to git
	dir := t.TempDir()
	writeFile(t, dir, "package.json", "{\n  \"name\": \"frontend\",\n  \"version\": \"0.0.0\"\n}")
	writeFile(t, dir, "version.properties", onapVersionProperties)

	info, err := extractBasic(dir, "typescript-npm")
	if err != nil {
		t.Fatalf("extractBasic() error = %v", err)
	}
	if info.Version != "0.2.0" {
		t.Errorf("Version = %q, want 0.2.0", info.Version)
	}
	if info.Source != "version.properties" {
		t.Errorf("Source = %q, want version.properties", info.Source)
	}
}

func TestExtractFallbackPrefersVersionProperties(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "version.properties", "major=1\nminor=0\npatch=3\n")

	info, err := extractFallback(dir)
	if err != nil {
		t.Fatalf("extractFallback() error = %v", err)
	}
	if info.Version != "1.0.3" {
		t.Errorf("Version = %q, want 1.0.3", info.Version)
	}
	if info.Source != "version.properties" {
		t.Errorf("Source = %q, want version.properties", info.Source)
	}
}

func TestIsPlaceholderVersion(t *testing.T) {
	placeholders := []string{"", "0.0.0"}
	for _, v := range placeholders {
		if !isPlaceholderVersion(v) {
			t.Errorf("isPlaceholderVersion(%q) = false, want true", v)
		}
	}
	real := []string{"1.0.0", "0.0.1", "0.0.0-development"}
	for _, v := range real {
		if isPlaceholderVersion(v) {
			t.Errorf("isPlaceholderVersion(%q) = true, want false", v)
		}
	}
}
