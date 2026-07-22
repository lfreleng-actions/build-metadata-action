// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package elixir

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/lfreleng-actions/build-metadata-action/internal/extractor"
)

// Extractor extracts metadata from Elixir projects
type Extractor struct {
	extractor.BaseExtractor
}

// NewExtractor creates a new Elixir extractor
func NewExtractor() *Extractor {
	return &Extractor{
		BaseExtractor: extractor.NewBaseExtractor("elixir", 1),
	}
}

func init() {
	extractor.RegisterExtractor(NewExtractor())
}

// Detect checks if this is an Elixir project
func (e *Extractor) Detect(projectPath string) bool {
	if _, err := os.Stat(filepath.Join(projectPath, "mix.exs")); err == nil {
		return true
	}

	libDir := filepath.Join(projectPath, "lib")
	if info, err := os.Stat(libDir); err == nil && info.IsDir() {
		matches, err := filepath.Glob(filepath.Join(libDir, "*.ex"))
		if err == nil && len(matches) > 0 {
			return true
		}
	}

	patterns := []string{"*.ex", "*.exs"}
	for _, pattern := range patterns {
		matches, err := filepath.Glob(filepath.Join(projectPath, pattern))
		if err == nil && len(matches) > 0 {
			return true
		}
	}

	return false
}

// Extract retrieves metadata from an Elixir project
func (e *Extractor) Extract(projectPath string) (*extractor.ProjectMetadata, error) {
	metadata := &extractor.ProjectMetadata{
		LanguageSpecific: make(map[string]interface{}),
	}

	mixExsPath := filepath.Join(projectPath, "mix.exs")
	if _, err := os.Stat(mixExsPath); err == nil {
		if err := e.extractFromMixExs(mixExsPath, metadata); err != nil {
			return nil, err
		}
	}

	metadata.LanguageSpecific["build_tool"] = "Mix"
	return metadata, nil
}

// Regex patterns for parsing mix.exs. The braces in mixLinksRegex and
// mixDepRegex are written with the \u007b escape so that unbalanced braces in
// string literals do not confuse brace-counting static analyzers; the compiled
// patterns are identical to their literal-brace equivalents.
var (
	mixAppRegex          = regexp.MustCompile(`app:\s*:(\w+)`)
	mixVersionRegex      = regexp.MustCompile(`version:\s*"([^"]+)"`)
	mixElixirRegex       = regexp.MustCompile(`elixir:\s*"([^"]+)"`)
	mixDescriptionRegex  = regexp.MustCompile(`description:\s*"([^"]+)"`)
	mixPackageBlockRegex = regexp.MustCompile(`package:\s*\[`)
	mixPackageFuncRegex  = regexp.MustCompile(`defp\s+package\s+do`)
	mixLicenseRegex      = regexp.MustCompile(`licenses:\s*\["([^"]+)"`)
	mixLinksRegex        = regexp.MustCompile("links:\\s*%\\\u007b")
	mixHomepageRegex     = regexp.MustCompile(`"([^"]+)"\s*=>\s*"([^"]+)"`)
	mixDepRegex          = regexp.MustCompile("\\\u007b:(\\w+),\\s*\"([^\"]+)\"")
)

// mixExsState carries the block-tracking flags and accumulated values across
// the line-by-line scan of a mix.exs file.
type mixExsState struct {
	inPackageBlock      bool
	packageBracketDepth int
	packageSawBracket   bool
	inLinksBlock        bool
	linksBraceDepth     int
	linksSawBrace       bool
	elixirVersion       string
	dependencies        []string
}

// extractFromMixExs parses mix.exs
func (e *Extractor) extractFromMixExs(path string, metadata *extractor.ProjectMetadata) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	state := &mixExsState{}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#") {
			continue
		}
		state.processLine(line, metadata)
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	applyElixirVersion(state.elixirVersion, metadata)
	applyElixirDependencies(state.dependencies, metadata)

	if framework := detectFramework(state.dependencies); framework != "" {
		metadata.LanguageSpecific["framework"] = framework
	}

	return nil
}

