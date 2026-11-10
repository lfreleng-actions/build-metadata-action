// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package main

import (
	"context"
	"io"
	"reflect"
	"testing"
	"time"

	"github.com/lfreleng-actions/build-metadata-action/internal/supported/supportedtest"
)

// pythonResolverNow is a clock at which the embedded snapshot supports
// Python 3.11 through 3.15.
func pythonResolverNow() time.Time {
	return time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
}

// pythonResolverMatrix resolves the unconstrained Python matrix
// through the primary resolver built from cfg, with the clock fixed.
func pythonResolverMatrix(t *testing.T, cfg runConfig, noCache bool) []string {
	t.Helper()
	s := resolverSettings(cfg)
	s.NoCache = noCache
	s.Now = pythonResolverNow
	primary, lifecycle := newPythonResolvers(s)
	if primary == nil || lifecycle == nil {
		t.Fatalf("newPythonResolvers built primary %v, lifecycle %v", primary, lifecycle)
	}
	for _, r := range []any{primary, lifecycle} {
		if c, ok := r.(io.Closer); ok {
			defer func() { _ = c.Close() }()
		}
	}
	rep, err := primary.Matrix(context.Background(), "python", "")
	if err != nil {
		t.Fatalf("Matrix: %v", err)
	}
	return rep.Matrix.Versions
}

// TestPythonResolverSettingsOfflineIgnoresCache checks that offline
// mode resolves from the embedded snapshot even when an earlier online
// step cached newer release facts and a manifest with a 3.16 build:
// the cached facts give 3.11 through 3.16, the snapshot 3.11 through
// 3.15.
func TestPythonResolverSettingsOfflineIgnoresCache(t *testing.T) {
	t.Setenv("SUPPORTED_VERSIONS_CACHE_DIR", t.TempDir())
	supportedtest.CacheResponse(t, supportedtest.DefaultPythonReleasesURL,
		supportedtest.PythonReleasesNewer, pythonResolverNow())
	supportedtest.CacheResponse(t, supportedtest.DefaultPythonManifestURL,
		supportedtest.PythonManifestNewer, pythonResolverNow())
	cfg := runConfig{pythonOffline: true, pythonTimeout: time.Second}

	s := resolverSettings(cfg)
	if !s.Offline || !s.NoCache {
		t.Fatalf("offline settings = %+v, want Offline and NoCache", s)
	}
	snapshot := []string{"3.11", "3.12", "3.13", "3.14", "3.15"}
	if got := pythonResolverMatrix(t, cfg, s.NoCache); !reflect.DeepEqual(got, snapshot) {
		t.Errorf("offline matrix = %v, want %v", got, snapshot)
	}
	// The cached entries are what an offline resolver reading the cache
	// would see, so the test fails without NoCache.
	cached := append(append([]string(nil), snapshot...), "3.16")
	if got := pythonResolverMatrix(t, cfg, false); !reflect.DeepEqual(got, cached) {
		t.Errorf("cache-reading offline matrix = %v, want %v", got, cached)
	}
}
