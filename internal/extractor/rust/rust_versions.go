// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package rust

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/lfreleng-actions/build-metadata-action/internal/supported"
)

// rustTool is the catalog id of Rust in the supported-versions module.
const rustTool = "rust"

// MatrixResolver answers matrix questions for a tool. A
// *supported.Resolver satisfies it; tests inject a recorded report.
type MatrixResolver = supported.MatrixResolver

// activeResolver resolves the Rust release lines. The CLI entry point
// (`cmd/build-metadata/run.go`) installs one built from the action
// inputs via SetResolver before invoking Extract on Rust projects, as
// it does for the Python and Go extractors: the `extractor.Extractor`
// interface has a fixed `Extract(path string)` signature. When none is
// installed, the snapshot resolver applies, so `go test` and library
// callers never touch the network.
var activeResolver MatrixResolver

// SetResolver installs the resolver used for matrix generation; nil
// restores the snapshot default.
func SetResolver(r MatrixResolver) {
	activeResolver = r
}

// NewResolver builds a resolver from s with the module's Rust policy:
// the six newest release lines and the stable label, read from
// endoflife.date and the stable channel manifest.
func NewResolver(s supported.Settings) (*supported.Resolver, error) {
	return supported.New(s)
}

// newSnapshotResolver builds a resolver that reads the module's
// embedded snapshot alone: offline, and without the runner cache, so a
// malformed response cached by an earlier fetch cannot defeat the
// fallback. Tests replace it to fix the clock or the report.
var newSnapshotResolver = func() (MatrixResolver, error) {
	return NewResolver(supported.Settings{Offline: true, NoCache: true})
}

// warnings receives the warnings matrix resolution emits. Tests
// replace it.
var warnings io.Writer = os.Stderr

func warnf(format string, args ...any) {
	_, _ = fmt.Fprintf(warnings, "[WARNING] "+format+"\n", args...)
}

// isSpecialVersion reports a channel name rather than a release line.
func isSpecialVersion(v string) bool {
	return v == "stable" || v == "beta" || v == "nightly"
}

// usable reports whether versions holds a numeric release line.
func usable(versions []string) bool {
	for _, v := range versions {
		if _, _, ok := parseMajorMinor(v); ok && !isSpecialVersion(v) {
			return true
		}
	}
	return false
}

// resolveRustVersions returns the module's unconstrained Rust matrix:
// the newest release lines, ascending, then the stable label. No
// constraint is passed, so the module never widens one; the caller
// applies the MSRV. When the installed resolver fails or yields no
// release line, it retries against the embedded snapshot.
func resolveRustVersions() ([]string, error) {
	if activeResolver != nil {
		rep, err := activeResolver.Matrix(context.Background(), rustTool, "")
		switch {
		case err != nil:
			warnf("Failed to resolve Rust versions (%v); retrying from the embedded snapshot", err)
		case !usable(rep.Matrix.Versions):
			warnf("Resolved no Rust release lines; retrying from the embedded snapshot")
		default:
			return rep.Matrix.Versions, nil
		}
	}
	snapshot, err := newSnapshotResolver()
	if err != nil {
		return nil, err
	}
	if c, ok := snapshot.(io.Closer); ok {
		defer func() { _ = c.Close() }()
	}
	rep, err := snapshot.Matrix(context.Background(), rustTool, "")
	if err != nil {
		return nil, err
	}
	if !usable(rep.Matrix.Versions) {
		return nil, fmt.Errorf("the embedded snapshot holds no Rust release line")
	}
	return rep.Matrix.Versions, nil
}

// parseMajorMinor parses the leading "major.minor" of a version string.
// It returns ok=false when either component is absent or non-numeric so
// callers can choose a safe fallback instead of treating the value as 0.0.
func parseMajorMinor(version string) (int, int, bool) {
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return 0, 0, false
	}
	major, errMajor := strconv.Atoi(parts[0])
	minor, errMinor := strconv.Atoi(parts[1])
	if errMajor != nil || errMinor != nil {
		return 0, 0, false
	}
	return major, minor, true
}

