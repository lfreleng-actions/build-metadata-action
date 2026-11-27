// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lfreleng-actions/build-metadata-action/internal/detector"
	"github.com/lfreleng-actions/build-metadata-action/internal/extractor/java"
	"github.com/sethvargo/go-githubactions"
)

type versionResult struct{ version, source, versioning string }

// resolveProjectVersion runs version extraction and the project's
// extractor on dir, as run does, and returns the common version fields
// and the extractor's language-specific versioning_type.
func resolveProjectVersion(t *testing.T, dir, projectType string, useExtract bool) (versionResult, any) {
	t.Helper()
	ctx := &appContext{action: githubactions.New(), isCI: false}
	metadata := newMetadata(dir)
	cfg := runConfig{absPath: dir, useVersionExtract: useExtract}
	extractVersionInfo(ctx, cfg, metadata, projectType)
	extractProjectMetadata(ctx, metadata, projectType, dir)
	return versionResult{
		metadata.Common.ProjectVersion,
		metadata.Common.VersionSource,
		metadata.Common.VersioningType,
	}, metadata.LanguageSpecific["versioning_type"]
}

type gradleVersionTest struct {
	name    string
	files   map[string]string
	extract versionResult
}

// gradleVersionTests are the Gradle fixtures. extract is the result
// with use_version_extract on; with it off the result is the same, or
// no version and no source where this names the tag.
var gradleVersionTests = append(append(append(gradleStatementTests,
	gradleScriptTests(gradleNameReads, true)...),
	gradleScriptTests(gradleLocalDeclarations, false)...),
	gradleMidLineTests()...)

// gradleMidLineAssignments assign the version where statements count
// but not first on their line, so the reader does not count them and
// the script computes its version.
var gradleMidLineAssignments = []gradleScript{
	{"allprojects on one line", "build.gradle", "allprojects { version = '2.0.0' }\n"},
	{"version after a semicolon", "build.gradle", "group = 'a'; version = '2.0.0'\n"},
	{"project.version after a semicolon", "build.gradle", "group = 'a'; project.version = '2.0.0'\n"},
	{"unbraced if body on its line", "build.gradle", "if (release) version = '2.0.0'\n"},
	{"command form after a semicolon", "build.gradle", "version = '1.0.0'\ngroup = 'a'; version '2.0.0'\n"},
	{"call form in allprojects on one line", "build.gradle", "version = '1.0.0'\nallprojects { version('2.0.0') }\n"},
}

// gradleMidLineTests runs each of gradleMidLineAssignments alone and
// beside a gradle.properties that holds version=1.0.0: both fall back
// to the tag.
func gradleMidLineTests() []gradleVersionTest {
	want := versionResult{"9.9.9", "git-tag", "dynamic"}
	tests := make([]gradleVersionTest, 0, 2*len(gradleMidLineAssignments))
	for _, sc := range gradleMidLineAssignments {
		tests = append(tests,
			gradleVersionTest{sc.name, map[string]string{sc.file: sc.script}, want},
			gradleVersionTest{sc.name + " beside gradle.properties", map[string]string{
				sc.file:             sc.script,
				"gradle.properties": "version=1.0.0\n",
			}, want})
	}
	return tests
}

type gradleScript struct{ name, file, script string }

// gradleNameReads read, pass or bind the name version or project
// without declaring a local by it, beside a version statement setting
// 1.2.3, which must keep its literal.
var gradleNameReads = []gradleScript{
	{"negated read", "build.gradle", "version = '1.2.3'\ndef isReleaseVersion = !version.endsWith('SNAPSHOT')\n"},
	{"concatenation", "build.gradle", "version = '1.2.3'\next.fullVersion = version + '-x'\n"},
	{"command argument", "build.gradle", "version = '1.2.3'\nprintln version\n"},
	{"comparison", "build.gradle", "version = '1.2.3'\ndef same = other == version\n"},
	{"cast of a read", "build.gradle", "version = '1.2.3'\ndef versionString = version as String\n"},
	{"Elvis read", "build.gradle", "version = '1.2.3'\ndef v = version ?: 'none'\n"},
	{"plugin version in a plugins block", "build.gradle", "plugins { id 'org.example' version '9.9' }\nversion = '1.2.3'\n"},
	{"Kotlin property delegated to project", "build.gradle.kts", "version = \"1.2.3\"\nval kotlinVersion: String by project\n"},
	{"initialiser reading project", "build.gradle", "version = '1.2.3'\nString v = project.findProperty('x')\n"},
	{"named argument after a comma", "build.gradle", "version = '1.2.3'\napply from: 'x.gradle', to: project\n"},
	{"Groovy parameter", "build.gradle", "version = '1.2.3'\ndef bump(String version) { version + '.1' }\n"},
	{"Kotlin parameter", "build.gradle.kts", "version = \"1.2.3\"\nfun bump(version: String) = version\n"},
	{"destructuring a read", "build.gradle", "version = '1.2.3'\ndef (major, minor) = version.tokenize('.')\n"},
	{"allprojects closure parameter", "build.gradle", "allprojects { project ->\n    version = '1.2.3'\n}\n"},
	{"allprojects closure parameter with a default", "build.gradle", "allprojects { project = null ->\n    version = '1.2.3'\n}\n"},
	{"named argument on the next line", "build.gradle", "version = '1.2.3'\nfoo group: 'a',\n    version: '1.0'\n"},
	{"local in a task block", "build.gradle", "task x { def version = 'inner' }\nversion = '1.2.3'\n"},
	{"allprojects above nested closures with parameters", "build.gradle", "allprojects {\n    version = '1.2.3'\n    tasks.withType(Jar) { t -> t.enabled = true }\n    configurations.all { c ->\n        c.transitive = false\n    }\n}\n"},
}

