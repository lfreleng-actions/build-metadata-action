// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package python

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/lfreleng-actions/build-metadata-action/internal/extractor"
	"github.com/lfreleng-actions/build-metadata-action/internal/pyversions"
)

// isSupportedPythonVersion returns true when v (in `X.Y` form) is part of
// the set of Python versions this action's matrix generator may emit. It
// consults the *active policy*'s SupportedSet (not the static
// `supportedPythonVersions` slice) so the classifier-derived matrix path
// agrees with the constraint-solver path even when the policy was widened
// from the live endoflife.date response. In offline mode the active
// policy's SupportedSet is seeded from `supportedPythonVersions` so the
// behaviour is identical to the previous static check.
func isSupportedPythonVersion(v string) bool {
	supported := ActivePolicy().SupportedSet
	if len(supported) == 0 {
		supported = supportedPythonVersions
	}
	for _, s := range supported {
		if s == v {
			return true
		}
	}
	return false
}

// derivePythonVersionsFromClassifiers extracts Python `X.Y` versions from
// PEP-301 trove classifiers. Returns a deduplicated, version-sorted list
// filtered to the set of versions allowed by the active policy (the live
// supported set in online mode, or `supportedPythonVersions` offline);
// EOL versions remain visible in the matrix and are surfaced via the
// `python_eol_versions` outputs rather than being silently dropped here.
func derivePythonVersionsFromClassifiers(classifiers []string) []string {
	re := regexp.MustCompile(`Programming Language\s*::\s*Python\s*::\s*(\d+\.\d+)`)
	seen := make(map[string]struct{})
	var versions []string
	for _, c := range classifiers {
		if matches := re.FindStringSubmatch(c); len(matches) > 1 {
			v := matches[1]
			if !isSupportedPythonVersion(v) {
				continue
			}
			if _, ok := seen[v]; !ok {
				seen[v] = struct{}{}
				versions = append(versions, v)
			}
		}
	}
	sort.Slice(versions, func(i, j int) bool {
		return compareVersions(versions[i], versions[j]) < 0
	})
	return versions
}

// propagateFallbackPythonMatrix copies python-version-related metadata
// from a setup.py/setup.cfg fallback extraction back into the primary
// (pyproject.toml-derived) metadata map. `requires_python`,
// `version_matrix`, `matrix_json`, `build_version`, and
// `requires_python_source` are each propagated independently so that a
// classifier-derived matrix (which produces `requires_python_source =
// "classifiers"` without populating `requires_python`) is honoured even
// when `requires_python` itself is empty in the fallback.
func propagateFallbackPythonMatrix(metadata, fallbackMetadata *extractor.ProjectMetadata) {
	if metadata == nil || metadata.LanguageSpecific == nil || fallbackMetadata == nil || fallbackMetadata.LanguageSpecific == nil {
		return
	}
	if requiresPython, ok := fallbackMetadata.LanguageSpecific["requires_python"].(string); ok && requiresPython != "" {
		metadata.LanguageSpecific["requires_python"] = requiresPython
	}
	if matrix, ok := fallbackMetadata.LanguageSpecific["version_matrix"].([]string); ok && len(matrix) > 0 {
		metadata.LanguageSpecific["version_matrix"] = matrix
	}
	if matrixJSON, ok := fallbackMetadata.LanguageSpecific["matrix_json"].(string); ok && matrixJSON != "" {
		metadata.LanguageSpecific["matrix_json"] = matrixJSON
	}
	if buildVersion, ok := fallbackMetadata.LanguageSpecific["build_version"].(string); ok && buildVersion != "" {
		metadata.LanguageSpecific["build_version"] = buildVersion
	}
	if source, ok := fallbackMetadata.LanguageSpecific["requires_python_source"].(string); ok && source != "" {
		metadata.LanguageSpecific["requires_python_source"] = source
	}
}

