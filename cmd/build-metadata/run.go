// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package main

import (
	"fmt"
	"io"
	"os"

	"github.com/lfreleng-actions/build-metadata-action/internal/detector"
	"github.com/lfreleng-actions/build-metadata-action/internal/environment"
	"github.com/lfreleng-actions/build-metadata-action/internal/extractor"
	_ "github.com/lfreleng-actions/build-metadata-action/internal/extractor/cpp"
	_ "github.com/lfreleng-actions/build-metadata-action/internal/extractor/dart"
	_ "github.com/lfreleng-actions/build-metadata-action/internal/extractor/docker"
	_ "github.com/lfreleng-actions/build-metadata-action/internal/extractor/dotnet"
	_ "github.com/lfreleng-actions/build-metadata-action/internal/extractor/elixir"
	golang "github.com/lfreleng-actions/build-metadata-action/internal/extractor/golang"
	_ "github.com/lfreleng-actions/build-metadata-action/internal/extractor/haskell"
	_ "github.com/lfreleng-actions/build-metadata-action/internal/extractor/helm"
	java "github.com/lfreleng-actions/build-metadata-action/internal/extractor/java"
	javascript "github.com/lfreleng-actions/build-metadata-action/internal/extractor/javascript"
	_ "github.com/lfreleng-actions/build-metadata-action/internal/extractor/julia"
	_ "github.com/lfreleng-actions/build-metadata-action/internal/extractor/php"
	python "github.com/lfreleng-actions/build-metadata-action/internal/extractor/python"
	_ "github.com/lfreleng-actions/build-metadata-action/internal/extractor/ruby"
	rust "github.com/lfreleng-actions/build-metadata-action/internal/extractor/rust"
	_ "github.com/lfreleng-actions/build-metadata-action/internal/extractor/scala"
	_ "github.com/lfreleng-actions/build-metadata-action/internal/extractor/swift"
	_ "github.com/lfreleng-actions/build-metadata-action/internal/extractor/terraform"
	"github.com/lfreleng-actions/build-metadata-action/internal/supported"
	"github.com/lfreleng-actions/build-metadata-action/internal/version"
)

// detectProjectType resolves the project type, preferring an explicit
// caller-supplied value over detection.
//
// Detection returns the first rule that matches in priority order, so a
// polyglot repository resolves to whichever marker file happens to rank
// highest rather than to the thing being built. A Maven project carrying
// a package.json for frontend-maven-plugin resolves as javascript-npm,
// and every java_* output is then absent. A reusable workflow dedicated
// to one build tool already knows better, so it can say so.
//
// An override naming no extractor this action knows is reported and
// discarded rather than honoured: detection still yields a real answer,
// whereas an unrecognised type yields none at all.
func detectProjectType(ctx *appContext, metadata *Metadata, cfg runConfig) string {
	projectType := ""
	usedOverride := false

	if override := cfg.projectTypeOverride; override != "" {
		if _, err := extractor.GetExtractor(override); err != nil {
			msg := fmt.Sprintf(
				"Supplied project_type %q matches no known extractor; detecting instead",
				override)
			if ctx.isCI {
				ctx.action.Warningf("%s", msg)
			} else {
				fmt.Printf("Warning: %s\n", msg)
			}
		} else {
			projectType = override
			usedOverride = true
		}
	}

	if projectType == "" {
		if ctx.isCI {
			ctx.action.Infof("Detecting project type in: %s", cfg.absPath)
		} else {
			fmt.Printf("Detecting project type in: %s\n", cfg.absPath)
		}

		detected, err := detector.DetectProjectType(cfg.absPath)
		if err != nil {
			if ctx.isCI {
				ctx.action.Warningf("Failed to detect project type: %v", err)
			} else {
				fmt.Printf("Warning: Failed to detect project type: %v\n", err)
			}
			detected = "unknown"
		}
		projectType = detected
	}

	metadata.Common.ProjectType = projectType
	metadata.Common.BuildTool = buildToolForProjectType(projectType)

	// Provenance comes from whether the override passed validation, not
	// from comparing the values. A rejected override can still equal what
	// detection returns -- supplying "unknown" where detection also fails
	// is the plain case -- and reporting that as caller-supplied would
	// contradict the warning issued moments earlier.
	source := "Detected"
	if usedOverride {
		source = "Caller supplied"
	}
	if ctx.isCI {
		ctx.action.Infof("%s project type: %s", source, projectType)
	} else {
		fmt.Printf("%s project type: %s\n", source, projectType)
	}
	return projectType
}

