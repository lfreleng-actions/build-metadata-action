// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lfreleng-actions/build-metadata-action/internal/detector"
	"github.com/lfreleng-actions/build-metadata-action/internal/environment"
	"github.com/lfreleng-actions/build-metadata-action/internal/extractor"
	_ "github.com/lfreleng-actions/build-metadata-action/internal/extractor/cpp"
	_ "github.com/lfreleng-actions/build-metadata-action/internal/extractor/dart"
	_ "github.com/lfreleng-actions/build-metadata-action/internal/extractor/docker"
	_ "github.com/lfreleng-actions/build-metadata-action/internal/extractor/dotnet"
	_ "github.com/lfreleng-actions/build-metadata-action/internal/extractor/elixir"
	_ "github.com/lfreleng-actions/build-metadata-action/internal/extractor/golang"
	_ "github.com/lfreleng-actions/build-metadata-action/internal/extractor/haskell"
	_ "github.com/lfreleng-actions/build-metadata-action/internal/extractor/helm"
	_ "github.com/lfreleng-actions/build-metadata-action/internal/extractor/java"
	_ "github.com/lfreleng-actions/build-metadata-action/internal/extractor/javascript"
	_ "github.com/lfreleng-actions/build-metadata-action/internal/extractor/julia"
	_ "github.com/lfreleng-actions/build-metadata-action/internal/extractor/php"
	_ "github.com/lfreleng-actions/build-metadata-action/internal/extractor/python"
	_ "github.com/lfreleng-actions/build-metadata-action/internal/extractor/ruby"
	_ "github.com/lfreleng-actions/build-metadata-action/internal/extractor/rust"
	_ "github.com/lfreleng-actions/build-metadata-action/internal/extractor/scala"
	_ "github.com/lfreleng-actions/build-metadata-action/internal/extractor/swift"
	_ "github.com/lfreleng-actions/build-metadata-action/internal/extractor/terraform"
	"github.com/lfreleng-actions/build-metadata-action/internal/output"
	"github.com/lfreleng-actions/build-metadata-action/internal/version"
	"github.com/sethvargo/go-githubactions"
)

const (
	// Action metadata
	actionName        = "build-metadata-action"
	actionVersion     = "1.0.0"
	actionDescription = "Universal action to capture and display metadata related to project builds"
)

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

// Metadata represents the complete metadata collected
type Metadata struct {
	// Common metadata
	Common CommonMetadata `json:"common"`

	// Environment metadata
	Environment environment.Metadata `json:"environment"`

	// Language-specific metadata
	LanguageSpecific map[string]interface{} `json:"language_specific,omitempty"`

	// Build metadata
	Build BuildMetadata `json:"build"`
}

// CommonMetadata contains metadata common to all project types
type CommonMetadata struct {
	ProjectType      string    `json:"project_type"`
	ProjectName      string    `json:"project_name"`
	ProjectVersion   string    `json:"project_version"`
	ProjectPath      string    `json:"project_path"`
	VersionSource    string    `json:"version_source"`
	VersioningType   string    `json:"versioning_type"`
	BuildTimestamp   time.Time `json:"build_timestamp"`
	GitSHA           string    `json:"git_sha,omitempty"`
	GitBranch        string    `json:"git_branch,omitempty"`
	GitTag           string    `json:"git_tag,omitempty"`
	ProjectMatchRepo bool      `json:"project_match_repo,omitempty"`
}

// BuildMetadata contains build-specific metadata
type BuildMetadata struct {
	CIPlatform string `json:"ci_platform"`
	CIRunID    string `json:"ci_run_id"`
	CIRunURL   string `json:"ci_run_url"`
	RunnerOS   string `json:"runner_os"`
	RunnerArch string `json:"runner_arch"`
}

