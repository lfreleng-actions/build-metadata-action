// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/lfreleng-actions/build-metadata-action/internal/extractor"
	"github.com/sethvargo/go-githubactions"
	"gopkg.in/yaml.v3"
)

// rustFixtures holds Rust projects that, between them, make the extractor
// report every key it can.
const rustFixtures = "../../internal/extractor/rust/testdata"

// outputLine matches the heredoc header go-githubactions writes for each
// output: name<<delimiter.
var outputLine = regexp.MustCompile(`(?m)^([A-Za-z0-9_]+)<<\S+$`)

func declaredOutputs(t *testing.T) map[string]string {
	t.Helper()
	body, err := os.ReadFile(filepath.Clean(actionYAML))
	if err != nil {
		t.Fatalf("reading %s: %v", actionYAML, err)
	}
	var action struct {
		Outputs map[string]struct {
			Value string `yaml:"value"`
		} `yaml:"outputs"`
	}
	if err := yaml.Unmarshal(body, &action); err != nil {
		t.Fatalf("parsing %s: %v", actionYAML, err)
	}
	values := make(map[string]string, len(action.Outputs))
	for name, output := range action.Outputs {
		values[name] = output.Value
	}
	return values
}

// writtenRustOutputs runs the Rust extractor over each fixture and emits
// its values through the binary's own output path, returning every
// output name written to GITHUB_OUTPUT.
func writtenRustOutputs(t *testing.T) map[string]bool {
	t.Helper()

	fixtures, err := os.ReadDir(rustFixtures)
	if err != nil {
		t.Fatalf("reading %s: %v", rustFixtures, err)
	}
	rustExtractor, err := extractor.GetExtractor("rust-cargo")
	if err != nil {
		t.Fatal(err)
	}

	outputFile := filepath.Join(t.TempDir(), "github_output")
	if err := os.WriteFile(outputFile, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GITHUB_OUTPUT", outputFile)
	ctx := &appContext{action: githubactions.New(), isCI: true}

	for _, fixture := range fixtures {
		if !fixture.IsDir() {
			continue
		}
		dir := filepath.Join(rustFixtures, fixture.Name())
		projectMetadata, err := rustExtractor.Extract(dir)
		if err != nil {
			t.Fatalf("extracting %s: %v", dir, err)
		}
		metadata := newMetadata(dir)
		metadata.LanguageSpecific = projectMetadata.LanguageSpecific
		emitLanguageSpecificOutputs(ctx, metadata, "rust-cargo")
	}

	body, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatal(err)
	}
	written := map[string]bool{}
	for _, match := range outputLine.FindAllStringSubmatch(string(body), -1) {
		written[match[1]] = true
	}
	if len(written) == 0 {
		t.Fatal("the Rust fixtures wrote no outputs; the fixtures or this test are wrong")
	}
	return written
}

// A composite action exposes only the outputs action.yaml declares, so a
// value the binary writes under an undeclared name never reaches the
// caller. Every rust_* output shipped in that state, reachable only by
// parsing metadata_json. Checking the two sets against each other keeps
// the declarations and the extractor in step.
func TestRustOutputsMatchTheirDeclarations(t *testing.T) {
	declared := declaredOutputs(t)
	written := writtenRustOutputs(t)

	var undeclared, unwritten []string
	for name := range written {
		if !strings.HasPrefix(name, "rust_") {
			t.Errorf("Rust extraction wrote %q without the rust_ prefix", name)
			continue
		}
		if _, ok := declared[name]; !ok {
			undeclared = append(undeclared, name)
		}
	}
	for name, value := range declared {
		if !strings.HasPrefix(name, "rust_") {
			continue
		}
		if !written[name] {
			unwritten = append(unwritten, name)
		}
		if want := "${{ steps.extract.outputs." + name + " }}"; value != want {
			t.Errorf("output %s maps to %q, want %q", name, value, want)
		}
	}

	sort.Strings(undeclared)
	sort.Strings(unwritten)
	if len(undeclared) > 0 {
		t.Errorf("written but not declared in action.yaml: %v", undeclared)
	}
	if len(unwritten) > 0 {
		t.Errorf("declared in action.yaml but never written by any Rust fixture: %v", unwritten)
	}
}