// configureExtractorPolicies wires up the Python, Go, Rust, Node.js and
// Java extractor resolvers from action inputs. This is deferred until after
// project type detection so that other projects never pay the
// upstream round-trips (nor surface unrelated fetch warnings) just to
// satisfy defaults they will never use.
func configureExtractorPolicies(projectType string, cfg runConfig) {
	switch normalizeProjectTypeToLanguage(projectType) {
	case "python":
		configurePythonResolvers(cfg)
	case "go":
		configureGoResolver(cfg)
	case "rust":
		configureRustResolver(cfg)
	case "javascript":
		configureNodeResolver(cfg)
	case "java":
		configureJavaResolver(cfg)
	}
}

// legacyGoEOLAPIURLEnv and legacyPythonEOLAPIURLEnv named mirrors of
// the endoflife.date v0 endpoints for the resolvers the
// supported-versions module replaced. The module reads the v1 API from
// a fixed root, so the variables no longer apply; the resolver set-up
// warns when one is still set.
const (
	legacyGoEOLAPIURLEnv     = "BUILD_METADATA_GO_EOL_API_URL"
	legacyPythonEOLAPIURLEnv = "BUILD_METADATA_PYTHON_EOL_API_URL"
)

// configureGoResolver builds the supported-versions resolver once, with
// the Go extractor's admission, and installs it into the Go extractor.
// The python_offline_mode, python_eol_timeout and
// python_eol_max_retries inputs govern it too. The module falls back
// to its embedded snapshot when the upstream is unreachable, so
// offline runners degrade gracefully. The resolver lives until the
// process exits, which releases its cache.
func configureGoResolver(cfg runConfig) {
	warnLegacyEOLAPIURL(os.Getenv, os.Stderr, legacyGoEOLAPIURLEnv, "Go")
	r, err := golang.NewResolver(resolverSettings(cfg))
	if err != nil {
		fmt.Fprintf(os.Stderr,
			"[WARNING] Failed to build the supported-versions resolver (%v); resolving Go versions offline\n", err)
		golang.SetResolver(nil)
		return
	}
	golang.SetResolver(r)
}

// configureRustResolver builds the supported-versions resolver once,
// with the module's Rust policy, and installs it into the Rust
// extractor. The python_offline_mode, python_eol_timeout and
// python_eol_max_retries inputs govern it too. The module falls back
// to its embedded snapshot when an upstream is unreachable, so offline
// runners degrade gracefully. The resolver lives until the process
// exits, which releases its cache.
func configureRustResolver(cfg runConfig) {
	r, err := rust.NewResolver(resolverSettings(cfg))
	if err != nil {
		fmt.Fprintf(os.Stderr,
			"[WARNING] Failed to build the supported-versions resolver (%v); resolving Rust versions offline\n", err)
		rust.SetResolver(nil)
		return
	}
	rust.SetResolver(r)
}

// configureNodeResolver builds the supported-versions resolver once,
// with the module's Node.js policy, and installs it into the
// JavaScript extractor. The python_offline_mode, python_eol_timeout
// and python_eol_max_retries inputs govern it too. The module falls
// back to its embedded snapshot when an upstream is unreachable, so
// offline runners degrade gracefully. The resolver lives until the
// process exits, which releases its cache.
func configureNodeResolver(cfg runConfig) {
	r, err := javascript.NewResolver(resolverSettings(cfg))
	if err != nil {
		fmt.Fprintf(os.Stderr,
			"[WARNING] Failed to build the supported-versions resolver (%v); resolving Node.js versions offline\n", err)
		javascript.SetResolver(nil)
		return
	}
	javascript.SetResolver(r)
}

