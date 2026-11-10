// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package rust

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/lfreleng-actions/build-metadata-action/internal/supported"
	"github.com/lfreleng-actions/build-metadata-action/internal/supported/supportedtest"
)

// fixedNow is the clock the matrix tests run at: after 1.99 shipped
// and before 1.100 is due.
func fixedNow() time.Time {
	return time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
}

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
	newSnapshotResolver = func() (MatrixResolver, error) {
		return NewResolver(supported.Settings{Offline: true, NoCache: true, Now: fixedNow})
	}
	t.Cleanup(func() { newSnapshotResolver = saved })
}

// useResolver installs a resolver built from s, with the clock fixed
// unless s sets one and the cache in a fresh directory, and makes the
// snapshot fallback use the fixed clock. Both are restored when the
// test ends.
func useResolver(t *testing.T, s supported.Settings) {
	t.Helper()
	t.Setenv("SUPPORTED_VERSIONS_CACHE_DIR", t.TempDir())
	if s.Now == nil {
		s.Now = fixedNow
	}
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

// useUpstream installs an online resolver reading up, and returns up.
func useUpstream(t *testing.T, up *supportedtest.Upstream) *supportedtest.Upstream {
	t.Helper()
	useResolver(t, supported.Settings{
		Timeout:          200 * time.Millisecond,
		EndOfLifeBaseURL: up.URL,
		RustBaseURL:      up.URL,
	})
	return up
}

// useInjected installs r and a snapshot fallback that answers snapshot
// and snapErr, restoring both when the test ends.
func useInjected(t *testing.T, r MatrixResolver, snapshot MatrixResolver, snapErr error) {
	t.Helper()
	saved := newSnapshotResolver
	newSnapshotResolver = func() (MatrixResolver, error) { return snapshot, snapErr }
	SetResolver(r)
	t.Cleanup(func() {
		SetResolver(nil)
		newSnapshotResolver = saved
	})
}

// failingResolver fails every request, as a broken upstream would.
type failingResolver struct{}

func (failingResolver) Matrix(context.Context, string, string) (supported.Report, error) {
	return supported.Report{}, errors.New("upstream unavailable")
}

// recordedResolver answers every request with one recorded matrix.
type recordedResolver struct {
	versions []string
}

func (r recordedResolver) Matrix(context.Context, string, string) (supported.Report, error) {
	var rep supported.Report
	rep.Matrix.Versions = r.versions
	return rep, nil
}

// matrixJSON runs the extractor on a Cargo.toml declaring msrv and
// returns its matrix_json, the value behind rust_matrix_json, after
// checking rust_version_matrix carries the same versions.
func matrixJSON(t *testing.T, msrv string) string {
	t.Helper()
	got := extractManifest(t, "[package]\nname = \"demo\"\nversion = \"0.1.0\"\n"+
		"edition = \"2021\"\nrust-version = \""+msrv+"\"\n")
	matrix, ok := got["rust_version_matrix"].([]string)
	if !ok {
		t.Fatalf("rust_version_matrix is %T, want []string", got["rust_version_matrix"])
	}
	js, ok := got["matrix_json"].(string)
	if !ok {
		t.Fatalf("matrix_json is %T, want string", got["matrix_json"])
	}
	if want := `{"rust-version": ["` + strings.Join(matrix, `", "`) + `"]}`; js != want {
		t.Errorf("matrix_json = %s, does not match rust_version_matrix %v", js, matrix)
	}
	return js
}

// msrv197Before is rust_matrix_json for an MSRV of 1.97 as the action
// produced it before the module took over, with 1.99 the current
// stable release.
const msrv197Before = `{"rust-version": ["1.97", "1.98", "1.99", "stable"]}`

// TestRustMatrixJSONOffline checks rust_matrix_json from the module's
// embedded snapshot for the MSRV forms the action handles. Each value
// is what the action emitted before the module took over.
func TestRustMatrixJSONOffline(t *testing.T) {
	useResolver(t, supported.Settings{Offline: true, NoCache: true})

	for _, tt := range []struct {
		name, msrv, want string
	}{
		{"MSRV older than every line", "1.80",
			`{"rust-version": ["1.80", "1.94", "1.95", "1.96", "1.97", "1.98", "1.99", "stable"]}`},
		{"MSRV within the lines", "1.97", msrv197Before},
		{"patch-level MSRV appears once", "1.97.0",
			`{"rust-version": ["1.97.0", "1.98", "1.99", "stable"]}`},
		{"MSRV at the newest line", "1.99", `{"rust-version": ["1.99", "stable"]}`},
		{"MSRV newer than stable", "1.120", `{"rust-version": ["1.120", "stable"]}`},
		{"malformed MSRV", "latest",
			`{"rust-version": ["1.99", "1.98", "1.97", "1.96", "1.95", "1.94", "stable"]}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := matrixJSON(t, tt.msrv); got != tt.want {
				t.Errorf("matrix_json = %s, want %s", got, tt.want)
			}
		})
	}
}

// TestRustMatrixJSONRecordedLive checks rust_matrix_json from recorded
// lifecycle and channel data served over HTTP.
func TestRustMatrixJSONRecordedLive(t *testing.T) {
	up := useUpstream(t, supportedtest.NewRustUpstream(t))
	if got := matrixJSON(t, "1.97"); got != msrv197Before {
		t.Errorf("matrix_json = %s, want %s", got, msrv197Before)
	}
	if up.Requests() == 0 {
		t.Error("resolver made no request to the upstream")
	}
}

// TestRustMatrixJSONChannelAhead checks that a stable release
// endoflife.date does not list yet joins no numeric line: stable
// covers it. The action before the module put 1.100 in the matrix as
// soon as the channel served it.
func TestRustMatrixJSONChannelAhead(t *testing.T) {
	up := supportedtest.NewRustUpstream(t)
	up.ServeManifest(supportedtest.RustChannelNewer)
	useUpstream(t, up)
	if got := matrixJSON(t, "1.97"); got != msrv197Before {
		t.Errorf("matrix_json = %s, want %s", got, msrv197Before)
	}
}

// TestRustMatrixJSONNewLine checks that, once 1.100 has shipped, a
// line both sources know of joins the matrix after 1.99, in numeric
// order, and the oldest line drops out of the six.
func TestRustMatrixJSONNewLine(t *testing.T) {
	up := supportedtest.NewRustUpstream(t)
	up.ServeReleases(supportedtest.RustReleasesNewer)
	up.ServeManifest(supportedtest.RustChannelNewer)
	useResolver(t, supported.Settings{
		Timeout:          200 * time.Millisecond,
		EndOfLifeBaseURL: up.URL,
		RustBaseURL:      up.URL,
		Now:              func() time.Time { return time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC) },
	})
	const want = `{"rust-version": ["1.80", "1.95", "1.96", "1.97", "1.98", "1.99", "1.100", "stable"]}`
	if got := matrixJSON(t, "1.80"); got != want {
		t.Errorf("matrix_json = %s, want %s", got, want)
	}
}

// TestRustMatrixJSONDegradedUpstream checks rust_matrix_json when the
// upstream fails in whole or in part: the module serves the snapshot
// for the failed sources, and the matrix keeps its shape.
func TestRustMatrixJSONDegradedUpstream(t *testing.T) {
	for _, tt := range []struct {
		name    string
		breakIt func(*supportedtest.Upstream)
	}{
		{"outage", func(u *supportedtest.Upstream) { u.Fail(http.StatusServiceUnavailable) }},
		{"channel 503", func(u *supportedtest.Upstream) { u.FailManifest(http.StatusServiceUnavailable) }},
		{"channel timeout", func(u *supportedtest.Upstream) { u.StallManifest(5 * time.Second) }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			up := supportedtest.NewRustUpstream(t)
			tt.breakIt(up)
			useUpstream(t, up)
			if got := matrixJSON(t, "1.97"); got != msrv197Before {
				t.Errorf("matrix_json = %s, want %s", got, msrv197Before)
			}
		})
	}
}

// TestRustMatrixJSONUnusableSource checks rust_matrix_json when an
// upstream answers HTTP 200 with data the module cannot use: the
// extractor warns and retries from the embedded snapshot.
func TestRustMatrixJSONUnusableSource(t *testing.T) {
	for _, tt := range []struct {
		name    string
		breakIt func(*supportedtest.Upstream)
	}{
		{"malformed lifecycle", func(u *supportedtest.Upstream) { u.ServeReleases("<html>not json</html>") }},
		{"empty lifecycle", func(u *supportedtest.Upstream) { u.ServeReleases(supportedtest.RustReleasesEmpty) }},
		{"malformed channel", func(u *supportedtest.Upstream) { u.ServeManifest("[pkg.rust\nversion = 1") }},
		{"channel without a release", func(u *supportedtest.Upstream) {
			u.ServeManifest("date = \"2026-10-15\"\n[pkg.rust]\nversion = \"unknown\"\n")
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			warned := captureWarnings(t)
			up := supportedtest.NewRustUpstream(t)
			tt.breakIt(up)
			useUpstream(t, up)
			if got := matrixJSON(t, "1.97"); got != msrv197Before {
				t.Errorf("matrix_json = %s, want %s", got, msrv197Before)
			}
			if !strings.Contains(warned.String(), "retrying from the embedded snapshot") {
				t.Errorf("warnings = %q, want a snapshot retry", warned.String())
			}
		})
	}
}

// TestRustMatrixJSONMalformedCachedResponse checks rust_matrix_json
// when the runner cache holds malformed HTTP 200 responses under the
// default upstream URLs: the fallback bypasses the cache and reaches
// the embedded snapshot.
func TestRustMatrixJSONMalformedCachedResponse(t *testing.T) {
	for name, url := range map[string]string{
		"lifecycle": supportedtest.DefaultRustReleasesURL,
		"channel":   supportedtest.DefaultRustChannelURL,
	} {
		t.Run(name, func(t *testing.T) {
			warned := captureWarnings(t)
			useResolver(t, supported.Settings{Offline: true})
			supportedtest.CacheResponse(t, url, "<html>not json</html>", fixedNow())
			if got := matrixJSON(t, "1.97"); got != msrv197Before {
				t.Errorf("matrix_json = %s, want %s", got, msrv197Before)
			}
			if !strings.Contains(warned.String(), "retrying from the embedded snapshot") {
				t.Errorf("warnings = %q, want a snapshot retry", warned.String())
			}
		})
	}
}

// TestGenerateRustVersionMatrixInjected exercises the fallbacks with
// injected resolvers.
func TestGenerateRustVersionMatrixInjected(t *testing.T) {
	recorded := recordedResolver{versions: []string{"1.98", "1.99", "stable"}}
	labelsOnly := recordedResolver{versions: []string{"stable"}}
	snapErr := errors.New("no snapshot")

	for _, tt := range []struct {
		name     string
		active   MatrixResolver
		snapshot MatrixResolver
		snapErr  error
		msrv     string
		want     []string
	}{
		{"installed report", recorded, nil, snapErr, "1.97", []string{"1.97", "1.98", "1.99", "stable"}},
		{"failure retries the snapshot", failingResolver{}, recorded, nil, "1.99", []string{"1.99", "stable"}},
		{"no line retries the snapshot", labelsOnly, recorded, nil, "1.80", []string{"1.80", "1.98", "1.99", "stable"}},
		{"none installed uses the snapshot", nil, recorded, nil, "1.98.2", []string{"1.98.2", "1.99", "stable"}},
		{"snapshot fails", failingResolver{}, nil, snapErr, "1.97", []string{"1.97", "stable"}},
		{"snapshot fails to answer", failingResolver{}, failingResolver{}, nil, "1.97", []string{"1.97", "stable"}},
		{"snapshot has no line", nil, labelsOnly, nil, "1.97", []string{"1.97", "stable"}},
		{"malformed MSRV, nothing resolves", nil, nil, snapErr, "latest", []string{"stable"}},
		{"malformed MSRV lists lines newest first", recorded, nil, snapErr, "1.x", []string{"1.99", "1.98", "stable"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			captureWarnings(t)
			useInjected(t, tt.active, tt.snapshot, tt.snapErr)
			if got := generateRustVersionMatrix(tt.msrv); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("generateRustVersionMatrix(%q) = %v, want %v", tt.msrv, got, tt.want)
			}
		})
	}
}

// Without an MSRV the matrix starts at the first release supporting the
// edition; edition 2024 was stabilised in 1.85.
func TestRustVersionMatrixFromEachEdition(t *testing.T) {
	SetResolver(failingResolver{})
	t.Cleanup(func() { SetResolver(nil) })
	tests := map[string]string{
		"2024": `{"rust-version": ["1.85", "stable"]}`,
		"2021": `{"rust-version": ["1.56", "stable"]}`,
		"2018": `{"rust-version": ["1.31", "stable"]}`,
		"2015": `{"rust-version": ["1.0", "stable"]}`,
		"2030": `{"rust-version": ["stable"]}`,
	}
	for edition, want := range tests {
		t.Run(edition, func(t *testing.T) {
			got := extractManifest(t, "[package]\nname = \"demo\"\nversion = \"0.1.0\"\nedition = \""+edition+"\"\n")
			if got["matrix_json"] != want {
				t.Errorf("matrix_json = %v, want %s", got["matrix_json"], want)
			}
		})
	}
}
