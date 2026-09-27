// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// applyReleaseFiles detects release request files under releases/ (the
// global-jjb / Linux Foundation release convention) and records their
// presence. Merging a releases/<name>.yaml file is the trigger for the
// promote/tag lane, so a workflow can gate that lane on is_release_ready.
// When exactly one release file is present its version and ref are surfaced
// directly; with several files the caller disambiguates via the changed
// files, so version and ref are left empty to avoid guessing.
func applyReleaseFiles(metadata *Metadata, absPath string) {
	files := findReleaseFiles(absPath)
	if len(files) == 0 {
		return
	}

	metadata.Common.ReleaseFiles = files
	metadata.Common.ReleaseFileCount = len(files)
	metadata.Common.IsReleaseReady = true

	if len(files) == 1 {
		release := parseReleaseFile(filepath.Join(absPath, files[0]))
		metadata.Common.ReleaseVersion = release.version
		metadata.Common.ReleaseRef = release.ref
		metadata.Common.ReleaseDistributionType = release.distributionType
	}
}

// findReleaseFiles returns repo-relative paths of the YAML files directly
// under releases/, sorted for deterministic output. Sub-directories are
// ignored; the convention places release files at the top of releases/.
func findReleaseFiles(absPath string) []string {
	releasesDir := filepath.Join(absPath, "releases")

	// Reject a symlinked releases/ directory. os.Lstat does not follow the
	// link, so in an untrusted checkout (for example a fork PR) a releases/
	// symlink pointing outside the workspace is treated as "not a directory"
	// and skipped rather than traversed.
	if info, err := os.Lstat(releasesDir); err != nil || !info.IsDir() {
		return nil
	}

	seen := make(map[string]bool)
	var files []string

	for _, pattern := range []string{"*.yaml", "*.yml"} {
		matches, err := filepath.Glob(filepath.Join(releasesDir, pattern))
		if err != nil {
			continue
		}
		for _, match := range matches {
			// os.Lstat (not os.Stat) so a symlinked release file is skipped
			// rather than followed: a crafted symlink could otherwise make
			// parseReleaseFile read a YAML file outside the workspace.
			info, err := os.Lstat(match)
			if err != nil || !info.Mode().IsRegular() {
				continue
			}
			relative := filepath.ToSlash(filepath.Join("releases", filepath.Base(match)))
			if !seen[relative] {
				seen[relative] = true
				files = append(files, relative)
			}
		}
	}

	sort.Strings(files)
	return files
}

// releaseFile holds the top-level scalars a release file declares.
type releaseFile struct {
	version          string
	ref              string
	distributionType string
}

// parseReleaseFile extracts the release version, ref and distribution type
// from a global-jjb release file. It uses a minimal line scan rather than a
// YAML parser because release files keep these as top-level scalars,
// keeping the action dependency-free. Missing keys yield empty strings.
//
// The version key depends on the type, as global-jjb's release-job.sh reads
// it: container release files carry container_release_tag, and every other
// schema (maven, artifact, packagecloud, pypi) carries version. A container
// file never falls back to version, which its schema does not define and
// whose indented per-image occurrences the scan already ignores.
func parseReleaseFile(path string) releaseFile {
	content, err := os.ReadFile(path)
	if err != nil {
		return releaseFile{}
	}

	var release releaseFile
	var version, containerTag string
	for _, line := range strings.Split(string(content), "\n") {
		if value, ok := releaseScalar(line, "version"); ok {
			version = value
		}
		if value, ok := releaseScalar(line, "container_release_tag"); ok {
			containerTag = value
		}
		if value, ok := releaseScalar(line, "ref"); ok {
			release.ref = value
		}
		if value, ok := releaseScalar(line, "distribution_type"); ok {
			release.distributionType = strings.ToLower(value)
		}
	}
	release.version = version
	if release.distributionType == "container" {
		release.version = containerTag
	}
	return release
}

// releaseScalar returns the value of a top-level "key: value" line (no
// leading indentation, so nested keys are ignored), stripped of surrounding
// quotes. ok is false when the line is not that key or the value is empty.
func releaseScalar(line, key string) (string, bool) {
	prefix := key + ":"
	if !strings.HasPrefix(line, prefix) {
		return "", false
	}
	value := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	value = strings.Trim(value, `"'`)
	if value == "" {
		return "", false
	}
	return value, true
}
