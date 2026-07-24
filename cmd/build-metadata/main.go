// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package main

import (
	"os"

	"github.com/sethvargo/go-githubactions"
)

const (
	// Action metadata
	actionName        = "build-metadata-action"
	actionVersion     = "1.0.0"
	actionDescription = "Universal action to capture and display metadata related to project builds"
)

func main() {
	action := githubactions.New()

	// Detect if running in CI environment
	isCI := os.Getenv("GITHUB_ACTIONS") == "true" || os.Getenv("CI") == "true"

	cfg := parseFlags(action, isCI)
	ctx := &appContext{
		action:        action,
		isCI:          isCI,
		verboseOutput: cfg.verboseOutput,
		exportEnvVars: cfg.exportEnvVars,
	}

	metadata := newMetadata(cfg.absPath)
	populateCIMetadata(metadata)

	projectType := detectProjectType(ctx, metadata, cfg.absPath)
	configureExtractorPolicies(projectType, cfg)
	extractVersionInfo(ctx, cfg, metadata, projectType)
	extractProjectMetadata(ctx, metadata, projectType, cfg.absPath)
	applyVersionProperties(metadata, cfg.absPath)
	applyReleaseFiles(metadata, cfg.absPath)
	collectEnvironmentMetadata(ctx, cfg, metadata)

	emitCommonOutputs(ctx, metadata)
	emitProjectMatchRepo(ctx, metadata)
	emitLanguageSpecificOutputs(ctx, metadata, projectType)
	metadataJSON := emitMetadataJSON(ctx, metadata)
	writeOutputFormats(ctx, cfg, metadata, metadataJSON)
	uploadArtifacts(ctx, cfg, metadata)
	printCompletionSummary(ctx, metadata)

	// Set success indicator
	ctx.setOutput("success", "true")
}
