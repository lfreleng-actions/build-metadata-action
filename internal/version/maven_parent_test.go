// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package version

import (
	"fmt"
	"testing"
)

const pomWithParent = `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
  <modelVersion>4.0.0</modelVersion>
  <parent>
    <groupId>org.example</groupId>
    <artifactId>example-parent</artifactId>
    <version>15.1.1-SNAPSHOT</version>
    <relativePath>parent</relativePath>
  </parent>
  <artifactId>example-aggregator</artifactId>
%s
  <dependencies>
    <dependency>
      <groupId>org.example</groupId>
      <artifactId>dep</artifactId>
      <version>9.9.9</version>
    </dependency>
  </dependencies>
</project>
`

// A POM that declares no version of its own inherits its parent's, as
// Maven does; a declared version still wins over the parent's.
func TestExtractMavenVersionParentInheritance(t *testing.T) {
	tests := []struct {
		name     string
		declared string
		want     string
	}{
		{name: "inherits parent version", declared: "", want: "15.1.1-SNAPSHOT"},
		{name: "declared version wins", declared: "  <version>2.0.0</version>", want: "2.0.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "pom.xml", fmt.Sprintf(pomWithParent, tt.declared))
			info, err := extractMavenVersion(dir)
			if err != nil {
				t.Fatalf("extractMavenVersion() error = %v", err)
			}
			if info.Version != tt.want {
				t.Errorf("Version = %q, want %q", info.Version, tt.want)
			}
			if info.Source != "pom.xml" {
				t.Errorf("Source = %q, want pom.xml", info.Source)
			}
		})
	}
}

// A POM may declare any IANA charset in its XML prolog; older Maven
// projects commonly use ISO-8859-1. The declared version must still come
// from pom.xml, including when the file holds non-ASCII text in that
// charset, and an unknown charset falls back like any unparsable POM.
func TestExtractMavenVersionDeclaredEncoding(t *testing.T) {
	// \xe9 is "é" in ISO-8859-1 and windows-1252; \x80 is "€" in
	// windows-1252. Neither byte is valid UTF-8 on its own.
	pom := func(encoding, name string) string {
		return `<?xml version="1.0" encoding="` + encoding + `"?>
<project>
  <name>` + name + `</name>
  <!-- ` + name + ` -->
  <version>1.2.3</version>
</project>
`
	}
	tests := []struct {
		name       string
		pom        string
		want       string
		wantSource string
	}{
		{name: "ISO-8859-1 with non-ASCII text", pom: pom("ISO-8859-1", "R\xe9seau"), want: "1.2.3", wantSource: "pom.xml"},
		{name: "lower-case label", pom: pom("iso-8859-1", "R\xe9seau"), want: "1.2.3", wantSource: "pom.xml"},
		{name: "windows-1252 with non-ASCII text", pom: pom("windows-1252", "\x80uro"), want: "1.2.3", wantSource: "pom.xml"},
		{name: "US-ASCII", pom: pom("US-ASCII", "plain"), want: "1.2.3", wantSource: "pom.xml"},
		{name: "unknown charset falls back", pom: pom("x-no-such-charset", "plain"), want: "3.2.1", wantSource: "version.properties"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "pom.xml", tt.pom)
			writeFile(t, dir, "version.properties", "major=3\nminor=2\npatch=1\n")
			info, err := extractMavenVersion(dir)
			if err != nil {
				t.Fatalf("extractMavenVersion() error = %v", err)
			}
			if info.Version != tt.want || info.Source != tt.wantSource {
				t.Errorf("got %q from %s, want %q from %s", info.Version, info.Source, tt.want, tt.wantSource)
			}
		})
	}
}

// Maven reads <project><version> and <parent><version> wherever they sit
// among <project>'s children, so element order must not change the
// answer, and versions nested in dependencies, plugins or profiles must
// never count. Cases without a usable direct version fall back to
// version.properties.
func TestExtractMavenVersionElementOrder(t *testing.T) {
	const parent = `<parent><groupId>g</groupId><artifactId>p</artifactId><version>1.0.0</version></parent>`
	const build = `<build><plugins><plugin><artifactId>x</artifactId><version>8.8.8</version></plugin></plugins></build>`
	const deps = `<dependencies><dependency><artifactId>d</artifactId><version>9.9.9</version></dependency></dependencies>`
	const profiles = `<profiles><profile><id>p</id><properties><version>7.7.7</version></properties></profile></profiles>`
	tests := []struct {
		name       string
		pom        string
		want       string
		wantSource string
	}{
		{
			name:       "project version after build wins over parent",
			pom:        `<project>` + parent + `<artifactId>a</artifactId>` + build + `<version>2.0.0</version></project>`,
			want:       "2.0.0",
			wantSource: "pom.xml",
		},
		{
			name:       "project version after dependencies wins over parent",
			pom:        `<project>` + parent + deps + `<version>2.0.0</version></project>`,
			want:       "2.0.0",
			wantSource: "pom.xml",
		},
		{
			name:       "parent after build is still inherited",
			pom:        `<project xmlns="http://maven.apache.org/POM/4.0.0"><artifactId>a</artifactId>` + build + deps + parent + `</project>`,
			want:       "1.0.0",
			wantSource: "pom.xml",
		},
		{
			name:       "nested versions alone are not the project version",
			pom:        `<project><artifactId>a</artifactId>` + build + deps + profiles + `</project>`,
			want:       "3.2.1",
			wantSource: "version.properties",
		},
		{
			name:       "malformed POM falls back",
			pom:        `<project><version>2.0.0</version>`,
			want:       "3.2.1",
			wantSource: "version.properties",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "pom.xml", tt.pom)
			writeFile(t, dir, "version.properties", "major=3\nminor=2\npatch=1\n")
			info, err := extractMavenVersion(dir)
			if err != nil {
				t.Fatalf("extractMavenVersion() error = %v", err)
			}
			if info.Version != tt.want || info.Source != tt.wantSource {
				t.Errorf("got %q from %s, want %q from %s", info.Version, info.Source, tt.want, tt.wantSource)
			}
		})
	}
}
