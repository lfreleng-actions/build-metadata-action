// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package version

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// VersionInfo contains version information extracted from a project
type VersionInfo struct {
	Version   string
	Source    string
	IsDynamic bool
	Tags      []string
}

// ExtractVersion extracts version information from a project
// This integrates with the version-extract-action tool if available,
// or falls back to basic extraction
func ExtractVersion(projectPath, projectType string) (*VersionInfo, error) {
	// Try to use version-extract-action if available
	versionExtractPath := os.Getenv("VERSION_EXTRACT_ACTION_PATH")
	if versionExtractPath != "" {
		return extractWithTool(projectPath, projectType, versionExtractPath)
	}

	// Fallback to basic extraction
	return extractBasic(projectPath, projectType)
}

// extractWithTool uses the version-extract-action tool to extract version
func extractWithTool(projectPath, projectType, toolPath string) (*VersionInfo, error) {
	cmd := exec.Command(toolPath, "--path", projectPath, "--type", projectType, "--format", "json")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("version-extract-action failed: %w (output: %s)", err, string(output))
	}

	// Parse the JSON output
	// This is a simplified version - actual implementation would parse JSON properly
	info := &VersionInfo{
		Version:   strings.TrimSpace(string(output)),
		Source:    "version-extract-action",
		IsDynamic: false,
	}

	return info, nil
}

// extractBasic performs basic version extraction without external tools
func extractBasic(projectPath, projectType string) (*VersionInfo, error) {
	// Ensure tags are fetched before any extraction (important for CI environments)
	ensureTagsAreFetched(projectPath)

	switch {
	case strings.HasPrefix(projectType, "python"):
		return extractPythonVersion(projectPath)
	case strings.HasPrefix(projectType, "javascript"), strings.HasPrefix(projectType, "typescript"):
		return extractJavaScriptVersion(projectPath)
	case strings.HasPrefix(projectType, "java"):
		return extractJavaVersion(projectPath, projectType)
	case strings.HasPrefix(projectType, "go"):
		return extractGoVersion(projectPath)
	case strings.HasPrefix(projectType, "rust"):
		return extractRustVersion(projectPath)
	default:
		return extractFallback(projectPath)
	}
}

// extractFallback is the shared fallback chain used when a language
// manifest does not provide a usable version: version.properties
// (the Linux Foundation / ONAP release convention) first, then git.
func extractFallback(projectPath string) (*VersionInfo, error) {
	if info, ok := ExtractVersionProperties(projectPath); ok {
		return info, nil
	}
	return extractFromGit(projectPath)
}

// ExtractVersionProperties extracts a version from a version.properties
// file, the convention used by Linux Foundation / ONAP projects (via
// global-jjb) to hold the authoritative release version as separate
// major/minor/patch keys. Literal release_version=X.Y.Z and
// version=X.Y.Z keys are also accepted. Values containing ${...}
// interpolation are ignored: Jenkins-era files interpolate derived
// keys (base_version, release_version) from the major/minor/patch
// trio when the file is sourced in shell, and this parser never
// executes the file.
func ExtractVersionProperties(projectPath string) (*VersionInfo, bool) {
	propsPath := filepath.Join(projectPath, "version.properties")
	content, err := os.ReadFile(propsPath)
	if err != nil {
		return nil, false
	}

	props := make(map[string]string)
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if value == "" || strings.Contains(value, "${") {
			continue
		}
		props[key] = value
	}

	major, hasMajor := props["major"]
	minor, hasMinor := props["minor"]
	patch, hasPatch := props["patch"]
	if hasMajor && hasMinor && hasPatch {
		return &VersionInfo{
			Version:   major + "." + minor + "." + patch,
			Source:    "version.properties",
			IsDynamic: false,
		}, true
	}

	if version, ok := props["release_version"]; ok {
		return &VersionInfo{
			Version:   version,
			Source:    "version.properties",
			IsDynamic: false,
		}, true
	}

	if version, ok := props["version"]; ok {
		return &VersionInfo{
			Version:   version,
			Source:    "version.properties",
			IsDynamic: false,
		}, true
	}

	return nil, false
}

// isPlaceholderVersion reports whether a manifest version is an
// unmaintained placeholder (e.g. package.json "version": "0.0.0")
// that should not be reported as the project version. Placeholders
// continue down the fallback chain (version.properties, then git).
func isPlaceholderVersion(version string) bool {
	return version == "" || version == "0.0.0"
}

// extractPythonVersion extracts version from Python projects
func extractPythonVersion(projectPath string) (*VersionInfo, error) {
	// Try pyproject.toml first
	pyprojectPath := filepath.Join(projectPath, "pyproject.toml")
	if _, err := os.Stat(pyprojectPath); err == nil {
		content, err := os.ReadFile(pyprojectPath)
		if err == nil {
			// Simple regex-like search for version = "X.Y.Z"
			lines := strings.Split(string(content), "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "version") && strings.Contains(line, "=") {
					parts := strings.Split(line, "=")
					if len(parts) == 2 {
						version := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
						return &VersionInfo{
							Version:   version,
							Source:    "pyproject.toml",
							IsDynamic: strings.Contains(string(content), "dynamic") && strings.Contains(string(content), "version"),
						}, nil
					}
				}
			}
		}
	}

	// Fall back to version.properties then git
	return extractFallback(projectPath)
}