func main() {
	action := githubactions.New()

	// Detect if running in CI environment
	isCI := os.Getenv("GITHUB_ACTIONS") == "true" || os.Getenv("CI") == "true"

	// Get inputs early so we can use verboseOutput for debugging
	verboseOutput := action.GetInput("verbose") == "true"

	// Get inputs
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

	outputFormatInput := action.GetInput("output_format")
	// Parse output formats (can be comma, space, or newline separated)
	// If explicitly set to empty string, no output will be generated
	// If not provided, action.yaml default "summary" is used
	outputFormats := parseMultiSeparatorInput(outputFormatInput)

	includeEnvironment := action.GetInput("include_environment") != "false"
	useVersionExtract := action.GetInput("use_version_extract") != "false"
	// verboseOutput already defined earlier for debugging

	// Artifact upload inputs
	artifactUpload := action.GetInput("artifact_upload") != "false"
	artifactNamePrefix := action.GetInput("artifact_name_prefix")
	if artifactNamePrefix == "" {
		artifactNamePrefix = "build-metadata"
	}
	artifactFormatsInput := action.GetInput("artifact_formats")
	if artifactFormatsInput == "" {
		artifactFormatsInput = "json"
	}
	// Parse artifact formats (can be comma, space, or newline separated)
	artifactFormats := parseMultiSeparatorInput(artifactFormatsInput)
	validateOutput := action.GetInput("validate_output") != "false"
	exportEnvVars := action.GetInput("export_env_vars") == "true"

	// Initialize metadata
	metadata := &Metadata{
		Common: CommonMetadata{
			ProjectPath:    absPath,
			BuildTimestamp: time.Now().UTC(),
		},
		Build: BuildMetadata{
			CIPlatform: os.Getenv("CI_PLATFORM"),
			RunnerOS:   os.Getenv("RUNNER_OS"),
			RunnerArch: os.Getenv("RUNNER_ARCH"),
		},
	}

	// Set CI platform specific values
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		metadata.Build.CIPlatform = "github"
		metadata.Build.CIRunID = os.Getenv("GITHUB_RUN_ID")
		metadata.Build.CIRunURL = fmt.Sprintf("https://github.com/%s/actions/runs/%s",
			os.Getenv("GITHUB_REPOSITORY"),
			os.Getenv("GITHUB_RUN_ID"))

		// Git information from GitHub context
		metadata.Common.GitSHA = os.Getenv("GITHUB_SHA")
		ref := os.Getenv("GITHUB_REF")
		if strings.HasPrefix(ref, "refs/heads/") {
			metadata.Common.GitBranch = strings.TrimPrefix(ref, "refs/heads/")
		} else if strings.HasPrefix(ref, "refs/tags/") {
			metadata.Common.GitTag = strings.TrimPrefix(ref, "refs/tags/")
		}
	}

	// Detect project type
	if isCI {
		action.Infof("Detecting project type in: %s", absPath)
	} else {
		fmt.Printf("Detecting project type in: %s\n", absPath)
	}
	projectType, err := detector.DetectProjectType(absPath)
	if err != nil {
		if isCI {
			action.Warningf("Failed to detect project type: %v", err)
		} else {
			fmt.Printf("Warning: Failed to detect project type: %v\n", err)
		}
		projectType = "unknown"
	}
	metadata.Common.ProjectType = projectType
	if isCI {
		action.Infof("Detected project type: %s", projectType)
	} else {
		fmt.Printf("Detected project type: %s\n", projectType)
	}

	// Extract version information
	if useVersionExtract {
		if isCI {
			action.Infof("Extracting version information...")
		} else {
			fmt.Println("Extracting version information...")
		}
		versionInfo, err := version.ExtractVersion(absPath, projectType)
		if err != nil {
			if isCI {
				action.Warningf("Failed to extract version: %v", err)
			} else {
				fmt.Printf("Warning: Failed to extract version: %v\n", err)
			}
		} else {
			metadata.Common.ProjectVersion = versionInfo.Version
			metadata.Common.VersionSource = versionInfo.Source
			if versionInfo.IsDynamic {
				metadata.Common.VersioningType = "dynamic"
			} else {
				metadata.Common.VersioningType = "static"
			}
		}
	}

	// Get appropriate extractor for the project type
	extractorImpl, err := extractor.GetExtractor(projectType)
	if err != nil {
		if isCI {
			action.Warningf("No specific extractor for project type %s: %v", projectType, err)
		} else {
			fmt.Printf("Warning: No specific extractor for project type %s: %v\n", projectType, err)
		}
	} else {
		if isCI {
			action.Infof("Extracting %s project metadata...", projectType)
		} else {
			fmt.Printf("Extracting %s project metadata...\n", projectType)
		}

		// Extract project-specific metadata
		projectMetadata, err := extractorImpl.Extract(absPath)
		if err != nil {
			if isCI {
				action.Warningf("Failed to extract project metadata: %v", err)
			} else {
				fmt.Printf("Warning: Failed to extract project metadata: %v\n", err)
			}
		} else {
			// Update common metadata
			if projectMetadata.Name != "" {
				metadata.Common.ProjectName = projectMetadata.Name
			}
			if projectMetadata.Version != "" && metadata.Common.ProjectVersion == "" {
				metadata.Common.ProjectVersion = projectMetadata.Version
				metadata.Common.VersionSource = projectMetadata.VersionSource
			}

			// Store language-specific metadata
			metadata.LanguageSpecific = projectMetadata.LanguageSpecific

			// Extract versioning_type from language-specific metadata
			if versioningType, ok := projectMetadata.LanguageSpecific["versioning_type"].(string); ok {
				metadata.Common.VersioningType = versioningType
			} else {
				// Default to "static" if not specified
				metadata.Common.VersioningType = "static"
			}
		}
	}

	// Collect environment metadata if requested
	if includeEnvironment {
		if isCI {
			action.Infof("Collecting environment metadata...")
		} else {
			fmt.Println("Collecting environment metadata...")
		}
		envMetadata, err := environment.Collect()
		if err != nil {
			if isCI {
				action.Warningf("Failed to collect environment metadata: %v", err)
			} else {
				fmt.Printf("Warning: Failed to collect environment metadata: %v\n", err)
			}
		} else {
			metadata.Environment = *envMetadata
		}
	}

	// Set outputs for common fields
	// When not in CI, print to stdout instead of trying to write to GitHub Actions files
	setOutput := func(name, value string) {
		if isCI {
			action.SetOutput(name, value)
			if exportEnvVars && value != "" {
				envName := strings.ToUpper(name)
				if verboseOutput {
					action.Infof("Exporting environment variable: %s", envName)
				}
				action.SetEnv(envName, value)
			}
		} else if verboseOutput {
			// Local execution - print to stdout if verbose
			if value != "" {
				fmt.Printf("%s=%s\n", name, value)
			}
		}
	}

	setOutput("project_type", metadata.Common.ProjectType)
	setOutput("project_name", metadata.Common.ProjectName)
	setOutput("project_version", metadata.Common.ProjectVersion)
	setOutput("project_path", metadata.Common.ProjectPath)
	setOutput("version_source", metadata.Common.VersionSource)
	setOutput("versioning_type", metadata.Common.VersioningType)
	setOutput("build_timestamp", metadata.Common.BuildTimestamp.Format(time.RFC3339))
	setOutput("git_sha", metadata.Common.GitSHA)
	setOutput("git_branch", metadata.Common.GitBranch)
	setOutput("git_tag", metadata.Common.GitTag)

	// Set outputs for build metadata
	setOutput("ci_platform", metadata.Build.CIPlatform)
	setOutput("ci_run_id", metadata.Build.CIRunID)
	setOutput("ci_run_url", metadata.Build.CIRunURL)
	setOutput("runner_os", metadata.Build.RunnerOS)
	setOutput("runner_arch", metadata.Build.RunnerArch)

	// Implement project_match_repo comparison (common to all project types)
	if metadata.Common.ProjectName != "" {
		repoFullName := os.Getenv("GITHUB_REPOSITORY")
		if repoFullName != "" {
			// Extract repo name from owner/repo format
			parts := strings.Split(repoFullName, "/")
			if len(parts) == 2 {
				repoName := parts[1]
				projectMatchRepo := metadata.Common.ProjectName == repoName
				metadata.Common.ProjectMatchRepo = projectMatchRepo
				setOutput("project_match_repo", fmt.Sprintf("%t", projectMatchRepo))
				if verboseOutput {
					if isCI {
						if projectMatchRepo {
							action.Infof("Project name matches repository name: %s", repoName)
						} else {
							action.Infof("Project name (%s) does not match repository name (%s)", metadata.Common.ProjectName, repoName)
						}
					} else {
						if projectMatchRepo {
							fmt.Printf("Project name matches repository name: %s\n", repoName)
						} else {
							fmt.Printf("Project name (%s) does not match repository name (%s)\n", metadata.Common.ProjectName, repoName)
						}
					}
				}
			}
		}
	}

	// Normalize project type to base language for output prefix
	// This ensures consistent output names across project type variants
	outputPrefix := normalizeProjectTypeToLanguage(projectType)

	// Set language-specific outputs
	for key, value := range metadata.LanguageSpecific {
		// Prefix language-specific outputs with the normalized language name
		prefix := outputPrefix
		outputKey := fmt.Sprintf("%s_%s", prefix, key)

		switch v := value.(type) {
		case string:
			setOutput(outputKey, v)
		case []string:
			setOutput(outputKey, strings.Join(v, ","))
		case map[string]interface{}:
			// Convert complex types to JSON
			jsonBytes, _ := json.Marshal(v)
			setOutput(outputKey, string(jsonBytes))
		default:
			setOutput(outputKey, fmt.Sprintf("%v", v))
		}
	}

	// Generate complete metadata JSON
	metadataJSON, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		if isCI {
			action.Warningf("Failed to marshal metadata to JSON: %v", err)
		} else {
			fmt.Printf("Warning: Failed to marshal metadata to JSON: %v\n", err)
		}
	} else {
		setOutput("metadata_json", string(metadataJSON))
	}

	// Generate output based on format(s)
	// Support multiple formats by processing each one
	for _, format := range outputFormats {
		format = strings.ToLower(strings.TrimSpace(format))

		switch format {
		case "summary":
			// Generate GitHub Step Summary
			summary := output.GenerateSummary(metadata)
			action.AddStepSummary(summary)

			// Also output to console if verbose
			if verboseOutput {
				fmt.Println(summary)
			}

		case "json":
			// Output JSON to stdout
			fmt.Println(string(metadataJSON))

		case "markdown":
			// Generate markdown output
			markdown := output.GenerateMarkdown(metadata)
			fmt.Println(markdown)
			action.SetOutput("markdown_output", markdown)

		case "yaml":
			// Generate YAML output
			action.SetOutput("metadata_yaml", string(metadataJSON)) // TODO: Implement YAML conversion
			if verboseOutput {
				action.Infof("YAML output format requested (using JSON for now)")
			}

		case "both":
			// Generate both summary and JSON (legacy support)
			summary := output.GenerateSummary(metadata)
			action.AddStepSummary(summary)
			fmt.Println(string(metadataJSON))

		case "":
			// Empty string means disable output - skip silently
			continue

		default:
			action.Warningf("Unknown output format: %s", format)
		}
	}

	// Upload artifacts if enabled
	if artifactUpload {
		action.Infof("Uploading build metadata artifacts...")

		// Formats already parsed as slice
		// Create artifact uploader
		uploader := output.NewArtifactUploader(
			true,
			artifactNamePrefix,
			artifactFormats,
			"", // Use temp dir
			validateOutput,
			true, // Strict mode
		)

		// Generate job name from context
		jobName := os.Getenv("GITHUB_JOB")
		if jobName == "" {
			jobName = "build"
		}

		// Upload artifacts
		artifactResult, err := uploader.Upload(metadata, jobName)
		if err != nil {
			action.Warningf("Failed to upload artifacts: %v", err)
		} else {
			action.Infof("✅ Artifacts uploaded to: %s", artifactResult.Path)
			setOutput("artifact_name", artifactResult.Name)
			setOutput("artifact_path", artifactResult.Path)
			setOutput("artifact_files", strings.Join(artifactResult.Files, ","))

			// Output artifact information
			if verboseOutput {
				action.Infof("Artifact details:")
				action.Infof("  Name: %s", artifactResult.Name)
				action.Infof("  Path: %s", artifactResult.Path)
				action.Infof("  Files: %s", strings.Join(artifactResult.Files, ", "))
			}
		}
	}

	// Success message and summary
	if isCI {
		action.Infof("✅ Build metadata extraction completed successfully")
	} else {
		// Print summary for local execution
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
		if !verboseOutput {
			fmt.Println("\nTip: Use INPUT_VERBOSE=true for detailed output")
			fmt.Println("     or pipe output with: ... 2>/dev/null | jq")
		}
	}

	// Set success indicator
	setOutput("success", "true")
}

