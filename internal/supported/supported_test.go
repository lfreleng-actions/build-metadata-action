// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package supported

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/lfreleng-actions/build-metadata-action/internal/supported/supportedtest"
	"github.com/lfreleng-actions/supported-versions-action/pkg/catalog"
)

// fixedNow is the clock every test runs at: after Go 1.25 reached
// end-of-life and while 1.26 and 1.27 are supported.
func fixedNow() time.Time {
	return time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
}

// isolate points the resolver's cache at a fresh directory and
// captures the warnings it emits.
func isolate(t *testing.T) *bytes.Buffer {
	t.Helper()
	t.Setenv("SUPPORTED_VERSIONS_CACHE_DIR", t.TempDir())
	var buf bytes.Buffer
	saved := warnings
	warnings = &buf
	t.Cleanup(func() { warnings = saved })
	return &buf
}

func newResolver(t *testing.T, s Settings) *Resolver {
	t.Helper()
	if s.Now == nil {
		s.Now = fixedNow
	}
	r, err := New(s)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r
}

func TestMatrixOffline(t *testing.T) {
	warned := isolate(t)
	r := newResolver(t, Settings{Offline: true})

	tests := []struct {
		constraint string
		want       []string
	}{
		{"", []string{"1.26", "1.27"}},
		{"1.24", []string{"1.26", "1.27"}},
		{"1.26", []string{"1.26", "1.27"}},
		{"1.26.3", []string{"1.26", "1.27"}},
		{"1.27", []string{"1.27"}},
	}
	for _, tt := range tests {
		t.Run(tt.constraint, func(t *testing.T) {
			rep, err := r.Matrix(context.Background(), "go", tt.constraint)
			if err != nil {
				t.Fatalf("Matrix: %v", err)
			}
			if rep.Matrix.Key != "go-version" {
				t.Errorf("matrix key = %q, want go-version", rep.Matrix.Key)
			}
			if !reflect.DeepEqual(rep.Matrix.Versions, tt.want) {
				t.Errorf("matrix = %v, want %v", rep.Matrix.Versions, tt.want)
			}
		})
	}
	if warned.Len() != 0 {
		t.Errorf("offline resolution warned: %q", warned.String())
	}
}

func TestMatrixAlias(t *testing.T) {
	isolate(t)
	r := newResolver(t, Settings{Offline: true})
	rep, err := r.Matrix(context.Background(), "golang", "")
	if err != nil {
		t.Fatalf("Matrix: %v", err)
	}
	if rep.Tool != "go" {
		t.Errorf("tool = %q, want go", rep.Tool)
	}
}

func TestMatrixConstraintNewerThanEveryLine(t *testing.T) {
	isolate(t)
	r := newResolver(t, Settings{Offline: true})
	rep, err := r.Matrix(context.Background(), "go", "1.30")
	if err != nil {
		t.Fatalf("Matrix: %v", err)
	}
	if rep.Constraint == nil || rep.Constraint.Outcome != OutcomeOutOfRangeFallback {
		t.Errorf("constraint = %+v, want outcome %q", rep.Constraint, OutcomeOutOfRangeFallback)
	}
}

func TestMatrixInvalidConstraint(t *testing.T) {
	isolate(t)
	r := newResolver(t, Settings{Offline: true})
	if _, err := r.Matrix(context.Background(), "go", "not-a-version"); err == nil {
		t.Error("Matrix accepted an invalid constraint")
	}
}

func TestMatrixUnknownTool(t *testing.T) {
	isolate(t)
	r := newResolver(t, Settings{Offline: true})
	if _, err := r.Matrix(context.Background(), "no-such-tool", ""); err == nil {
		t.Error("Matrix accepted an unknown tool")
	}
}

func TestMatrixRecordedLive(t *testing.T) {
	warned := isolate(t)
	up := supportedtest.NewGoUpstream(t)
	r := newResolver(t, Settings{
		Timeout:          time.Second,
		EndOfLifeBaseURL: up.URL,
		ManifestBaseURL:  up.URL,
	})

	rep, err := r.Matrix(context.Background(), "go", "1.24")
	if err != nil {
		t.Fatalf("Matrix: %v", err)
	}
	if want := []string{"1.26", "1.27"}; !reflect.DeepEqual(rep.Matrix.Versions, want) {
		t.Errorf("matrix = %v, want %v", rep.Matrix.Versions, want)
	}
	if rep.Source.Kind != "live" {
		t.Errorf("source kind = %q, want live", rep.Source.Kind)
	}
	if up.Requests() == 0 {
		t.Error("resolver made no request to the upstream")
	}
	if warned.Len() != 0 {
		t.Errorf("live resolution warned: %q", warned.String())
	}
}