// extractJavaScriptVersion extracts version from JavaScript/Node.js projects
func extractJavaScriptVersion(projectPath string) (*VersionInfo, error) {
	packageJSONPath := filepath.Join(projectPath, "package.json")
	content, err := os.ReadFile(packageJSONPath)
	if err != nil {
		return extractFallback(projectPath)
	}

	var pkg struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(content, &pkg); err != nil {
		return extractFallback(projectPath)
	}

	if isPlaceholderVersion(pkg.Version) {
		// Unmaintained placeholder (common in projects that keep
		// the real version elsewhere, e.g. ONAP version.properties):
		// use the fallback chain instead.
		return extractFallback(projectPath)
	}

	isDynamic := pkg.Version == "0.0.0-development" || pkg.Version == "0.0.0-semantic-release"
	return &VersionInfo{
		Version:   pkg.Version,
		Source:    "package.json",
		IsDynamic: isDynamic,
	}, nil
}

// extractJavaVersion extracts version from Java projects
func extractJavaVersion(projectPath, projectType string) (*VersionInfo, error) {
	if strings.Contains(projectType, "maven") {
		return extractMavenVersion(projectPath)
	} else if strings.Contains(projectType, "gradle") {
		return extractGradleVersion(projectPath)
	}
	return extractFallback(projectPath)
}

// extractMavenVersion extracts version from Maven pom.xml
func extractMavenVersion(projectPath string) (*VersionInfo, error) {
	pomPath := filepath.Join(projectPath, "pom.xml")
	content, err := os.ReadFile(pomPath)
	if err != nil {
		return extractFallback(projectPath)
	}

	// Simple XML parsing for <version>X.Y.Z</version>
	lines := strings.Split(string(content), "\n")
	inProject := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "<project") {
			inProject = true
		}
		if inProject && strings.Contains(line, "<version>") && strings.Contains(line, "</version>") {
			start := strings.Index(line, "<version>") + 9
			end := strings.Index(line, "</version>")
			if start > 9 && end > start {
				version := line[start:end]
				isDynamic := strings.Contains(version, "${") || strings.Contains(version, "SNAPSHOT")
				return &VersionInfo{
					Version:   version,
					Source:    "pom.xml",
					IsDynamic: isDynamic,
				}, nil
			}
		}
	}

	return extractFallback(projectPath)
}

// extractGradleVersion extracts version from Gradle build files
func extractGradleVersion(projectPath string) (*VersionInfo, error) {
	// Try build.gradle first, then build.gradle.kts
	buildFiles := []string{"build.gradle", "build.gradle.kts"}

	for _, buildFile := range buildFiles {
		buildPath := filepath.Join(projectPath, buildFile)
		content, err := os.ReadFile(buildPath)
		if err != nil {
			continue
		}

		// Simple search for version = "X.Y.Z" or version = 'X.Y.Z'
		lines := strings.Split(string(content), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "version") && strings.Contains(line, "=") {
				parts := strings.Split(line, "=")
				if len(parts) == 2 {
					version := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
					return &VersionInfo{
						Version:   version,
						Source:    buildFile,
						IsDynamic: false,
					}, nil
				}
			}
		}
	}

	return extractFallback(projectPath)
}

// extractGoVersion extracts version from Go modules
func extractGoVersion(projectPath string) (*VersionInfo, error) {
	// Go modules don't typically contain version information in go.mod
	// The /vX suffix in module paths (e.g., github.com/moby/moby/v2) is not a version
	// It's a major version indicator for Go module resolution
	// Always fall back to version.properties / git tags for Go projects
	return extractFallback(projectPath)
}

// extractRustVersion extracts version from Rust Cargo.toml
func extractRustVersion(projectPath string) (*VersionInfo, error) {
	cargoPath := filepath.Join(projectPath, "Cargo.toml")
	content, err := os.ReadFile(cargoPath)
	if err != nil {
		return extractFallback(projectPath)
	}

	// Simple TOML parsing for version = "X.Y.Z"
	// Note: This is a basic parser that handles common cases but may not handle all TOML features
	lines := strings.Split(string(content), "\n")
	inPackage := false
	inWorkspacePackage := false
	workspaceVersion := ""

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Track sections
		if line == "[package]" {
			inPackage = true
			inWorkspacePackage = false
			continue
		}
		if line == "[workspace.package]" {
			inPackage = false
			inWorkspacePackage = true
			continue
		}
		if strings.HasPrefix(line, "[") && line != "[package]" && line != "[workspace.package]" {
			inPackage = false
			inWorkspacePackage = false
			continue
		}

		// Extract workspace version if in [workspace.package] section
		if inWorkspacePackage && strings.HasPrefix(line, "version") && strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				workspaceVersion = strings.Trim(strings.TrimSpace(parts[1]), `"'`)
			}
		}

		// Extract version from [package] section
		if inPackage && strings.HasPrefix(line, "version") {
			// Check if it's a workspace reference like: version.workspace = true
			if strings.Contains(line, ".workspace") {
				// This is a workspace reference, use the workspace version if we found it
				if workspaceVersion != "" {
					isDynamic := workspaceVersion == "0.0.0" || workspaceVersion == "0.1.0-dev"
					return &VersionInfo{
						Version:   workspaceVersion,
						Source:    "Cargo.toml",
						IsDynamic: isDynamic,
					}, nil
				}
				// If we haven't found workspace version yet, continue searching
				continue
			}

			// Regular version = "X.Y.Z" format
			if strings.Contains(line, "=") {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					version := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
					if version != "true" && version != "false" && version != "" {
						isDynamic := version == "0.0.0" || version == "0.1.0-dev"
						return &VersionInfo{
							Version:   version,
							Source:    "Cargo.toml",
							IsDynamic: isDynamic,
						}, nil
					}
				}
			}
		}
	}

	return extractFallback(projectPath)
}

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
