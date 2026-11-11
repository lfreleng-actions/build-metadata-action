// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package java

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/lfreleng-actions/build-metadata-action/internal/extractor"
	"github.com/lfreleng-actions/build-metadata-action/internal/supported"
)

// javaTool is the catalog id of Java in the supported-versions module.
const javaTool = "java"

// activeResolver resolves the supported Java lines. The CLI entry
// point (`cmd/build-metadata/run.go`) installs one built from the
// action inputs via SetResolver before invoking Extract on Maven and
// Gradle projects, as it does for the Go and Rust extractors. When
// none is installed, the snapshot resolver applies, so `go test` and
// library callers never touch the network.
var activeResolver supported.ProjectMatrixResolver

// SetResolver installs the resolver used for matrix generation; nil
// restores the snapshot default.
func SetResolver(r supported.ProjectMatrixResolver) {
	activeResolver = r
}

// NewResolver builds a resolver from s with the module's Java policy:
// the supported Temurin LTS lines from 17, read from endoflife.date
// and the Adoptium API.
func NewResolver(s supported.Settings) (*supported.Resolver, error) {
	return supported.New(s)
}

// newSnapshotResolver builds a resolver that reads the module's
// embedded snapshot alone: offline, and without the runner cache, so a
// malformed response cached by an earlier fetch cannot defeat the
// fallback. Tests replace it to fix the clock or the report.
var newSnapshotResolver = func() (supported.ProjectMatrixResolver, error) {
	return NewResolver(supported.Settings{Offline: true, NoCache: true})
}

// warnings receives the warnings matrix resolution emits. Tests
// replace it.
var warnings io.Writer = os.Stderr

func warnf(format string, args ...any) {
	_, _ = fmt.Fprintf(warnings, "[WARNING] "+format+"\n", args...)
}

// applyJavaVersionMatrix records the supported Java lines at or above
// the level in java_version as matrix_json, published as
// java_matrix_json, and the newest of them as latest (java_latest).
// The module reads the level as "this release or newer". Without a
// level, or with one that does not parse, the matrix is unconstrained;
// when no matrix resolves the keys stay unset: extraction never fails.
func applyJavaVersionMatrix(metadata *extractor.ProjectMetadata) {
	level, _ := metadata.LanguageSpecific["version"].(string)
	req := supported.MatrixRequest{Constraint: strings.TrimSpace(level)}
	rep, err := resolveJavaMatrix(req)
	if errors.Is(err, supported.ErrConstraintSyntax) {
		warnf("Could not parse the Java version %q (%v); using the full supported matrix", level, err)
		rep, err = resolveJavaMatrix(supported.MatrixRequest{})
	}
	if err != nil {
		warnf("Failed to resolve supported Java versions (%v); leaving java_matrix_json and java_latest unset", err)
		return
	}
	for _, w := range rep.Warnings {
		warnf("%s", w)
	}
	metadata.LanguageSpecific["matrix_json"] = supported.MatrixJSON(rep)
	if rep.Latest != "" {
		metadata.LanguageSpecific["latest"] = rep.Latest
	}
}

func resolveJavaMatrix(req supported.MatrixRequest) (supported.Report, error) {
	return supported.ResolveWithFallback(context.Background(), javaTool, req, activeResolver, newSnapshotResolver, warnf)
}
