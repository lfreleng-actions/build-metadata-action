// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/lfreleng-actions/build-metadata-action/internal/output"
	"github.com/sethvargo/go-githubactions"
)

// appContext carries the runtime wiring shared across output emission:
// the GitHub Actions client and the flags that decide how outputs are
// surfaced (CI vs. local, verbosity, environment export).
type appContext struct {
	action        *githubactions.Action
	isCI          bool
	verboseOutput bool
	exportEnvVars bool
}

// setOutput sets an action output. In CI it writes to the GitHub
// Actions output file (optionally also exporting an environment
// variable); locally it prints to stdout only when verbose.
func (c *appContext) setOutput(name, value string) {
	if c.isCI {
		c.action.SetOutput(name, value)
		if c.exportEnvVars && value != "" {
			envName := strings.ToUpper(name)
			if c.verboseOutput {
				c.action.Infof("Exporting environment variable: %s", envName)
			}
			c.action.SetEnv(envName, value)
		}
	} else if c.verboseOutput {
		// Local execution - print to stdout if verbose
		if value != "" {
			fmt.Printf("%s=%s\n", name, value)
		}
	}
}

func emitCommonOutputs(ctx *appContext, metadata *Metadata) {
	ctx.setOutput("project_type", metadata.Common.ProjectType)
	ctx.setOutput("project_name", metadata.Common.ProjectName)
	ctx.setOutput("project_version", metadata.Common.ProjectVersion)
	ctx.setOutput("project_path", metadata.Common.ProjectPath)
	ctx.setOutput("version_source", metadata.Common.VersionSource)
	ctx.setOutput("versioning_type", metadata.Common.VersioningType)
	ctx.setOutput("version_properties_version", metadata.Common.VersionPropertiesVersion)
	ctx.setOutput("version_properties_match", metadata.Common.VersionPropertiesMatch)
	ctx.setOutput("snapshot_version", metadata.Common.SnapshotVersion)
	ctx.setOutput("build_timestamp", metadata.Common.BuildTimestamp.Format(time.RFC3339))
	ctx.setOutput("git_sha", metadata.Common.GitSHA)
	ctx.setOutput("git_branch", metadata.Common.GitBranch)
	ctx.setOutput("git_tag", metadata.Common.GitTag)

	ctx.setOutput("ci_platform", metadata.Build.CIPlatform)
	ctx.setOutput("ci_run_id", metadata.Build.CIRunID)
	ctx.setOutput("ci_run_url", metadata.Build.CIRunURL)
	ctx.setOutput("runner_os", metadata.Build.RunnerOS)
	ctx.setOutput("runner_arch", metadata.Build.RunnerArch)
}

// emitProjectMatchRepo compares the detected project name against the
// GitHub repository name and records the result (common to all project
// types).
func emitProjectMatchRepo(ctx *appContext, metadata *Metadata) {
	if metadata.Common.ProjectName == "" {
		return
	}
	repoFullName := os.Getenv("GITHUB_REPOSITORY")
	if repoFullName == "" {
		return
	}
	parts := strings.Split(repoFullName, "/")
	if len(parts) != 2 {
		return
	}

	repoName := parts[1]
	projectMatchRepo := metadata.Common.ProjectName == repoName
	metadata.Common.ProjectMatchRepo = projectMatchRepo
	ctx.setOutput("project_match_repo", fmt.Sprintf("%t", projectMatchRepo))

	if !ctx.verboseOutput {
		return
	}
	if ctx.isCI {
		if projectMatchRepo {
			ctx.action.Infof("Project name matches repository name: %s", repoName)
		} else {
			ctx.action.Infof("Project name (%s) does not match repository name (%s)", metadata.Common.ProjectName, repoName)
		}
	} else {
		if projectMatchRepo {
			fmt.Printf("Project name matches repository name: %s\n", repoName)
		} else {
			fmt.Printf("Project name (%s) does not match repository name (%s)\n", metadata.Common.ProjectName, repoName)
		}
	}
}

// emitLanguageSpecificOutputs writes each language-specific value under
// a prefix derived from the normalized base language, serializing
// complex types to JSON.
func emitLanguageSpecificOutputs(ctx *appContext, metadata *Metadata, projectType string) {
	outputPrefix := normalizeProjectTypeToLanguage(projectType)

	for key, value := range metadata.LanguageSpecific {
		outputKey := fmt.Sprintf("%s_%s", outputPrefix, key)

		switch v := value.(type) {
		case string:
			ctx.setOutput(outputKey, v)
		case []string:
			ctx.setOutput(outputKey, strings.Join(v, ","))
		default:
			ctx.setOutput(outputKey, formatComplexValue(v))
		}
	}
}

