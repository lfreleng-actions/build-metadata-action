// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package environment

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Metadata contains environment information
type Metadata struct {
	// CI Environment
	CI CIEnvironment `json:"ci"`

	// Runtime Environment
	Runtime RuntimeEnvironment `json:"runtime"`

	// Setup Actions detected
	SetupActions map[string]SetupActionInfo `json:"setup_actions,omitempty"`

	// Tool versions
	Tools map[string]string `json:"tools,omitempty"`
}

// CIEnvironment contains CI platform information
type CIEnvironment struct {
	Platform   string `json:"platform"`
	IsCI       bool   `json:"is_ci"`
	RunnerOS   string `json:"runner_os"`
	RunnerArch string `json:"runner_arch"`
	RunnerName string `json:"runner_name,omitempty"`

	// GitHub-specific
	GitHubAction     string `json:"github_action,omitempty"`
	GitHubActor      string `json:"github_actor,omitempty"`
	GitHubRepository string `json:"github_repository,omitempty"`
	GitHubEventName  string `json:"github_event_name,omitempty"`
	GitHubWorkflow   string `json:"github_workflow,omitempty"`
	GitHubRunNumber  string `json:"github_run_number,omitempty"`
	GitHubRunAttempt string `json:"github_run_attempt,omitempty"`
}

// RuntimeEnvironment contains runtime system information
type RuntimeEnvironment struct {
	OS          string            `json:"os"`
	Arch        string            `json:"arch"`
	GoVersion   string            `json:"go_version"`
	Shell       string            `json:"shell,omitempty"`
	Environment map[string]string `json:"env,omitempty"`
}

// SetupActionInfo contains information about a detected setup action
type SetupActionInfo struct {
	Name    string            `json:"name"`
	Version string            `json:"version,omitempty"`
	Inputs  map[string]string `json:"inputs,omitempty"`
}

// Collect gathers environment metadata
func Collect() (*Metadata, error) {
	metadata := &Metadata{
		Tools:        make(map[string]string),
		SetupActions: make(map[string]SetupActionInfo),
	}

	// Collect CI environment
	metadata.CI = collectCIEnvironment()

	// Collect runtime environment
	metadata.Runtime = collectRuntimeEnvironment()

	// Detect setup actions (GitHub Actions specific)
	if metadata.CI.Platform == "github" {
		detectSetupActions(metadata)
	}

	// Detect tool versions
	detectToolVersions(metadata)

	return metadata, nil
}

// collectCIEnvironment gathers CI platform information
func collectCIEnvironment() CIEnvironment {
	env := CIEnvironment{
		RunnerOS:   os.Getenv("RUNNER_OS"),
		RunnerArch: os.Getenv("RUNNER_ARCH"),
		RunnerName: os.Getenv("RUNNER_NAME"),
	}

	// Detect CI platform
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		env.Platform = "github"
		env.IsCI = true
		env.GitHubAction = os.Getenv("GITHUB_ACTION")
		env.GitHubActor = os.Getenv("GITHUB_ACTOR")
		env.GitHubRepository = os.Getenv("GITHUB_REPOSITORY")
		env.GitHubEventName = os.Getenv("GITHUB_EVENT_NAME")
		env.GitHubWorkflow = os.Getenv("GITHUB_WORKFLOW")
		env.GitHubRunNumber = os.Getenv("GITHUB_RUN_NUMBER")
		env.GitHubRunAttempt = os.Getenv("GITHUB_RUN_ATTEMPT")
	} else if os.Getenv("GITLAB_CI") == "true" {
		env.Platform = "gitlab"
		env.IsCI = true
	} else if os.Getenv("CIRCLECI") == "true" {
		env.Platform = "circleci"
		env.IsCI = true
	} else if os.Getenv("TRAVIS") == "true" {
		env.Platform = "travis"
		env.IsCI = true
	} else if os.Getenv("JENKINS_HOME") != "" {
		env.Platform = "jenkins"
		env.IsCI = true
	} else if os.Getenv("CI") == "true" {
		env.Platform = "unknown"
		env.IsCI = true
	} else {
		env.Platform = "local"
		env.IsCI = false
	}

	return env
}

// collectRuntimeEnvironment gathers runtime system information
func collectRuntimeEnvironment() RuntimeEnvironment {
	env := RuntimeEnvironment{
		OS:          runtime.GOOS,
		Arch:        runtime.GOARCH,
		GoVersion:   runtime.Version(),
		Shell:       os.Getenv("SHELL"),
		Environment: make(map[string]string),
	}

	// Collect relevant environment variables
	relevantEnvVars := []string{
		"PATH",
		"HOME",
		"USER",
		"PWD",
		"LANG",
		"LC_ALL",
	}

	for _, key := range relevantEnvVars {
		if value := os.Getenv(key); value != "" {
			env.Environment[key] = value
		}
	}

	return env
}

