// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package pyversions

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Constraint represents a single version constraint
type Constraint struct {
	Operator string // >=, >, <, <=, ==, !=, ~=, ^
	Version  string // Version number (e.g., "3.10")
}

// ParseConstraints parses a requires-python string into individual constraints
// Examples: ">=3.10", ">=3.10,<3.14", "~=3.10", "^3.10"
func ParseConstraints(requiresPython string) ([]Constraint, error) {
	if requiresPython == "" {
		return nil, fmt.Errorf("empty constraint string")
	}

	// Normalize the constraint string
	normalized := NormalizeConstraint(requiresPython)

	// Split by comma
	parts := strings.Split(normalized, ",")
	constraints := make([]Constraint, 0, len(parts))

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		constraint, err := parseConstraint(part)
		if err != nil {
			return nil, fmt.Errorf("failed to parse constraint '%s': %w", part, err)
		}
		constraints = append(constraints, constraint)
	}

	if len(constraints) == 0 {
		return nil, fmt.Errorf("no valid constraints found")
	}

	return constraints, nil
}

// parseConstraint parses a single constraint like ">=3.10" or "!=3.11"
func parseConstraint(s string) (Constraint, error) {
	// Match operator and version
	// Operators: >=, >, <=, <, ==, !=, ~=, ^
	re := regexp.MustCompile(`^(>=|>|<=|<|==|!=|~=|\^)\s*(\d+\.\d+(?:\.\d+)?)$`)
	matches := re.FindStringSubmatch(s)

	if len(matches) != 3 {
		return Constraint{}, fmt.Errorf("invalid constraint format: %s", s)
	}

	operator := matches[1]
	version := matches[2]

	// Strip patch version if present (3.10.1 -> 3.10)
	version = stripPatchVersion(version)

	return Constraint{
		Operator: operator,
		Version:  version,
	}, nil
}

// NormalizeConstraint normalizes various constraint formats to standard format
// Handles ~=, ^, ==X.Y.*, and patch version normalization
func NormalizeConstraint(constraint string) string {
	constraint = strings.TrimSpace(constraint)

	// Don't normalize exclusions (!=)
	if strings.HasPrefix(constraint, "!") {
		// Just strip patch versions from != constraints
		return stripPatchVersionFromOperators(constraint)
	}

	// Handle Poetry caret with patch: ^3.10.1 -> >=3.10,<4.0
	if re := regexp.MustCompile(`^\^(\d+)\.(\d+)\.(\d+)$`); re.MatchString(constraint) {
		matches := re.FindStringSubmatch(constraint)
		if len(matches) == 4 {
			major, _ := strconv.Atoi(matches[1])
			minor := matches[2]
			nextMajor := major + 1
			return fmt.Sprintf(">=%s.%s,<%d.0", matches[1], minor, nextMajor)
		}
	}

	// Handle Poetry caret: ^3.10 -> >=3.10,<4.0
	if re := regexp.MustCompile(`^\^(\d+)\.(\d+)$`); re.MatchString(constraint) {
		matches := re.FindStringSubmatch(constraint)
		if len(matches) == 3 {
			major, _ := strconv.Atoi(matches[1])
			minor := matches[2]
			nextMajor := major + 1
			return fmt.Sprintf(">=%s.%s,<%d.0", matches[1], minor, nextMajor)
		}
	}

	// Handle compatible release with patch: ~=3.10.1 -> >=3.10,<3.11
	if re := regexp.MustCompile(`^~=(\d+)\.(\d+)\.(\d+)$`); re.MatchString(constraint) {
		matches := re.FindStringSubmatch(constraint)
		if len(matches) == 4 {
			major := matches[1]
			minor, _ := strconv.Atoi(matches[2])
			nextMinor := minor + 1
			return fmt.Sprintf(">=%s.%d,<%s.%d", major, minor, major, nextMinor)
		}
	}

	// Handle compatible release: ~=3.10 -> >=3.10,<3.11
	if re := regexp.MustCompile(`^~=(\d+)\.(\d+)$`); re.MatchString(constraint) {
		matches := re.FindStringSubmatch(constraint)
		if len(matches) == 3 {
			major := matches[1]
			minor, _ := strconv.Atoi(matches[2])
			nextMinor := minor + 1
			return fmt.Sprintf(">=%s.%d,<%s.%d", major, minor, major, nextMinor)
		}
	}

	// Handle wildcard: ==3.10.* -> >=3.10,<3.11
	if re := regexp.MustCompile(`^==(\d+)\.(\d+)\.\*$`); re.MatchString(constraint) {
		matches := re.FindStringSubmatch(constraint)
		if len(matches) == 3 {
			major := matches[1]
			minor, _ := strconv.Atoi(matches[2])
			nextMinor := minor + 1
			return fmt.Sprintf(">=%s.%d,<%s.%d", major, minor, major, nextMinor)
		}
	}

	// Strip patch versions from bounds: <3.13.0 -> <3.13
	return stripPatchVersionFromOperators(constraint)
}

