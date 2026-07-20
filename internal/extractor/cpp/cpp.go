// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package cpp

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/lfreleng-actions/build-metadata-action/internal/extractor"
)

// Extractor extracts metadata from C++ projects
type Extractor struct {
	extractor.BaseExtractor
}

// NewExtractor creates a new C++ extractor
func NewExtractor() *Extractor {
	return &Extractor{
		BaseExtractor: extractor.NewBaseExtractor("cpp", 1),
	}
}

func init() {
	extractor.RegisterExtractor(NewExtractor())
}

// CMakeProject represents parsed CMakeLists.txt metadata
type CMakeProject struct {
	ProjectName    string
	Version        string
	Description    string
	Languages      []string
	CXXStandard    string
	CStandard      string
	Dependencies   []string
	Subdirectories []string
	Executables    []string
	Libraries      []string
	Tests          []string
}

// Detect checks if this is a C++ project
func (e *Extractor) Detect(projectPath string) bool {
	if _, err := os.Stat(filepath.Join(projectPath, "CMakeLists.txt")); err == nil {
		return true
	}

	// Check for .qmake.conf (Qt qmake)
	if _, err := os.Stat(filepath.Join(projectPath, ".qmake.conf")); err == nil {
		return true
	}

	if _, err := os.Stat(filepath.Join(projectPath, "Makefile")); err == nil {
		return true
	}

	// Check for configure.ac (Autotools)
	if _, err := os.Stat(filepath.Join(projectPath, "configure.ac")); err == nil {
		return true
	}

	if _, err := os.Stat(filepath.Join(projectPath, "meson.build")); err == nil {
		return true
	}

	patterns := []string{"*.cpp", "*.cc", "*.cxx", "*.hpp", "*.hxx", "*.h"}
	for _, pattern := range patterns {
		matches, err := filepath.Glob(filepath.Join(projectPath, pattern))
		if err == nil && len(matches) > 0 {
			return true
		}
	}

	srcDir := filepath.Join(projectPath, "src")
	if info, err := os.Stat(srcDir); err == nil && info.IsDir() {
		for _, pattern := range patterns {
			matches, err := filepath.Glob(filepath.Join(srcDir, pattern))
			if err == nil && len(matches) > 0 {
				return true
			}
		}
	}

	return false
}

// Extract retrieves metadata from a C++ project
func (e *Extractor) Extract(projectPath string) (*extractor.ProjectMetadata, error) {
	metadata := &extractor.ProjectMetadata{
		LanguageSpecific: make(map[string]interface{}),
	}

	// Try CMakeLists.txt first
	cmakePath := filepath.Join(projectPath, "CMakeLists.txt")
	if _, err := os.Stat(cmakePath); err == nil {
		if err := e.extractFromCMake(cmakePath, metadata); err == nil {
			metadata.LanguageSpecific["build_system"] = "CMake"
			return metadata, nil
		}
	}

	// Try Qt qmake
	qmakePath := filepath.Join(projectPath, ".qmake.conf")
	if _, err := os.Stat(qmakePath); err == nil {
		if err := e.extractFromQmake(qmakePath, metadata); err == nil {
			metadata.LanguageSpecific["build_system"] = "qmake"
			return metadata, nil
		}
	}

	// Try Meson
	mesonPath := filepath.Join(projectPath, "meson.build")
	if _, err := os.Stat(mesonPath); err == nil {
		if err := e.extractFromMeson(mesonPath, metadata); err == nil {
			metadata.LanguageSpecific["build_system"] = "Meson"
			return metadata, nil
		}
	}

	// Try Autotools
	configurePath := filepath.Join(projectPath, "configure.ac")
	if _, err := os.Stat(configurePath); err == nil {
		if err := e.extractFromAutotools(configurePath, metadata); err == nil {
			metadata.LanguageSpecific["build_system"] = "Autotools"
			return metadata, nil
		}
	}

	// Fallback to basic detection
	metadata.LanguageSpecific["build_system"] = "Makefile"
	return metadata, nil
}

