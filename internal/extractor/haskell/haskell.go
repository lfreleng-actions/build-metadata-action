// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package haskell

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/lfreleng-actions/build-metadata-action/internal/extractor"
)

// Extractor extracts metadata from Haskell projects
type Extractor struct {
	extractor.BaseExtractor
}

// NewExtractor creates a new Haskell extractor
func NewExtractor() *Extractor {
	return &Extractor{
		BaseExtractor: extractor.NewBaseExtractor("haskell", 1),
	}
}

func init() {
	extractor.RegisterExtractor(NewExtractor())
}

// Detect checks if this is a Haskell project
func (e *Extractor) Detect(projectPath string) bool {
	matches, err := filepath.Glob(filepath.Join(projectPath, "*.cabal"))
	if err == nil && len(matches) > 0 {
		return true
	}

	if _, err := os.Stat(filepath.Join(projectPath, "stack.yaml")); err == nil {
		return true
	}

	// Check for package.yaml (hpack)
	if _, err := os.Stat(filepath.Join(projectPath, "package.yaml")); err == nil {
		return true
	}

	if _, err := os.Stat(filepath.Join(projectPath, "cabal.project")); err == nil {
		return true
	}

	srcDir := filepath.Join(projectPath, "src")
	if info, err := os.Stat(srcDir); err == nil && info.IsDir() {
		matches, err := filepath.Glob(filepath.Join(srcDir, "*.hs"))
		if err == nil && len(matches) > 0 {
			return true
		}
	}

	return false
}

// Extract retrieves metadata from a Haskell project
func (e *Extractor) Extract(projectPath string) (*extractor.ProjectMetadata, error) {
	metadata := &extractor.ProjectMetadata{
		LanguageSpecific: make(map[string]interface{}),
	}

	// Try to find .cabal file
	cabalFiles, err := filepath.Glob(filepath.Join(projectPath, "*.cabal"))
	if err == nil && len(cabalFiles) > 0 {
		if err := e.extractFromCabal(cabalFiles[0], metadata); err == nil {
			metadata.LanguageSpecific["build_tool"] = "Cabal"
		}
	}

	stackPath := filepath.Join(projectPath, "stack.yaml")
	if _, err := os.Stat(stackPath); err == nil {
		e.extractFromStack(stackPath, metadata)
		if metadata.LanguageSpecific["build_tool"] == "" {
			metadata.LanguageSpecific["build_tool"] = "Stack"
		} else {
			metadata.LanguageSpecific["build_tool"] = "Stack + Cabal"
		}
	}

	// Check for package.yaml (hpack)
	packageYamlPath := filepath.Join(projectPath, "package.yaml")
	if _, err := os.Stat(packageYamlPath); err == nil {
		e.extractFromPackageYaml(packageYamlPath, metadata)
	}

	return metadata, nil
}

// cabalBuildDependsRegex matches the opening line of a build-depends stanza.
var cabalBuildDependsRegex = regexp.MustCompile(`(?i)^build-depends:\s*(.+)$`)

// cabalMatcher pairs a single-value field regex with the assignment it drives.
type cabalMatcher struct {
	re     *regexp.Regexp
	assign func(value string)
}

// cabalFieldMatchers builds the ordered set of single-value cabal field
// matchers, closing over the destinations they write to.
func cabalFieldMatchers(path string, metadata *extractor.ProjectMetadata, authors *[]string) []cabalMatcher {
	field := func(name string) *regexp.Regexp {
		return regexp.MustCompile(`(?i)^` + name + `:\s*(.+)$`)
	}
	return []cabalMatcher{
		{field("name"), func(v string) { metadata.Name = v }},
		{field("version"), func(v string) {
			metadata.Version = v
			metadata.VersionSource = filepath.Base(path)
		}},
		{field("synopsis"), func(v string) { metadata.Description = v }},
		{field("description"), func(v string) {
			if metadata.Description == "" {
				metadata.Description = v
			}
		}},
		{field("homepage"), func(v string) { metadata.Homepage = v }},
		{field("license"), func(v string) { metadata.License = v }},
		{field("author"), func(v string) {
			if v != "" {
				*authors = append(*authors, v)
			}
		}},
		{field("maintainer"), func(v string) {
			if v != "" {
				metadata.LanguageSpecific["maintainer"] = v
			}
		}},
		{field("category"), func(v string) { metadata.LanguageSpecific["category"] = v }},
		{field("tested-with"), func(v string) { metadata.LanguageSpecific["tested_with"] = v }},
	}
}

