// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package python

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lfreleng-actions/build-metadata-action/internal/extractor"
	"github.com/lfreleng-actions/build-metadata-action/internal/supported"
	"github.com/lfreleng-actions/build-metadata-action/internal/supported/supportedtest"
)

// snapshotMatrix is the matrix the embedded snapshot holds at
// fixedNow, and the one v0.12.0 emitted offline.
var snapshotMatrix = []string{"3.11", "3.12", "3.13", "3.14", "3.15"}

// fixedNow is a clock at which the embedded snapshot supports Python
// 3.11 through 3.15, none of them end-of-life.
func fixedNow() time.Time {
	return time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
}

// laterNow is a clock at which 3.11 (end-of-life 2027-10-31) and 3.12
// (end-of-life 2028-10-31) have reached end-of-life.
func laterNow() time.Time {
	return time.Date(2028, 11, 15, 12, 0, 0, 0, time.UTC)
}

// snapshotAt returns a snapshot resolver factory reading the clock now.
func snapshotAt(now func() time.Time) func() (MatrixResolver, error) {
	return func() (MatrixResolver, error) {
		return NewResolver(supported.Settings{Offline: true, NoCache: true, Now: now})
	}
}

// TestMain fixes the snapshot fallback's clock for every test in the
// package, so no test depends on the day it runs.
func TestMain(m *testing.M) {
	newSnapshotResolver = snapshotAt(fixedNow)
	os.Exit(m.Run())
}

// useResolvers installs primary and lifecycle for the duration of t.
func useResolvers(t *testing.T, primary, lifecycle MatrixResolver) {
	t.Helper()
	SetResolvers(primary, lifecycle)
	t.Cleanup(func() { SetResolvers(nil, nil) })
}

// useSnapshot replaces the snapshot resolver factory for the duration
// of t.
func useSnapshot(t *testing.T, f func() (MatrixResolver, error)) {
	t.Helper()
	saved := newSnapshotResolver
	newSnapshotResolver = f
	t.Cleanup(func() { newSnapshotResolver = saved })
}

// upstreamSettings returns settings reading u with the clock fixed and
// the cache in a fresh directory.
func upstreamSettings(t *testing.T, u *supportedtest.Upstream) supported.Settings {
	t.Helper()
	t.Setenv("SUPPORTED_VERSIONS_CACHE_DIR", t.TempDir())
	return supported.Settings{
		EndOfLifeBaseURL: u.URL,
		ManifestBaseURL:  u.URL,
		Now:              fixedNow,
	}
}

// useUpstream installs the primary and lifecycle resolvers over u.
// withLifecycle false leaves the lifecycle resolver out.
func useUpstream(t *testing.T, u *supportedtest.Upstream, withLifecycle bool) {
	t.Helper()
	s := upstreamSettings(t, u)
	primary, err := NewResolver(s)
	require.NoError(t, err)
	t.Cleanup(func() { _ = primary.Close() })
	var lifecycle MatrixResolver
	if withLifecycle {
		r, err := NewLifecycleResolver(s)
		require.NoError(t, err)
		t.Cleanup(func() { _ = r.Close() })
		lifecycle = r
	}
	useResolvers(t, primary, lifecycle)
}

// failingResolver fails every request, as a broken upstream would,
// and counts the calls.
type failingResolver struct{ calls *int }

func (r failingResolver) Matrix(context.Context, string, string) (supported.Report, error) {
	if r.calls != nil {
		*r.calls++
	}
	return supported.Report{}, errors.New("upstream unavailable")
}

// recordedResolver answers every request with one recorded report.
type recordedResolver struct {
	report supported.Report
}

func (r recordedResolver) Matrix(context.Context, string, string) (supported.Report, error) {
	return r.report, nil
}

// extractSetupCfg runs the extractor over a project holding setupCfg.
func extractSetupCfg(t *testing.T, setupCfg string) map[string]interface{} {
	t.Helper()
	tmpDir := createTempProject(t, map[string]string{"setup.cfg": setupCfg})
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	metadata, err := NewExtractor().Extract(tmpDir)
	require.NoError(t, err)
	return metadata.LanguageSpecific
}

