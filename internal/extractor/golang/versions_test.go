// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package golang

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/lfreleng-actions/build-metadata-action/internal/supported"
	"github.com/lfreleng-actions/build-metadata-action/internal/supported/supportedtest"
	"github.com/lfreleng-actions/supported-versions-action/pkg/catalog"
)

// fixedNow is the clock the matrix tests run at: after Go 1.25 reached
// end-of-life and while 1.26 and 1.27 are supported.
func fixedNow() time.Time {
	return time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
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

// useOfflineResolver installs an offline resolver over the module's
// embedded snapshot.
func useOfflineResolver(t *testing.T) {
	t.Helper()
	useResolver(t, supported.Settings{Offline: true})
}

// failingResolver fails every request, as a broken upstream would.
type failingResolver struct{}

func (failingResolver) Matrix(context.Context, string, string) (supported.Report, error) {
	return supported.Report{}, errors.New("upstream unavailable")
}

// recordedResolver answers every request with one recorded report.
type recordedResolver struct {
	report supported.Report
}

func (r recordedResolver) Matrix(context.Context, string, string) (supported.Report, error) {
	return r.report, nil
}

// TestGenerateGoVersionMatrix exercises the matrix against the
// module's embedded snapshot at a fixed date.
func TestGenerateGoVersionMatrix(t *testing.T) {
	useOfflineResolver(t)

	tests := []struct {
		name      string
		goVersion string
		expected  []string
	}{
		{
			name:      "minimum below supported set returns full set",
			goVersion: "1.17",
			expected:  []string{"1.26", "1.27"},
		},
		{
			name:      "minimum at an end-of-life line returns full set",
			goVersion: "1.24",
			expected:  []string{"1.26", "1.27"},
		},
		{
			name:      "minimum at oldest supported returns full set",
			goVersion: "1.26",
			expected:  []string{"1.26", "1.27"},
		},
		{
			name:      "minimum at newest returns single entry",
			goVersion: "1.27",
			expected:  []string{"1.27"},
		},
		{
			name:      "patch component is normalized to major.minor",
			goVersion: "1.26.3",
			expected:  []string{"1.26", "1.27"},
		},
		{
			name:      "patch component on newest line is normalized",
			goVersion: "1.27.1",
			expected:  []string{"1.27"},
		},
		{
			name:      "minimum newer than supported set returns the minimum",
			goVersion: "1.28",
			expected:  []string{"1.28"},
		},
		{
			name:      "patch component on unsupported minimum is normalized",
			goVersion: "1.28.1",
			expected:  []string{"1.28"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matrix := generateGoVersionMatrix(tt.goVersion)
			if !reflect.DeepEqual(matrix, tt.expected) {
				t.Errorf("generateGoVersionMatrix(%q) = %v, expected %v",
					tt.goVersion, matrix, tt.expected)
			}
		})
	}
}

// writeGoMod writes a go.mod declaring goVersion to a temporary
// directory and returns the directory.
func writeGoMod(t *testing.T, goVersion string) string {
	t.Helper()
	dir := t.TempDir()
	content := "module github.com/example/project\n\ngo " + goVersion + "\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

// extractMatrixJSON runs the extractor on a go.mod declaring goVersion
// and returns its matrix_json, the value behind go_matrix_json.
func extractMatrixJSON(t *testing.T, goVersion string) string {
	t.Helper()
	metadata, err := NewExtractor().Extract(writeGoMod(t, goVersion))
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	got, ok := metadata.LanguageSpecific["matrix_json"].(string)
	if !ok {
		t.Fatalf("matrix_json is %T, want string", metadata.LanguageSpecific["matrix_json"])
	}
	return got
}

// goMatrixJSONBefore is go_matrix_json for a go.mod declaring go 1.24
// as the action produced it before the module took over.
const goMatrixJSONBefore = `{"go-version": ["1.26", "1.27"]}`

// TestGoMatrixJSONOffline checks go_matrix_json for go 1.24 in offline
// mode, from the module's embedded snapshot.
func TestGoMatrixJSONOffline(t *testing.T) {
	useOfflineResolver(t)
	if got := extractMatrixJSON(t, "1.24"); got != goMatrixJSONBefore {
		t.Errorf("matrix_json = %s, want %s", got, goMatrixJSONBefore)
	}
}

// TestGoMatrixJSONRecordedLive checks go_matrix_json for go 1.24 from
// recorded upstream data served over HTTP.
func TestGoMatrixJSONRecordedLive(t *testing.T) {
	up := supportedtest.NewGoUpstream(t)
	useResolver(t, supported.Settings{
		Timeout:          time.Second,
		EndOfLifeBaseURL: up.URL,
		ManifestBaseURL:  up.URL,
	})
	if got := extractMatrixJSON(t, "1.24"); got != goMatrixJSONBefore {
		t.Errorf("matrix_json = %s, want %s", got, goMatrixJSONBefore)
	}
	if up.Requests() == 0 {
		t.Error("resolver made no request to the upstream")
	}
}

// TestGoMatrixJSONLaggingManifest checks go_matrix_json for go 1.24
// when endoflife.date lists 1.27 but the actions/go-versions manifest
// has no 1.27 build yet: admission follows the lifecycle data alone,
// as it did before the module, so 1.27 stays in the matrix.
func TestGoMatrixJSONLaggingManifest(t *testing.T) {
	up := supportedtest.NewGoUpstream(t)
	up.ServeManifest(supportedtest.GoManifestLagging)
	useResolver(t, supported.Settings{
		Timeout:          time.Second,
		EndOfLifeBaseURL: up.URL,
		ManifestBaseURL:  up.URL,
	})
	if got := extractMatrixJSON(t, "1.24"); got != goMatrixJSONBefore {
		t.Errorf("matrix_json = %s, want %s", got, goMatrixJSONBefore)
	}
}

// TestGoMatrixJSONStaleLifecycle checks go_matrix_json for go 1.24
// when endoflife.date still lists 1.25 as supported with no
// end-of-life date: a line the embedded snapshot already holds to be
// end-of-life stays out of the matrix, as the baseline kept it out
// before the module.
func TestGoMatrixJSONStaleLifecycle(t *testing.T) {
	up := supportedtest.NewGoUpstream(t)
	up.ServeReleases(supportedtest.GoReleasesStale)
	useResolver(t, supported.Settings{
		Timeout:          time.Second,
		EndOfLifeBaseURL: up.URL,
		ManifestBaseURL:  up.URL,
	})
	if got := extractMatrixJSON(t, "1.24"); got != goMatrixJSONBefore {
		t.Errorf("matrix_json = %s, want %s", got, goMatrixJSONBefore)
	}
}

// TestGoMatrixJSONBrokenManifest checks go_matrix_json for go 1.24
// when endoflife.date has moved past the embedded snapshot (1.28
// released, 1.26 end-of-life) and the actions/go-versions manifest
// answers 404 or a body that does not parse: the live lifecycle data
// stands, rather than a snapshot retry that drops 1.28 and restores
// 1.26.
func TestGoMatrixJSONBrokenManifest(t *testing.T) {
	const want = `{"go-version": ["1.27", "1.28"]}`
	for _, tt := range []struct {
		name    string
		breakIt func(*supportedtest.Upstream)
	}{
		{"not found", func(u *supportedtest.Upstream) { u.FailManifest(http.StatusNotFound) }},
		{"malformed", func(u *supportedtest.Upstream) { u.ServeManifest("<html>not json</html>") }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			up := supportedtest.NewGoUpstream(t)
			up.ServeReleases(supportedtest.GoReleasesNewer)
			tt.breakIt(up)
			useResolver(t, supported.Settings{
				Timeout:          time.Second,
				EndOfLifeBaseURL: up.URL,
				ManifestBaseURL:  up.URL,
			})
			if got := extractMatrixJSON(t, "1.24"); got != want {
				t.Errorf("matrix_json = %s, want %s", got, want)
			}
		})
	}
}

// TestGenerateGoVersionMatrixResolverFailsFallsBackToSnapshot verifies
// a resolver error degrades to the embedded snapshot.
func TestGenerateGoVersionMatrixResolverFailsFallsBackToSnapshot(t *testing.T) {
	useOfflineResolver(t)
	SetResolver(failingResolver{})

	if got, want := generateGoVersionMatrix("1.24"), []string{"1.26", "1.27"}; !reflect.DeepEqual(got, want) {
		t.Errorf("generateGoVersionMatrix(\"1.24\") = %v, want %v", got, want)
	}
}

// TestGenerateGoVersionMatrixEmptyReportFallsBackToSnapshot verifies
// an empty report, such as an upstream listing no release, recovers
// from the embedded snapshot rather than yielding the minimum.
func TestGenerateGoVersionMatrixEmptyReportFallsBackToSnapshot(t *testing.T) {
	useSnapshotFallback(t)
	SetResolver(recordedResolver{})
	t.Cleanup(func() { SetResolver(nil) })

	if got, want := generateGoVersionMatrix("1.24.2"), []string{"1.26", "1.27"}; !reflect.DeepEqual(got, want) {
		t.Errorf("generateGoVersionMatrix(\"1.24.2\") = %v, want %v", got, want)
	}
}

// TestGoMatrixJSONEmptyUpstreamReleases checks go_matrix_json for
// go 1.24 when endoflife.date answers HTTP 200 with no release.
func TestGoMatrixJSONEmptyUpstreamReleases(t *testing.T) {
	up := supportedtest.NewGoUpstream(t)
	up.ServeReleases(supportedtest.GoReleasesEmpty)
	useResolver(t, supported.Settings{
		Timeout:          time.Second,
		EndOfLifeBaseURL: up.URL,
		ManifestBaseURL:  up.URL,
	})
	if got := extractMatrixJSON(t, "1.24"); got != goMatrixJSONBefore {
		t.Errorf("matrix_json = %s, want %s", got, goMatrixJSONBefore)
	}
}

// TestGoMatrixJSONMalformedCachedResponse checks go_matrix_json for
// go 1.24 when the runner cache holds a malformed HTTP 200 endoflife
// response under the default upstream URL: the fallback must bypass
// the cache and reach the embedded snapshot.
func TestGoMatrixJSONMalformedCachedResponse(t *testing.T) {
	useOfflineResolver(t)
	supportedtest.CacheMalformedGoReleases(t, fixedNow())
	if got := extractMatrixJSON(t, "1.24"); got != goMatrixJSONBefore {
		t.Errorf("matrix_json = %s, want %s", got, goMatrixJSONBefore)
	}
}

// TestGenerateGoVersionMatrixNoSupportedLines verifies the declared
// minimum is used only when the snapshot yields no line either.
func TestGenerateGoVersionMatrixNoSupportedLines(t *testing.T) {
	saved := newSnapshotResolver
	newSnapshotResolver = func() (MatrixResolver, error) { return recordedResolver{}, nil }
	SetResolver(recordedResolver{})
	t.Cleanup(func() {
		SetResolver(nil)
		newSnapshotResolver = saved
	})

	if got, want := generateGoVersionMatrix("1.24.2"), []string{"1.24"}; !reflect.DeepEqual(got, want) {
		t.Errorf("generateGoVersionMatrix(\"1.24.2\") = %v, want %v", got, want)
	}
}

// TestGenerateGoVersionMatrixSnapshotFails verifies the declared
// minimum is used when the snapshot resolver cannot be built.
func TestGenerateGoVersionMatrixSnapshotFails(t *testing.T) {
	saved := newSnapshotResolver
	newSnapshotResolver = func() (MatrixResolver, error) { return nil, errors.New("no snapshot") }
	SetResolver(failingResolver{})
	t.Cleanup(func() {
		SetResolver(nil)
		newSnapshotResolver = saved
	})

	if got, want := generateGoVersionMatrix("1.24"), []string{"1.24"}; !reflect.DeepEqual(got, want) {
		t.Errorf("generateGoVersionMatrix(\"1.24\") = %v, want %v", got, want)
	}
}

// TestNewResolverAdmitsReleasedLines verifies NewResolver admits Go
// lines once released and keeps the rest of a caller's override.
func TestNewResolverAdmitsReleasedLines(t *testing.T) {
	warnDays := 30
	r, err := NewResolver(supported.Settings{
		Offline:  true,
		NoCache:  true,
		Now:      fixedNow,
		Override: &catalog.PolicyOverride{WarnDaysBeforeEOL: &warnDays},
	})
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })

	rep, err := r.Matrix(context.Background(), goTool, "")
	if err != nil {
		t.Fatalf("Matrix: %v", err)
	}
	if rep.Policy.Admit != catalog.AdmitReleased {
		t.Errorf("admit = %q, want %q", rep.Policy.Admit, catalog.AdmitReleased)
	}
	if rep.Policy.WarnDaysBeforeEOL != warnDays {
		t.Errorf("warn_days_before_eol = %d, want %d", rep.Policy.WarnDaysBeforeEOL, warnDays)
	}
	for _, s := range rep.Source.Sources {
		if s.Type == supported.SourceActionsManifest {
			t.Errorf("report lists source %+v, want no manifest source", s)
		}
	}
}