// applyFallbackPythonMatrix populates Python version matrix metadata with
// a sensible default when the project does not declare a supported Python
// version range. Many legacy projects (notably PBR-based packages with a
// setup.cfg or setup.py that omits python_requires) rely on the build
// environment's installed Python rather than declaring a constraint.
// Without a fallback, downstream actions cannot determine which Python
// version to use for the build and fail outright.
//
// The fallback only fires when build_version is missing; projects that
// declared requires-python or version classifiers keep their derived
// matrix unchanged. The fallback emits requires_python_fallback=true so
// downstream consumers can surface a warning to the user.
//
// applyFallbackPythonMatrix sets the matrix to the policy's supported
// set when no Python version signal could be derived from the project
// itself. This is the "static-fallback" path; downstream consumers can
// detect it via `python_requires_python_source = "static-fallback"`.
//
// EOL filtering is NOT applied here: build-metadata-action is purely
// informational about EOL status. EOL versions remain in the matrix and
// are surfaced via `eol_versions` / `eol_versions_present`; downstream
// consumers decide what to do.
//
// `build_version` is set to the LATEST matrix entry to match the
// action's documented contract (`python_build_version` is described as
// "Recommended Python version for building (latest from matrix)" in
// action.yaml). The constraint-derived path uses the same selection so
// the two paths produce consistent outputs.
func applyFallbackPythonMatrix(metadata *extractor.ProjectMetadata, source string) {
	if metadata == nil || metadata.LanguageSpecific == nil {
		return
	}
	if buildVersion, ok := metadata.LanguageSpecific["build_version"].(string); ok && buildVersion != "" {
		return
	}

	policy := ActivePolicy()
	// requiresPython is empty in this path, so the generator always
	// reports matrixEmptyConstraint and returns the supported set
	// verbatim; bail out defensively on anything else or an empty set.
	fallback, status := generatePythonVersionMatrix("", policy.SupportedSet)
	if status != matrixEmptyConstraint || len(fallback) == 0 {
		return
	}

	metadata.LanguageSpecific["version_matrix"] = fallback
	metadata.LanguageSpecific["matrix_json"] = fmt.Sprintf(`{"python-version": [%s]}`,
		strings.Join(quoteStrings(fallback), ", "))
	metadata.LanguageSpecific["build_version"] = fallback[len(fallback)-1]
	metadata.LanguageSpecific["requires_python_fallback"] = true
	emitEOLOutputs(metadata, fallback)
	// Mark the source of the resulting matrix so downstream consumers can
	// tell a fallback guess apart from a matrix derived from
	// `requires-python` or trove classifiers. Only set when no upstream
	// path has already declared a source (e.g. "classifiers").
	if src, ok := metadata.LanguageSpecific["requires_python_source"].(string); !ok || src == "" {
		metadata.LanguageSpecific["requires_python_source"] = "static-fallback"
	}

	fmt.Fprintf(os.Stderr,
		"[WARNING] %s does not declare requires-python or Python classifiers; using fallback Python matrix %v (build_version=%s, latest supported)\n",
		source, fallback, fallback[len(fallback)-1])
}

// emitEOLOutputs writes the `eol_versions` and `eol_versions_present`
// language-specific fields for the supplied matrix. It is safe to call
// from every code path that sets `version_matrix`; it always emits the
// keys (with empty/false values when no EOL versions are present) so
// downstream `if:` predicates have a stable value to read regardless of
// which extractor branch produced the matrix.
func emitEOLOutputs(metadata *extractor.ProjectMetadata, matrix []string) {
	if metadata == nil || metadata.LanguageSpecific == nil {
		return
	}
	eolHits := detectEOLInMatrix(matrix, ActivePolicy())
	metadata.LanguageSpecific["eol_versions"] = strings.Join(eolHits, " ")
	metadata.LanguageSpecific["eol_versions_present"] = len(eolHits) > 0
}