// stripPatchVersionFromOperators removes patch versions from operators in a constraint string
func stripPatchVersionFromOperators(constraint string) string {
	// Match operators followed by version with patch
	re := regexp.MustCompile(`([<>=!~^]+)(\d+\.\d+)\.\d+`)
	return re.ReplaceAllString(constraint, "$1$2")
}

// stripPatchVersion removes the patch version if present (3.10.1 -> 3.10)
func stripPatchVersion(version string) string {
	parts := strings.Split(version, ".")
	if len(parts) >= 2 {
		return parts[0] + "." + parts[1]
	}
	return version
}

// FilterVersions filters a list of versions based on constraints
func FilterVersions(versions []string, constraints []Constraint) ([]string, error) {
	if len(constraints) == 0 {
		return versions, nil
	}

	var filtered []string
	for _, version := range versions {
		if matchesAllConstraints(version, constraints) {
			filtered = append(filtered, version)
		}
	}

	return filtered, nil
}

// matchesAllConstraints checks if a version satisfies all constraints
func matchesAllConstraints(version string, constraints []Constraint) bool {
	for _, c := range constraints {
		if !matchesConstraint(version, c) {
			return false
		}
	}
	return true
}

// matchesConstraint checks if a version satisfies a single constraint
func matchesConstraint(version string, constraint Constraint) bool {
	cmp := compareVersions(version, constraint.Version)

	switch constraint.Operator {
	case ">=":
		return cmp >= 0
	case ">":
		return cmp > 0
	case "<=":
		return cmp <= 0
	case "<":
		return cmp < 0
	case "==":
		return cmp == 0
	case "!=":
		return cmp != 0
	case "~=":
		// Compatible release: matches same major.minor
		return cmp >= 0 && hasSameMinorVersion(version, constraint.Version)
	case "^":
		// Poetry caret: matches same major
		return cmp >= 0 && hasSameMajorVersion(version, constraint.Version)
	default:
		return false
	}
}

// compareVersions compares two version strings
// Returns: -1 if v1 < v2, 0 if v1 == v2, 1 if v1 > v2
func compareVersions(v1, v2 string) int {
	// Parse versions
	var major1, minor1, major2, minor2 int
	fmt.Sscanf(v1, "%d.%d", &major1, &minor1)
	fmt.Sscanf(v2, "%d.%d", &major2, &minor2)

	// Compare major version
	if major1 < major2 {
		return -1
	}
	if major1 > major2 {
		return 1
	}

	// Compare minor version
	if minor1 < minor2 {
		return -1
	}
	if minor1 > minor2 {
		return 1
	}

	return 0
}

// hasSameMajorVersion checks if two versions have the same major version
func hasSameMajorVersion(v1, v2 string) bool {
	var major1, minor1, major2, minor2 int
	fmt.Sscanf(v1, "%d.%d", &major1, &minor1)
	fmt.Sscanf(v2, "%d.%d", &major2, &minor2)
	return major1 == major2
}

// hasSameMinorVersion checks if two versions have the same major.minor version
func hasSameMinorVersion(v1, v2 string) bool {
	var major1, minor1, major2, minor2 int
	fmt.Sscanf(v1, "%d.%d", &major1, &minor1)
	fmt.Sscanf(v2, "%d.%d", &major2, &minor2)
	return major1 == major2 && minor1 == minor2
}

// ResolveVersions resolves the final list of Python versions based on:
// - requires-python constraint
// - supported versions (from API or fallback)
// - EOL filtering
func ResolveVersions(requiresPython string, supportedVersions []string) ([]string, error) {
	if requiresPython == "" {
		return nil, fmt.Errorf("requires-python constraint is empty")
	}

	if len(supportedVersions) == 0 {
		return nil, fmt.Errorf("no supported versions available")
	}

	// Parse constraints
	constraints, err := ParseConstraints(requiresPython)
	if err != nil {
		return nil, fmt.Errorf("failed to parse constraints: %w", err)
	}

	// Filter versions based on constraints
	filtered, err := FilterVersions(supportedVersions, constraints)
	if err != nil {
		return nil, fmt.Errorf("failed to filter versions: %w", err)
	}

	if len(filtered) == 0 {
		return nil, fmt.Errorf("no versions match the constraint '%s'", requiresPython)
	}

	return filtered, nil
}