// TestOfflineMatrixMatchesV012 checks the acceptance case: offline,
// `>=3.10` resolves to 3.11 through 3.15, as v0.12.0 emitted, with no
// end-of-life line reported.
func TestOfflineMatrixMatchesV012(t *testing.T) {
	useResolvers(t, nil, nil)
	ls := extractSetupCfg(t, "[metadata]\nname = clean-pkg\nversion = 1.0\npython_requires = >=3.10\n")

	assert.Equal(t, snapshotMatrix, ls["version_matrix"])
	assert.Equal(t, `{"python-version": ["3.11", "3.12", "3.13", "3.14", "3.15"]}`, ls["matrix_json"])
	assert.Equal(t, "3.15", ls["build_version"])
	assert.Equal(t, "requires-python", ls["requires_python_source"])
	assert.Equal(t, "", ls["eol_versions"],
		"eol_versions must be the empty string when no EOL versions are in the matrix")
	assert.Equal(t, false, ls["eol_versions_present"],
		"eol_versions_present must be false when no EOL versions are in the matrix")
}

// TestEOLVersionsSurfacedInOutput confirms that when the resolved
// matrix contains lines past their end-of-life, the extractor leaves
// them in the matrix and lists them, space-separated and in matrix
// order, in `eol_versions`. Downstream consumers such as
// python-build-action decide what to do.
func TestEOLVersionsSurfacedInOutput(t *testing.T) {
	useResolvers(t, nil, nil)
	useSnapshot(t, snapshotAt(laterNow))
	ls := extractSetupCfg(t, "[metadata]\nname = eol-detect-pkg\nversion = 1.0\npython_requires = >=3.10\n")

	assert.Equal(t, snapshotMatrix, ls["version_matrix"],
		"matrix must retain EOL versions; downstream consumers decide what to do")
	assert.Equal(t, "3.11 3.12", ls["eol_versions"],
		"multiple EOL hits must be joined by a single space, in matrix order")
	assert.Equal(t, true, ls["eol_versions_present"])
}

// TestClassifierMatrixEmitsEOLOutputs checks the classifier-derived
// matrix drops lines outside the supported set and reports the
// end-of-life lines it keeps.
func TestClassifierMatrixEmitsEOLOutputs(t *testing.T) {
	useResolvers(t, nil, nil)
	useSnapshot(t, snapshotAt(laterNow))
	ls := extractSetupCfg(t, "[metadata]\n"+
		"name = classifier-eol\n"+
		"version = 1.0\n"+
		"classifiers =\n"+
		"    Programming Language :: Python :: 3.10\n"+
		"    Programming Language :: Python :: 3.11\n"+
		"    Programming Language :: Python :: 3.12\n"+
		"    Programming Language :: Python :: 3.13\n")

	assert.Equal(t, "classifiers", ls["requires_python_source"])
	assert.Equal(t, []string{"3.11", "3.12", "3.13"}, ls["version_matrix"])
	assert.Equal(t, "3.11 3.12", ls["eol_versions"],
		"classifier-derived matrices must surface EOL versions for downstream consumers")
	assert.Equal(t, true, ls["eol_versions_present"])
}

// TestStaticFallbackUsesSupportedSet checks a project with no version
// signal gets the supported set, tagged static-fallback, with the EOL
// outputs present for stable downstream reads.
func TestStaticFallbackUsesSupportedSet(t *testing.T) {
	useResolvers(t, nil, nil)
	ls := extractSetupCfg(t, "[metadata]\nname = fallback-pkg\n")

	assert.Equal(t, snapshotMatrix, ls["version_matrix"])
	assert.Equal(t, "static-fallback", ls["requires_python_source"])
	assert.Equal(t, true, ls["requires_python_fallback"])
	assert.Equal(t, "", ls["eol_versions"])
	assert.Equal(t, false, ls["eol_versions_present"])
}

