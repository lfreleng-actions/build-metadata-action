// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package python

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/lfreleng-actions/build-metadata-action/internal/supported"
)

// pythonTool is the catalog id of Python in the supported-versions
// module.
const pythonTool = "python"

// MatrixResolver answers matrix questions for a tool. A
// *supported.Resolver satisfies it; tests inject a recorded report.
type MatrixResolver = supported.MatrixResolver

// activeResolver and lifecycleResolver resolve the Python matrix. The
// CLI entry point (`cmd/build-metadata/run.go`) installs them, built
// from the action inputs, via SetResolvers before invoking Extract on
// Python projects: the `extractor.Extractor` interface has a fixed
// `Extract(path string)` signature that we cannot widen. When none is
// installed, the snapshot resolver applies, so `go test` and library
// callers never touch the network.
var (
	activeResolver    MatrixResolver
	lifecycleResolver MatrixResolver
)

// SetResolvers installs the resolvers used for matrix generation.
// primary answers first; lifecycle, when set, answers once when
// primary fails, before the embedded snapshot. nil for both restores
// the snapshot default.
func SetResolvers(primary, lifecycle MatrixResolver) {
	activeResolver = primary
	lifecycleResolver = lifecycle
}

// NewResolver builds the primary Python resolver from s with the
// module's catalog admission: a line enters the matrix once
// actions/setup-python can install it, as the actions/python-versions
// manifest says.
func NewResolver(s supported.Settings) (*supported.Resolver, error) {
	return supported.New(s)
}

// NewLifecycleResolver builds the resolver tried when the primary one
// fails: it reads endoflife.date alone and admits a line once released.
// The module fails a whole report when the actions/python-versions
// manifest answers 404 or does not parse, which would otherwise discard
// valid lifecycle data for the snapshot.
func NewLifecycleResolver(s supported.Settings) (*supported.Resolver, error) {
	return supported.NewReleased(pythonTool, s)
}

// newSnapshotResolver builds a resolver that reads the module's
// embedded snapshot alone: offline, and without the runner cache, so a
// malformed response cached by an earlier fetch cannot defeat the
// fallback. Tests replace it to fix the clock or the report.
var newSnapshotResolver = func() (MatrixResolver, error) {
	return NewResolver(supported.Settings{Offline: true, NoCache: true})
}

// resolvePythonReport resolves the Python report for constraint
// through the installed resolvers, trying the primary one, then the
// lifecycle one when the primary fails, then the embedded snapshot.
// A report with no line in its matrix (such as an upstream answering
// with an empty release list) counts as a failure. A constraint that
// does not parse returns supported.ErrConstraintSyntax at once, as no
// other resolver would parse it either.
//
// The primary report also goes to the lifecycle resolver when its
// matrix is empty, as when the manifest parses but lists no stable
// build, and when it pairs fresher lifecycle data with a manifest read
// from an expired cache entry or the snapshot, as when the manifest
// answers 503 or times out: that manifest can lack a line released
// since it was fetched, so it would drop a line the lifecycle data
// lists.
// The primary report stands when the lifecycle resolver yields no
// matrix.
func resolvePythonReport(constraint string) (supported.Report, error) {
	if activeResolver != nil {
		rep, err := activeResolver.Matrix(context.Background(), pythonTool, constraint)
		if lifecycleResolver != nil {
			rep, err = retryLifecycle(constraint, rep, err)
		}
		switch {
		case errors.Is(err, supported.ErrConstraintSyntax):
			return supported.Report{}, err
		case err != nil:
			fmt.Fprintf(os.Stderr,
				"[WARNING] Failed to resolve supported Python versions (%v); retrying from the embedded snapshot\n", err)
		case len(rep.Matrix.Versions) == 0:
			fmt.Fprintf(os.Stderr,
				"[WARNING] Resolved no supported Python versions; retrying from the embedded snapshot\n")
		default:
			return rep, nil
		}
	}
	snapshot, err := newSnapshotResolver()
	if err != nil {
		return supported.Report{}, err
	}
	if c, ok := snapshot.(io.Closer); ok {
		defer func() { _ = c.Close() }()
	}
	return snapshot.Matrix(context.Background(), pythonTool, constraint)
}