// detectSetupActions detects GitHub setup-* actions that have been run
func detectSetupActions(metadata *Metadata) {
	// Check for setup-python
	if pythonVersion := os.Getenv("pythonLocation"); pythonVersion != "" {
		metadata.SetupActions["setup-python"] = SetupActionInfo{
			Name:    "setup-python",
			Version: getToolVersion("python", "--version"),
		}
	}

	// Check for setup-node
	if nodeVersion := getToolVersion("node", "--version"); nodeVersion != "" {
		metadata.SetupActions["setup-node"] = SetupActionInfo{
			Name:    "setup-node",
			Version: nodeVersion,
		}
	}

	// Check for setup-java
	if javaHome := os.Getenv("JAVA_HOME"); javaHome != "" {
		metadata.SetupActions["setup-java"] = SetupActionInfo{
			Name:    "setup-java",
			Version: getToolVersion("java", "-version"),
		}
	}

	// Check for setup-go
	if goVersion := getToolVersion("go", "version"); goVersion != "" {
		metadata.SetupActions["setup-go"] = SetupActionInfo{
			Name:    "setup-go",
			Version: goVersion,
		}
	}

	// Check for setup-dotnet
	if dotnetVersion := getToolVersion("dotnet", "--version"); dotnetVersion != "" {
		metadata.SetupActions["setup-dotnet"] = SetupActionInfo{
			Name:    "setup-dotnet",
			Version: dotnetVersion,
		}
	}

	// Check for setup-ruby
	if rubyVersion := getToolVersion("ruby", "--version"); rubyVersion != "" {
		metadata.SetupActions["setup-ruby"] = SetupActionInfo{
			Name:    "setup-ruby",
			Version: rubyVersion,
		}
	}
}

// detectToolVersions detects versions of common development tools
func detectToolVersions(metadata *Metadata) {
	tools := map[string][]string{
		"python":   {"--version"},
		"python3":  {"--version"},
		"node":     {"--version"},
		"npm":      {"--version"},
		"yarn":     {"--version"},
		"pnpm":     {"--version"},
		"java":     {"-version"},
		"javac":    {"-version"},
		"mvn":      {"--version"},
		"gradle":   {"--version"},
		"go":       {"version"},
		"cargo":    {"--version"},
		"rustc":    {"--version"},
		"dotnet":   {"--version"},
		"ruby":     {"--version"},
		"gem":      {"--version"},
		"bundler":  {"--version"},
		"php":      {"--version"},
		"composer": {"--version"},
		"swift":    {"--version"},
		"gcc":      {"--version"},
		"clang":    {"--version"},
		"make":     {"--version"},
		"cmake":    {"--version"},
		"git":      {"--version"},
		"docker":   {"--version"},
		"kubectl":  {"version", "--client"},
	}

	for tool, args := range tools {
		if version := getToolVersion(tool, args...); version != "" {
			metadata.Tools[tool] = version
		}
	}
}

// getToolVersion attempts to get the version of a tool
func getToolVersion(tool string, args ...string) string {
	cmd := exec.Command(tool, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}

	// Clean up the output
	version := strings.TrimSpace(string(output))

	// Extract version number from common patterns
	version = extractVersion(version)

	return version
}

// extractVersion extracts a version number from tool output
func extractVersion(output string) string {
	// Take only the first line for most tools
	lines := strings.Split(output, "\n")
	if len(lines) > 0 {
		firstLine := lines[0]

		// Common patterns
		// "tool version X.Y.Z" or "tool X.Y.Z"
		parts := strings.Fields(firstLine)
		for i, part := range parts {
			// Look for version-like strings
			if isVersionLike(part) {
				return part
			}
			// Check if this is a "version" keyword followed by version number
			if strings.ToLower(part) == "version" && i+1 < len(parts) {
				return parts[i+1]
			}
		}

		// If nothing found, return the first line
		return firstLine
	}

	return output
}

// isVersionLike checks if a string looks like a version number
func isVersionLike(s string) bool {
	// Remove common prefixes
	s = strings.TrimPrefix(s, "v")
	s = strings.TrimPrefix(s, "V")

	// Check if it starts with a digit and contains a dot
	if len(s) > 0 && s[0] >= '0' && s[0] <= '9' && strings.Contains(s, ".") {
		return true
	}

	return false
}

// GetEnvironmentVariable returns a specific environment variable value
func GetEnvironmentVariable(key string) string {
	return os.Getenv(key)
}

// GetAllEnvironmentVariables returns all environment variables
func GetAllEnvironmentVariables() map[string]string {
	env := make(map[string]string)
	for _, e := range os.Environ() {
		pair := strings.SplitN(e, "=", 2)
		if len(pair) == 2 {
			env[pair[0]] = pair[1]
		}
	}
	return env
}

// IsCI returns true if running in a CI environment
func IsCI() bool {
	return os.Getenv("CI") == "true" ||
		os.Getenv("GITHUB_ACTIONS") == "true" ||
		os.Getenv("GITLAB_CI") == "true" ||
		os.Getenv("CIRCLECI") == "true" ||
		os.Getenv("TRAVIS") == "true" ||
		os.Getenv("JENKINS_HOME") != ""
}

// GetCIPlatform returns the detected CI platform name
func GetCIPlatform() string {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		return "github"
	} else if os.Getenv("GITLAB_CI") == "true" {
		return "gitlab"
	} else if os.Getenv("CIRCLECI") == "true" {
		return "circleci"
	} else if os.Getenv("TRAVIS") == "true" {
		return "travis"
	} else if os.Getenv("JENKINS_HOME") != "" {
		return "jenkins"
	} else if os.Getenv("CI") == "true" {
		return "unknown"
	}
	return "local"
}
