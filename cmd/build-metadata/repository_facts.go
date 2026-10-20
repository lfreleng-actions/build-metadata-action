// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package main

import (
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/lfreleng-actions/build-metadata-action/internal/detector"
)

// maxDockerfileDepth bounds the Dockerfile search below the project
// path, so a large tree costs a bounded walk rather than a full scan.
const maxDockerfileDepth = 8

// defaultDockerfileExcludes names directories that hold vendored or
// generated content rather than images the repository builds.
var defaultDockerfileExcludes = []string{
	".git", "node_modules", "vendor", "target", ".tox", ".venv",
}

// applyRepositoryFacts records facts about the tree that hold whatever
// single project type won detection: every project type that matches,
// and whether the tree carries Dockerfiles. A Maven project with a
// Dockerfile resolves to java-maven, so without these a Java lane cannot
// tell that an image build is also needed.
//
// Image discovery and per-image tags belong to docker-build-matrix-action;
// this reports presence and a count only.
func applyRepositoryFacts(metadata *Metadata, absPath string, dockerfileExcludes []string) {
	if types, err := detector.DetectAllProjectTypes(absPath); err == nil {
		metadata.Common.ProjectTypes = types
	}

	excludes := append(append([]string{}, defaultDockerfileExcludes...), dockerfileExcludes...)
	count := countDockerfiles(absPath, excludes)
	metadata.Common.DockerfileCount = count
	metadata.Common.HasDockerfiles = count > 0
}

// countDockerfiles walks absPath to maxDockerfileDepth and counts the
// regular files isDockerfileName accepts. Symlinks are neither followed
// nor counted, so a crafted link in an untrusted checkout cannot point
// the walk outside the workspace. An exclude entry is a glob matched
// against both the base name and the slash-separated path relative to
// absPath, and applies to directories and files alike.
func countDockerfiles(absPath string, excludes []string) int {
	count := 0
	_ = filepath.WalkDir(absPath, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if entry != nil && entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if path == absPath {
			return nil
		}

		rel, relErr := filepath.Rel(absPath, path)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)

		if isExcluded(rel, entry.Name(), excludes) {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		if entry.IsDir() {
			if strings.Count(rel, "/")+1 >= maxDockerfileDepth {
				return fs.SkipDir
			}
			return nil
		}

		if entry.Type().IsRegular() && isDockerfileName(entry.Name()) {
			count++
		}
		return nil
	})
	return count
}

// isExcluded reports whether an exclude glob matches the entry's base
// name or its relative path. Malformed globs match nothing.
func isExcluded(rel, name string, excludes []string) bool {
	for _, pattern := range excludes {
		pattern = strings.TrimSuffix(filepath.ToSlash(pattern), "/")
		if pattern == "" {
			continue
		}
		if ok, _ := filepath.Match(pattern, name); ok {
			return true
		}
		if ok, _ := filepath.Match(pattern, rel); ok {
			return true
		}
	}
	return false
}

// isDockerfileName accepts the usual Dockerfile naming conventions:
// Dockerfile, Dockerfile.<variant> and <variant>.Dockerfile.
func isDockerfileName(name string) bool {
	return name == "Dockerfile" ||
		strings.HasPrefix(name, "Dockerfile.") ||
		strings.HasSuffix(name, ".Dockerfile")
}