// TestOutOfRangeSetsRequiresPythonFallback checks a constraint no
// supported line satisfies widens the matrix, tags the source, sets
// requires_python_fallback and writes the step summary notice.
func TestOutOfRangeSetsRequiresPythonFallback(t *testing.T) {
	useResolvers(t, nil, nil)
	summary := filepath.Join(t.TempDir(), "summary.md")
	t.Setenv("GITHUB_STEP_SUMMARY", summary)
	ls := extractSetupCfg(t, "[metadata]\nname = out-of-range-fb\nversion = 1.0\npython_requires = >=4.0\n")

	assert.Equal(t, snapshotMatrix, ls["version_matrix"])
	assert.Equal(t, "out-of-range-fallback", ls["requires_python_source"])
	assert.Equal(t, true, ls["requires_python_fallback"],
		"requires_python_fallback must be true for any -fallback source path")
	body, err := os.ReadFile(summary)
	require.NoError(t, err)
	assert.Contains(t, string(body), "### Python requires-python out of range")
	assert.Contains(t, string(body), strings.Join(snapshotMatrix, ", "))
}

// TestUnparsableConstraintFallsBack checks text that does not parse
// widens the matrix to the supported set, tagged parse-error-fallback.
func TestUnparsableConstraintFallsBack(t *testing.T) {
	for _, requires := range []string{"garbage", "3.12"} {
		t.Run(requires, func(t *testing.T) {
			useResolvers(t, nil, nil)
			ls := extractSetupCfg(t, "[metadata]\nname = bad\nversion = 1.0\npython_requires = "+requires+"\n")

			assert.Equal(t, snapshotMatrix, ls["version_matrix"])
			assert.Equal(t, "parse-error-fallback", ls["requires_python_source"])
			assert.Equal(t, true, ls["requires_python_fallback"])
		})
	}
}

// TestSyntaxErrorSkipsLifecycleRetry checks a constraint that does not
// parse is not retried against the lifecycle resolver.
func TestSyntaxErrorSkipsLifecycleRetry(t *testing.T) {
	primary, err := NewResolver(supported.Settings{Offline: true, NoCache: true, Now: fixedNow})
	require.NoError(t, err)
	t.Cleanup(func() { _ = primary.Close() })
	calls := 0
	useResolvers(t, primary, failingResolver{calls: &calls})

	matrix, _, status := generatePythonVersionMatrix("garbage")
	assert.Equal(t, matrixParseError, status)
	assert.Equal(t, snapshotMatrix, matrix)
	assert.Equal(t, 0, calls, "a syntax error must not reach the lifecycle resolver")
}

// TestEmitEOLOutputsPreservesMatrixOrder checks the hits follow the
// matrix order so the output is stable across runs.
func TestEmitEOLOutputsPreservesMatrixOrder(t *testing.T) {
	ls := map[string]interface{}{}
	metadata := &extractor.ProjectMetadata{LanguageSpecific: ls}
	emitEOLOutputs(metadata, []string{"3.7", "3.8", "3.9", "3.10"}, map[string]bool{"3.9": true, "3.7": true})
	assert.Equal(t, "3.7 3.9", ls["eol_versions"])
	assert.Equal(t, true, ls["eol_versions_present"])

	emitEOLOutputs(metadata, []string{"3.10", "3.11"}, nil)
	assert.Equal(t, "", ls["eol_versions"])
	assert.Equal(t, false, ls["eol_versions_present"])
}

// TestLiveMatrixFromRecordedUpstream checks recorded endoflife.date
// and manifest data resolve to the same matrix as the snapshot.
func TestLiveMatrixFromRecordedUpstream(t *testing.T) {
	u := supportedtest.NewPythonUpstream(t)
	useUpstream(t, u, true)

	matrix, eol, status := generatePythonVersionMatrix(">=3.10")
	assert.Equal(t, matrixOK, status)
	assert.Equal(t, snapshotMatrix, matrix)
	assert.Empty(t, eol["3.11"])
	assert.True(t, eol["3.10"], "3.10 is end-of-life in the recorded data")
}