// gradleLocalDeclarations declare a local named version or project
// where statements count, beside a version statement setting 1.2.3, so
// the script computes its version.
var gradleLocalDeclarations = []gradleScript{
	{"def version", "build.gradle", "version = '1.2.3'\ndef version\n"},
	{"String version", "build.gradle", "version = '1.2.3'\nString version\n"},
	{"generic type", "build.gradle", "version = '1.2.3'\nMap<String, String> version\n"},
	{"array type", "build.gradle", "version = '1.2.3'\nString[] version\n"},
	{"qualified type", "build.gradle", "project.version = '1.2.3'\njava.lang.String version = 'helper'\nversion = '8.8.8'\n"},
	{"first of two declarators", "build.gradle", "version = '1.2.3'\nString version, label\n"},
	{"declarator after an initialiser", "build.gradle", "version = '1.2.3'\nString label = 'a', version = 'b'\n"},
	{"declarator after a trailing comma", "build.gradle", "project.version = '1.2.3'\nString label = 'a',\n    version = 'b'\nversion = '8.8.8'\n"},
	{"initialiser on the next line", "build.gradle", "project.version = '1.2.3'\nString label =\n    'a', version = 'b'\nversion = '8.8.8'\n"},
	{"later initialiser on the next line", "build.gradle", "project.version = '1.2.3'\nString label = 'a', suffix =\n    'b', version = 'helper'\nversion = '8.8.8'\n"},
	{"initialiser continued by +", "build.gradle", "project.version = '1.2.3'\nString label = 'a' +\n    'b', version = 'helper'\nversion = '8.8.8'\n"},
	{"initialiser continued by .", "build.gradle", "project.version = '1.2.3'\nString label = 'a'.\n    trim(), version = 'helper'\nversion = '8.8.8'\n"},
	{"project after an initialiser continued by +", "build.gradle", "version = '1.2.3'\nString label = 'a' +\n    'b', project = rootProject.project(':child')\nproject.version = '8.8.8'\n"},
	{"project after an initialiser continued by .", "build.gradle", "version = '1.2.3'\nString label = 'a'.\n    trim(), project = rootProject.project(':child')\nproject.version = '8.8.8'\n"},
	{"second statement on a line", "build.gradle", "version = '1.2.3'\ndef a = 1; def version = 'x'\n"},
	{"def on the line above", "build.gradle", "version = '1.2.3'\ndef\nversion = 'x'\n"},
	{"destructuring", "build.gradle", "version = '1.2.3'\ndef (version, project) = ['helper', rootProject]\n"},
	{"Kotlin var in backticks", "build.gradle.kts", "version = \"1.2.3\"\nvar `version`: String = \"helper\"\n"},
	{"Kotlin delegated val", "build.gradle.kts", "version = \"1.2.3\"\nval version by extra(\"1.0\")\n"},
	{"Kotlin typed val named project", "build.gradle.kts", "version = \"1.2.3\"\nval project: Project = rootProject.project(\":child\")\n"},
	{"def project", "build.gradle", "version = '1.2.3'\ndef project = rootProject.project(':child')\n"},
	{"Project project", "build.gradle", "version = '1.2.3'\nProject project = rootProject.project(':child')\n"},
	{"project declarator after an initialiser", "build.gradle", "version = '1.2.3'\ndef a = 1, project = rootProject.project(':child')\n"},
	{"method named version", "build.gradle", "version = '1.2.3'\ndef version(String v) { }\n"},
	{"def version in allprojects", "build.gradle", "allprojects {\n    def version = 'x'\n}\nversion = '1.2.3'\n"},
	{"allprojects closure parameter named version", "build.gradle", "project.version = '1.2.3'\nallprojects { version ->\n    version = '8.8.8'\n}\n"},
	{"closure parameter named version before a comment", "build.gradle", "project.version = '1.2.3'\nallprojects { version/* p */->\n    version = '8.8.8'\n}\n"},
	{"defaulted closure parameter named version", "build.gradle", "project.version = '1.2.3'\nallprojects { version = null ->\n    version = '8.8.8'\n}\n"},
	{"closure parameter named version before an arrow on the next line", "build.gradle", "project.version = '1.2.3'\nallprojects { version\n    ->\n    version = '8.8.8'\n}\n"},
	{"closure parameter named version with a closure default", "build.gradle", "project.version = '1.2.3'\nallprojects { version = {} ->\n    version = '8.8.8'\n}\n"},
	{"closure default on the line after the brace", "build.gradle", "project.version = '1.2.3'\nallprojects {\n    version = {} ->\n    version = '8.8.8'\n}\n"},
	{"call passing the name after a comma", "build.gradle", "version = '1.2.3'\nLogger.info \"v\", version\n"},
	{"name after a trailing comma", "build.gradle", "version = '1.2.3'\napply from: 'a.gradle',\n    version = '8.8.8'\n"},
}