// resolveAndEmitMatrix is the canonical matrix-emission helper shared by
// every code path that has a non-empty `requires-python` style constraint
// in hand. It:
//
//  1. Generates the matrix scoped to the active policy's supported set.
//  2. Surfaces the set of EOL Python versions present in the matrix
//     via `eol_versions` / `eol_versions_present`, leaving the matrix
//     itself unchanged. Downstream consumers (e.g. python-build-action)
//     decide whether to warn, strip, or fail on EOL hits.
//
// The returned error is always nil; the signature keeps the error slot
// so callers can chain it into their existing error-propagation paths
// without a special-case wrapper. build-metadata-action is not
// opinionated about failing the workflow on EOL hits.
func resolveAndEmitMatrix(metadata *extractor.ProjectMetadata, requiresPython, source string) error {
	policy := ActivePolicy()
	matrix, status := generatePythonVersionMatrix(requiresPython, policy.SupportedSet)
	debugf("[DEBUG] Generated matrix: %v (len=%d, status=%d)\n", matrix, len(matrix), status)

	effectiveSource := source
	switch status {
	case matrixNoMatch:
		// Constraint parsed cleanly but no supported Python satisfies
		// it (e.g. `<3.10` or `>=4.0`). Widen to the policy supported
		// set so the workflow still has something to build against,
		// and tag the situation so downstream consumers can detect it.
		matrix = append([]string(nil), policy.SupportedSet...)
		effectiveSource = "out-of-range-fallback"
		msg := fmt.Sprintf(
			"Project requires-python %q matches no supported Python; "+
				"widening matrix to the supported set %v",
			requiresPython, matrix)
		fmt.Fprintf(os.Stderr, "::warning::%s\n", msg)
		writeOutOfRangeStepSummary(requiresPython, matrix)
	case matrixParseError:
		// generatePythonVersionMatrix already emitted a ::warning::
		// describing the parse failure and returned the supported set
		// as a defensive fallback. Override the source so consumers do
		// not mistake the widened matrix for a constraint-satisfied
		// derivation.
		effectiveSource = "parse-error-fallback"
	case matrixOK, matrixEmptyConstraint:
		// Keep the caller-supplied source as-is.
	}

	if len(matrix) == 0 {
		debugf("[DEBUG] Matrix generation returned empty slice\n")
		return nil
	}

	metadata.LanguageSpecific["version_matrix"] = matrix
	metadata.LanguageSpecific["matrix_json"] = fmt.Sprintf(`{"python-version": [%s]}`,
		strings.Join(quoteStrings(matrix), ", "))
	metadata.LanguageSpecific["build_version"] = matrix[len(matrix)-1]
	if effectiveSource != "" {
		metadata.LanguageSpecific["requires_python_source"] = effectiveSource
	}
	// Any path that widens the matrix to the supported set is a
	// fallback from the consumer's point of view (the matrix is no
	// longer scoped to what the project asked for). Set
	// `requires_python_fallback=true` whenever we landed on one of the
	// -fallback source tags so consumers reading the boolean output
	// see a consistent signal regardless of which path fired.
	if strings.HasSuffix(effectiveSource, "-fallback") {
		metadata.LanguageSpecific["requires_python_fallback"] = true
	}
	// Surface the EOL versions present in the constraint-derived matrix
	// so downstream consumers can implement their own fail-fast policy.
	// `eol_versions` is space-separated; `eol_versions_present` is the
	// boolean shortcut for workflow `if:` predicates.
	emitEOLOutputs(metadata, matrix)
	return nil
}

// supportedPythonVersions is the static, offline-safe default set of
// Python versions this action knows how to emit. It seeds the policy's
// SupportedSet in offline mode and acts as a fallback whenever the live
// endoflife.date lookup fails. At runtime the matrix generators consult
// `ActivePolicy().SupportedSet` (which may be wider in online mode
// because it includes EOL cycles for informational surfacing), so both
// the requires-python constraint path and the PEP-301 classifier path
// agree on which versions are buildable for the run in question.
//
// The slice is initialised from `pyversions.GetFallbackVersions()` so
// the supported range is controlled by the single set of baseline /
// latest constants in `internal/pyversions/eol.go`. To bump either
// bound (e.g. drop an EOL version, add a freshly released minor) edit
// `baselineMinor` / `latestMinor` over there; the rest of the extractor
// follows automatically.
var supportedPythonVersions = pyversions.GetFallbackVersions()