// TestNewLineWaitsForInstallableBuild checks the installable
// admission: a released 3.16 enters the matrix only once the manifest
// lists a stable build of it.
func TestNewLineWaitsForInstallableBuild(t *testing.T) {
	for name, tc := range map[string]struct {
		manifest string
		want     []string
	}{
		"lagging manifest":  {supportedtest.PythonManifest, snapshotMatrix},
		"installable build": {supportedtest.PythonManifestNewer, append(append([]string(nil), snapshotMatrix...), "3.16")},
	} {
		t.Run(name, func(t *testing.T) {
			u := supportedtest.NewPythonUpstream(t)
			u.ServeReleases(supportedtest.PythonReleasesNewer)
			u.ServeManifest(tc.manifest)
			useUpstream(t, u, true)

			matrix, _, _ := generatePythonVersionMatrix("")
			assert.Equal(t, tc.want, matrix)
		})
	}
}

// TestBrokenManifestKeepsLifecycleData checks a manifest that answers
// 404 or does not parse does not discard newer lifecycle data: the
// lifecycle resolver answers, with 3.16 released.
func TestBrokenManifestKeepsLifecycleData(t *testing.T) {
	want := append(append([]string(nil), snapshotMatrix...), "3.16")
	for name, breakManifest := range map[string]func(*supportedtest.Upstream){
		"404":       func(u *supportedtest.Upstream) { u.FailManifest(http.StatusNotFound) },
		"malformed": func(u *supportedtest.Upstream) { u.ServeManifest("<html>not json</html>") },
	} {
		t.Run(name, func(t *testing.T) {
			u := supportedtest.NewPythonUpstream(t)
			u.ServeReleases(supportedtest.PythonReleasesNewer)
			breakManifest(u)
			useUpstream(t, u, true)

			matrix, _, status := generatePythonVersionMatrix(">=3.10")
			assert.Equal(t, matrixOK, status)
			assert.Equal(t, want, matrix)
		})
	}
}

// countingResolver passes requests to next and counts them.
type countingResolver struct {
	next  MatrixResolver
	calls *int
}

func (r countingResolver) Matrix(ctx context.Context, tool, constraint string) (supported.Report, error) {
	*r.calls++
	return r.next.Matrix(ctx, tool, constraint)
}

// TestManifestOutageKeepsLifecycleData checks a manifest that answers
// 503 or times out, with nothing cached, does not hide newer lifecycle
// data: the module reads the snapshot's manifest, which lacks 3.16, so
// the lifecycle resolver answers instead and `>=3.16` stays in range.
func TestManifestOutageKeepsLifecycleData(t *testing.T) {
	for name, breakManifest := range map[string]func(*supportedtest.Upstream, *supported.Settings){
		"503": func(u *supportedtest.Upstream, _ *supported.Settings) {
			u.FailManifest(http.StatusServiceUnavailable)
		},
		"timeout": func(u *supportedtest.Upstream, s *supported.Settings) {
			u.StallManifest(5 * time.Second)
			s.Timeout = 100 * time.Millisecond
		},
	} {
		t.Run(name, func(t *testing.T) {
			u := supportedtest.NewPythonUpstream(t)
			u.ServeReleases(supportedtest.PythonReleasesNewer)
			s := upstreamSettings(t, u)
			breakManifest(u, &s)
			primary, err := NewResolver(s)
			require.NoError(t, err)
			t.Cleanup(func() { _ = primary.Close() })
			lifecycle, err := NewLifecycleResolver(s)
			require.NoError(t, err)
			t.Cleanup(func() { _ = lifecycle.Close() })

			rep, err := primary.Matrix(context.Background(), pythonTool, ">=3.16")
			require.NoError(t, err, "the module must answer from the snapshot's manifest")
			require.True(t, manifestLagsLifecycle(rep))
			require.Equal(t, supported.OutcomeOutOfRangeFallback, rep.Constraint.Outcome,
				"the snapshot's manifest must hide 3.16 from the primary resolver")

			useResolvers(t, primary, lifecycle)
			matrix, _, status := generatePythonVersionMatrix(">=3.16")
			assert.Equal(t, matrixOK, status)
			assert.Equal(t, []string{"3.16"}, matrix)
		})
	}
}