// appendBuildDepends processes a line against the (possibly multi-line)
// build-depends stanza and returns whether the stanza is still open.
func appendBuildDepends(line, trimmed string, inBuildDepends bool, dependencies *[]string) bool {
	if matches := cabalBuildDependsRegex.FindStringSubmatch(trimmed); matches != nil {
		*dependencies = append(*dependencies, parseDependencies(strings.TrimSpace(matches[1]))...)
		return true
	}
	if inBuildDepends && strings.HasPrefix(line, " ") {
		*dependencies = append(*dependencies, parseDependencies(trimmed)...)
		return true
	}
	if inBuildDepends {
		return false
	}
	return inBuildDepends
}

// extractFromCabal parses a .cabal file
func (e *Extractor) extractFromCabal(path string, metadata *extractor.ProjectMetadata) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	var authors []string
	var dependencies []string
	inBuildDepends := false
	matchers := cabalFieldMatchers(path, metadata, &authors)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Skip comments
		if strings.HasPrefix(trimmed, "--") {
			continue
		}

		for _, matcher := range matchers {
			if matches := matcher.re.FindStringSubmatch(trimmed); matches != nil {
				matcher.assign(strings.TrimSpace(matches[1]))
			}
		}

		inBuildDepends = appendBuildDepends(line, trimmed, inBuildDepends, &dependencies)
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	if len(authors) > 0 {
		metadata.Authors = authors
	}

	if len(dependencies) > 0 {
		metadata.LanguageSpecific["dependencies"] = dependencies
		metadata.LanguageSpecific["dependency_count"] = len(dependencies)
	}

	return nil
}

// extractFromStack parses stack.yaml
func (e *Extractor) extractFromStack(path string, metadata *extractor.ProjectMetadata) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	resolverRegex := regexp.MustCompile(`^resolver:\s*(.+)$`)

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "#") {
			continue
		}

		if matches := resolverRegex.FindStringSubmatch(trimmed); matches != nil {
			resolver := strings.TrimSpace(matches[1])
			metadata.LanguageSpecific["stack_resolver"] = resolver

			// Extract GHC version from resolver (e.g., lts-21.22 -> GHC 9.4.8)
			if ghcVersion := extractGHCVersionFromResolver(resolver); ghcVersion != "" {
				metadata.LanguageSpecific["ghc_version"] = ghcVersion
			}
		}
	}

	return scanner.Err()
}

// extractFromPackageYaml parses package.yaml (hpack format)
func (e *Extractor) extractFromPackageYaml(path string, metadata *extractor.ProjectMetadata) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	nameRegex := regexp.MustCompile(`^name:\s*(.+)$`)
	versionRegex := regexp.MustCompile(`^version:\s*(.+)$`)

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "#") {
			continue
		}

		if matches := nameRegex.FindStringSubmatch(trimmed); matches != nil && metadata.Name == "" {
			metadata.Name = strings.TrimSpace(matches[1])
		}

		if matches := versionRegex.FindStringSubmatch(trimmed); matches != nil && metadata.Version == "" {
			metadata.Version = strings.TrimSpace(matches[1])
			metadata.VersionSource = "package.yaml"
		}
	}

	metadata.LanguageSpecific["uses_hpack"] = true

	return scanner.Err()
}

// parseDependencies parses a dependency line from cabal file
func parseDependencies(line string) []string {
	var deps []string

	line = strings.TrimSuffix(line, ",")

	// Split by comma
	parts := strings.Split(line, ",")

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Extract package name (before version constraint)
		fields := strings.Fields(part)
		if len(fields) > 0 {
			pkgName := fields[0]
			// Exclude "base" package itself
			if pkgName == "base" {
				continue
			}
			// Keep other package names as-is, including those with "base-" prefix
			if pkgName != "" {
				deps = append(deps, pkgName)
			}
		}
	}

	return deps
}

// extractGHCVersionFromResolver attempts to extract GHC version from Stack resolver
func extractGHCVersionFromResolver(resolver string) string {
	// Map common LTS versions to GHC versions
	ltsToGHC := map[string]string{
		"lts-22": "9.6.4",
		"lts-21": "9.4.8",
		"lts-20": "9.2.8",
		"lts-19": "9.0.2",
		"lts-18": "8.10.7",
	}

	for lts, ghc := range ltsToGHC {
		if strings.HasPrefix(resolver, lts) {
			return ghc
		}
	}

	// Try to extract from nightly
	if strings.HasPrefix(resolver, "nightly-") {
		return "9.6" // Latest stable
	}

	return ""
}

// generateGHCVersionMatrix generates a matrix of GHC versions
func generateGHCVersionMatrix(ghcVersion string) []string {
	return []string{"9.4", "9.6", "9.8"}
}
