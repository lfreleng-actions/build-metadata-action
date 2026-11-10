// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package main

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/lfreleng-actions/build-metadata-action/internal/extractor/rust"
	"github.com/lfreleng-actions/build-metadata-action/internal/supported/supportedtest"
)

// rustResolverNow is a clock after 1.100 shipped, so cached facts
// listing it put it in the matrix.
func rustResolverNow() time.Time {
	return time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
}

// rustResolverMatrix resolves the unconstrained Rust matrix through a
// resolver built from cfg, with the clock fixed.
func rustResolverMatrix(t *testing.T, cfg runConfig, noCache bool) []string {
	t.Helper()
	s := resolverSettings(cfg)
	s.NoCache = noCache
	s.Now = rustResolverNow
	r, err := rust.NewResolver(s)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	defer func() { _ = r.Close() }()
	rep, err := r.Matrix(context.Background(), "rust", "")
	if err != nil {
		t.Fatalf("Matrix: %v", err)
	}
	return rep.Matrix.Versions
}

// TestRustResolverSettingsOfflineIgnoresCache checks that offline mode
// resolves from the embedded snapshot even when an earlier online step
// cached newer lifecycle and channel data: the cached data give 1.95
// through 1.100, the snapshot 1.94 through 1.99.
func TestRustResolverSettingsOfflineIgnoresCache(t *testing.T) {
	t.Setenv("SUPPORTED_VERSIONS_CACHE_DIR", t.TempDir())
	supportedtest.CacheResponse(t, supportedtest.DefaultRustReleasesURL,
		supportedtest.RustReleasesNewer, rustResolverNow())
	supportedtest.CacheResponse(t, supportedtest.DefaultRustChannelURL,
		supportedtest.RustChannelNewer, rustResolverNow())
	cfg := runConfig{pythonOffline: true, pythonTimeout: time.Second}

	s := resolverSettings(cfg)
	if !s.Offline || !s.NoCache {
		t.Fatalf("offline settings = %+v, want Offline and NoCache", s)
	}
	snapshot := []string{"1.94", "1.95", "1.96", "1.97", "1.98", "1.99", "stable"}
	if got := rustResolverMatrix(t, cfg, s.NoCache); !reflect.DeepEqual(got, snapshot) {
		t.Errorf("offline matrix = %v, want %v", got, snapshot)
	}
	// The cached entries are what an offline resolver reading the cache
	// would see, so the test fails without NoCache.
	cached := []string{"1.95", "1.96", "1.97", "1.98", "1.99", "1.100", "stable"}
	if got := rustResolverMatrix(t, cfg, false); !reflect.DeepEqual(got, cached) {
		t.Errorf("cache-reading offline matrix = %v, want %v", got, cached)
	}
}