// TestCachedManifestOutage checks a manifest that answers 503 with an
// entry already in the runner cache. An expired entry lacking 3.16 is
// served as stale, so the lifecycle resolver answers and `>=3.16`
// stays in range. A fresh entry lacking 3.16 stands: the line still
// waits for an installable build, and the lifecycle resolver is not
// asked.
func TestCachedManifestOutage(t *testing.T) {
	for name, tc := range map[string]struct {
		age        time.Duration
		kind       supported.Kind
		retried    bool
		want       []string
		wantStatus matrixStatus
	}{
		"expired entry": {48 * time.Hour, supported.KindStale, true, []string{"3.16"}, matrixOK},
		"fresh entry":   {0, supported.KindCache, false, snapshotMatrix, matrixNoMatch},
	} {
		t.Run(name, func(t *testing.T) {
			u := supportedtest.NewPythonUpstream(t)
			u.ServeReleases(supportedtest.PythonReleasesNewer)
			u.FailManifest(http.StatusServiceUnavailable)
			s := upstreamSettings(t, u)
			u.CacheManifest(t, supportedtest.PythonManifest, fixedNow().Add(-tc.age))
			primary, err := NewResolver(s)
			require.NoError(t, err)
			t.Cleanup(func() { _ = primary.Close() })
			lifecycle, err := NewLifecycleResolver(s)
			require.NoError(t, err)
			t.Cleanup(func() { _ = lifecycle.Close() })

			rep, err := primary.Matrix(context.Background(), pythonTool, ">=3.16")
			require.NoError(t, err, "the module must answer from the cached manifest")
			for _, src := range rep.Source.Sources {
				if src.Type == supported.SourceActionsManifest {
					require.Equal(t, tc.kind, src.Kind)
				}
			}
			require.Equal(t, supported.OutcomeOutOfRangeFallback, rep.Constraint.Outcome,
				"the cached manifest must hide 3.16 from the primary resolver")
			require.Equal(t, tc.retried, manifestLagsLifecycle(rep))

			calls := 0
			useResolvers(t, primary, countingResolver{next: lifecycle, calls: &calls})
			matrix, _, status := generatePythonVersionMatrix(">=3.16")
			assert.Equal(t, tc.wantStatus, status)
			assert.Equal(t, tc.want, matrix)
			assert.Equal(t, tc.retried, calls == 1)
		})
	}
}

// TestManifestLagsLifecycle checks which source kinds send a report to
// the lifecycle resolver: a fallback manifest paired with fresher
// lifecycle data only.
func TestManifestLagsLifecycle(t *testing.T) {
	earlier := fixedNow().Add(-48 * time.Hour)
	later := fixedNow()
	entry := func(typ string, kind supported.Kind, at *time.Time) supported.SourceEntry {
		return supported.SourceEntry{Type: typ, Kind: kind, FetchedAt: at}
	}
	for name, tc := range map[string]struct {
		manifest, lifecycle supported.SourceEntry
		want                bool
	}{
		"live both": {
			entry(supported.SourceActionsManifest, supported.KindLive, &later),
			entry(supported.SourceEndOfLife, supported.KindLive, &later), false},
		"fresh cached manifest": {
			entry(supported.SourceActionsManifest, supported.KindCache, &earlier),
			entry(supported.SourceEndOfLife, supported.KindLive, &later), false},
		"stale manifest, live lifecycle": {
			entry(supported.SourceActionsManifest, supported.KindStale, &earlier),
			entry(supported.SourceEndOfLife, supported.KindLive, &later), true},
		"stale manifest, cached lifecycle": {
			entry(supported.SourceActionsManifest, supported.KindStale, &earlier),
			entry(supported.SourceEndOfLife, supported.KindCache, &later), true},
		"stale both, newer lifecycle": {
			entry(supported.SourceActionsManifest, supported.KindStale, &earlier),
			entry(supported.SourceEndOfLife, supported.KindStale, &later), true},
		"stale both, older lifecycle": {
			entry(supported.SourceActionsManifest, supported.KindStale, &later),
			entry(supported.SourceEndOfLife, supported.KindStale, &earlier), false},
		"snapshot manifest, stale lifecycle": {
			entry(supported.SourceActionsManifest, supported.KindSnapshot, &earlier),
			entry(supported.SourceEndOfLife, supported.KindStale, &later), true},
		"snapshot both": {
			entry(supported.SourceActionsManifest, supported.KindSnapshot, &earlier),
			entry(supported.SourceEndOfLife, supported.KindSnapshot, &earlier), false},
		"stale manifest, unavailable lifecycle": {
			entry(supported.SourceActionsManifest, supported.KindStale, &earlier),
			entry(supported.SourceEndOfLife, supported.KindUnavailable, nil), false},
	} {
		t.Run(name, func(t *testing.T) {
			var rep supported.Report
			rep.Source.Sources = []supported.SourceEntry{tc.lifecycle, tc.manifest}
			assert.Equal(t, tc.want, manifestLagsLifecycle(rep))
		})
	}
}