// extractFromCMake parses CMakeLists.txt
func (e *Extractor) extractFromCMake(path string, metadata *extractor.ProjectMetadata) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	// Regex patterns
	projectRegex := regexp.MustCompile(`(?i)project\s*\(\s*([^\s)]+)(?:\s+VERSION\s+([0-9.]+))?(?:\s+DESCRIPTION\s+"([^"]+)")?`)
	cxxStandardRegex := regexp.MustCompile(`(?i)set\s*\(\s*CMAKE_CXX_STANDARD\s+(\d+)\s*\)`)
	cStandardRegex := regexp.MustCompile(`(?i)set\s*\(\s*CMAKE_C_STANDARD\s+(\d+)\s*\)`)
	addExecutableRegex := regexp.MustCompile(`(?i)add_executable\s*\(\s*([^\s)]+)`)
	addLibraryRegex := regexp.MustCompile(`(?i)add_library\s*\(\s*([^\s)]+)`)
	findPackageRegex := regexp.MustCompile(`(?i)find_package\s*\(\s*([^\s)]+)`)

	var executables []string
	var libraries []string
	var dependencies []string

	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)

		// Skip comments
		if strings.HasPrefix(line, "#") {
			continue
		}

		if matches := projectRegex.FindStringSubmatch(line); matches != nil {
			metadata.Name = matches[1]
			if len(matches) > 2 && matches[2] != "" {
				metadata.Version = matches[2]
				metadata.VersionSource = "CMakeLists.txt"
			}
			if len(matches) > 3 && matches[3] != "" {
				metadata.Description = matches[3]
			}
		}

		if matches := cxxStandardRegex.FindStringSubmatch(line); matches != nil {
			metadata.LanguageSpecific["cxx_standard"] = matches[1]
		}

		if matches := cStandardRegex.FindStringSubmatch(line); matches != nil {
			metadata.LanguageSpecific["c_standard"] = matches[1]
		}

		if matches := addExecutableRegex.FindStringSubmatch(line); matches != nil {
			executables = append(executables, matches[1])
		}

		if matches := addLibraryRegex.FindStringSubmatch(line); matches != nil {
			libraries = append(libraries, matches[1])
		}

		if matches := findPackageRegex.FindStringSubmatch(line); matches != nil {
			dependencies = append(dependencies, matches[1])
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	// Store extracted information
	if len(executables) > 0 {
		metadata.LanguageSpecific["executables"] = executables
	}
	if len(libraries) > 0 {
		metadata.LanguageSpecific["libraries"] = libraries
	}
	if len(dependencies) > 0 {
		metadata.LanguageSpecific["dependencies"] = dependencies
		metadata.LanguageSpecific["dependency_count"] = len(dependencies)
	}

	return nil
}

// extractFromQmake parses .qmake.conf
func (e *Extractor) extractFromQmake(path string, metadata *extractor.ProjectMetadata) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	// Regex patterns for qmake configuration
	moduleVersionRegex := regexp.MustCompile(`MODULE_VERSION\s*=\s*([0-9]+\.[0-9]+(?:\.[0-9]+)?)`)
	versionRegex := regexp.MustCompile(`VERSION\s*=\s*([0-9]+\.[0-9]+(?:\.[0-9]+)?)`)

	for scanner.Scan() {
		line := scanner.Text()

		if matches := moduleVersionRegex.FindStringSubmatch(line); len(matches) > 1 {
			metadata.Version = matches[1]
			metadata.VersionSource = ".qmake.conf"
		}

		// Check for VERSION (if MODULE_VERSION not found)
		if metadata.Version == "" {
			if matches := versionRegex.FindStringSubmatch(line); len(matches) > 1 {
				metadata.Version = matches[1]
				metadata.VersionSource = ".qmake.conf"
			}
		}
	}

	return scanner.Err()
}

// stripMesonComments removes single-line comments from Meson build file content.
// Meson uses # for comments, similar to Python.
func stripMesonComments(content string) string {
	var result strings.Builder
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		// Find comment start (# not inside a string)
		inString := false
		stringChar := rune(0)
		commentStart := -1
		for i, ch := range line {
			if !inString && (ch == '\'' || ch == '"') {
				inString = true
				stringChar = ch
			} else if inString && ch == stringChar {
				// Determine if this quote is escaped by counting consecutive backslashes before it.
				backslashCount := 0
				for j := i - 1; j >= 0 && line[j] == '\\'; j-- {
					backslashCount++
				}
				// Only treat the quote as closing the string if there is an even number of backslashes.
				if backslashCount%2 == 0 {
					inString = false
				}
			} else if !inString && ch == '#' {
				commentStart = i
				break
			}
		}
		if commentStart >= 0 {
			result.WriteString(line[:commentStart])
		} else {
			result.WriteString(line)
		}
		result.WriteString("\n")
	}
	return result.String()
}