// normalizeProjectTypeToLanguage converts project type variants to base language names
// for consistent output prefixing (e.g., "python-modern" -> "python")
func normalizeProjectTypeToLanguage(projectType string) string {
	// Map project types to their base language
	typeMap := map[string]string{
		"python-modern":      "python",
		"python-legacy":      "python",
		"javascript-npm":     "javascript",
		"javascript-yarn":    "javascript",
		"javascript-pnpm":    "javascript",
		"typescript-npm":     "javascript",
		"java-maven":         "java",
		"java-gradle":        "java",
		"java-gradle-kts":    "java",
		"csharp-project":     "csharp",
		"csharp-solution":    "csharp",
		"csharp-props":       "csharp",
		"dotnet-project":     "dotnet",
		"go-module":          "go",
		"rust-cargo":         "rust",
		"ruby-gemspec":       "ruby",
		"ruby-bundler":       "ruby",
		"php-composer":       "php",
		"swift-package":      "swift",
		"dart-flutter":       "dart",
		"dart-package":       "dart",
		"docker":             "docker",
		"helm-chart":         "helm",
		"terraform":          "terraform",
		"terraform-module":   "terraform",
		"terraform-opentofu": "terraform",
		"c-cmake":            "c",
		"c-autoconf":         "c",
	}

	if normalized, ok := typeMap[projectType]; ok {
		return normalized
	}

	// If no specific mapping, try to extract base by removing suffix after hyphen
	if idx := strings.Index(projectType, "-"); idx > 0 {
		return projectType[:idx]
	}

	// Return as-is if no mapping found
	return strings.ToLower(projectType)
}