// TestDropBelowFloor verifies lines older than the snapshot floor are
// dropped and an empty floor keeps every line.
func TestDropBelowFloor(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		floor string
		want  []string
	}{
		{"no floor keeps every line", []string{"1.25", "1.26"}, "", []string{"1.25", "1.26"}},
		{"lines below the floor are dropped", []string{"1.25", "1.26", "1.27"}, "1.26", []string{"1.26", "1.27"}},
		{"every line below the floor", []string{"1.24", "1.25"}, "1.26", []string{}},
		{"two-digit minors compare numerically", []string{"1.9", "1.10"}, "1.10", []string{"1.10"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dropBelowFloor(tt.lines, tt.floor); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("dropBelowFloor(%v, %q) = %v, want %v", tt.lines, tt.floor, got, tt.want)
			}
		})
	}
}

// TestNormalizeGoVersion verifies major.minor reduction of go.mod
// version directives.
func TestNormalizeGoVersion(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"1.25", "1.25"},
		{"1.25.4", "1.25"},
		{"1.26.0", "1.26"},
		{"2.0", "2.0"},
		{"invalid", "invalid"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := normalizeGoVersion(tt.input); got != tt.expected {
				t.Errorf("normalizeGoVersion(%q) = %q, expected %q",
					tt.input, got, tt.expected)
			}
		})
	}
}

// TestCompareGoVersionStrings verifies the major.minor comparator.
func TestCompareGoVersionStrings(t *testing.T) {
	tests := []struct {
		v1, v2   string
		expected int
	}{
		{"1.25", "1.26", -1},
		{"1.26", "1.25", 1},
		{"1.25", "1.25", 0},
		{"2.0", "1.99", 1},
		{"1.9", "1.10", -1},
	}

	for _, tt := range tests {
		t.Run(tt.v1+" vs "+tt.v2, func(t *testing.T) {
			if got := compareGoVersionStrings(tt.v1, tt.v2); got != tt.expected {
				t.Errorf("compareGoVersionStrings(%q, %q) = %d, expected %d",
					tt.v1, tt.v2, got, tt.expected)
			}
		})
	}
}