// retryLifecycle asks the lifecycle resolver for constraint when the
// primary resolver failed with anything but a syntax error, when its
// report rep has an empty matrix, or when rep read the manifest from
// a fallback older than its lifecycle data. In the last two cases rep stands unless the lifecycle
// resolver yields a matrix. The module widens a constraint no line
// satisfies to the whole matrix, so an empty matrix means no line was
// admitted at all, never a line still awaiting an installable build.
func retryLifecycle(constraint string, rep supported.Report, err error) (supported.Report, error) {
	switch {
	case err != nil && !errors.Is(err, supported.ErrConstraintSyntax):
		fmt.Fprintf(os.Stderr,
			"[WARNING] Failed to resolve supported Python versions (%v); retrying from endoflife.date alone\n", err)
		return lifecycleResolver.Matrix(context.Background(), pythonTool, constraint)
	case err == nil && len(rep.Matrix.Versions) == 0:
		fmt.Fprintf(os.Stderr,
			"[WARNING] Resolved no supported Python versions; retrying from endoflife.date alone\n")
	case err == nil && manifestLagsLifecycle(rep):
		fmt.Fprintf(os.Stderr,
			"[WARNING] Read the actions/python-versions manifest from an expired cache entry or the embedded snapshot; retrying from endoflife.date alone\n")
	default:
		return rep, err
	}
	lrep, lerr := lifecycleResolver.Matrix(context.Background(), pythonTool, constraint)
	if lerr == nil && len(lrep.Matrix.Versions) > 0 {
		return lrep, nil
	}
	return rep, err
}

// manifestLagsLifecycle reports whether rep read its manifest from a
// fallback, an expired cache entry or the embedded snapshot, while
// reading fresher lifecycle data from upstream or the runner cache. A
// fallback manifest can lack a line the lifecycle data lists. A fresh
// cached manifest does not count, so a line still waits for an
// installable build; nor does a report read wholly from the snapshot,
// as offline, since a lifecycle-only retry would read the same one.
func manifestLagsLifecycle(rep supported.Report) bool {
	var manifest, lifecycle *supported.SourceEntry
	for i := range rep.Source.Sources {
		s := &rep.Source.Sources[i]
		switch s.Type {
		case supported.SourceActionsManifest:
			manifest = s
		case supported.SourceEndOfLife:
			lifecycle = s
		}
	}
	if manifest == nil || lifecycle == nil {
		return false
	}
	if manifest.Kind != supported.KindStale && manifest.Kind != supported.KindSnapshot {
		return false
	}
	if lifecycle.Kind == supported.KindSnapshot || lifecycle.Kind == supported.KindUnavailable {
		return false
	}
	if kindRank(lifecycle.Kind) < kindRank(manifest.Kind) {
		return true
	}
	return manifest.FetchedAt != nil && lifecycle.FetchedAt != nil &&
		lifecycle.FetchedAt.After(*manifest.FetchedAt)
}

// kindRank orders report source kinds from the freshest to the worst.
func kindRank(k supported.Kind) int {
	switch k {
	case supported.KindLive:
		return 0
	case supported.KindCache:
		return 1
	case supported.KindStale:
		return 2
	case supported.KindSnapshot:
		return 3
	default:
		return 4
	}
}

// eolLines returns the set of lines rep marks end-of-life.
func eolLines(rep supported.Report) map[string]bool {
	eol := map[string]bool{}
	for _, l := range rep.Lines {
		if l.Status == supported.StatusEOL {
			eol[l.Line] = true
		}
	}
	return eol
}

// debugf writes a debug log line to stderr only when INPUT_VERBOSE is
// set to "true" (matching the convention used by other extractors and
// documented by the action's `verbose` input). Without this gate, the
// extractor would spam logs even on quiet runs and bury more important
// `::warning::` / `::error::` lines.
func debugf(format string, args ...interface{}) {
	if os.Getenv("INPUT_VERBOSE") != "true" {
		return
	}
	fmt.Fprintf(os.Stderr, format, args...)
}

// writeOutOfRangeStepSummary appends a notice to the GitHub Actions
// step summary when the project's requires-python constraint matches
// no supported Python and the matrix had to widen as a fallback. The
// step summary is the user-visible surface; failures to write it are
// silently swallowed because the notice is purely advisory.
func writeOutOfRangeStepSummary(requiresPython string, matrix []string) {
	path := os.Getenv("GITHUB_STEP_SUMMARY")
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	_, _ = fmt.Fprintf(f, "### Python requires-python out of range\n\n")
	_, _ = fmt.Fprintf(f, "Project declared `requires-python = %q`, which matches no "+
		"Python version supported by this action. The build matrix was widened to "+
		"the supported set so the workflow can proceed:\n\n%s\n\n",
		requiresPython, strings.Join(matrix, ", "))
}
