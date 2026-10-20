// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package version

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/text/encoding/ianaindex"
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
	case strings.HasPrefix(projectType, "java"), projectType == "kotlin-gradle":
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
// global-jjb) to hold the authoritative release version. It takes, in
// order: the major/minor/patch trio, then base_version, then
// release_version, then a literal version key. Values may reference
// other keys of the same file as ${name}, which ONAP files use to
// derive base_version from keys such as major_version/minor_version/
// patch_version or release_name/sprint_number/feature_revision. Those
// references resolve against the file's own literal keys only: the
// parser never executes or sources the file and never reads the
// environment. A key whose references cannot be resolved counts as
// absent, as does one that exceeds the resolution bounds below.
// snapshot_version is never used, since a -SNAPSHOT value is
// not a release version.
func ExtractVersionProperties(projectPath string) (*VersionInfo, bool) {
	propsPath := filepath.Join(projectPath, "version.properties")
	content, err := os.ReadFile(propsPath)
	if err != nil {
		return nil, false
	}

	raw := parseVersionProperties(string(content))
	resolver := newPropertyResolver(raw)

	if version, ok := composeSemanticVersion(resolver); ok {
		return newPropertiesVersion(version), true
	}

	for _, key := range []string{"base_version", "release_version"} {
		if version, ok := resolver.resolve(key, 0); ok {
			return newPropertiesVersion(version), true
		}
	}

	if version, ok := raw["version"]; ok && !strings.Contains(version, "$\u007b") {
		return newPropertiesVersion(version), true
	}

	return nil, false
}

// parseVersionProperties parses key=value lines into a map, skipping
// blanks and comments. Values keep any ${name} references unresolved.
func parseVersionProperties(content string) map[string]string {
	props := make(map[string]string)
	for _, line := range strings.Split(content, "\n") {
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
		if key == "" || value == "" {
			continue
		}
		props[key] = value
	}
	return props
}

// Bounds on ${name} resolution. The file is untrusted input, so nesting,
// the size of any one expanded value and the total bytes produced per
// file are all capped: depth alone does not stop a chain of keys that
// each repeat the previous reference from expanding exponentially.
const (
	maxPropertyDepth      = 16
	maxPropertyValueBytes = 256
	maxPropertyWorkBytes  = 64 * 1024
)

// propertyResolver expands ${name} references against one file's own
// keys on demand. Only the keys a version is read from are resolved,
// each at most once; a key whose expansion would exceed a budget is
// rejected before its content is copied, and counts as unresolvable.
type propertyResolver struct {
	raw      map[string]string
	memo     map[string]resolvedProperty
	visiting map[string]bool
	work     int
}

// resolvedProperty is a cached expansion with its height: how many
// reference levels it needs below the key itself. A cache hit at some
// depth is valid only while depth plus height stays within the nesting
// limit, so the result does not depend on the order keys resolve in.
type resolvedProperty struct {
	value  string
	height int
}

func newPropertyResolver(raw map[string]string) *propertyResolver {
	return &propertyResolver{
		raw:      raw,
		memo:     make(map[string]resolvedProperty),
		visiting: make(map[string]bool),
	}
}

// resolve returns key's value with every reference substituted, or false
// when the key is missing, unresolvable, cyclic, nested too deep or over
// budget.
func (r *propertyResolver) resolve(key string, depth int) (string, bool) {
	value, _, ok := r.resolveAt(key, depth)
	return value, ok
}

func (r *propertyResolver) resolveAt(key string, depth int) (string, int, bool) {
	if cached, ok := r.memo[key]; ok {
		if depth+cached.height > maxPropertyDepth {
			return "", 0, false
		}
		return cached.value, cached.height, true
	}
	value, ok := r.raw[key]
	if !ok || r.visiting[key] || depth > maxPropertyDepth {
		return "", 0, false
	}
	r.visiting[key] = true
	defer delete(r.visiting, key)

	var sb strings.Builder
	height := 0
	rest := value
	for {
		start := strings.Index(rest, "$\u007b")
		if start < 0 {
			if !r.write(&sb, rest) {
				return "", 0, false
			}
			break
		}
		end := strings.Index(rest[start:], "}")
		if end < 0 {
			return "", 0, false
		}
		end += start
		ref, refHeight, ok := r.resolveAt(rest[start+2:end], depth+1)
		if !ok || !r.write(&sb, rest[:start]) || !r.write(&sb, ref) {
			return "", 0, false
		}
		height = max(height, refHeight+1)
		rest = rest[end+1:]
	}
	resolved := sb.String()
	r.memo[key] = resolvedProperty{value: resolved, height: height}
	return resolved, height, true
}

