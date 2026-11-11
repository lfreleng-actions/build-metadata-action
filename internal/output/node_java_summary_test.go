// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package output

import (
	"strings"
	"testing"
)

func TestJSRowsShowTheNodeMatrix(t *testing.T) {
	var sb strings.Builder
	addLanguageSpecificToTable(&sb, "javascript-npm", map[string]interface{}{
		"requires_node":    ">=24",
		"node_matrix_json": `{"node-version": ["24", "26"]}`,
		"node_latest":      "26",
	})
	for _, want := range []string{
		"| Node.js Matrix JSON (node_matrix_json) | `{\"node-version\": [\"24\", \"26\"]}` |\n",
		"| Latest Node.js Line (node_latest) | 26 |\n",
	} {
		if !strings.Contains(sb.String(), want) {
			t.Errorf("rows %q lack %q", sb.String(), want)
		}
	}
}