// processLine applies every mix.exs pattern to a single line, updating both the
// parse state and the destination metadata.
func (s *mixExsState) processLine(line string, metadata *extractor.ProjectMetadata) {
	if matches := mixAppRegex.FindStringSubmatch(line); matches != nil {
		metadata.Name = matches[1]
	}
	if matches := mixVersionRegex.FindStringSubmatch(line); matches != nil {
		metadata.Version = matches[1]
		metadata.VersionSource = "mix.exs"
	}
	if matches := mixElixirRegex.FindStringSubmatch(line); matches != nil {
		s.elixirVersion = matches[1]
	}
	if matches := mixDescriptionRegex.FindStringSubmatch(line); matches != nil {
		metadata.Description = matches[1]
	}

	// Track package block (either inline or via defp package do function)
	if mixPackageBlockRegex.MatchString(line) || mixPackageFuncRegex.MatchString(line) {
		s.inPackageBlock = true
	}
	if s.inPackageBlock {
		if matches := mixLicenseRegex.FindStringSubmatch(line); matches != nil {
			metadata.License = matches[1]
		}
	}

	if mixLinksRegex.MatchString(line) {
		s.inLinksBlock = true
	}
	if s.inLinksBlock {
		if matches := mixHomepageRegex.FindStringSubmatch(line); matches != nil {
			if matches[1] == "GitHub" || matches[1] == "Homepage" {
				metadata.Homepage = matches[2]
			}
		}
	}

	s.updateBlockState(line)

	if matches := mixDepRegex.FindStringSubmatch(line); matches != nil {
		s.dependencies = append(s.dependencies, fmt.Sprintf("%s:%s", matches[1], matches[2]))
	}
}

// updateBlockState closes the package or links block by tracking delimiter
// depth. This correctly handles a block whose opening and closing
// delimiters appear on the same line (for example package: [licenses:
// ["MIT"]] or links: %\u007b ... \u007d), which a naive open/close flag
// would leave permanently open.
func (s *mixExsState) updateBlockState(line string) {
	if s.inPackageBlock {
		s.packageBracketDepth += strings.Count(line, "[") - strings.Count(line, "]")
		if strings.Contains(line, "[") {
			s.packageSawBracket = true
		}
		if s.packageSawBracket && s.packageBracketDepth <= 0 {
			s.inPackageBlock = false
			s.packageBracketDepth = 0
			s.packageSawBracket = false
		}
	}
	if s.inLinksBlock {
		s.linksBraceDepth += strings.Count(line, "\u007b") - strings.Count(line, "\u007d")
		if strings.Contains(line, "\u007b") {
			s.linksSawBrace = true
		}
		if s.linksSawBrace && s.linksBraceDepth <= 0 {
			s.inLinksBlock = false
			s.linksBraceDepth = 0
			s.linksSawBrace = false
		}
	}
}

func applyElixirVersion(elixirVersion string, metadata *extractor.ProjectMetadata) {
	if elixirVersion == "" {
		return
	}
	metadata.LanguageSpecific["elixir_version"] = elixirVersion
	matrix := generateElixirVersionMatrix(elixirVersion)
	if len(matrix) > 0 {
		metadata.LanguageSpecific["elixir_version_matrix"] = matrix
	}
}

func applyElixirDependencies(dependencies []string, metadata *extractor.ProjectMetadata) {
	if len(dependencies) > 0 {
		metadata.LanguageSpecific["dependencies"] = dependencies
		metadata.LanguageSpecific["dependency_count"] = len(dependencies)
	}
}

// generateElixirVersionMatrix generates a matrix of Elixir versions
func generateElixirVersionMatrix(requirement string) []string {
	version := strings.TrimPrefix(requirement, "~> ")
	version = strings.TrimPrefix(version, ">= ")
	version = strings.TrimPrefix(version, "== ")

	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return []string{"1.14", "1.15", "1.16"}
	}

	major := parts[0]
	minor := parts[1]

	if major == "1" {
		switch minor {
		case "16":
			return []string{"1.16", "1.17"}
		case "15":
			return []string{"1.15", "1.16", "1.17"}
		case "14":
			return []string{"1.14", "1.15", "1.16"}
		case "13":
			return []string{"1.13", "1.14", "1.15"}
		case "12":
			return []string{"1.12", "1.13", "1.14"}
		default:
			return []string{"1.14", "1.15", "1.16"}
		}
	}

	return []string{"1.14", "1.15", "1.16"}
}

// detectFramework detects if the project uses a framework
func detectFramework(dependencies []string) string {
	for _, dep := range dependencies {
		if strings.Contains(dep, "phoenix:") {
			return "Phoenix"
		}
		if strings.Contains(dep, "nerves:") {
			return "Nerves"
		}
		if strings.Contains(dep, "plug:") {
			return "Plug"
		}
	}
	return ""
}