// gradleScriptTests turns scripts into fixtures: each keeps its 1.2.3
// literal, or each computes its version and falls back to the tag.
func gradleScriptTests(scripts []gradleScript, keeps bool) []gradleVersionTest {
	tests := make([]gradleVersionTest, 0, len(scripts))
	for _, sc := range scripts {
		want := versionResult{"9.9.9", "git-tag", "dynamic"}
		if keeps {
			want = versionResult{"1.2.3", sc.file, "static"}
		}
		tests = append(tests, gradleVersionTest{sc.name, map[string]string{sc.file: sc.script}, want})
	}
	return tests
}

// gradleStatementTests are the fixtures written out in full.
var gradleStatementTests = []gradleVersionTest{
	{
		name:    "G1 command form",
		files:   map[string]string{"build.gradle": "version '1.2.3'\n"},
		extract: versionResult{"1.2.3", "build.gradle", "static"},
	},
	{
		name:    "G2 project.version",
		files:   map[string]string{"build.gradle": "project.version = '1.2.3'\n"},
		extract: versionResult{"1.2.3", "build.gradle", "static"},
	},
	{
		name:    "G3 trailing comment",
		files:   map[string]string{"build.gradle": "version = '1.2.3' // bump on release\n"},
		extract: versionResult{"1.2.3", "build.gradle", "static"},
	},
	{
		name: "G4 versionJunit in ext",
		files: map[string]string{"build.gradle": `ext {
    versionJunit = "5.10.0"
}
version = "2.1.0"
`},
		extract: versionResult{"2.1.0", "build.gradle", "static"},
	},
	{
		name: "G5 versions map in ext",
		files: map[string]string{"build.gradle": `ext {
    versions = [
        junit: "5.10.0",
    ]
}
version = "2.1.0"
`},
		extract: versionResult{"2.1.0", "build.gradle", "static"},
	},
	{
		name: "G6 versionCode in android",
		files: map[string]string{"build.gradle.kts": `android {
    defaultConfig {
        versionCode = 42
    }
}
version = "2.1.0"
`},
		extract: versionResult{"2.1.0", "build.gradle.kts", "static"},
	},
	{
		name: "G7 plugin version",
		files: map[string]string{"build.gradle": `plugins {
    id 'org.springframework.boot' version '3.2.0'
}
version = '2.1.0'
`},
		extract: versionResult{"2.1.0", "build.gradle", "static"},
	},
	{
		name:    "G8 line comment",
		files:   map[string]string{"build.gradle": "// version = '0.0.1'\nversion = '2.1.0'\n"},
		extract: versionResult{"2.1.0", "build.gradle", "static"},
	},
	{
		name:    "G9 block comment",
		files:   map[string]string{"build.gradle": "/*\nversion = '0.0.1'\n*/\nversion = '2.1.0'\n"},
		extract: versionResult{"2.1.0", "build.gradle", "static"},
	},
	{
		name: "G10 publication version",
		files: map[string]string{"build.gradle": `publishing {
    publications {
        mavenJava(MavenPublication) {
            version = "0.0.1-pub"
        }
    }
}
version = "2.1.0"
`},
		extract: versionResult{"2.1.0", "build.gradle", "static"},
	},
	{
		name:    "G11 subprojects version only",
		files:   map[string]string{"build.gradle": "subprojects {\n    version = \"7.7.7\"\n}\n"},
		extract: versionResult{"9.9.9", "git-tag", "static"},
	},
	{
		name:    "G12 allprojects version",
		files:   map[string]string{"build.gradle": "allprojects {\n    version = \"1.2.3\"\n}\n"},
		extract: versionResult{"1.2.3", "build.gradle", "static"},
	},
	{
		name:    "G13 rootProject.version",
		files:   map[string]string{"build.gradle": "version = rootProject.version\n"},
		extract: versionResult{"9.9.9", "git-tag", "dynamic"},
	},
	{
		name:    "G14 interpolated string",
		files:   map[string]string{"build.gradle": "version = \"1.2.3-${buildNumber}\"\n"},
		extract: versionResult{"9.9.9", "git-tag", "dynamic"},
	},
	{
		name:    "G15 version catalogue",
		files:   map[string]string{"build.gradle.kts": "version = libs.versions.app.get()\n"},
		extract: versionResult{"9.9.9", "git-tag", "dynamic"},
	},
	{
		name: "G16 gradle.properties",
		files: map[string]string{
			"build.gradle":      "plugins {\n    id 'java'\n}\n",
			"gradle.properties": "version=1.2.3\n",
		},
		extract: versionResult{"1.2.3", "gradle.properties", "static"},
	},
	{
		name: "G17 computed script beside gradle.properties",
		files: map[string]string{
			"build.gradle":      "version = rootProject.version\n",
			"gradle.properties": "version=1.2.3\n",
		},
		extract: versionResult{"9.9.9", "git-tag", "dynamic"},
	},
	{
		name:    "G18 SNAPSHOT",
		files:   map[string]string{"build.gradle": "version = '1.2.3-SNAPSHOT'\n"},
		extract: versionResult{"1.2.3-SNAPSHOT", "build.gradle", "dynamic"},
	},
	{
		name:    "G19 no version",
		files:   map[string]string{"build.gradle": "plugins {\n    id 'java'\n}\n"},
		extract: versionResult{"9.9.9", "git-tag", "static"},
	},
	{
		name: "script literal outranks gradle.properties",
		files: map[string]string{
			"build.gradle":      "version = \"2.0.0\"\n",
			"gradle.properties": "version=1.0.0\n",
		},
		extract: versionResult{"2.0.0", "build.gradle", "static"},
	},
	{
		name:    "last literal decides",
		files:   map[string]string{"build.gradle": "version = '1.0.0'\nversion = '2.0.0'\n"},
		extract: versionResult{"2.0.0", "build.gradle", "static"},
	},
	{
		name:    "last computed statement decides",
		files:   map[string]string{"build.gradle": "version = '1.0.0'\nversion = rootProject.version\n"},
		extract: versionResult{"9.9.9", "git-tag", "dynamic"},
	},
	{
		name: "last version key in gradle.properties decides",
		files: map[string]string{
			"build.gradle":      "plugins {\n    id 'java'\n}\n",
			"gradle.properties": "version=1.0.0\nversion=2.0.0\n",
		},
		extract: versionResult{"2.0.0", "gradle.properties", "static"},
	},
	{
		name: "SNAPSHOT in gradle.properties",
		files: map[string]string{
			"build.gradle":      "plugins {\n    id 'java'\n}\n",
			"gradle.properties": "version=1.2.3-SNAPSHOT\n",
		},
		extract: versionResult{"1.2.3-SNAPSHOT", "gradle.properties", "dynamic"},
	},
	{
		name:    "unterminated block comment",
		files:   map[string]string{"build.gradle": "version = '1.2.3'\n/* never closed\n"},
		extract: versionResult{"9.9.9", "git-tag", "dynamic"},
	},
	{
		name:    "unterminated triple-quoted string",
		files:   map[string]string{"build.gradle": "version = '1.2.3'\ndef text = \"\"\"never closed\n"},
		extract: versionResult{"9.9.9", "git-tag", "dynamic"},
	},
	{
		name:    "one closing brace too many",
		files:   map[string]string{"build.gradle": "version = '1.2.3'\n}\n"},
		extract: versionResult{"9.9.9", "git-tag", "dynamic"},
	},
	{
		name: "slashy string with an odd number of quotes",
		files: map[string]string{"build.gradle": `def matcher = text =~ /x\s*=\s*"([^"]+)"/
version = '1.2.3'
`},
		extract: versionResult{"1.2.3", "build.gradle", "static"},
	},
	{
		name: "slashy string across lines",
		files: map[string]string{"build.gradle": `version = '1.2.3'
def template = /
version = '9.9.9'
/
`},
		extract: versionResult{"1.2.3", "build.gradle", "static"},
	},
	{
		name: "slashy string after a block comment",
		files: map[string]string{"build.gradle": `version = '1.2.3'
def template = /* template */ /prefix
version = '9.9.9'
suffix/
`},
		extract: versionResult{"1.2.3", "build.gradle", "static"},
	},
	{
		name:    "unterminated slashy string",
		files:   map[string]string{"build.gradle": "version = '1.2.3'\ndef template = /never closed\n"},
		extract: versionResult{"9.9.9", "git-tag", "dynamic"},
	},
	{
		name:    "slashy string holding a brace",
		files:   map[string]string{"build.gradle": "def r = /\\{/\nversion = '1.2.3'\n"},
		extract: versionResult{"1.2.3", "build.gradle", "static"},
	},
	{
		name:    "division by a number",
		files:   map[string]string{"build.gradle": "def half = total / 2\nversion = '1.2.3'\n"},
		extract: versionResult{"1.2.3", "build.gradle", "static"},
	},
	{
		name: "dollar-slashy string across lines",
		files: map[string]string{"build.gradle": `version = '1.0.0'
def template = $/
version = '9.9.9'
/$
`},
		extract: versionResult{"1.0.0", "build.gradle", "static"},
	},
	{
		name:    "division is not a slashy string",
		files:   map[string]string{"build.gradle": "def half = (total) / 2 / 1\nversion = '1.2.3'\n"},
		extract: versionResult{"1.2.3", "build.gradle", "static"},
	},
	{
		name:    "parenthesised value with no space after =",
		files:   map[string]string{"build.gradle": "version = '1.0.0'\nversion=(\"2.0.0\")\n"},
		extract: versionResult{"9.9.9", "git-tag", "dynamic"},
	},
	{
		name:    "negative value with no space after =",
		files:   map[string]string{"build.gradle": "version = '1.0.0'\nversion=-1\n"},
		extract: versionResult{"9.9.9", "git-tag", "dynamic"},
	},
	{
		name:    "compound assignment after a literal",
		files:   map[string]string{"build.gradle": "version = '1.2.3'\nversion += '-SNAPSHOT'\n"},
		extract: versionResult{"9.9.9", "git-tag", "dynamic"},
	},
	{
		name:    "compound assignment to project.version in Kotlin",
		files:   map[string]string{"build.gradle.kts": "version = \"1.2.3\"\nproject.version += \"-rc\"\n"},
		extract: versionResult{"9.9.9", "git-tag", "dynamic"},
	},
	{
		name:    "elvis assignment after a literal",
		files:   map[string]string{"build.gradle": "version = '1.2.3'\nversion ?= '2.0.0'\n"},
		extract: versionResult{"9.9.9", "git-tag", "dynamic"},
	},
	{
		name:    "literal after a compound assignment",
		files:   map[string]string{"build.gradle": "version += '-SNAPSHOT'\nversion = '1.2.3'\n"},
		extract: versionResult{"1.2.3", "build.gradle", "static"},
	},
	{
		name:    "local named version in a nested block",
		files:   map[string]string{"build.gradle": "task foo {\n    def version = 'x'\n}\nversion = '1.2.3'\n"},
		extract: versionResult{"1.2.3", "build.gradle", "static"},
	},
	{
		name:    "property named version on a type is no local",
		files:   map[string]string{"build.gradle": "Build.version = 'x'\nversion = '1.2.3'\n"},
		extract: versionResult{"1.2.3", "build.gradle", "static"},
	},
	{
		name:    "call passing version declares no local",
		files:   map[string]string{"build.gradle": "project.version = '1.2.3'\nSystem.out.println(version)\nversion = '2.0.0'\n"},
		extract: versionResult{"2.0.0", "build.gradle", "static"},
	},
	{
		name:    "later initialiser on the next line with no later declarator",
		files:   map[string]string{"build.gradle": "project.version = '1.2.3'\nString label = 'a', suffix =\n    'b'\nversion = '8.8.8'\n"},
		extract: versionResult{"8.8.8", "build.gradle", "static"},
	},
	{
		name:    "initialiser on the next line with no later declarator",
		files:   map[string]string{"build.gradle": "project.version = '1.2.3'\nString label =\n    'a'\nversion = '8.8.8'\n"},
		extract: versionResult{"8.8.8", "build.gradle", "static"},
	},
	{
		name:    "declaration ending its line without a comma",
		files:   map[string]string{"build.gradle": "project.version = '1.2.3'\nString label = 'a'\nversion = '8.8.8'\n"},
		extract: versionResult{"8.8.8", "build.gradle", "static"},
	},
	{
		name:    "version in a list initialiser declares no local",
		files:   map[string]string{"build.gradle": "project.version = '1.2.3'\ndef pair = [label, version]\nversion = '2.0.0'\n"},
		extract: versionResult{"2.0.0", "build.gradle", "static"},
	},
	{
		name:    "Kotlin version in backticks",
		files:   map[string]string{"build.gradle.kts": "version = \"1.2.3\"\n`version` = \"2.0.0\"\nproject.`version` = \"3.0.0\"\n"},
		extract: versionResult{"3.0.0", "build.gradle.kts", "static"},
	},
	{
		name:    "Kotlin keyword in backticks is a name",
		files:   map[string]string{"build.gradle.kts": "`if`(true)\nversion = \"1.2.3\"\n"},
		extract: versionResult{"1.2.3", "build.gradle.kts", "static"},
	},
	{
		name:    "Kotlin name in backticks that does not close",
		files:   map[string]string{"build.gradle.kts": "version = \"1.2.3\"\nval `x = 1\n"},
		extract: versionResult{"9.9.9", "git-tag", "dynamic"},
	},
	{
		name:    "version inside a declaration's call declares no local",
		files:   map[string]string{"build.gradle": "def name = tool(a, version = 'x')\nversion = '1.2.3'\n"},
		extract: versionResult{"1.2.3", "build.gradle", "static"},
	},
	{
		name:    "comparison is not a compound assignment",
		files:   map[string]string{"build.gradle": "version = '1.2.3'\nversion >= '1.0'\n"},
		extract: versionResult{"1.2.3", "build.gradle", "static"},
	},
	{
		name: "Kotlin named argument across lines",
		files: map[string]string{"build.gradle.kts": `version = "1.2.3"
tool(
    version = "9.9.9"
)
`},
		extract: versionResult{"1.2.3", "build.gradle.kts", "static"},
	},
	{
		name: "Groovy list across lines",
		files: map[string]string{"build.gradle": `version = '1.2.3'
def entries = [
    version = '9.9.9'
]
`},
		extract: versionResult{"1.2.3", "build.gradle", "static"},
	},
	{
		name:    "unbalanced parenthesis",
		files:   map[string]string{"build.gradle": "version = '1.2.3'\nfoo(\n"},
		extract: versionResult{"9.9.9", "git-tag", "dynamic"},
	},
	{
		name: "unbraced if body",
		files: map[string]string{"build.gradle.kts": `version = "1.2.3"
if (false)
    version = "9.9.9"
`},
		extract: versionResult{"9.9.9", "git-tag", "dynamic"},
	},
	{
		name: "unbraced else body",
		files: map[string]string{"build.gradle": `version = '1.2.3'
if (release) println('release')
else
    // fallback
    version '9.9.9'
`},
		extract: versionResult{"9.9.9", "git-tag", "dynamic"},
	},
	{
		name: "statement after a call that closes on its own line",
		files: map[string]string{"build.gradle.kts": `configure(
    "x"
)
version = "1.2.3"
`},
		extract: versionResult{"1.2.3", "build.gradle.kts", "static"},
	},
	{
		name: "statement after an unbraced if body",
		files: map[string]string{"build.gradle": `if (release)
    println('release')
version = '1.2.3'
`},
		extract: versionResult{"1.2.3", "build.gradle", "static"},
	},
	{
		name: "nested string in a Kotlin interpolation",
		files: map[string]string{"build.gradle.kts": `val app = "${project.property("name")}-app"
version = "1.2.3"
`},
		extract: versionResult{"1.2.3", "build.gradle.kts", "static"},
	},
	{
		name:    "unterminated quoted string",
		files:   map[string]string{"build.gradle": "version = '1.2.3'\ndef text = \"never closed\n"},
		extract: versionResult{"9.9.9", "git-tag", "dynamic"},
	},
	{
		name: "unbraced if body after a comment in the header",
		files: map[string]string{"build.gradle": `version = '1.2.3'
if /* explanation */ (false)
    version = "8.8.8"
`},
		extract: versionResult{"9.9.9", "git-tag", "dynamic"},
	},
	{
		name: "allprojects as an unbraced if body",
		files: map[string]string{"build.gradle": `version = '1.2.3'
if (false)
    allprojects {
        version = '8.8.8'
    }
`},
		extract: versionResult{"9.9.9", "git-tag", "dynamic"},
	},
	{
		name: "allprojects after a braced if",
		files: map[string]string{"build.gradle": `if (release) {
    println('release')
}
allprojects /* every project */ {
    version = '1.2.3'
}
`},
		extract: versionResult{"1.2.3", "build.gradle", "static"},
	},
	{
		name:    "unterminated triple-quoted string ending in a backslash",
		files:   map[string]string{"build.gradle": "version = '1.2.3'\ndef text = \"\"\"never closed\\"},
		extract: versionResult{"9.9.9", "git-tag", "dynamic"},
	},
	{
		name: "child version after a trailing dot",
		files: map[string]string{"build.gradle": `version = "1.2.3"
project(":child").
    version = "8.8.8"
`},
		extract: versionResult{"1.2.3", "build.gradle", "static"},
	},
	{
		name: "child allprojects after a trailing dot",
		files: map[string]string{"build.gradle": `version = "1.2.3"
project(":child").

    allprojects {
        version = "8.8.8"
    }
`},
		extract: versionResult{"1.2.3", "build.gradle", "static"},
	},
	{
		name: "call form after a comment",
		files: map[string]string{"build.gradle": `version = '1.0.0'
version/* release */('2.0.0')
`},
		extract: versionResult{"2.0.0", "build.gradle", "static"},
	},
	{
		name: "computed call form after a space",
		files: map[string]string{"build.gradle": `version = '1.0.0'
version (computeVersion())
`},
		extract: versionResult{"9.9.9", "git-tag", "dynamic"},
	},
	{
		name: "slashy string after assert",
		files: map[string]string{"build.gradle": `version = '1.2.3'
assert /
version = '8.8.8'
/
`},
		extract: versionResult{"1.2.3", "build.gradle", "static"},
	},
	{
		name:    "slashy string after a contextual keyword",
		files:   map[string]string{"build.gradle": "version = '1.2.3'\nassert name in /\nversion = '8.8.8'\n/\n"},
		extract: versionResult{"1.2.3", "build.gradle", "static"},
	},
	{
		name:    "slashy string after return",
		files:   map[string]string{"build.gradle": "version = '1.2.3'\nreturn /\nversion = '8.8.8'\n/\n"},
		extract: versionResult{"1.2.3", "build.gradle", "static"},
	},
	{
		name:    "division after this",
		files:   map[string]string{"build.gradle": "def half = this / 2\nversion = '1.2.3'\ndef x = 1 / 2\n"},
		extract: versionResult{"1.2.3", "build.gradle", "static"},
	},
	{
		name:    "division after an increment",
		files:   map[string]string{"build.gradle": "def half = count++ / 2\nversion = '1.2.3'\ndef x = 1 / 2\n"},
		extract: versionResult{"1.2.3", "build.gradle", "static"},
	},
	{
		name:    "division by a property named after a keyword",
		files:   map[string]string{"build.gradle": "def ratio = total / meta.return\nversion = '1.2.3'\ndef x = 1 / 2\n"},
		extract: versionResult{"1.2.3", "build.gradle", "static"},
	},
	{
		name: "division by a keyword-named property after white space",
		files: map[string]string{"build.gradle": `version = '1.2.3'
def a = meta. return / 2
version = '4.5.6'
def b = c / 2
`},
		extract: versionResult{"4.5.6", "build.gradle", "static"},
	},
	{
		name: "division by a keyword-named property after a comment",
		files: map[string]string{"build.gradle": `version = '1.2.3'
def a = meta./* numerator */return / 2
version = '4.5.6'
def b = c / 2
`},
		extract: versionResult{"4.5.6", "build.gradle", "static"},
	},
	{
		name: "unbraced if body with the header split over lines",
		files: map[string]string{"build.gradle": `version = '1.2.3'
if
(false)
    version = "8.8.8"
`},
		extract: versionResult{"9.9.9", "git-tag", "dynamic"},
	},
	{
		name: "slashy string inside an interpolation",
		files: map[string]string{"build.gradle": `version = '1.2.3'
def template = """${ /}
"""
version = '8.8.8'
def dummy = """
/ }"""
`},
		extract: versionResult{"1.2.3", "build.gradle", "static"},
	},
	{
		name: "division after a non-ASCII name",
		files: map[string]string{"build.gradle": `version = '1.2.3'
def a = café / 2
version = '4.5.6'
def b = c / 2
`},
		extract: versionResult{"4.5.6", "build.gradle", "static"},
	},
	{
		name: "version after a do-while loop",
		files: map[string]string{"build.gradle": `do { println("once") } while (false)
version = "1.2.3"
`},
		extract: versionResult{"1.2.3", "build.gradle", "static"},
	},
	{
		name: "unbraced while body after a braced if",
		files: map[string]string{"build.gradle": `version = '1.2.3'
if (x) { println("x") }
while (false)
    version = "8.8.8"
`},
		extract: versionResult{"9.9.9", "git-tag", "dynamic"},
	},
	{
		name:    "value continued on the next line",
		files:   map[string]string{"build.gradle.kts": "version = \"1.2.3\"\n    .trim()\n"},
		extract: versionResult{"9.9.9", "git-tag", "dynamic"},
	},
}

