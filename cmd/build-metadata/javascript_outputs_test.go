// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package main

import "testing"

// javascriptFixtures holds Node.js projects that, between them, make the
// extractor report every package manager and workspace output.
const javascriptFixtures = "../../internal/extractor/javascript/testdata"

// Callers such as node-workflows choose how to install dependencies from
// these outputs, so each must be both written by the extractor and
// declared in action.yaml; an undeclared one never reaches the caller.
func TestJavaScriptPackageManagerOutputsAreDeclared(t *testing.T) {
	declared := declaredOutputs(t)
	written := writtenOutputs(t, javascriptFixtures, "javascript-npm")

	for _, name := range []string{
		"javascript_package_manager",
		"javascript_package_manager_version",
		"javascript_lock_file",
		"javascript_has_lock_file",
		"javascript_is_workspace",
	} {
		if !written[name] {
			t.Errorf("%s is never written by any JavaScript fixture", name)
		}
		value, ok := declared[name]
		if !ok {
			t.Errorf("%s is written but not declared in action.yaml", name)
			continue
		}
		if want := "${{ steps.extract.outputs." + name + " }}"; value != want {
			t.Errorf("output %s maps to %q, want %q", name, value, want)
		}
	}
}
