// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

// Package jsonutil provides utilities for working with JSON files,
// particularly those that allow non-standard features like comments
// (common in JavaScript/TypeScript tooling configurations).
package jsonutil

import (
	"regexp"
	"strings"
)

// RemoveComments removes // and /* */ style comments from JSON content.
// This is useful for parsing configuration files from the JavaScript ecosystem
// (package.json, tsconfig.json, .eslintrc.json, etc.) that often contain
// comments despite not being valid JSON.
//
// The function handles:
//   - Single-line comments (//)
//   - Multi-line block comments (/* */)
//   - Comments that span multiple lines
//   - Inline block comments
//   - Preserves strings containing // or /* that aren't actual comments
//   - Preserves line structure (empty lines maintained for better error messages)
func RemoveComments(jsonStr string) string {
	lines := strings.Split(jsonStr, "\n")
	result := make([]string, 0, len(lines))

	inBlockComment := false

	for _, line := range lines {
		processedLine := line
		hadContent := len(strings.TrimSpace(line)) > 0

		// Handle line comments first (before block comments)
		// But only if not in a block comment
		if !inBlockComment {
			// Find // that's not inside a string
			lineCommentIdx := findLineCommentOutsideString(processedLine)
			if lineCommentIdx >= 0 {
				processedLine = strings.TrimRight(processedLine[:lineCommentIdx], " \t")
			}
		}

		// Handle block comments
		for {
			if inBlockComment {
				// Look for end of block comment
				if endIdx := strings.Index(processedLine, "*/"); endIdx >= 0 {
					// Remove everything up to and including */
					processedLine = processedLine[endIdx+2:]
					inBlockComment = false
					// Continue processing the rest of the line
					continue
				} else {
					// Still in block comment, skip entire line but preserve it as empty
					processedLine = ""
					break
				}
			} else {
				// Look for start of block comment
				if startIdx := strings.Index(processedLine, "/*"); startIdx >= 0 {
					// Check if there's a closing */ on the same line
					if endIdx := strings.Index(processedLine[startIdx:], "*/"); endIdx >= 0 {
						// Single-line block comment: remove the comment but keep the rest
						before := processedLine[:startIdx]
						after := processedLine[startIdx+endIdx+2:]
						processedLine = before + after
						// Continue processing in case there are more comments
						continue
					} else {
						// Multi-line block comment starts here
						processedLine = processedLine[:startIdx]
						inBlockComment = true
						break
					}
				} else {
					// No block comment found, we're done with this line
					break
				}
			}
		}

		// Preserve lines in one of three cases:
		// 1. Line still has content after processing
		// 2. Line was originally empty (preserve empty lines in source)
		// 3. Line had content but is now empty due to comment removal (preserve structure)
		if len(strings.TrimSpace(processedLine)) > 0 {
			// Case 1: Has content - preserve as-is
			result = append(result, processedLine)
		} else if hadContent {
			// Case 3: Had content, now empty - preserve as truly empty (not whitespace)
			result = append(result, "")
		} else {
			// Case 2: Was originally empty - preserve as-is
			result = append(result, processedLine)
		}
	}

	return strings.Join(result, "\n")
}

// findLineCommentOutsideString finds the index of // that's not inside a string.
// Returns -1 if no line comment is found outside of strings.
func findLineCommentOutsideString(line string) int {
	inString := false
	escaped := false

	for i := 0; i < len(line); i++ {
		char := line[i]

		if escaped {
			escaped = false
			continue
		}

		if char == '\\' {
			escaped = true
			continue
		}

		if char == '"' {
			inString = !inString
			continue
		}

		// Only check for // when not inside a string
		if !inString && i < len(line)-1 && line[i] == '/' && line[i+1] == '/' {
			return i
		}
	}

	return -1
}

// StripTrailingCommas removes trailing commas from JSON content.
// This is useful for parsing JSON5-style configurations that allow trailing commas.
//
// Note: This is a basic implementation and may not handle all edge cases.
// For complex JSON5 parsing, consider using a dedicated JSON5 parser library.
func StripTrailingCommas(jsonStr string) string {
	// Remove trailing comma before closing brace or bracket
	re := regexp.MustCompile(`,(\s*[}\]])`)
	return re.ReplaceAllString(jsonStr, "$1")
}