// formatComplexValue renders a language-specific value for output. Scalar
// values use fmt's default representation, while composite values (slices,
// arrays, maps, structs) are JSON-encoded so action consumers receive
// stable, machine-readable output rather than Go's fmt rendering (for
// example []map[string]interface{} used by Helm deps and Swift products).
func formatComplexValue(v interface{}) string {
	if v == nil {
		return ""
	}
	switch reflect.ValueOf(v).Kind() {
	case reflect.Slice, reflect.Array, reflect.Map, reflect.Struct, reflect.Ptr:
		if jsonBytes, err := json.Marshal(v); err == nil {
			return string(jsonBytes)
		}
	}
	return fmt.Sprintf("%v", v)
}

// emitMetadataJSON marshals the full metadata document and publishes it
// as the metadata_json output, returning the marshaled bytes (nil on
// error) for reuse by the output-format writers.
func emitMetadataJSON(ctx *appContext, metadata *Metadata) []byte {
	metadataJSON, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		if ctx.isCI {
			ctx.action.Warningf("Failed to marshal metadata to JSON: %v", err)
		} else {
			fmt.Printf("Warning: Failed to marshal metadata to JSON: %v\n", err)
		}
		return metadataJSON
	}

	ctx.setOutput("metadata_json", string(metadataJSON))
	return metadataJSON
}

// writeOutputFormats renders each requested output format, supporting
// multiple formats per invocation.
func writeOutputFormats(ctx *appContext, cfg runConfig, metadata *Metadata, metadataJSON []byte) {
	for _, format := range cfg.outputFormats {
		format = strings.ToLower(strings.TrimSpace(format))

		switch format {
		case "summary":
			summary := output.GenerateSummary(metadata)
			ctx.action.AddStepSummary(summary)
			if ctx.verboseOutput {
				fmt.Println(summary)
			}

		case "json":
			fmt.Println(string(metadataJSON))

		case "markdown":
			markdown := output.GenerateMarkdown(metadata)
			fmt.Println(markdown)
			ctx.action.SetOutput("markdown_output", markdown)

		case "yaml":
			// YAML output currently emits the JSON representation; native YAML
			// serialisation is not yet implemented.
			ctx.action.SetOutput("metadata_yaml", string(metadataJSON))
			if ctx.verboseOutput {
				ctx.action.Infof("YAML output format requested (using JSON for now)")
			}

		case "both":
			summary := output.GenerateSummary(metadata)
			ctx.action.AddStepSummary(summary)
			fmt.Println(string(metadataJSON))

		case "":
			// Empty string means disable output - skip silently
			continue

		default:
			ctx.action.Warningf("Unknown output format: %s", format)
		}
	}
}

func uploadArtifacts(ctx *appContext, cfg runConfig, metadata *Metadata) {
	if !cfg.artifactUpload {
		return
	}

	ctx.action.Infof("Uploading build metadata artifacts...")

	uploader := output.NewArtifactUploader(
		true,
		cfg.artifactNamePrefix,
		cfg.artifactFormats,
		"", // Use temp dir
		cfg.validateOutput,
		true, // Strict mode
	)

	// Generate job name from context
	jobName := os.Getenv("GITHUB_JOB")
	if jobName == "" {
		jobName = "build"
	}

	artifactResult, err := uploader.Upload(metadata, jobName)
	if err != nil {
		ctx.action.Warningf("Failed to upload artifacts: %v", err)
		return
	}

	ctx.action.Infof("✅ Artifacts uploaded to: %s", artifactResult.Path)
	ctx.setOutput("artifact_name", artifactResult.Name)
	ctx.setOutput("artifact_path", artifactResult.Path)
	ctx.setOutput("artifact_files", strings.Join(artifactResult.Files, ","))

	if ctx.verboseOutput {
		ctx.action.Infof("Artifact details:")
		ctx.action.Infof("  Name: %s", artifactResult.Name)
		ctx.action.Infof("  Path: %s", artifactResult.Path)
		ctx.action.Infof("  Files: %s", strings.Join(artifactResult.Files, ", "))
	}
}

func printCompletionSummary(ctx *appContext, metadata *Metadata) {
	if ctx.isCI {
		ctx.action.Infof("✅ Build metadata extraction completed successfully")
		return
	}

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("✅ Build Metadata Extraction Complete")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Project Type:    %s\n", metadata.Common.ProjectType)
	if metadata.Common.ProjectName != "" {
		fmt.Printf("Project Name:    %s\n", metadata.Common.ProjectName)
	}
	if metadata.Common.ProjectVersion != "" {
		fmt.Printf("Project Version: %s\n", metadata.Common.ProjectVersion)
		if metadata.Common.VersionSource != "" {
			fmt.Printf("Version Source:  %s\n", metadata.Common.VersionSource)
		}
	}
	fmt.Printf("Project Path:    %s\n", metadata.Common.ProjectPath)
	fmt.Println(strings.Repeat("=", 60))

	// Offer to show full JSON
	if !ctx.verboseOutput {
		fmt.Println("\nTip: Use INPUT_VERBOSE=true for detailed output")
		fmt.Println("     or pipe output with: ... 2>/dev/null | jq")
	}
}