// generateRustVersionMatrix generates a list of Rust versions from MSRV.
//
// The matrix holds the MSRV, the module's recent release lines at or
// above it, and "stable". The lines come from the supported-versions
// module: live endoflife.date and stable channel data, the runner
// cache, or the snapshot embedded in the module when neither is
// available. When no release line resolves at all, the matrix is the
// MSRV and "stable" alone; no release is ever estimated. A malformed
// MSRV leaves the lines unfiltered, newest first, then "stable".
func generateRustVersionMatrix(msrv string) []string {
	versions, err := resolveRustVersions()
	if err != nil {
		warnf("Failed to resolve supported Rust versions (%v); using the MSRV %s and stable", err, msrv)
		versions = []string{"stable"}
	}
	if _, _, ok := parseMajorMinor(msrv); !ok {
		return newestFirst(versions)
	}
	return filterVersionsFromMSRV(msrv, versions)
}

// newestFirst returns the release lines in versions newest first,
// followed by the special versions in their original order. The
// module reports lines ascending; the action has always emitted the
// unfiltered matrix of a malformed MSRV descending.
func newestFirst(versions []string) []string {
	numeric := []string{}
	special := []string{}
	for _, v := range versions {
		if isSpecialVersion(v) {
			special = append(special, v)
		} else {
			numeric = append(numeric, v)
		}
	}
	sort.SliceStable(numeric, func(i, j int) bool {
		return lineLess(numeric[j], numeric[i])
	})
	return append(numeric, special...)
}

// filterVersionsFromMSRV filters versions to only include those >= MSRV.
//
// This ensures we don't test against versions older than the project's
// Minimum Supported Rust Version (MSRV), as those tests would fail.
//
// On success the MSRV is included as the first version in the result,
// followed by newer versions in ascending order, with special versions
// (stable, beta, nightly) at the end. If the MSRV cannot be parsed, the
// input slice is returned unchanged (no filtering or reordering).
func filterVersionsFromMSRV(msrv string, allVersions []string) []string {
	filtered := []string{}

	msrvMajor, msrvMinor, ok := parseMajorMinor(msrv)
	if !ok {
		// Malformed MSRV: return the unfiltered set rather than treat MSRV
		// as 0.0 and admit versions that should be filtered out.
		return allVersions
	}

	for _, version := range allVersions {
		if isSpecialVersion(version) {
			filtered = append(filtered, version)
			continue
		}

		major, minor, ok := parseMajorMinor(version)
		if !ok {
			continue
		}

		if major > msrvMajor || (major == msrvMajor && minor >= msrvMinor) {
			filtered = append(filtered, version)
		}
	}

	numericVersions := []string{}
	specialVersions := []string{}
	for _, v := range filtered {
		if isSpecialVersion(v) {
			specialVersions = append(specialVersions, v)
		} else {
			numericVersions = append(numericVersions, v)
		}
	}

	sort.Slice(numericVersions, func(i, j int) bool {
		return lineLess(numericVersions[i], numericVersions[j])
	})

	result := []string{msrv}
	for _, v := range numericVersions {
		// Skip the MSRV's own minor release: "1.97" duplicates an MSRV
		// written as "1.97.0", adding a redundant matrix job.
		major, minor, ok := parseMajorMinor(v)
		if !ok || major != msrvMajor || minor != msrvMinor {
			result = append(result, v)
		}
	}
	result = append(result, specialVersions...)

	return result
}

// lineLess orders release lines numerically by major, then minor,
// and falls back to text order for a line that does not parse.
func lineLess(a, b string) bool {
	aMajor, aMinor, aOK := parseMajorMinor(a)
	bMajor, bMinor, bOK := parseMajorMinor(b)
	switch {
	case !aOK || !bOK:
		return a < b
	case aMajor != bMajor:
		return aMajor < bMajor
	default:
		return aMinor < bMinor
	}
}

// generateRustVersionMatrixFromEdition pairs an edition with the first
// stable release that supports it.
func generateRustVersionMatrixFromEdition(edition string) []string {
	switch edition {
	case "2024":
		return []string{"1.85", "stable"}
	case "2021":
		return []string{"1.56", "stable"}
	case "2018":
		return []string{"1.31", "stable"}
	case "2015":
		return []string{"1.0", "stable"}
	default:
		return []string{"stable"}
	}
}
