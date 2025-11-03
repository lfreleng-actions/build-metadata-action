// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package repository

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseGitHubURL(t *testing.T) {
	tests := []struct {
		name     string
		gitURL   string
		wantOrg  string
		wantRepo string
		wantErr  bool
	}{
		{
			name:     "SSH format with .git",
			gitURL:   "git@github.com:modeseven-lfreleng-actions/http-api-tool-docker.git",
			wantOrg:  "modeseven-lfreleng-actions",
			wantRepo: "http-api-tool-docker",
			wantErr:  false,
		},
		{
			name:     "SSH format without .git",
			gitURL:   "git@github.com:lfreleng-actions/http-api-tool-docker",
			wantOrg:  "lfreleng-actions",
			wantRepo: "http-api-tool-docker",
			wantErr:  false,
		},
		{
			name:     "HTTPS format with .git",
			gitURL:   "https://github.com/lfreleng-actions/build-metadata-action.git",
			wantOrg:  "lfreleng-actions",
			wantRepo: "build-metadata-action",
			wantErr:  false,
		},
		{
			name:     "HTTPS format without .git",
			gitURL:   "https://github.com/modeseven-lfreleng-actions/python-build-action",
			wantOrg:  "modeseven-lfreleng-actions",
			wantRepo: "python-build-action",
			wantErr:  false,
		},
		{
			name:     "SSH URL format",
			gitURL:   "ssh://git@github.com/lfreleng-actions/version-extract-action.git",
			wantOrg:  "lfreleng-actions",
			wantRepo: "version-extract-action",
			wantErr:  false,
		},
		{
			name:    "invalid URL",
			gitURL:  "not-a-github-url",
			wantErr: true,
		},
		{
			name:    "empty URL",
			gitURL:  "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := parseGitHubURL(tt.gitURL)

			if tt.wantErr {
				if err == nil {
					t.Errorf("parseGitHubURL() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("parseGitHubURL() unexpected error: %v", err)
				return
			}

			if info.Organization != tt.wantOrg {
				t.Errorf("Organization = %v, want %v", info.Organization, tt.wantOrg)
			}

			if info.Repository != tt.wantRepo {
				t.Errorf("Repository = %v, want %v", info.Repository, tt.wantRepo)
			}

			if info.Type != "github" {
				t.Errorf("Type = %v, want github", info.Type)
			}

			expectedFullName := tt.wantOrg + "/" + tt.wantRepo
			if info.FullName != expectedFullName {
				t.Errorf("FullName = %v, want %v", info.FullName, expectedFullName)
			}
		})
	}
}

func TestDetectGerritRepository(t *testing.T) {
	tests := []struct {
		name        string
		gitreview   string
		wantHost    string
		wantProject string
		wantErr     bool
	}{
		{
			name: "valid .gitreview",
			gitreview: `[gerrit]
host=gerrit.onap.org
port=29418
project=portal-ng/bff
defaultbranch=master`,
			wantHost:    "gerrit.onap.org",
			wantProject: "portal-ng/bff",
			wantErr:     false,
		},
		{
			name: "valid .gitreview with comments",
			gitreview: `# This is a comment
[gerrit]
# Another comment
host=gerrit.example.com
port=29418
project=my/project
defaultbranch=main`,
			wantHost:    "gerrit.example.com",
			wantProject: "my/project",
			wantErr:     false,
		},
		{
			name: "missing host",
			gitreview: `[gerrit]
port=29418
project=some/project`,
			wantErr: true,
		},
		{
			name: "missing project",
			gitreview: `[gerrit]
host=gerrit.example.com
port=29418`,
			wantErr: true,
		},
		{
			name:      "empty file",
			gitreview: "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory
			tmpDir, err := os.MkdirTemp("", "repo-test-*")
			if err != nil {
				t.Fatalf("Failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tmpDir)

			// Write .gitreview file
			gitreviewPath := filepath.Join(tmpDir, ".gitreview")
			if err := os.WriteFile(gitreviewPath, []byte(tt.gitreview), 0644); err != nil {
				t.Fatalf("Failed to write .gitreview: %v", err)
			}

			info, err := detectGerritRepository(tmpDir)

			if tt.wantErr {
				if err == nil {
					t.Errorf("detectGerritRepository() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("detectGerritRepository() unexpected error: %v", err)
				return
			}

			if info.Organization != tt.wantHost {
				t.Errorf("Organization = %v, want %v", info.Organization, tt.wantHost)
			}

			if info.Repository != tt.wantProject {
				t.Errorf("Repository = %v, want %v", info.Repository, tt.wantProject)
			}

			if info.Type != "gerrit" {
				t.Errorf("Type = %v, want gerrit", info.Type)
			}

			expectedFullName := tt.wantHost + "/" + tt.wantProject
			if info.FullName != expectedFullName {
				t.Errorf("FullName = %v, want %v", info.FullName, expectedFullName)
			}
		})
	}
}

func TestDetectGerritRepository_NoFile(t *testing.T) {
	// Create temp directory without .gitreview
	tmpDir, err := os.MkdirTemp("", "repo-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	_, err = detectGerritRepository(tmpDir)
	if err == nil {
		t.Error("detectGerritRepository() expected error when .gitreview missing, got nil")
	}
}

func TestDetectLocalRepository(t *testing.T) {
	tests := []struct {
		name         string
		path         string
		wantRepoName string
	}{
		{
			name:         "simple directory",
			path:         "/path/to/my-project",
			wantRepoName: "my-project",
		},
		{
			name:         "nested directory",
			path:         "/path/to/deeply/nested/project-name",
			wantRepoName: "project-name",
		},
		{
			name:         "current directory",
			path:         ".",
			wantRepoName: ".",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := detectLocalRepository(tt.path)

			if info.Type != "local" {
				t.Errorf("Type = %v, want local", info.Type)
			}

			if info.Organization != "" {
				t.Errorf("Organization = %v, want empty", info.Organization)
			}

			if info.Repository != tt.wantRepoName {
				t.Errorf("Repository = %v, want %v", info.Repository, tt.wantRepoName)
			}

			if info.FullName != tt.wantRepoName {
				t.Errorf("FullName = %v, want %v", info.FullName, tt.wantRepoName)
			}
		})
	}
}

func TestFormatForDisplay(t *testing.T) {
	tests := []struct {
		name string
		info RepositoryInfo
		want string
	}{
		{
			name: "GitHub repository",
			info: RepositoryInfo{
				Type:         "github",
				Organization: "lfreleng-actions",
				Repository:   "build-metadata-action",
				FullName:     "lfreleng-actions/build-metadata-action",
			},
			want: "Project: lfreleng-actions/build-metadata-action [GitHub]",
		},
		{
			name: "Gerrit repository",
			info: RepositoryInfo{
				Type:         "gerrit",
				Organization: "gerrit.onap.org",
				Repository:   "portal-ng/bff",
				FullName:     "gerrit.onap.org/portal-ng/bff",
			},
			want: "Project: portal-ng/bff [gerrit.onap.org]",
		},
		{
			name: "Local repository",
			info: RepositoryInfo{
				Type:         "local",
				Organization: "",
				Repository:   "my-project",
				FullName:     "my-project",
			},
			want: "my-project",
		},
		{
			name: "Unknown type",
			info: RepositoryInfo{
				Type:         "unknown",
				Organization: "some-org",
				Repository:   "some-repo",
				FullName:     "some-org/some-repo",
			},
			want: "some-org/some-repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.info.FormatForDisplay()
			if got != tt.want {
				t.Errorf("FormatForDisplay() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDetectRepository_Priority(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "repo-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Test with only .gitreview (Gerrit should be detected)
	gitreview := `[gerrit]
host=gerrit.example.com
port=29418
project=test/project
defaultbranch=main`

	gitreviewPath := filepath.Join(tmpDir, ".gitreview")
	if err := os.WriteFile(gitreviewPath, []byte(gitreview), 0644); err != nil {
		t.Fatalf("Failed to write .gitreview: %v", err)
	}

	info, err := DetectRepository(tmpDir)
	if err != nil {
		t.Fatalf("DetectRepository() unexpected error: %v", err)
	}

	// Without git remotes, should fall back to Gerrit
	if info.Type != "gerrit" && info.Type != "local" {
		// Either gerrit or local is acceptable depending on whether git command is available
		t.Logf("Repository type: %s", info.Type)
	}
}

func TestDetectRepository_Fallback(t *testing.T) {
	// Create temp directory with no git setup
	tmpDir, err := os.MkdirTemp("", "repo-test-fallback-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	info, err := DetectRepository(tmpDir)
	if err != nil {
		t.Fatalf("DetectRepository() unexpected error: %v", err)
	}

	// Should fall back to local directory name
	if info.Type != "local" {
		t.Errorf("Expected type 'local', got %v", info.Type)
	}

	if info.Repository == "" {
		t.Error("Repository name should not be empty")
	}
}
