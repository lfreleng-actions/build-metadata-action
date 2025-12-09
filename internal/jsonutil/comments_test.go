// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package jsonutil

import (
	"testing"
)

func TestRemoveComments(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no comments",
			input:    `{"key": "value"}`,
			expected: `{"key": "value"}`,
		},
		{
			name: "single line comment",
			input: `{
  "key": "value", // this is a comment
  "other": "data"
}`,
			expected: `{
  "key": "value",
  "other": "data"
}`,
		},
		{
			name: "full line comment",
			input: `{
  // This is a comment
  "key": "value"
}`,
			expected: `{

  "key": "value"
}`,
		},
		{
			name: "multi-line block comment",
			input: `{
  "key": "value",
  /* this is a
     multi-line comment */
  "other": "data"
}`,
			expected: `{
  "key": "value",


  "other": "data"
}`,
		},
		{
			name: "single-line block comment",
			input: `{
  "key": /* inline comment */ "value"
}`,
			expected: `{
  "key":  "value"
}`,
		},
		{
			name: "multiple single-line block comments",
			input: `{
  "key": /* comment1 */ "value", /* comment2 */ "end"
}`,
			expected: `{
  "key":  "value",  "end"
}`,
		},
		{
			name: "block comment at start",
			input: `/* header comment */
{
  "key": "value"
}`,
			expected: `
{
  "key": "value"
}`,
		},
		{
			name: "block comment at end",
			input: `{
  "key": "value"
}
/* footer comment */`,
			expected: `{
  "key": "value"
}
`,
		},
		{
			name: "mixed comments",
			input: `{
  // line comment
  "key": /* inline */ "value", // another line comment
  /* block
     comment */ "other": "data"
}`,
			expected: `{

  "key":  "value",

 "other": "data"
}`,
		},
		{
			name: "line comment with forward slashes in string",
			input: `{
  "url": "https://example.com", // actual comment
  "path": "some/path/here"
}`,
			expected: `{
  "url": "https://example.com",
  "path": "some/path/here"
}`,
		},
		{
			name: "empty lines preserved",
			input: `{
  "key": "value",

  "other": "data"
}`,
			expected: `{
  "key": "value",

  "other": "data"
}`,
		},
		{
			name:     "empty input",
			input:    ``,
			expected: ``,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RemoveComments(tt.input)
			if result != tt.expected {
				t.Errorf("RemoveComments() mismatch\nInput:\n%s\n\nExpected:\n%s\n\nGot:\n%s",
					tt.input, tt.expected, result)
			}
		})
	}
}

func TestFindLineCommentOutsideString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{
			name:     "no comment",
			input:    `{"key": "value"}`,
			expected: -1,
		},
		{
			name:     "comment at end",
			input:    `"key": "value" // comment`,
			expected: 15,
		},
		{
			name:     "comment in middle",
			input:    `"key": // comment`,
			expected: 7,
		},
		{
			name:     "double slash in string",
			input:    `"url": "https://example.com"`,
			expected: -1,
		},
		{
			name:     "double slash in string with comment after",
			input:    `"url": "https://example.com" // real comment`,
			expected: 29,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findLineCommentOutsideString(tt.input)
			if result != tt.expected {
				t.Errorf("findLineCommentOutsideString(%q) = %d, expected %d",
					tt.input, result, tt.expected)
			}
		})
	}
}

func TestStripTrailingCommas(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no trailing comma",
			input:    `{"key": "value"}`,
			expected: `{"key": "value"}`,
		},
		{
			name:     "trailing comma in object",
			input:    `{"key": "value",}`,
			expected: `{"key": "value"}`,
		},
		{
			name:     "trailing comma in array",
			input:    `["one", "two",]`,
			expected: `["one", "two"]`,
		},
		{
			name: "trailing comma with whitespace",
			input: `{
  "key": "value",
}`,
			expected: `{
  "key": "value"
}`,
		},
		{
			name: "multiple trailing commas",
			input: `{
  "obj": {"nested": "value",},
  "arr": [1, 2, 3,],
}`,
			expected: `{
  "obj": {"nested": "value"},
  "arr": [1, 2, 3]
}`,
		},
		{
			name:     "comma not at end",
			input:    `{"key": "value", "other": "data"}`,
			expected: `{"key": "value", "other": "data"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StripTrailingCommas(tt.input)
			if result != tt.expected {
				t.Errorf("StripTrailingCommas() mismatch\nInput:\n%s\n\nExpected:\n%s\n\nGot:\n%s",
					tt.input, tt.expected, result)
			}
		})
	}
}
