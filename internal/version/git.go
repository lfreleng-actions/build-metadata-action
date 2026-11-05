// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package version

import (
	"fmt"
	"os/exec"
	"strings"
)

// ensureTagsAreFetched attempts to fetch git tags from remote
// This is useful in CI environments with shallow clones where tags aren't fetched by default
func ensureTagsAreFetched(projectPath string) {
	// Check if this is a git repository
	cmd := exec.Command("git", "-C", projectPath, "rev-parse", "--git-dir")
	if err := cmd.Run(); err != nil {
		// Not a git repo, skip
		return
	}

	// Try to fetch tags quietly - don't fail if this doesn't work
	// (repo might be offline, or tags might already be present)
	cmd = exec.Command("git", "-C", projectPath, "fetch", "--tags", "--quiet")
	_ = cmd.Run() // Ignore errors - this is best-effort
}

// extractFromGit extracts version from git tags as a fallback
func extractFromGit(projectPath string) (*VersionInfo, error) {
	// Ensure tags are fetched (important for CI environments)
	ensureTagsAreFetched(projectPath)

	// Try to get the latest git tag
	cmd := exec.Command("git", "-C", projectPath, "describe", "--tags", "--abbrev=0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		// If no tags, try to get a short commit hash
		cmd = exec.Command("git", "-C", projectPath, "rev-parse", "--short", "HEAD")
		output, err = cmd.CombinedOutput()
		if err != nil {
			return &VersionInfo{
				Version:   "unknown",
				Source:    "none",
				IsDynamic: false,
			}, fmt.Errorf("could not determine version")
		}

		return &VersionInfo{
			Version:   "0.0.0-dev+" + strings.TrimSpace(string(output)),
			Source:    "git-commit",
			IsDynamic: true,
		}, nil
	}

	version := strings.TrimSpace(string(output))
	// Remove 'v' prefix if present
	version = strings.TrimPrefix(version, "v")

	return &VersionInfo{
		Version:   version,
		Source:    "git-tag",
		IsDynamic: false,
	}, nil
}

// GetLatestGitTag returns the latest git tag for a repository
func GetLatestGitTag(projectPath string) (string, error) {
	cmd := exec.Command("git", "-C", projectPath, "describe", "--tags", "--abbrev=0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get git tag: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// GetAllGitTags returns all git tags for a repository
func GetAllGitTags(projectPath string) ([]string, error) {
	cmd := exec.Command("git", "-C", projectPath, "tag", "--list")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to get git tags: %w", err)
	}

	tags := strings.Split(strings.TrimSpace(string(output)), "\n")
	return tags, nil
}
