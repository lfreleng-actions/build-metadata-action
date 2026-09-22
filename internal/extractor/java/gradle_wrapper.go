// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package java

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/lfreleng-actions/build-metadata-action/internal/extractor"
)

// wrapperDistributionPattern matches the Gradle version inside a wrapper
// distributionUrl, for example
// https\://services.gradle.org/distributions/gradle-9.7.1-bin.zip.
//
// The version is dotted numeric components, optionally followed by a
// qualifier such as -rc-3 or -milestone-1, and the -bin/-all suffix
// bounds it. Deliberately strict: a permissive tail would accept
// malformed names like gradle-9foo-bin.zip or gradle-8..1-bin.zip and
// report them as versions, which breaks a consumer comparing against a
// tool's floor and contradicts the promise to stay empty when the
// version is unrecognisable.
//
// Anchored at the end for the same reason. Without it the match may sit
// anywhere in the value, so gradle-9.7.1-bin.zip.backup would report
// 9.7.1 for a URL naming no distribution Gradle can download.
//
// A query or fragment may follow the archive, because a signed mirror
// URL ends in something like gradle-9.7.1-bin.zip?token=... and Gradle
// downloads it happily. Only those two may follow: a bare '.' or '/'
// after the suffix names a different file.
var wrapperDistributionPattern = regexp.MustCompile(
	`gradle-([0-9]+(?:\.[0-9]+)*(?:-[A-Za-z][A-Za-z0-9]*(?:-[0-9]+)?)?)-(?:bin|all)\.zip(?:[?#]\S*)?$`)

// splitProperty splits a Java properties line into its key and value.
//
// The format permits '=', ':' or plain whitespace as the separator, and
// Gradle's wrapper task only happens to write '='. A hand-edited file
// using either of the others loads fine for Gradle, so refusing to read
// it here would report no version for a wrapper that works.
//
// Escapes are tracked while scanning, because the value routinely
// contains one: distributionUrl values escape the scheme colon as
// 'https\://', which must not be mistaken for the separator.
//
// Line continuations are not handled. A wrapper URL split across lines
// is vanishingly rare, and the failure is to report nothing, which is
// the safe direction.
//
// A line carrying no separator at all is a key with an empty value,
// which the format allows. Reporting it as a property rather than as
// unparsable matters for duplicates: a bare 'distributionUrl' following
// a populated one clears the value for Gradle, so treating the line as
// absent here would leave the earlier, stale URL standing.
func splitProperty(line string) (string, string, bool) {
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '\\':
			i++ // skip whatever the backslash escapes
		case '=', ':', ' ', '\t', '\f':
			key := strings.TrimSpace(line[:i])
			rest := strings.TrimLeft(line[i+1:], " \t\f")
			// 'key = value' separates with whitespace AND an explicit
			// character, so one may still follow the whitespace.
			if line[i] != '=' && line[i] != ':' && len(rest) > 0 &&
				(rest[0] == '=' || rest[0] == ':') {
				rest = strings.TrimLeft(rest[1:], " \t\f")
			}
			return key, strings.TrimSpace(rest), key != ""
		}
	}

	key := strings.TrimSpace(line)
	return key, "", key != ""
}

// applyGradleWrapper reports the Gradle version the project declares in
// its wrapper.
//
// This is the version the project asks to build with, which is not the
// same fact as the version a CI step provisioned: gradle/actions
// setup-gradle reports what it set up itself, and sets up nothing when a
// build defers to the wrapper. For wrapper-driven projects, which are
// the majority, that output is empty and this is the only statement of
// intent available before a build runs.
//
// Absent when there is no wrapper, or when distributionUrl names no
// recognisable version. Emitting a guess would be worse than staying
// quiet: a consumer comparing against a tool's floor needs to tell
// "too old" from "unknown".
func applyGradleWrapper(projectPath string, metadata *extractor.ProjectMetadata) {
	propertiesPath := filepath.Join(
		projectPath, "gradle", "wrapper", "gradle-wrapper.properties")

	content, err := os.ReadFile(filepath.Clean(propertiesPath))
	if err != nil {
		return
	}

	// Java properties match an exact key, so distributionUrlBackup is a
	// different property rather than this one, and a later entry replaces
	// an earlier one. Scanning to the end rather than stopping at the
	// first match keeps both rules.
	distributionURL := ""
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		key, value, found := splitProperty(line)
		if !found || key != "distributionUrl" {
			continue
		}
		distributionURL = value
	}

	if distributionURL == "" {
		return
	}

	match := wrapperDistributionPattern.FindStringSubmatch(distributionURL)
	if match == nil {
		return
	}

	metadata.LanguageSpecific["gradle_version"] = match[1]
	metadata.LanguageSpecific["gradle_version_source"] =
		"gradle/wrapper/gradle-wrapper.properties"
}
