// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package repository

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// RepositoryInfo contains information about the repository source
type RepositoryInfo struct {
	Type         string // "github", "gerrit", or "local"
	Organization string // GitHub org or Gerrit server
	Repository   string // Repository/project name
	FullName     string // Formatted full name for display
}

// DetectRepository detects repository information from git remotes and .gitreview
func DetectRepository(projectPath string) (*RepositoryInfo, error) {
	// Try GitHub first (from git remotes)
	if info, err := detectGitHubRepository(projectPath); err == nil {
		return info, nil
	}

	// Try Gerrit (from .gitreview file)
	if info, err := detectGerritRepository(projectPath); err == nil {
		return info, nil
	}

	// Fallback to local directory name
	return detectLocalRepository(projectPath), nil
}

// detectGitHubRepository detects GitHub repository from git remotes
func detectGitHubRepository(projectPath string) (*RepositoryInfo, error) {
	cmd := exec.Command("git", "remote", "-v")
	cmd.Dir = projectPath
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get git remotes: %w", err)
	}

	remotes := string(output)
	lines := strings.Split(remotes, "\n")

	// Look for upstream first, then origin
	var upstreamURL, originURL string
	for _, line := range lines {
		if strings.Contains(line, "(fetch)") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				remoteName := parts[0]
				remoteURL := parts[1]

				if remoteName == "upstream" {
					upstreamURL = remoteURL
				} else if remoteName == "origin" {
					originURL = remoteURL
				}
			}
		}
	}

	// Prefer upstream (for forks), fallback to origin
	gitURL := upstreamURL
	if gitURL == "" {
		gitURL = originURL
	}

	if gitURL == "" {
		return nil, fmt.Errorf("no git remotes found")
	}

	// Parse GitHub URL
	return parseGitHubURL(gitURL)
}

// parseGitHubURL extracts org and repo from GitHub URL
func parseGitHubURL(gitURL string) (*RepositoryInfo, error) {
	// Remove .git suffix if present
	gitURL = strings.TrimSuffix(gitURL, ".git")

	// Handle different GitHub URL formats:
	// - git@github.com:org/repo.git
	// - https://github.com/org/repo
	// - ssh://git@github.com/org/repo

	var org, repo string

	// SSH format: git@github.com:org/repo
	if strings.Contains(gitURL, "github.com:") {
		re := regexp.MustCompile(`github\.com:([^/]+)/(.+)$`)
		matches := re.FindStringSubmatch(gitURL)
		if len(matches) == 3 {
			org = matches[1]
			repo = matches[2]
		}
	} else if strings.Contains(gitURL, "github.com/") {
		// HTTPS format: https://github.com/org/repo
		re := regexp.MustCompile(`github\.com/([^/]+)/(.+)$`)
		matches := re.FindStringSubmatch(gitURL)
		if len(matches) == 3 {
			org = matches[1]
			repo = matches[2]
		}
	}

	if org == "" || repo == "" {
		return nil, fmt.Errorf("could not parse GitHub URL: %s", gitURL)
	}

	return &RepositoryInfo{
		Type:         "github",
		Organization: org,
		Repository:   repo,
		FullName:     fmt.Sprintf("%s/%s", org, repo),
	}, nil
}

// detectGerritRepository detects Gerrit repository from .gitreview file
func detectGerritRepository(projectPath string) (*RepositoryInfo, error) {
	gitreviewPath := filepath.Join(projectPath, ".gitreview")

	file, err := os.Open(gitreviewPath)
	if err != nil {
		return nil, fmt.Errorf(".gitreview file not found: %w", err)
	}
	defer file.Close()

	var host, project string

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip comments and empty lines
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}

		// Parse key=value pairs
		if strings.HasPrefix(line, "host=") {
			host = strings.TrimPrefix(line, "host=")
		} else if strings.HasPrefix(line, "project=") {
			project = strings.TrimPrefix(line, "project=")
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading .gitreview: %w", err)
	}

	if host == "" || project == "" {
		return nil, fmt.Errorf(".gitreview missing required fields")
	}

	return &RepositoryInfo{
		Type:         "gerrit",
		Organization: host,
		Repository:   project,
		FullName:     fmt.Sprintf("%s/%s", host, project),
	}, nil
}

// detectLocalRepository creates repository info from local directory name
func detectLocalRepository(projectPath string) *RepositoryInfo {
	// Get the directory name
	dirName := filepath.Base(projectPath)

	return &RepositoryInfo{
		Type:         "local",
		Organization: "",
		Repository:   dirName,
		FullName:     dirName,
	}
}

// FormatForDisplay formats the repository info for display in summaries
func (r *RepositoryInfo) FormatForDisplay() string {
	switch r.Type {
	case "github":
		return fmt.Sprintf("Project: %s [GitHub]", r.FullName)
	case "gerrit":
		return fmt.Sprintf("Project: %s [%s]", r.Repository, r.Organization)
	case "local":
		return r.FullName
	default:
		return r.FullName
	}
}