// TestGradleProjectVersion checks project_version, version_source and
// versioning_type for Gradle projects under both settings of
// use_version_extract. Each fixture is a git repository tagged v9.9.9,
// so "9.9.9, git-tag" is the fallback; with use_version_extract off
// there is no fallback. The extractor's versioning_type must always
// equal the common one.
func TestGradleProjectVersion(t *testing.T) {
	for _, tt := range gradleVersionTests {
		dir := gitFixture(t, tt.files)
		projectType, err := detector.DetectProjectType(dir)
		if err != nil {
			t.Fatalf("%s: DetectProjectType: %v", tt.name, err)
		}
		noExtract := tt.extract
		if tt.extract.source == "git-tag" {
			noExtract = versionResult{"", "", tt.extract.versioning}
		}
		for _, useExtract := range []bool{true, false} {
			want := noExtract
			if useExtract {
				want = tt.extract
			}
			got, languageType := resolveProjectVersion(t, dir, projectType, useExtract)
			if got != want {
				t.Errorf("%s (use_version_extract=%v): got %+v, want %+v",
					tt.name, useExtract, got, want)
			}
			if languageType != got.versioning {
				t.Errorf("%s (use_version_extract=%v): versioning_type %q, language-specific %v",
					tt.name, useExtract, got.versioning, languageType)
			}
		}
	}
}