// TestSnapshotOnlyReportSkipsLifecycleRetry checks a report read
// wholly from the snapshot, offline or with every upstream down, is
// not retried against the lifecycle resolver, which would read the
// same snapshot.
func TestSnapshotOnlyReportSkipsLifecycleRetry(t *testing.T) {
	for name, settings := range map[string]func(*testing.T) supported.Settings{
		"offline": func(*testing.T) supported.Settings {
			return supported.Settings{Offline: true, NoCache: true, Now: fixedNow}
		},
		"outage": func(t *testing.T) supported.Settings {
			u := supportedtest.NewPythonUpstream(t)
			u.Fail(http.StatusServiceUnavailable)
			return upstreamSettings(t, u)
		},
	} {
		t.Run(name, func(t *testing.T) {
			s := settings(t)
			primary, err := NewResolver(s)
			require.NoError(t, err)
			t.Cleanup(func() { _ = primary.Close() })
			lifecycle, err := NewLifecycleResolver(s)
			require.NoError(t, err)
			t.Cleanup(func() { _ = lifecycle.Close() })
			calls := 0
			useResolvers(t, primary, countingResolver{next: lifecycle, calls: &calls})

			matrix, _, status := generatePythonVersionMatrix(">=3.10")
			assert.Equal(t, matrixOK, status)
			assert.Equal(t, snapshotMatrix, matrix)
			assert.Equal(t, 0, calls, "a snapshot-only report must not reach the lifecycle resolver")
		})
	}
}

// TestEmptyManifestKeepsLifecycleData checks a manifest that parses
// but lists no stable build, so the primary report has an empty matrix
// and no error, does not hide newer lifecycle data: the lifecycle
// resolver answers before the snapshot, and `>=3.16` stays in range.
func TestEmptyManifestKeepsLifecycleData(t *testing.T) {
	for name, manifest := range map[string]string{
		"empty list": `[]`,
		"no stable build": `[
{"version":"3.16.0-rc.1","stable":false,"files":[{"platform":"linux","arch":"x64"}]}
]`,
	} {
		t.Run(name, func(t *testing.T) {
			u := supportedtest.NewPythonUpstream(t)
			u.ServeReleases(supportedtest.PythonReleasesNewer)
			u.ServeManifest(manifest)
			s := upstreamSettings(t, u)
			primary, err := NewResolver(s)
			require.NoError(t, err)
			t.Cleanup(func() { _ = primary.Close() })
			lifecycle, err := NewLifecycleResolver(s)
			require.NoError(t, err)
			t.Cleanup(func() { _ = lifecycle.Close() })

			rep, err := primary.Matrix(context.Background(), pythonTool, ">=3.16")
			require.NoError(t, err, "the module must answer the manifest without an error")
			require.Empty(t, rep.Matrix.Versions, "the manifest must admit no line")
			require.False(t, manifestLagsLifecycle(rep), "the manifest must come from upstream")

			calls := 0
			useResolvers(t, primary, countingResolver{next: lifecycle, calls: &calls})
			matrix, _, status := generatePythonVersionMatrix(">=3.16")
			assert.Equal(t, matrixOK, status)
			assert.Equal(t, []string{"3.16"}, matrix)
			assert.Equal(t, 1, calls)
		})
	}
}

