// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package rust

import (
	"sort"
	"strings"
)

// parseDependencies parses the dependencies from Cargo.toml
func parseDependencies(deps map[string]interface{}) []Dependency {
	result := []Dependency{}

	for name, spec := range deps {
		dep := Dependency{
			Name: name,
		}

		switch v := spec.(type) {
		case string:
			// Simple version string: package = "1.0.0"
			dep.Version = v
			dep.Source = "crates.io"

		case map[string]interface{}:
			// Detailed dependency specification
			if version, ok := v["version"].(string); ok {
				dep.Version = version
			}
			if optional, ok := v["optional"].(bool); ok {
				dep.Optional = optional
			}
			if features, ok := v["features"].([]interface{}); ok {
				dep.Features = make([]string, 0, len(features))
				for _, f := range features {
					if fs, ok := f.(string); ok {
						dep.Features = append(dep.Features, fs)
					}
				}
			}

			if _, ok := v["git"]; ok {
				dep.Source = "git"
			} else if _, ok := v["path"]; ok {
				dep.Source = "path"
			} else if registry, ok := v["registry"].(string); ok {
				dep.Source = registry
			} else {
				dep.Source = "crates.io"
			}
		}

		result = append(result, dep)
	}

	// Sort by name so the emitted dependency outputs are deterministic
	// regardless of Go's randomized map iteration order.
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })

	return result
}

// formatDependencies formats dependencies for output
func formatDependencies(deps []Dependency) []string {
	formatted := make([]string, 0, len(deps))
	for _, dep := range deps {
		depStr := dep.Name
		if dep.Version != "" {
			depStr += "@" + dep.Version
		}
		if dep.Optional {
			depStr += " (optional)"
		}
		if len(dep.Features) > 0 {
			depStr += " [" + strings.Join(dep.Features, ", ") + "]"
		}
		formatted = append(formatted, depStr)
	}
	return formatted
}

// detectRustFrameworks detects common Rust frameworks from dependencies
func detectRustFrameworks(deps map[string]interface{}) []string {
	frameworks := []string{}
	frameworkMap := map[string]string{
		"tokio":     "Tokio (Async Runtime)",
		"async-std": "async-std (Async Runtime)",
		"actix-web": "Actix Web (Web Framework)",
		"rocket":    "Rocket (Web Framework)",
		"axum":      "Axum (Web Framework)",
		"warp":      "Warp (Web Framework)",
		"tide":      "Tide (Web Framework)",
		"serde":     "Serde (Serialization)",
		"clap":      "Clap (CLI Parser)",
		"diesel":    "Diesel (ORM)",
		"sqlx":      "SQLx (SQL Toolkit)",
		"reqwest":   "Reqwest (HTTP Client)",
		"hyper":     "Hyper (HTTP Library)",
		"tonic":     "Tonic (gRPC)",
		"tracing":   "Tracing (Logging/Diagnostics)",
		"log":       "Log (Logging Facade)",
		"anyhow":    "Anyhow (Error Handling)",
		"thiserror": "thiserror (Error Derive)",
		"rayon":     "Rayon (Parallelism)",
		"crossbeam": "Crossbeam (Concurrency)",
		"gtk":       "GTK (GUI)",
		"bevy":      "Bevy (Game Engine)",
		"yew":       "Yew (Web Framework)",
		"tauri":     "Tauri (Desktop Apps)",
	}

	seen := make(map[string]bool)
	for depName := range deps {
		if name, ok := frameworkMap[depName]; ok && !seen[name] {
			frameworks = append(frameworks, name)
			seen[name] = true
		}
	}

	// Sort for deterministic output across runs (deps is a map).
	sort.Strings(frameworks)

	return frameworks
}
