// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package main

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/lfreleng-actions/build-metadata-action/internal/extractor/golang"
	"github.com/lfreleng-actions/build-metadata-action/internal/supported/supportedtest"
)

// goResolverNow is a clock at which the embedded snapshot supports Go
// 1.26 and 1.27.
func goResolverNow() time.Time {
	return time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
}

// goResolverMatrix resolves the Go matrix for go 1.24 through a
// resolver built from cfg, with the clock fixed.
func goResolverMatrix(t *testing.T, cfg runConfig, noCache bool) []string {
	t.Helper()
	s := resolverSettings(cfg)
	s.NoCache = noCache
	s.Now = goResolverNow
	r, err := golang.NewResolver(s)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	defer func() { _ = r.Close() }()
	rep, err := r.Matrix(context.Background(), "go", "1.24")
	if err != nil {
		t.Fatalf("Matrix: %v", err)
	}
	return rep.Matrix.Versions
}

// TestGoResolverSettingsOfflineIgnoresCache checks that offline mode
// resolves from the embedded snapshot even when an earlier online step
// cached newer release facts: the cached facts give [1.27 1.28], the
// snapshot [1.26 1.27].
func TestGoResolverSettingsOfflineIgnoresCache(t *testing.T) {
	t.Setenv("SUPPORTED_VERSIONS_CACHE_DIR", t.TempDir())
	supportedtest.CacheGoReleases(t, supportedtest.GoReleasesNewer, goResolverNow())
	cfg := runConfig{pythonOffline: true, pythonTimeout: time.Second}

	s := resolverSettings(cfg)
	if !s.Offline || !s.NoCache {
		t.Fatalf("offline settings = %+v, want Offline and NoCache", s)
	}
	if got, want := goResolverMatrix(t, cfg, s.NoCache), []string{"1.26", "1.27"}; !reflect.DeepEqual(got, want) {
		t.Errorf("offline matrix = %v, want %v", got, want)
	}
	// The cached entry is what an offline resolver reading the cache
	// would see, so the test fails without NoCache.
	if got, want := goResolverMatrix(t, cfg, false), []string{"1.27", "1.28"}; !reflect.DeepEqual(got, want) {
		t.Errorf("cache-reading offline matrix = %v, want %v", got, want)
	}
}

// TestGoResolverSettingsOnline checks the online mapping of the
// inputs: the runner cache stays in use and the timeout and retry
// budget pass through.
func TestGoResolverSettingsOnline(t *testing.T) {
	s := resolverSettings(runConfig{pythonTimeout: 7 * time.Second, pythonRetries: 3})
	if s.Offline || s.NoCache {
		t.Errorf("online settings = %+v, want neither Offline nor NoCache", s)
	}
	if s.Timeout != 7*time.Second || s.Retries != 3 {
		t.Errorf("Timeout, Retries = %v, %d, want 7s, 3", s.Timeout, s.Retries)
	}
}

// TestWarnLegacyEOLAPIURL checks that a set
// BUILD_METADATA_GO_EOL_API_URL or BUILD_METADATA_PYTHON_EOL_API_URL
// draws a warning naming it, and an unset one draws none.
func TestWarnLegacyEOLAPIURL(t *testing.T) {
	for _, tc := range []struct{ env, language string }{
		{legacyGoEOLAPIURLEnv, "Go"},
		{legacyPythonEOLAPIURLEnv, "Python"},
	} {
		var unset strings.Builder
		warnLegacyEOLAPIURL(func(string) string { return "" }, &unset, tc.env, tc.language)
		if unset.Len() != 0 {
			t.Errorf("%s: warning with the variable unset: %q", tc.env, unset.String())
		}

		var set strings.Builder
		warnLegacyEOLAPIURL(func(name string) string {
			if name == tc.env {
				return "https://mirror.example/api/eol.json"
			}
			return ""
		}, &set, tc.env, tc.language)
		if !strings.Contains(set.String(), "[WARNING] "+tc.env) ||
			!strings.Contains(set.String(), tc.language+" versions") {
			t.Errorf("warning = %q, want one naming %s and %s", set.String(), tc.env, tc.language)
		}
	}
}
