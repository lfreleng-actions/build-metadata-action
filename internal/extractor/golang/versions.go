// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package golang

import (
	"fmt"
	"os"
	"sort"

	"github.com/lfreleng-actions/build-metadata-action/internal/goversions"
)

// activeSupportedVersions is the set of Go versions (major.minor,
// ascending) that generateGoVersionMatrix is allowed to emit. It
// defaults to the static goversions fallback list so `go test` and
// offline invocations never touch the network; the CLI entry point
// (`cmd/build-metadata/main.go`) swaps in the live EOL-aware set via
// SetSupportedVersions before invoking Extract on Go projects. This
// package-scoped wiring mirrors the Python extractor's policy pattern
// (see `internal/extractor/python/policy.go`): the
// `extractor.Extractor` interface has a fixed `Extract(path string)`
// signature that we cannot widen.
var activeSupportedVersions = goversions.GetFallbackVersions()

// SetSupportedVersions replaces the supported Go version set used for
// matrix generation. Passing nil or an empty slice resets to the static
// goversions fallback list.
func SetSupportedVersions(versions []string) {
	if len(versions) == 0 {
		activeSupportedVersions = goversions.GetFallbackVersions()
		return
	}
	activeSupportedVersions = versions
}

// ResolveSupportedVersions consults the live endoflife.date Go feed and
// returns the currently supported (non-EOL) Go versions at or above the
// goversions baseline, sorted ascending. When the live API is
// unavailable (offline runners, outages) it falls back to the static
// goversions fallback list after emitting a warning, so the action
// always exits cleanly. Uses the goversions package defaults for
// timeout and retry budget; no runtime configuration is currently
// plumbed through action inputs.
func ResolveSupportedVersions() []string {
	client := goversions.NewEOLClient(goversions.DefaultTimeout, goversions.DefaultMaxRetries)
	supported, err := client.GetSupportedVersions()
	if err != nil {
		fmt.Fprintf(os.Stderr,
			"[WARNING] Failed to fetch live Go EOL data (%v); using static supported set\n", err)
		return goversions.GetFallbackVersions()
	}
	if len(supported) == 0 {
		fmt.Fprintf(os.Stderr,
			"[WARNING] Live Go EOL data yielded no supported versions at or above %s; using static supported set\n",
			goversions.Baseline())
		return goversions.GetFallbackVersions()
	}
	// endoflife.date returns cycles in newest-first order; downstream
	// consumers expect ascending order so the last element is the
	// newest release.
	sort.Slice(supported, func(i, j int) bool {
		return compareGoVersionStrings(supported[i], supported[j]) < 0
	})
	return supported
}

// generateGoVersionMatrix generates the list of Go versions a workflow
// should test against, given the minimum Go version declared in go.mod.
// The matrix is the subset of the supported (non-EOL) Go version set at
// or above that minimum. When the go.mod minimum is older than every
// supported release the full supported set is returned (testing on EOL
// toolchains adds no value); when it is newer than every supported
// release (e.g. a pre-release toolchain) the matrix contains just the
// declared minimum so the workflow still tests something meaningful.
func generateGoVersionMatrix(goVersion string) []string {
	minimum := normalizeGoVersion(goVersion)

	var matrix []string
	for _, v := range activeSupportedVersions {
		if compareGoVersionStrings(v, minimum) >= 0 {
			matrix = append(matrix, v)
		}
	}

	if len(matrix) == 0 {
		return []string{minimum}
	}
	return matrix
}

// normalizeGoVersion reduces a go.mod version directive to major.minor
// form (e.g. "1.24.3" -> "1.24"). go.mod `go` directives may carry a
// patch component since Go 1.21; the version matrix operates on release
// cycles, which are major.minor.
func normalizeGoVersion(version string) string {
	var major, minor int
	if n, err := fmt.Sscanf(version, "%d.%d", &major, &minor); err != nil || n != 2 {
		return version
	}
	return fmt.Sprintf("%d.%d", major, minor)
}

// compareGoVersionStrings compares two major.minor Go version strings.
// Returns -1, 0, or 1. A local copy of the comparator used by the
// Python extractor's policy.go, duplicated for the same reason: to keep
// this file free of cross-extractor dependencies.
func compareGoVersionStrings(v1, v2 string) int {
	var maj1, min1, maj2, min2 int
	_, _ = fmt.Sscanf(v1, "%d.%d", &maj1, &min1)
	_, _ = fmt.Sscanf(v2, "%d.%d", &maj2, &min2)
	switch {
	case maj1 != maj2:
		if maj1 < maj2 {
			return -1
		}
		return 1
	case min1 != min2:
		if min1 < min2 {
			return -1
		}
		return 1
	}
	return 0
}
