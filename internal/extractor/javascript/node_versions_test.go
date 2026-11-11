// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package javascript

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/lfreleng-actions/build-metadata-action/internal/supported"
	"github.com/lfreleng-actions/build-metadata-action/internal/supported/supportedtest"
)

// fixedNow is the clock the matrix tests run at: Node.js 22, 24 and 26
// are the supported LTS lines and 20 has reached end-of-life.
func fixedNow() time.Time {
	return time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
}

// snapshotNodeMatrix is node_matrix_json with no constraint, from the
// recorded upstream data and the embedded snapshot alike.
const snapshotNodeMatrix = `{"node-version": ["22", "24", "26"]}`

// captureWarnings collects the warnings matrix resolution emits until
// the test ends.
func captureWarnings(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	saved := warnings
	warnings = &buf
	t.Cleanup(func() { warnings = saved })
	return &buf
}

// useSnapshotFallback makes the snapshot fallback run at the fixed
// clock until the test ends.
func useSnapshotFallback(t *testing.T) {
	t.Helper()
	saved := newSnapshotResolver
	newSnapshotResolver = func() (supported.ProjectMatrixResolver, error) {
		return NewResolver(supported.Settings{Offline: true, NoCache: true, Now: fixedNow})
	}
	t.Cleanup(func() { newSnapshotResolver = saved })
}

// useResolver installs a resolver built from s, with the clock fixed
// and the cache in a fresh directory, and makes the snapshot fallback
// use the same clock. Both are restored when the test ends.
func useResolver(t *testing.T, s supported.Settings) {
	t.Helper()
	t.Setenv("SUPPORTED_VERSIONS_CACHE_DIR", t.TempDir())
	s.Now = fixedNow
	r, err := NewResolver(s)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	useSnapshotFallback(t)
	SetResolver(r)
	t.Cleanup(func() {
		SetResolver(nil)
		_ = r.Close()
	})
}

// useFakeResolver installs r and the fixed-clock snapshot fallback.
func useFakeResolver(t *testing.T, r supported.ProjectMatrixResolver) {
	t.Helper()
	t.Setenv("SUPPORTED_VERSIONS_CACHE_DIR", t.TempDir())
	useSnapshotFallback(t)
	SetResolver(r)
	t.Cleanup(func() { SetResolver(nil) })
}

// failingResolver fails every request, as a broken upstream would.
type failingResolver struct{}

func (failingResolver) MatrixFor(context.Context, string, supported.MatrixRequest) (supported.Report, error) {
	return supported.Report{}, errors.New("upstream unavailable")
}

// emptyResolver answers every request with a report holding no line.
type emptyResolver struct{}

func (emptyResolver) MatrixFor(context.Context, string, supported.MatrixRequest) (supported.Report, error) {
	return supported.Report{}, nil
}

// writeNodeProject creates a project directory holding packageJSON as
// package.json and the extra files.
func writeNodeProject(t *testing.T, packageJSON string, extra map[string]string) string {
	t.Helper()
	files := map[string]string{"package.json": packageJSON}
	for name, body := range extra {
		files[name] = body
	}
	return writeProject(t, files)
}

// extractNode runs the extractor on dir and returns node_matrix_json
// and node_latest; a key left unset reads as "".
func extractNode(t *testing.T, dir string) (matrix, latest string) {
	t.Helper()
	metadata, err := NewExtractor().Extract(dir)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	matrix, _ = metadata.LanguageSpecific["node_matrix_json"].(string)
	latest, _ = metadata.LanguageSpecific["node_latest"].(string)
	return matrix, latest
}

// newestLine returns the last line in a node_matrix_json value.
func newestLine(matrixJSON string) string {
	trimmed := strings.TrimSuffix(matrixJSON, `"]}`)
	return trimmed[strings.LastIndex(trimmed, `"`)+1:]
}

