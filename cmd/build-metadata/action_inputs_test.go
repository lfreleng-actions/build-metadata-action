// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// actionYAML is the composite action definition, two directories up from
// this package.
const actionYAML = "../../action.yaml"

type compositeAction struct {
	Inputs map[string]struct {
		Default *string `yaml:"default"`
	} `yaml:"inputs"`
	Runs struct {
		Steps []struct {
			ID  string            `yaml:"id"`
			Env map[string]string `yaml:"env"`
		} `yaml:"steps"`
	} `yaml:"runs"`
}

// extractStepID names the step that runs the binary. Only its env block
// reaches the process, so that is the one the checks below inspect.
const extractStepID = "extract"

func loadAction(t *testing.T) compositeAction {
	t.Helper()
	body, err := os.ReadFile(filepath.Clean(actionYAML))
	if err != nil {
		t.Fatalf("reading %s: %v", actionYAML, err)
	}
	var action compositeAction
	if err := yaml.Unmarshal(body, &action); err != nil {
		t.Fatalf("parsing %s: %v", actionYAML, err)
	}
	return action
}

// forwardedByExtractStep collects the INPUT_* names the binary's own step
// exports.
//
// Scoped to that step deliberately. Gathering them from every step would
// accept a mapping placed on Setup Go, or on some step added later, even
// though the process never sees it -- the test would pass while the
// input stayed inert, which is the failure it exists to catch.
func forwardedByExtractStep(t *testing.T, action compositeAction) map[string]bool {
	t.Helper()

	for _, step := range action.Runs.Steps {
		if step.ID != extractStepID {
			continue
		}
		forwarded := map[string]bool{}
		for name := range step.Env {
			if strings.HasPrefix(name, "INPUT_") {
				forwarded[name] = true
			}
		}
		if len(forwarded) == 0 {
			t.Fatalf("step %q exports no INPUT_* variables", extractStepID)
		}
		return forwarded
	}

	t.Fatalf("no step with id %q found", extractStepID)
	return nil
}

// Every declared input must reach the Go process.
//
// go-githubactions reads inputs from INPUT_* environment variables, and a
// composite action only populates those for the variables its step names
// explicitly. An input declared in action.yaml but absent from that env
// block is accepted from the caller, documented in the README, and then
// silently ignored -- the binary falls back to its default and the
// workflow still succeeds.
//
// project_type shipped in exactly that state: declared, documented, and
// inert, because the mapping was missing. Nothing failed, because the
// unit tests built runConfig directly and never crossed the boundary the
// defect lived on. Asserting the two lists against each other is what
// closes that gap for every input, not just this one.
func TestEveryInputReachesTheBinary(t *testing.T) {
	action := loadAction(t)

	if len(action.Inputs) == 0 {
		t.Fatal("no inputs parsed; the action definition or this test is wrong")
	}

	forwarded := forwardedByExtractStep(t, action)

	for input := range action.Inputs {
		want := "INPUT_" + strings.ToUpper(input)
		if !forwarded[want] {
			t.Errorf("input %q is declared but never forwarded; add %s to the %s step env",
				input, want, extractStepID)
		}
	}
}

// The reverse direction: a mapping naming an input that no longer exists
// is dead weight that outlives the input it served.
func TestForwardedInputsAreDeclared(t *testing.T) {
	action := loadAction(t)

	declared := map[string]bool{}
	for input := range action.Inputs {
		declared["INPUT_"+strings.ToUpper(input)] = true
	}

	for name := range forwardedByExtractStep(t, action) {
		if !declared[name] {
			t.Errorf("%s is forwarded but names no declared input", name)
		}
	}
}