// matrixStatus encodes how generatePythonVersionMatrix arrived at its
// returned slice. resolveAndEmitMatrix branches on this to tag
// `requires_python_source` correctly.
type matrixStatus int

const (
	// matrixOK -- constraint parsed cleanly and at least one supported
	// version satisfied it. The returned matrix is non-empty.
	matrixOK matrixStatus = iota
	// matrixEmptyConstraint -- the caller supplied no constraint. The
	// returned matrix is the full supported set; callers should retain
	// the source label they passed (e.g. "requires-python" is
	// inappropriate but they would never invoke us in that case anyway).
	matrixEmptyConstraint
	// matrixParseError -- the constraint failed to parse. The returned
	// matrix is the supported set as a defensive fallback; callers
	// should tag the source as "parse-error-fallback".
	matrixParseError
	// matrixNoMatch -- the constraint parsed cleanly but no supported
	// version satisfied it. The returned matrix is nil; callers should
	// widen to the supported set and tag the source as
	// "out-of-range-fallback".
	matrixNoMatch
)

// generatePythonVersionMatrix generates a list of Python versions from
// a requires-python specifier, scoped to the supplied supported set.
//
// The function is a thin wrapper around `pyversions.ResolveVersions` so
// build-metadata-action benefits from the full PEP 440 / Poetry caret
// constraint solver that previously lived only in the dormant
// `internal/pyversions` package.
//
// Return-value contract:
//
//   - matrixOK: constraint satisfied, len(matrix) > 0.
//   - matrixEmptyConstraint: caller passed no constraint; matrix is the
//     supplied supported set verbatim.
//   - matrixParseError: constraint failed to parse; matrix is the
//     supported set as a defensive fallback and a `::warning::` is
//     emitted to stderr so the user sees the parse failure.
//   - matrixNoMatch: constraint parsed cleanly but no supported version
//     satisfies it (e.g. `<3.10` or `>=4.0`); matrix is nil and the
//     caller MUST widen explicitly.
//
// When `supported` is empty the package-level `supportedPythonVersions`
// slice is used.
func generatePythonVersionMatrix(requiresPython string, supported []string) ([]string, matrixStatus) {
	if len(supported) == 0 {
		supported = supportedPythonVersions
	}
	if requiresPython == "" {
		return append([]string(nil), supported...), matrixEmptyConstraint
	}
	versions, err := pyversions.ResolveVersions(requiresPython, supported)
	if err != nil {
		// Use the typed sentinel from the pyversions package so callers
		// detect the no-match case via errors.Is rather than substring
		// matching, which would silently change behaviour if the error
		// wording in pyversions ever drifted.
		if errors.Is(err, pyversions.ErrNoVersionsMatch) {
			return nil, matrixNoMatch
		}
		fmt.Fprintf(os.Stderr,
			"::warning::Could not parse requires-python %q (%v); using supported set as fallback\n",
			requiresPython, err)
		return append([]string(nil), supported...), matrixParseError
	}
	if len(versions) == 0 {
		return nil, matrixNoMatch
	}
	return versions, matrixOK
}

// compareVersions compares two version strings (e.g., "3.9" vs "3.11")
// Returns: -1 if v1 < v2, 0 if v1 == v2, 1 if v1 > v2
func compareVersions(v1, v2 string) int {
	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")

	// Compare each part numerically
	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	for i := 0; i < maxLen; i++ {
		var p1, p2 int
		if i < len(parts1) {
			if n, err := strconv.Atoi(parts1[i]); err == nil {
				p1 = n
			}
		}
		if i < len(parts2) {
			if n, err := strconv.Atoi(parts2[i]); err == nil {
				p2 = n
			}
		}

		if p1 < p2 {
			return -1
		}
		if p1 > p2 {
			return 1
		}
	}
	return 0
}

// quoteStrings adds quotes around each string
func quoteStrings(strs []string) []string {
	quoted := make([]string, len(strs))
	for i, s := range strs {
		quoted[i] = fmt.Sprintf(`"%s"`, s)
	}
	return quoted
}
