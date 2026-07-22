// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/sethvargo/go-githubactions"
)

// runConfig holds the resolved action inputs for a single invocation.
type runConfig struct {
	verboseOutput      bool
	absPath            string
	outputFormats      []string
	includeEnvironment bool
	useVersionExtract  bool
	artifactUpload     bool
	artifactNamePrefix string
	artifactFormats    []string
	validateOutput     bool
	exportEnvVars      bool
	pythonOffline      bool
	pythonTimeout      time.Duration
	pythonRetries      int
}

// parseFlags resolves every action input. Failure to resolve the
// project path is fatal and terminates the process, matching the
// original behavior (action.Fatalf in CI, os.Exit(1) locally).
func parseFlags(action *githubactions.Action, isCI bool) runConfig {
	verboseOutput := action.GetInput("verbose") == "true"

	projectPath := action.GetInput("path_prefix")
	if projectPath == "" {
		projectPath = "."
	}

	// Convert to absolute path
	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		if isCI {
			action.Fatalf("Failed to resolve project path: %v", err)
		} else {
			fmt.Fprintf(os.Stderr, "Error: Failed to resolve project path: %v\n", err)
			os.Exit(1)
		}
	}

	// Artifact upload inputs
	artifactNamePrefix := action.GetInput("artifact_name_prefix")
	if artifactNamePrefix == "" {
		artifactNamePrefix = "build-metadata"
	}
	artifactFormatsInput := action.GetInput("artifact_formats")
	if artifactFormatsInput == "" {
		artifactFormatsInput = "json"
	}

	pythonTimeout, pythonRetries := parsePythonEOLSettings(action)

	return runConfig{
		verboseOutput: verboseOutput,
		absPath:       absPath,
		// Output formats can be comma, space, or newline separated. An
		// explicit empty string disables output; when unset the
		// action.yaml default ("summary") is already applied upstream.
		outputFormats:      parseMultiSeparatorInput(action.GetInput("output_format")),
		includeEnvironment: action.GetInput("include_environment") != "false",
		useVersionExtract:  action.GetInput("use_version_extract") != "false",
		artifactUpload:     action.GetInput("artifact_upload") != "false",
		artifactNamePrefix: artifactNamePrefix,
		artifactFormats:    parseMultiSeparatorInput(artifactFormatsInput),
		validateOutput:     action.GetInput("validate_output") != "false",
		exportEnvVars:      action.GetInput("export_env_vars") == "true",
		pythonOffline:      action.GetInput("python_offline_mode") == "true",
		pythonTimeout:      pythonTimeout,
		pythonRetries:      pythonRetries,
	}
}

// parsePythonEOLSettings parses the Python end-of-life probe timeout
// and retry inputs. The fallback values MUST stay aligned with the
// defaults declared in action.yaml, which is the single source of
// truth for user-facing defaults; these are only consulted when the
// action runs outside GitHub Actions or the supplied input is
// unparsable.
func parsePythonEOLSettings(action *githubactions.Action) (time.Duration, int) {
	const (
		defaultPythonEOLTimeoutSeconds = 5 // matches action.yaml
		defaultPythonEOLMaxRetries     = 2 // matches action.yaml
	)

	timeout := time.Duration(defaultPythonEOLTimeoutSeconds) * time.Second
	if raw := action.GetInput("python_eol_timeout"); raw != "" {
		if parsed, perr := strconv.Atoi(raw); perr == nil && parsed > 0 {
			timeout = time.Duration(parsed) * time.Second
		}
	}

	retries := defaultPythonEOLMaxRetries
	if raw := action.GetInput("python_eol_max_retries"); raw != "" {
		if parsed, perr := strconv.Atoi(raw); perr == nil && parsed >= 0 {
			retries = parsed
		}
	}

	return timeout, retries
}

// parseMultiSeparatorInput normalizes input that can be comma, space, or newline separated
// into a slice of trimmed strings. Empty strings are filtered out.
func parseMultiSeparatorInput(input string) []string {
	if input == "" {
		return []string{}
	}

	// Replace commas and newlines with spaces for uniform splitting
	normalized := strings.ReplaceAll(input, ",", " ")
	normalized = strings.ReplaceAll(normalized, "\n", " ")

	// Split by spaces and filter empty strings
	parts := strings.Fields(normalized)

	return parts
}