// TestNodeMatrixConstraintSources checks node_matrix_json from the
// recorded upstream data for each constraint source. The comment on
// each case names the source the report origin shows won.
func TestNodeMatrixConstraintSources(t *testing.T) {
	tests := []struct {
		name        string
		packageJSON string
		files       map[string]string
		want        string
		wantWarning string
	}{
		// engines.node, an explicit constraint.
		{"engines.node >=24", `{"name":"a","engines":{"node":">=24"}}`, nil,
			`{"node-version": ["24", "26"]}`, ""},
		// engines.node, an explicit constraint.
		{"engines.node ^20 || ^22", `{"name":"a","engines":{"node":"^20 || ^22"}}`, nil,
			`{"node-version": ["22"]}`, ""},
		// No constraint source at all.
		{"no engines.node", `{"name":"a"}`, nil, snapshotNodeMatrix, ""},
		// No constraint source: engines lists npm alone.
		{"engines without node", `{"name":"a","engines":{"npm":">=10"}}`, nil, snapshotNodeMatrix, ""},
		// engines.node does not parse: no constraint applies.
		{"unparsable engines.node", `{"name":"a","engines":{"node":"garbage"}}`, nil,
			snapshotNodeMatrix, "Could not parse the Node.js version constraint"},
		// engines.node, widened by the module as it admits no line.
		{"out-of-range engines.node", `{"name":"a","engines":{"node":">=40"}}`, nil,
			snapshotNodeMatrix, `constraint ">=40" allows no line of the matrix`},
		// .nvmrc, detected by the module.
		{".nvmrc", `{"name":"a"}`, map[string]string{".nvmrc": "24\n"},
			`{"node-version": ["24"]}`, ""},
		// .node-version, detected by the module.
		{".node-version", `{"name":"a"}`, map[string]string{".node-version": "v22.3.0\n"},
			`{"node-version": ["22"]}`, ""},
		// engines.node wins over .nvmrc.
		{"engines.node overrides .nvmrc", `{"name":"a","engines":{"node":">=24"}}`,
			map[string]string{".nvmrc": "22\n"}, `{"node-version": ["24", "26"]}`, ""},
		// engines.node, in a TypeScript project.
		{"TypeScript", `{"name":"a","engines":{"node":">=24"},"devDependencies":{"typescript":"^5.6.0"}}`,
			map[string]string{"tsconfig.json": "{}"}, `{"node-version": ["24", "26"]}`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warned := captureWarnings(t)
			up := supportedtest.NewNodeUpstream(t)
			useResolver(t, supported.Settings{Timeout: time.Second, EndOfLifeBaseURL: up.URL, ManifestBaseURL: up.URL})
			matrix, latest := extractNode(t, writeNodeProject(t, tt.packageJSON, tt.files))
			if matrix != tt.want {
				t.Errorf("node_matrix_json = %s, want %s", matrix, tt.want)
			}
			if want := newestLine(tt.want); latest != want {
				t.Errorf("node_latest = %q, want %q", latest, want)
			}
			if tt.wantWarning == "" && warned.Len() != 0 {
				t.Errorf("warnings = %q, want none", warned.String())
			}
			if tt.wantWarning != "" && !strings.Contains(warned.String(), tt.wantWarning) {
				t.Errorf("warnings = %q, want %q", warned.String(), tt.wantWarning)
			}
			if up.Requests() == 0 {
				t.Error("resolver made no request to the upstream")
			}
		})
	}
}

// TestNodeLatestFollowsTheMatrix checks that node_latest is the newest
// line node_matrix_json holds, not a patch version.
func TestNodeLatestFollowsTheMatrix(t *testing.T) {
	captureWarnings(t)
	up := supportedtest.NewNodeUpstream(t)
	useResolver(t, supported.Settings{Timeout: time.Second, EndOfLifeBaseURL: up.URL, ManifestBaseURL: up.URL})

	for packageJSON, want := range map[string]string{
		`{"name":"a","engines":{"node":">=22"}}`:       "26",
		`{"name":"a","engines":{"node":"^20 || ^22"}}`: "22",
	} {
		if _, latest := extractNode(t, writeNodeProject(t, packageJSON, nil)); latest != want {
			t.Errorf("%s: node_latest = %q, want %q", packageJSON, latest, want)
		}
	}
}

// TestNodeMatrixNewerUpstream checks that a line newer than the
// embedded snapshot reaches node_matrix_json from live data.
func TestNodeMatrixNewerUpstream(t *testing.T) {
	captureWarnings(t)
	up := supportedtest.NewNodeUpstream(t)
	up.ServeReleases(supportedtest.NodeReleasesNewer)
	up.ServeManifest(supportedtest.NodeManifestNewer)
	useResolver(t, supported.Settings{Timeout: time.Second, EndOfLifeBaseURL: up.URL, ManifestBaseURL: up.URL})

	matrix, latest := extractNode(t, writeNodeProject(t, `{"name":"a","engines":{"node":">=24"}}`, nil))
	if want := `{"node-version": ["24", "26", "28"]}`; matrix != want {
		t.Errorf("node_matrix_json = %s, want %s", matrix, want)
	}
	if latest != "28" {
		t.Errorf("node_latest = %q, want 28", latest)
	}
}

