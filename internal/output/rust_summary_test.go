// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package output

import (
	"strings"
	"testing"
)

func TestRustRowsShowTheToolchain(t *testing.T) {
	var sb strings.Builder
	addLanguageSpecificToTable(&sb, "rust-cargo", map[string]interface{}{
		"edition":           "2024",
		"toolchain_kind":    "channel",
		"toolchain_channel": "1.85.0",
	})
	if want := "| Rust Toolchain | `1.85.0` |\n"; !strings.Contains(sb.String(), want) {
		t.Errorf("rows %q lack %q", sb.String(), want)
	}

	sb.Reset()
	addLanguageSpecificToTable(&sb, "rust-cargo", map[string]interface{}{
		"edition":        "2024",
		"toolchain_kind": "path",
	})
	if strings.Contains(sb.String(), "Rust Toolchain") {
		t.Errorf("a path toolchain rendered a toolchain row: %q", sb.String())
	}
}