// extractFromMeson parses meson.build
func (e *Extractor) extractFromMeson(path string, metadata *extractor.ProjectMetadata) error {
	// Read entire file to handle multi-line project() declarations
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	// Strip comments before applying regex patterns to avoid matching commented code
	fileContent := stripMesonComments(string(content))

	// Regex for project name - matches project('name', ...)
	projectNameRegex := regexp.MustCompile(`project\s*\(\s*'([^']+)'`)
	// Regex for version - can be on same line or different line within project()
	// Use (?s) for DOTALL mode to match across newlines
	projectVersionRegex := regexp.MustCompile(`(?s)project\s*\([^)]*version\s*:\s*'([^']+)'`)
	executableRegex := regexp.MustCompile(`executable\s*\(\s*'([^']+)'`)
	libraryRegex := regexp.MustCompile(`(?:shared_|static_)?library\s*\(\s*'([^']+)'`)
	dependencyRegex := regexp.MustCompile(`dependency\s*\(\s*'([^']+)'`)

	var executables []string
	var libraries []string
	var dependencies []string

	if matches := projectNameRegex.FindStringSubmatch(fileContent); matches != nil {
		metadata.Name = matches[1]
	}

	// Extract version (handles multi-line project declarations)
	if matches := projectVersionRegex.FindStringSubmatch(fileContent); matches != nil {
		metadata.Version = matches[1]
		metadata.VersionSource = "meson.build"
	}

	execMatches := executableRegex.FindAllStringSubmatch(fileContent, -1)
	for _, match := range execMatches {
		if len(match) > 1 {
			executables = append(executables, match[1])
		}
	}

	libMatches := libraryRegex.FindAllStringSubmatch(fileContent, -1)
	for _, match := range libMatches {
		if len(match) > 1 {
			libraries = append(libraries, match[1])
		}
	}

	depMatches := dependencyRegex.FindAllStringSubmatch(fileContent, -1)
	for _, match := range depMatches {
		if len(match) > 1 {
			dependencies = append(dependencies, match[1])
		}
	}

	if len(executables) > 0 {
		metadata.LanguageSpecific["executables"] = executables
	}
	if len(libraries) > 0 {
		metadata.LanguageSpecific["libraries"] = libraries
	}
	if len(dependencies) > 0 {
		metadata.LanguageSpecific["dependencies"] = dependencies
		metadata.LanguageSpecific["dependency_count"] = len(dependencies)
	}

	return nil
}

// extractFromAutotools parses configure.ac
func (e *Extractor) extractFromAutotools(path string, metadata *extractor.ProjectMetadata) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	acInitRegex := regexp.MustCompile(`AC_INIT\s*\(\s*\[?([^\],]+)\]?\s*,\s*\[?([^\],]+)\]?`)
	pkgCheckRegex := regexp.MustCompile(`PKG_CHECK_MODULES\s*\(\s*\[?[^\],]+\]?\s*,\s*\[?([^\],]+)\]?`)

	var dependencies []string

	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, "dnl") {
			continue
		}

		if matches := acInitRegex.FindStringSubmatch(line); matches != nil {
			metadata.Name = strings.TrimSpace(matches[1])
			if len(matches) > 2 {
				metadata.Version = strings.TrimSpace(matches[2])
				metadata.VersionSource = "configure.ac"
			}
		}

		if matches := pkgCheckRegex.FindStringSubmatch(line); matches != nil {
			dep := strings.TrimSpace(matches[1])
			dep = strings.Split(dep, " ")[0]
			dependencies = append(dependencies, dep)
		}
	}

	if len(dependencies) > 0 {
		metadata.LanguageSpecific["dependencies"] = dependencies
		metadata.LanguageSpecific["dependency_count"] = len(dependencies)
	}

	return scanner.Err()
}