// configureJavaResolver builds the supported-versions resolver once,
// with the module's Java policy, and installs it into the Maven and
// Gradle extractors. The python_offline_mode, python_eol_timeout and
// python_eol_max_retries inputs govern it too. The module falls back
// to its embedded snapshot when an upstream is unreachable, so offline
// runners degrade gracefully. The resolver lives until the process
// exits, which releases its cache.
func configureJavaResolver(cfg runConfig) {
	r, err := java.NewResolver(resolverSettings(cfg))
	if err != nil {
		fmt.Fprintf(os.Stderr,
			"[WARNING] Failed to build the supported-versions resolver (%v); resolving Java versions offline\n", err)
		java.SetResolver(nil)
		return
	}
	java.SetResolver(r)
}

// configurePythonResolvers builds the Python extractor's
// supported-versions resolvers once and installs them: the primary one
// with the module's installable admission, and the lifecycle one it
// retries when the actions/python-versions manifest breaks. The
// resolvers live until the process exits, which releases their cache.
func configurePythonResolvers(cfg runConfig) {
	warnLegacyEOLAPIURL(os.Getenv, os.Stderr, legacyPythonEOLAPIURLEnv, "Python")
	python.SetResolvers(newPythonResolvers(resolverSettings(cfg)))
}

// newPythonResolvers builds the Python resolvers from s. A resolver
// that cannot be built is returned as nil, with a warning; the
// extractor then falls through to the next one, and to the embedded
// snapshot last.
func newPythonResolvers(s supported.Settings) (primary, lifecycle python.MatrixResolver) {
	if r, err := python.NewResolver(s); err != nil {
		fmt.Fprintf(os.Stderr,
			"[WARNING] Failed to build the supported-versions resolver (%v); resolving Python versions offline\n", err)
	} else {
		primary = r
	}
	if r, err := python.NewLifecycleResolver(s); err != nil {
		fmt.Fprintf(os.Stderr,
			"[WARNING] Failed to build the Python lifecycle resolver (%v); a broken manifest falls back to the snapshot\n", err)
	} else {
		lifecycle = r
	}
	return primary, lifecycle
}

// resolverSettings maps the action inputs onto the supported-versions
// resolver settings the Python, Go, Rust, Node.js and Java extractors
// share. Offline
// mode also skips the runner cache: the module serves cached release
// facts of any age when offline, so a cache filled by an earlier
// online step would otherwise change the matrix, and offline mode
// promises the embedded snapshot alone.
func resolverSettings(cfg runConfig) supported.Settings {
	return supported.Settings{
		Offline: cfg.pythonOffline,
		NoCache: cfg.pythonOffline,
		Timeout: cfg.pythonTimeout,
		Retries: cfg.pythonRetries,
	}
}

// warnLegacyEOLAPIURL writes a warning to w when getenv reports env
// set, so a mirror configured through it does not go unnoticed now
// that the module reads the public endpoint for language.
func warnLegacyEOLAPIURL(getenv func(string) string, w io.Writer, env, language string) {
	if getenv(env) == "" {
		return
	}
	_, _ = fmt.Fprintf(w,
		"[WARNING] %s is no longer honoured: %s versions resolve through the supported-versions module, "+
			"which reads the endoflife.date v1 API at https://endoflife.date/api/v1/\n",
		env, language)
}

