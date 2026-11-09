// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package golang

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/lfreleng-actions/build-metadata-action/internal/supported"
	"github.com/lfreleng-actions/supported-versions-action/pkg/catalog"
)

// goTool is the catalog id of Go in the supported-versions module.
const goTool = "go"

// MatrixResolver answers matrix questions for a tool. A
// *supported.Resolver satisfies it; tests inject a recorded report.
type MatrixResolver interface {
	Matrix(ctx context.Context, tool, constraint string) (supported.Report, error)
}

// activeResolver resolves the Go matrix. The CLI entry point
// (`cmd/build-metadata/run.go`) installs one built from the action
// inputs via SetResolver before invoking Extract on Go projects. This
// package-scoped wiring mirrors the Python extractor's policy pattern:
// the `extractor.Extractor` interface has a fixed `Extract(path string)`
// signature that we cannot widen. When none is installed, the snapshot
// resolver applies, so `go test` and library callers never touch the
// network.
var activeResolver MatrixResolver

// SetResolver installs the resolver used for matrix generation; nil
// restores the snapshot default.
func SetResolver(r MatrixResolver) {
	activeResolver = r
}

// NewResolver builds a resolver from s with the Go admission this
// extractor has always applied: a line is admitted once released, as
// the lifecycle data says, whether or not the actions/go-versions
// manifest has a build of it yet. The module's catalog admits a Go line
// only once it is installable, which would drop a new line from the
// matrix until its builds land.
//
// With that admission the manifest cannot change the matrix, so the
// resolver reads endoflife.date alone: s.Catalog is replaced by the
// embedded catalog without Go's manifest source. The module fails a
// whole report when the manifest answers 404 or does not parse, which
// would otherwise discard valid lifecycle data for the snapshot.
func NewResolver(s supported.Settings) (*supported.Resolver, error) {
	cat, err := supported.CatalogWithoutSource(goTool, supported.SourceActionsManifest)
	if err != nil {
		return nil, err
	}
	s.Catalog = cat
	override := catalog.PolicyOverride{}
	if s.Override != nil {
		override = *s.Override
	}
	admit := catalog.AdmitReleased
	override.Admit = &admit
	s.Override = &override
	return supported.New(s)
}

// newSnapshotResolver builds a resolver that reads the module's
// embedded snapshot alone: offline, and without the runner cache, so a
// malformed response cached by an earlier fetch cannot defeat the
// fallback. Tests replace it to fix the clock or the report.
var newSnapshotResolver = func() (MatrixResolver, error) {
	return NewResolver(supported.Settings{Offline: true, NoCache: true})
}

// resolveMatrix asks r for the Go lines the go.mod minimum allows.
func resolveMatrix(r MatrixResolver, minimum string) (supported.Report, error) {
	return r.Matrix(context.Background(), goTool, minimum)
}

// generateGoVersionMatrix generates the list of Go versions a workflow
// should test against, given the minimum Go version declared in go.mod.
// The matrix is the supported Go release lines at or above that
// minimum, ascending, as the supported-versions module resolves them.
// When the go.mod minimum is older than every supported line the full
// supported set is returned (testing on EOL toolchains adds no value);
// when it is newer than every supported line (e.g. a pre-release
// toolchain) the matrix contains just the declared minimum so the
// workflow still tests something meaningful.
func generateGoVersionMatrix(goVersion string) []string {
	minimum := normalizeGoVersion(goVersion)

	rep, err := resolveGoReport(minimum)
	if err != nil {
		fmt.Fprintf(os.Stderr,
			"[WARNING] Failed to resolve supported Go versions (%v); using the go.mod minimum %s\n",
			err, minimum)
		return []string{minimum}
	}

	lines := rep.Matrix.Versions
	if len(lines) == 0 {
		fmt.Fprintf(os.Stderr,
			"[WARNING] No supported Go versions resolved; using the go.mod minimum %s\n", minimum)
		return []string{minimum}
	}
	// The module widens a constraint that allows no line to the whole
	// matrix. A minimum older than every line is already "applied", so
	// this outcome only arises for a minimum newer than every line,
	// where this action tests the declared minimum alone.
	if rep.Constraint != nil && rep.Constraint.Outcome == supported.OutcomeOutOfRangeFallback &&
		compareGoVersionStrings(minimum, lines[len(lines)-1]) > 0 {
		return []string{minimum}
	}
	return lines
}

// resolveGoReport resolves the Go report through the installed
// resolver, dropping any line older than the embedded snapshot's
// oldest supported line: upstream data that lags an end-of-life date
// cannot put a line back that the snapshot already retired. When the
// installed resolver fails, or yields no supported line (such as an
// upstream answering with an empty release list), it retries against
// the snapshot, as the static fallback list served before.
func resolveGoReport(minimum string) (supported.Report, error) {
	snapshot, snapErr := newSnapshotResolver()
	if c, ok := snapshot.(io.Closer); snapErr == nil && ok {
		defer func() { _ = c.Close() }()
	}
	if activeResolver != nil {
		rep, err := resolveMatrix(activeResolver, minimum)
		if err == nil && snapErr == nil {
			rep.Matrix.Versions = dropBelowFloor(rep.Matrix.Versions, snapshotFloor(snapshot))
		}
		switch {
		case err != nil:
			fmt.Fprintf(os.Stderr,
				"[WARNING] Failed to resolve supported Go versions (%v); retrying from the embedded snapshot\n", err)
		case len(rep.Matrix.Versions) == 0:
			fmt.Fprintf(os.Stderr,
				"[WARNING] Resolved no supported Go versions; retrying from the embedded snapshot\n")
		default:
			return rep, nil
		}
	}
	if snapErr != nil {
		return supported.Report{}, snapErr
	}
	return resolveMatrix(snapshot, minimum)
}

// snapshotFloor returns the oldest Go line r holds supported, or ""
// when it holds none or cannot tell.
func snapshotFloor(r MatrixResolver) string {
	rep, err := resolveMatrix(r, "")
	if err != nil || len(rep.Matrix.Versions) == 0 {
		return ""
	}
	return rep.Matrix.Versions[0]
}

// dropBelowFloor returns the lines at or above floor, in a new slice;
// an empty floor keeps every line.
func dropBelowFloor(lines []string, floor string) []string {
	if floor == "" {
		return lines
	}
	kept := make([]string, 0, len(lines))
	var dropped []string
	for _, line := range lines {
		if compareGoVersionStrings(line, floor) < 0 {
			dropped = append(dropped, line)
			continue
		}
		kept = append(kept, line)
	}
	if len(dropped) > 0 {
		fmt.Fprintf(os.Stderr,
			"[WARNING] Dropping Go %v: older than %s, the oldest line the embedded snapshot supports\n",
			dropped, floor)
	}
	return kept
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