// TestEmptyReportsUseSnapshot checks an empty primary report retried
// against a lifecycle resolver that also yields no matrix falls back
// to the snapshot.
func TestEmptyReportsUseSnapshot(t *testing.T) {
	calls := 0
	useResolvers(t, recordedResolver{}, countingResolver{next: recordedResolver{}, calls: &calls})

	matrix, _, status := generatePythonVersionMatrix(">=3.10")
	assert.Equal(t, matrixOK, status)
	assert.Equal(t, snapshotMatrix, matrix)
	assert.Equal(t, 1, calls, "an empty primary report must reach the lifecycle resolver")
}

// TestBrokenManifestWithoutLifecycleUsesSnapshot checks the snapshot
// answers when the manifest breaks and no lifecycle resolver is set.
func TestBrokenManifestWithoutLifecycleUsesSnapshot(t *testing.T) {
	u := supportedtest.NewPythonUpstream(t)
	u.ServeReleases(supportedtest.PythonReleasesNewer)
	u.FailManifest(http.StatusNotFound)
	useUpstream(t, u, false)

	matrix, _, _ := generatePythonVersionMatrix("")
	assert.Equal(t, snapshotMatrix, matrix)
}

// TestMalformedCachedResponseUsesSnapshot checks a malformed HTTP 200
// endoflife response in the runner cache, read by offline resolvers
// that use the cache, cannot defeat the snapshot fallback.
func TestMalformedCachedResponseUsesSnapshot(t *testing.T) {
	t.Setenv("SUPPORTED_VERSIONS_CACHE_DIR", t.TempDir())
	supportedtest.CacheMalformedPythonReleases(t, fixedNow())
	s := supported.Settings{Offline: true, Now: fixedNow}
	primary, err := NewResolver(s)
	require.NoError(t, err)
	t.Cleanup(func() { _ = primary.Close() })
	lifecycle, err := NewLifecycleResolver(s)
	require.NoError(t, err)
	t.Cleanup(func() { _ = lifecycle.Close() })
	useResolvers(t, primary, lifecycle)

	_, err = primary.Matrix(context.Background(), pythonTool, "")
	require.Error(t, err, "the cached malformed response must reach the primary resolver")
	matrix, _, status := generatePythonVersionMatrix(">=3.10")
	assert.Equal(t, matrixOK, status)
	assert.Equal(t, snapshotMatrix, matrix)
}

// TestEmptyReleaseListUsesSnapshot checks an upstream answering HTTP
// 200 with no release counts as a failure and falls back to the
// snapshot rather than to the declared constraint.
func TestEmptyReleaseListUsesSnapshot(t *testing.T) {
	u := supportedtest.NewPythonUpstream(t)
	u.ServeReleases(supportedtest.PythonReleasesEmpty)
	useUpstream(t, u, true)

	matrix, _, status := generatePythonVersionMatrix(">=3.12")
	assert.Equal(t, matrixOK, status)
	assert.Equal(t, []string{"3.12", "3.13", "3.14", "3.15"}, matrix)
}

// TestResolverFailuresUseSnapshot checks a failing primary resolver
// and a lifecycle resolver with an empty recorded report fall back to
// the snapshot.
func TestResolverFailuresUseSnapshot(t *testing.T) {
	useResolvers(t, failingResolver{}, recordedResolver{})

	matrix, _, status := generatePythonVersionMatrix(">=3.10")
	assert.Equal(t, matrixOK, status)
	assert.Equal(t, snapshotMatrix, matrix)
}

// TestNoReportEmitsNoMatrix checks that when the snapshot fails as
// well, the extractor emits no matrix rather than a guessed one.
func TestNoReportEmitsNoMatrix(t *testing.T) {
	useResolvers(t, failingResolver{}, nil)
	useSnapshot(t, func() (MatrixResolver, error) { return nil, errors.New("no snapshot") })

	matrix, _, _ := generatePythonVersionMatrix(">=3.10")
	assert.Empty(t, matrix)
	ls := extractSetupCfg(t, "[metadata]\nname = none\nversion = 1.0\npython_requires = >=3.10\n")
	assert.NotContains(t, ls, "version_matrix")
	assert.NotContains(t, ls, "build_version")
}