// write appends s to sb when both the value and the per-file work
// budgets allow it, charging the work budget; otherwise it copies
// nothing and reports false.
func (r *propertyResolver) write(sb *strings.Builder, s string) bool {
	if sb.Len()+len(s) > maxPropertyValueBytes || r.work+len(s) > maxPropertyWorkBytes {
		return false
	}
	r.work += len(s)
	sb.WriteString(s)
	return true
}

// composeSemanticVersion builds major.minor.patch when all three keys
// resolve, the authoritative form used by global-jjb release files.
func composeSemanticVersion(r *propertyResolver) (string, bool) {
	major, hasMajor := r.resolve("major", 0)
	minor, hasMinor := r.resolve("minor", 0)
	patch, hasPatch := r.resolve("patch", 0)
	if hasMajor && hasMinor && hasPatch {
		return major + "." + minor + "." + patch, true
	}
	return "", false
}

func newPropertiesVersion(version string) *VersionInfo {
	return &VersionInfo{
		Version:   version,
		Source:    "version.properties",
		IsDynamic: false,
	}
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

// mavenProjectVersions holds the direct <version> children of <project>
// and of <parent>. Nothing nested deeper is read, so dependency, plugin
// and profile versions cannot be mistaken for the project's, wherever
// in the file the direct elements sit.
type mavenProjectVersions struct {
	XMLName xml.Name `xml:"project"`
	Version string   `xml:"version"`
	Parent  *struct {
		Version string `xml:"version"`
	} `xml:"parent"`
}

// extractMavenVersion extracts version from Maven pom.xml. A project
// that declares no <version> of its own inherits its parent's, as
// Maven does, so <parent><version> is used only when <project> has no
// direct <version>. A POM that does not parse falls back to
// version.properties, then git.
func extractMavenVersion(projectPath string) (*VersionInfo, error) {
	content, err := os.ReadFile(filepath.Join(projectPath, "pom.xml"))
	if err != nil {
		return extractFallback(projectPath)
	}
	var pom mavenProjectVersions
	decoder := xml.NewDecoder(bytes.NewReader(content))
	decoder.CharsetReader = pomCharsetReader
	if err := decoder.Decode(&pom); err != nil {
		return extractFallback(projectPath)
	}
	if version := strings.TrimSpace(pom.Version); version != "" {
		return newMavenVersion(version), nil
	}
	if pom.Parent != nil {
		if version := strings.TrimSpace(pom.Parent.Version); version != "" {
			return newMavenVersion(version), nil
		}
	}
	return extractFallback(projectPath)
}

// pomCharsetReader decodes POMs that declare an encoding other than
// UTF-8, such as the ISO-8859-1 many older Maven projects use, which the
// XML decoder otherwise rejects outright. The label is looked up in the
// IANA registry; an unknown or unsupported charset is an error, so such
// a POM falls back like any other that does not parse.
func pomCharsetReader(label string, input io.Reader) (io.Reader, error) {
	enc, err := ianaindex.IANA.Encoding(label)
	if err != nil {
		return nil, err
	}
	if enc == nil {
		return nil, fmt.Errorf("unsupported POM encoding %q", label)
	}
	return enc.NewDecoder().Reader(input), nil
}

func newMavenVersion(version string) *VersionInfo {
	return &VersionInfo{
		Version:   version,
		Source:    "pom.xml",
		IsDynamic: isMavenDynamicVersion(version),
	}
}

func isMavenDynamicVersion(version string) bool {
	return strings.Contains(version, "$\u007b") || strings.Contains(version, "SNAPSHOT")
}

// extractGradleVersion extracts version from Gradle build files
func extractGradleVersion(projectPath string) (*VersionInfo, error) {
	// Kotlin DSL first, matching GradleExtractor.detectBuildFile and the
	// detector itself, which ranks kotlin-gradle above java-gradle when
	// both scripts are present. The order is only observable during a DSL
	// migration, when a project carries both -- and that is exactly when
	// disagreeing would be worst: the common project_version would come
	// from the Groovy script while every java_* value came from the Kotlin
	// one, describing two different files as one project.
	buildFiles := []string{"build.gradle.kts", "build.gradle"}

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
