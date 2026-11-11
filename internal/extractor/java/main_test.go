// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package java

import (
	"fmt"
	"os"
	"testing"
)

// TestMain clears GITHUB_WORKSPACE before the tests run. The fixtures
// live in temporary directories, which a GitHub Actions runner's
// workspace never contains, so an inherited value would make the
// workspace-boundary checks skip every scan. Tests that exercise the
// boundary set the variable themselves with t.Setenv.
func TestMain(m *testing.M) {
	if err := os.Unsetenv("GITHUB_WORKSPACE"); err != nil {
		fmt.Fprintf(os.Stderr, "unsetting GITHUB_WORKSPACE: %v\n", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}
