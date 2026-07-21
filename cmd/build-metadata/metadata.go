// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/lfreleng-actions/build-metadata-action/internal/environment"
)

// Metadata represents the complete metadata collected
type Metadata struct {
	// Common metadata
	Common CommonMetadata `json:"common"`

	// Environment metadata
	Environment environment.Metadata `json:"environment"`

	// Language-specific metadata
	LanguageSpecific map[string]interface{} `json:"language_specific,omitempty"`

	Build BuildMetadata `json:"build"`
}

// CommonMetadata contains metadata common to all project types
type CommonMetadata struct {
	ProjectType    string    `json:"project_type"`
	ProjectName    string    `json:"project_name"`
	ProjectVersion string    `json:"project_version"`
	ProjectPath    string    `json:"project_path"`
	VersionSource  string    `json:"version_source"`
	VersioningType string    `json:"versioning_type"`
	BuildTimestamp time.Time `json:"build_timestamp"`
	GitSHA         string    `json:"git_sha,omitempty"`
	GitBranch      string    `json:"git_branch,omitempty"`
	GitTag         string    `json:"git_tag,omitempty"`
	// VersionPropertiesVersion is the version parsed from a
	// version.properties file (the Linux Foundation / ONAP release
	// convention), extracted independently of whichever source won
	// ProjectVersion, so release tooling can treat the file as
	// authoritative even when a language manifest also declares a
	// version.
	VersionPropertiesVersion string `json:"version_properties_version,omitempty"`
	// VersionPropertiesMatch reports ("true"/"false") whether
	// VersionPropertiesVersion equals ProjectVersion; empty when the
	// project has no usable version.properties file or no project
	// version.
	VersionPropertiesMatch string `json:"version_properties_match,omitempty"`
	// SnapshotVersion is the synthesized interim/development version
	// (the Jenkins-heritage X.Y.Z-SNAPSHOT convention), derived from
	// VersionPropertiesVersion when available, else ProjectVersion.
	SnapshotVersion  string `json:"snapshot_version,omitempty"`
	ProjectMatchRepo bool   `json:"project_match_repo,omitempty"`
}

// BuildMetadata contains build-specific metadata
type BuildMetadata struct {
	CIPlatform string `json:"ci_platform"`
	CIRunID    string `json:"ci_run_id"`
	CIRunURL   string `json:"ci_run_url"`
	RunnerOS   string `json:"runner_os"`
	RunnerArch string `json:"runner_arch"`
}

// newMetadata seeds the metadata with the resolved project path and a
// UTC build timestamp, plus any CI platform values available from the
// environment.
func newMetadata(absPath string) *Metadata {
	return &Metadata{
		Common: CommonMetadata{
			ProjectPath:    absPath,
			BuildTimestamp: time.Now().UTC(),
		},
		Build: BuildMetadata{
			CIPlatform: os.Getenv("CI_PLATFORM"),
			RunnerOS:   os.Getenv("RUNNER_OS"),
			RunnerArch: os.Getenv("RUNNER_ARCH"),
		},
	}
}

// populateCIMetadata fills in GitHub-specific build and git fields when
// running under GitHub Actions.
func populateCIMetadata(metadata *Metadata) {
	if os.Getenv("GITHUB_ACTIONS") != "true" {
		return
	}

	metadata.Build.CIPlatform = "github"
	metadata.Build.CIRunID = os.Getenv("GITHUB_RUN_ID")
	metadata.Build.CIRunURL = fmt.Sprintf("https://github.com/%s/actions/runs/%s",
		os.Getenv("GITHUB_REPOSITORY"),
		os.Getenv("GITHUB_RUN_ID"))

	// Git information from GitHub context
	metadata.Common.GitSHA = os.Getenv("GITHUB_SHA")
	ref := os.Getenv("GITHUB_REF")
	if strings.HasPrefix(ref, "refs/heads/") {
		metadata.Common.GitBranch = strings.TrimPrefix(ref, "refs/heads/")
	} else if strings.HasPrefix(ref, "refs/tags/") {
		metadata.Common.GitTag = strings.TrimPrefix(ref, "refs/tags/")
	}
}

// versionPropertiesMatch returns "true"/"false" comparing the
// version.properties version against the resolved project version,
// or "" when either side is empty (not comparable).
func versionPropertiesMatch(propsVersion, projectVersion string) string {
	if propsVersion == "" || projectVersion == "" {
		return ""
	}
	return fmt.Sprintf("%t", propsVersion == projectVersion)
}

// synthesizeSnapshotVersion derives the interim/development version
// using the Jenkins-heritage X.Y.Z-SNAPSHOT convention. The
// version.properties value is authoritative when present, otherwise
// the resolved project version is used. A base already carrying the
// suffix (case-insensitive, common in Maven/Gradle metadata) passes
// through unchanged rather than double-appending. Returns "" when no
// base version exists.
func synthesizeSnapshotVersion(propsVersion, projectVersion string) string {
	base := propsVersion
	if base == "" {
		base = projectVersion
	}
	if base == "" {
		return ""
	}
	if !strings.HasSuffix(strings.ToUpper(base), "-SNAPSHOT") {
		base += "-SNAPSHOT"
	}
	return base
}

// normalizeProjectTypeToLanguage converts project type variants to base language names
// for consistent output prefixing (e.g., "python-modern" -> "python")
func normalizeProjectTypeToLanguage(projectType string) string {
	// Map project types to their base language
	typeMap := map[string]string{
		"python-modern":      "python",
		"python-legacy":      "python",
		"javascript-npm":     "javascript",
		"javascript-yarn":    "javascript",
		"javascript-pnpm":    "javascript",
		"typescript-npm":     "javascript",
		"java-maven":         "java",
		"java-gradle":        "java",
		"java-gradle-kts":    "java",
		"csharp-project":     "csharp",
		"csharp-solution":    "csharp",
		"csharp-props":       "csharp",
		"dotnet-project":     "dotnet",
		"go-module":          "go",
		"rust-cargo":         "rust",
		"ruby-gemspec":       "ruby",
		"ruby-bundler":       "ruby",
		"php-composer":       "php",
		"swift-package":      "swift",
		"dart-flutter":       "dart",
		"dart-package":       "dart",
		"docker":             "docker",
		"helm-chart":         "helm",
		"terraform":          "terraform",
		"terraform-module":   "terraform",
		"terraform-opentofu": "terraform",
		"c-cmake":            "c",
		"c-autoconf":         "c",
	}

	if normalized, ok := typeMap[projectType]; ok {
		return normalized
	}

	// If no specific mapping, try to extract base by removing suffix after hyphen
	if idx := strings.Index(projectType, "-"); idx > 0 {
		return projectType[:idx]
	}

	// Return as-is if no mapping found
	return strings.ToLower(projectType)
}