func extractVersionInfo(ctx *appContext, cfg runConfig, metadata *Metadata, projectType string) {
	if !cfg.useVersionExtract {
		return
	}

	if ctx.isCI {
		ctx.action.Infof("Extracting version information...")
	} else {
		fmt.Println("Extracting version information...")
	}

	versionInfo, err := version.ExtractVersion(cfg.absPath, projectType)
	if err != nil {
		if ctx.isCI {
			ctx.action.Warningf("Failed to extract version: %v", err)
		} else {
			fmt.Printf("Warning: Failed to extract version: %v\n", err)
		}
		return
	}

	metadata.Common.ProjectVersion = versionInfo.Version
	metadata.Common.VersionSource = versionInfo.Source
	if versionInfo.IsDynamic {
		metadata.Common.VersioningType = "dynamic"
	} else {
		metadata.Common.VersioningType = "static"
	}
}

func extractProjectMetadata(ctx *appContext, metadata *Metadata, projectType, absPath string) {
	extractorImpl, err := extractor.GetExtractor(projectType)
	if err != nil {
		if ctx.isCI {
			ctx.action.Warningf("No specific extractor for project type %s: %v", projectType, err)
		} else {
			fmt.Printf("Warning: No specific extractor for project type %s: %v\n", projectType, err)
		}
		return
	}

	if ctx.isCI {
		ctx.action.Infof("Extracting %s project metadata...", projectType)
	} else {
		fmt.Printf("Extracting %s project metadata...\n", projectType)
	}

	projectMetadata, err := extractorImpl.Extract(absPath)
	if err != nil {
		if ctx.isCI {
			ctx.action.Warningf("Failed to extract project metadata: %v", err)
		} else {
			fmt.Printf("Warning: Failed to extract project metadata: %v\n", err)
		}
		return
	}

	if projectMetadata.Name != "" {
		metadata.Common.ProjectName = projectMetadata.Name
	}
	if projectMetadata.Version != "" && metadata.Common.ProjectVersion == "" {
		metadata.Common.ProjectVersion = projectMetadata.Version
		metadata.Common.VersionSource = projectMetadata.VersionSource
	}

	metadata.LanguageSpecific = projectMetadata.LanguageSpecific

	// Extract versioning_type from language-specific metadata, but never
	// clobber a value already derived from the actual version source
	// chosen above (e.g. git fallback marking the version as dynamic
	// when the manifest holds a placeholder).
	if metadata.Common.VersioningType == "" {
		if versioningType, ok := projectMetadata.LanguageSpecific["versioning_type"].(string); ok {
			metadata.Common.VersioningType = versioningType
		} else {
			metadata.Common.VersioningType = "static"
		}
	}
}

// applyVersionProperties surfaces version.properties (the Linux
// Foundation / ONAP release convention) explicitly even when a language
// manifest won the version_source selection, then synthesizes the
// snapshot version. It runs after all version sources settle so the
// comparison uses the final resolved project version.
func applyVersionProperties(metadata *Metadata, absPath string) {
	if propsInfo, ok := version.ExtractVersionProperties(absPath); ok {
		metadata.Common.VersionPropertiesVersion = propsInfo.Version
		metadata.Common.VersionPropertiesMatch = versionPropertiesMatch(
			propsInfo.Version, metadata.Common.ProjectVersion)
	}

	metadata.Common.SnapshotVersion = synthesizeSnapshotVersion(
		metadata.Common.VersionPropertiesVersion,
		metadata.Common.ProjectVersion)
}

func collectEnvironmentMetadata(ctx *appContext, cfg runConfig, metadata *Metadata) {
	if !cfg.includeEnvironment {
		return
	}

	if ctx.isCI {
		ctx.action.Infof("Collecting environment metadata...")
	} else {
		fmt.Println("Collecting environment metadata...")
	}

	envMetadata, err := environment.Collect()
	if err != nil {
		if ctx.isCI {
			ctx.action.Warningf("Failed to collect environment metadata: %v", err)
		} else {
			fmt.Printf("Warning: Failed to collect environment metadata: %v\n", err)
		}
		return
	}

	metadata.Environment = *envMetadata
}