// TestGradleScriptPrefixesDoNotPanic cuts each Gradle script above at
// every byte and requires that the extractor reads every prefix
// without panicking: a script cut short is where a lexer runs off its
// end.
func TestGradleScriptPrefixesDoNotPanic(t *testing.T) {
	gradle := java.NewGradleExtractor()
	dir := t.TempDir()
	for _, tt := range gradleVersionTests {
		for name, script := range tt.files {
			if name != "build.gradle" && name != "build.gradle.kts" {
				continue
			}
			path := filepath.Join(dir, name)
			for n := 0; n <= len(script); n++ {
				if err := os.WriteFile(path, []byte(script[:n]), 0o600); err != nil {
					t.Fatal(err)
				}
				func() {
					defer func() {
						if r := recover(); r != nil {
							t.Errorf("%s: %s cut at byte %d: panic: %v", tt.name, name, n, r)
						}
					}()
					_, _ = gradle.Extract(dir)
				}()
			}
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
		}
	}
}

const kotlinBuildScript = `plugins { java }
group = "org.example"
version = "4.5.6"
`

// The detector returns kotlin-gradle for a build.gradle.kts root, and
// java-gradle-kts describes the same tree; both must read the version
// from the build script rather than the tag.
func TestKotlinGradleReadsTheBuildScript(t *testing.T) {
	dir := gitFixture(t, map[string]string{"build.gradle.kts": kotlinBuildScript})
	want := versionResult{"4.5.6", "build.gradle.kts", "static"}
	for _, projectType := range []string{"kotlin-gradle", "java-gradle-kts"} {
		for _, useExtract := range []bool{true, false} {
			if got, _ := resolveProjectVersion(t, dir, projectType, useExtract); got != want {
				t.Errorf("%s (use_version_extract=%v): got %+v, want %+v",
					projectType, useExtract, got, want)
			}
		}
	}
}

// A project mid-migration carries both scripts. The Kotlin one wins,
// matching the detector, which ranks kotlin-gradle above java-gradle.
func TestMixedDSLPrefersTheKotlinScript(t *testing.T) {
	dir := gitFixture(t, map[string]string{
		"build.gradle.kts": kotlinBuildScript,
		"build.gradle":     "group = 'org.example'\nversion = '7.7.7'\n",
	})
	want := versionResult{"4.5.6", "build.gradle.kts", "static"}
	for _, useExtract := range []bool{true, false} {
		if got, _ := resolveProjectVersion(t, dir, "kotlin-gradle", useExtract); got != want {
			t.Errorf("use_version_extract=%v: got %+v, want %+v", useExtract, got, want)
		}
	}
}