// TestNodeMatrixOfflineIgnoresCache checks that offline mode, as the
// action configures it, reads the embedded snapshot even when the
// runner cache holds newer release facts.
func TestNodeMatrixOfflineIgnoresCache(t *testing.T) {
	captureWarnings(t)
	cacheDir := t.TempDir()
	t.Setenv("SUPPORTED_VERSIONS_CACHE_DIR", cacheDir)
	fetched := fixedNow().Add(-time.Hour)
	supportedtest.CacheResponse(t, supportedtest.DefaultNodeReleasesURL, supportedtest.NodeReleasesNewer, fetched)
	supportedtest.CacheResponse(t, supportedtest.DefaultNodeManifestURL, supportedtest.NodeManifestNewer, fetched)

	dir := writeNodeProject(t, `{"name":"a","engines":{"node":">=24"}}`, nil)

	// A resolver that reads the cache sees the newer line 28.
	cached, err := NewResolver(supported.Settings{Offline: true, Now: fixedNow})
	if err != nil {
		t.Fatal(err)
	}
	rep, err := cached.MatrixFor(context.Background(), nodeTool, supported.MatrixRequest{Constraint: ">=24"})
	_ = cached.Close()
	if err != nil {
		t.Fatal(err)
	}
	if rep.Latest != "28" {
		t.Fatalf("cache-reading resolver latest = %q, want 28: the seeded cache was not read", rep.Latest)
	}

	r, err := NewResolver(supported.Settings{Offline: true, NoCache: true, Now: fixedNow})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	useSnapshotFallback(t)
	SetResolver(r)
	t.Cleanup(func() { SetResolver(nil) })

	matrix, latest := extractNode(t, dir)
	if want := `{"node-version": ["24", "26"]}`; matrix != want {
		t.Errorf("node_matrix_json = %s, want %s", matrix, want)
	}
	if latest != "26" {
		t.Errorf("node_latest = %q, want 26", latest)
	}
}

// TestNodeMatrixRecoversFromTheSnapshot checks that a resolver error
// or an empty report falls back to the embedded snapshot.
func TestNodeMatrixRecoversFromTheSnapshot(t *testing.T) {
	for name, r := range map[string]supported.ProjectMatrixResolver{
		"resolver error": failingResolver{},
		"empty report":   emptyResolver{},
	} {
		t.Run(name, func(t *testing.T) {
			warned := captureWarnings(t)
			useFakeResolver(t, r)
			matrix, latest := extractNode(t, writeNodeProject(t, `{"name":"a","engines":{"node":">=24"}}`, nil))
			if want := `{"node-version": ["24", "26"]}`; matrix != want {
				t.Errorf("node_matrix_json = %s, want %s", matrix, want)
			}
			if latest != "26" {
				t.Errorf("node_latest = %q, want 26", latest)
			}
			if !strings.Contains(warned.String(), "retrying from the embedded snapshot") {
				t.Errorf("warnings = %q, want a snapshot retry", warned.String())
			}
		})
	}
}

// TestNodeMatrixUnsetWhenNothingResolves checks that extraction
// succeeds, with the new keys unset and a warning, when the snapshot
// fails too.
func TestNodeMatrixUnsetWhenNothingResolves(t *testing.T) {
	warned := captureWarnings(t)
	SetResolver(failingResolver{})
	saved := newSnapshotResolver
	newSnapshotResolver = func() (supported.ProjectMatrixResolver, error) { return emptyResolver{}, nil }
	t.Cleanup(func() {
		SetResolver(nil)
		newSnapshotResolver = saved
	})

	metadata, err := NewExtractor().Extract(writeNodeProject(t, `{"name":"a","engines":{"node":">=24"}}`, nil))
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	for _, key := range []string{"node_matrix_json", "node_latest"} {
		if v, ok := metadata.LanguageSpecific[key]; ok {
			t.Errorf("%s = %v, want unset", key, v)
		}
	}
	if got := metadata.LanguageSpecific["requires_node"]; got != ">=24" {
		t.Errorf("requires_node = %v, want >=24", got)
	}
	if !strings.Contains(warned.String(), "leaving node_matrix_json and node_latest unset") {
		t.Errorf("warnings = %q, want the unset warning", warned.String())
	}
}
