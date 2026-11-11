// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package javascript

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

// nodeTool is the catalog id of Node.js in the supported-versions
// module.
const nodeTool = "nodejs"

// activeResolver resolves the supported Node.js lines. The CLI entry
// point (`cmd/build-metadata/run.go`) installs one built from the
// action inputs via SetResolver before invoking Extract on JavaScript
// and TypeScript projects, as it does for the Go and Rust extractors.
// When none is installed, the snapshot resolver applies, so `go test`
// and library callers never touch the network.
var activeResolver supported.ProjectMatrixResolver

// SetResolver installs the resolver used for matrix generation; nil
// restores the snapshot default.
func SetResolver(r supported.ProjectMatrixResolver) {
	activeResolver = r
}

// NewResolver builds a resolver from s with the module's Node.js
// policy: the supported LTS lines with an installable build, read from
// endoflife.date and the actions/node-versions manifest.
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

// applyNodeVersionMatrix records the supported Node.js lines the
// project accepts as node_matrix_json and the newest of them as
// node_latest. The explicit engines.node constraint wins; without one
// the module reads .nvmrc or .node-version in projectPath. A
// constraint that does not parse yields the unconstrained matrix, and
// when no matrix resolves the keys stay unset: extraction never fails.
func applyNodeVersionMatrix(projectPath string, metadata *extractor.ProjectMetadata) {
	req := supported.MatrixRequest{ProjectDir: projectPath}
	if constraint, _ := metadata.LanguageSpecific["requires_node"].(string); strings.TrimSpace(constraint) != "" {
		req = supported.MatrixRequest{Constraint: constraint}
	}
	rep, err := resolveNodeMatrix(req)
	if errors.Is(err, supported.ErrConstraintSyntax) {
		warnf("Could not parse the Node.js version constraint (%v); using the full supported matrix", err)
		rep, err = resolveNodeMatrix(supported.MatrixRequest{})
	}
	if err != nil {
		warnf("Failed to resolve supported Node.js versions (%v); leaving node_matrix_json and node_latest unset", err)
		return
	}
	for _, w := range rep.Warnings {
		warnf("%s", w)
	}
	metadata.LanguageSpecific["node_matrix_json"] = supported.MatrixJSON(rep)
	if rep.Latest != "" {
		metadata.LanguageSpecific["node_latest"] = rep.Latest
	}
}

func resolveNodeMatrix(req supported.MatrixRequest) (supported.Report, error) {
	return supported.ResolveWithFallback(context.Background(), nodeTool, req, activeResolver, newSnapshotResolver, warnf)
}