func TestMatrixOfflineMakesNoRequest(t *testing.T) {
	isolate(t)
	up := supportedtest.NewGoUpstream(t)
	r := newResolver(t, Settings{
		Offline:          true,
		EndOfLifeBaseURL: up.URL,
		ManifestBaseURL:  up.URL,
	})
	if _, err := r.Matrix(context.Background(), "go", ""); err != nil {
		t.Fatalf("Matrix: %v", err)
	}
	if n := up.Requests(); n != 0 {
		t.Errorf("offline resolver made %d requests", n)
	}
}

func TestMatrixUpstreamDownFallsBackToSnapshot(t *testing.T) {
	warned := isolate(t)
	up := supportedtest.NewGoUpstream(t)
	up.Fail(http.StatusServiceUnavailable)
	r := newResolver(t, Settings{
		Timeout:          time.Second,
		EndOfLifeBaseURL: up.URL,
		ManifestBaseURL:  up.URL,
	})

	rep, err := r.Matrix(context.Background(), "go", "")
	if err != nil {
		t.Fatalf("Matrix: %v", err)
	}
	if want := []string{"1.26", "1.27"}; !reflect.DeepEqual(rep.Matrix.Versions, want) {
		t.Errorf("matrix = %v, want %v", rep.Matrix.Versions, want)
	}
	if rep.Source.Kind != "snapshot" {
		t.Errorf("source kind = %q, want snapshot", rep.Source.Kind)
	}
	if !strings.Contains(warned.String(), "Could not fetch live Go release data") {
		t.Errorf("warnings = %q, want a live-fetch warning", warned.String())
	}
}

// TestMatrixBrokenManifestFailsReport records the module behaviour
// CatalogWithoutSource exists for: a manifest answering 404 or a body
// that does not parse fails the whole report, even when endoflife.date
// answered.
func TestMatrixBrokenManifestFailsReport(t *testing.T) {
	for _, tt := range []struct {
		name    string
		breakIt func(*supportedtest.Upstream)
	}{
		{"not found", func(u *supportedtest.Upstream) { u.FailManifest(http.StatusNotFound) }},
		{"malformed", func(u *supportedtest.Upstream) { u.ServeManifest("<html>not json</html>") }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			isolate(t)
			up := supportedtest.NewGoUpstream(t)
			tt.breakIt(up)
			r := newResolver(t, Settings{
				Timeout:          time.Second,
				EndOfLifeBaseURL: up.URL,
				ManifestBaseURL:  up.URL,
			})
			if _, err := r.Matrix(context.Background(), "go", ""); err == nil {
				t.Error("Matrix succeeded with a broken manifest")
			}
		})
	}
}

// TestCatalogWithoutSourceIgnoresBrokenManifest verifies a catalog
// without Go's manifest source resolves from endoflife.date alone, so a
// broken manifest leaves the live lifecycle data in force.
func TestCatalogWithoutSourceIgnoresBrokenManifest(t *testing.T) {
	isolate(t)
	cat, err := CatalogWithoutSource("go", SourceActionsManifest)
	if err != nil {
		t.Fatalf("CatalogWithoutSource: %v", err)
	}
	up := supportedtest.NewGoUpstream(t)
	up.ServeReleases(supportedtest.GoReleasesNewer)
	up.FailManifest(http.StatusNotFound)
	r := newResolver(t, Settings{
		Catalog:          cat,
		Timeout:          time.Second,
		EndOfLifeBaseURL: up.URL,
		ManifestBaseURL:  up.URL,
	})

	rep, err := r.Matrix(context.Background(), "go", "")
	if err != nil {
		t.Fatalf("Matrix: %v", err)
	}
	if want := []string{"1.27", "1.28"}; !reflect.DeepEqual(rep.Matrix.Versions, want) {
		t.Errorf("matrix = %v, want %v", rep.Matrix.Versions, want)
	}
	if rep.Source.Kind != "live" {
		t.Errorf("source kind = %q, want live", rep.Source.Kind)
	}
	for _, s := range rep.Source.Sources {
		if s.Type == SourceActionsManifest {
			t.Errorf("report lists source %+v, want no manifest source", s)
		}
	}
}

// TestCatalogWithoutSourceLeavesEmbeddedCatalog verifies removing a
// source does not reach later catalogs, and that an unknown tool is an
// error.
func TestCatalogWithoutSourceLeavesEmbeddedCatalog(t *testing.T) {
	if _, err := CatalogWithoutSource("go", SourceActionsManifest); err != nil {
		t.Fatalf("CatalogWithoutSource: %v", err)
	}
	cat, err := CatalogWithoutSource("nodejs", SourceActionsManifest)
	if err != nil {
		t.Fatalf("CatalogWithoutSource: %v", err)
	}
	if !slices.ContainsFunc(cat.Tools["go"].Sources, func(s catalog.Source) bool {
		return s.Type == SourceActionsManifest
	}) {
		t.Error("go lost its manifest source in a later catalog")
	}
	if _, err := CatalogWithoutSource("no-such-tool", SourceActionsManifest); err == nil {
		t.Error("CatalogWithoutSource accepted an unknown tool")
	}
}

func TestNewCacheFailsToOpen(t *testing.T) {
	warned := isolate(t)
	// A regular file where the cache directory should be cannot open.
	notDir := filepath.Join(t.TempDir(), "cache")
	if err := os.WriteFile(notDir, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SUPPORTED_VERSIONS_CACHE_DIR", notDir)

	r := newResolver(t, Settings{Offline: true})
	if r.cache != nil {
		t.Error("resolver kept a cache that failed to open")
	}
	if !strings.Contains(warned.String(), "Cannot open the supported-versions cache") {
		t.Errorf("warnings = %q, want a cache warning", warned.String())
	}
	rep, err := r.Matrix(context.Background(), "go", "")
	if err != nil {
		t.Fatalf("Matrix without a cache: %v", err)
	}
	if want := []string{"1.26", "1.27"}; !reflect.DeepEqual(rep.Matrix.Versions, want) {
		t.Errorf("matrix = %v, want %v", rep.Matrix.Versions, want)
	}
}

// TestMatrixEmptyUpstreamReleases records that an upstream answering
// with no release yields an empty matrix and no error, so callers must
// treat an empty report as a fallback condition themselves.
func TestMatrixEmptyUpstreamReleases(t *testing.T) {
	isolate(t)
	up := supportedtest.NewGoUpstream(t)
	up.ServeReleases(supportedtest.GoReleasesEmpty)
	r := newResolver(t, Settings{
		Timeout:          time.Second,
		EndOfLifeBaseURL: up.URL,
		ManifestBaseURL:  up.URL,
	})
	rep, err := r.Matrix(context.Background(), "go", "1.24")
	if err != nil {
		t.Fatalf("Matrix: %v", err)
	}
	if len(rep.Matrix.Versions) != 0 {
		t.Errorf("matrix = %v, want empty", rep.Matrix.Versions)
	}
}

func TestNoCacheIgnoresMalformedCachedResponse(t *testing.T) {
	warned := isolate(t)
	supportedtest.CacheMalformedGoReleases(t, fixedNow())

	cached := newResolver(t, Settings{Offline: true})
	if _, err := cached.Matrix(context.Background(), "go", ""); err == nil {
		t.Fatal("Matrix over a malformed cached response succeeded; the seed did not bite")
	}

	r := newResolver(t, Settings{Offline: true, NoCache: true})
	if r.cache != nil {
		t.Error("NoCache resolver opened a cache")
	}
	rep, err := r.Matrix(context.Background(), "go", "")
	if err != nil {
		t.Fatalf("Matrix with NoCache: %v", err)
	}
	if want := []string{"1.26", "1.27"}; !reflect.DeepEqual(rep.Matrix.Versions, want) {
		t.Errorf("matrix = %v, want %v", rep.Matrix.Versions, want)
	}
	if rep.Source.Kind != "snapshot" {
		t.Errorf("source kind = %q, want snapshot", rep.Source.Kind)
	}
	if warned.Len() != 0 {
		t.Errorf("NoCache resolution warned: %q", warned.String())
	}
}

func TestNewCachesResponses(t *testing.T) {
	isolate(t)
	up := supportedtest.NewGoUpstream(t)
	s := Settings{Timeout: time.Second, EndOfLifeBaseURL: up.URL, ManifestBaseURL: up.URL}

	first := newResolver(t, s)
	if _, err := first.Matrix(context.Background(), "go", ""); err != nil {
		t.Fatalf("Matrix: %v", err)
	}
	fetched := up.Requests()

	second := newResolver(t, s)
	if _, err := second.Matrix(context.Background(), "go", ""); err != nil {
		t.Fatalf("Matrix: %v", err)
	}
	if up.Requests() != fetched {
		t.Errorf("second resolver fetched again (%d requests, want %d)", up.Requests(), fetched)
	}
}

func TestRetriesZeroDisablesRetries(t *testing.T) {
	isolate(t)
	up := supportedtest.NewGoUpstream(t)
	r := newResolver(t, Settings{
		Timeout:          time.Second,
		Retries:          0,
		EndOfLifeBaseURL: up.URL,
		ManifestBaseURL:  up.URL,
	})
	up.Fail(http.StatusServiceUnavailable)
	if _, err := r.Matrix(context.Background(), "go", ""); err != nil {
		t.Fatalf("Matrix: %v", err)
	}
	// One attempt per source: endoflife.date and the manifest.
	if n := up.Requests(); n != 2 {
		t.Errorf("upstream saw %d requests, want 2", n)
	}
}
