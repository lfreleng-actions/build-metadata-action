// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package main

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/lfreleng-actions/build-metadata-action/internal/extractor/javascript"
	"github.com/lfreleng-actions/build-metadata-action/internal/supported"
	"github.com/lfreleng-actions/build-metadata-action/internal/supported/supportedtest"
)

// TestNodeOutputsAreDeclaredUnprefixed checks that the Node.js matrix
// outputs are written and declared under their own names, while the
// other JavaScript outputs keep the javascript_ prefix.
func TestNodeOutputsAreDeclaredUnprefixed(t *testing.T) {
	declared := declaredOutputs(t)
	written := writtenOutputs(t, javascriptFixtures, "javascript-npm")

	for _, name := range []string{"node_matrix_json", "node_latest"} {
		if !written[name] {
			t.Errorf("%s is never written by any JavaScript fixture", name)
		}
		if written["javascript_"+name] {
			t.Errorf("javascript_%s is written; the Node.js outputs carry no language prefix", name)
		}
		value, ok := declared[name]
		if !ok {
			t.Errorf("%s is written but not declared in action.yaml", name)
			continue
		}
		if want := "${{ steps.extract.outputs." + name + " }}"; value != want {
			t.Errorf("output %s maps to %q, want %q", name, value, want)
		}
	}
	if !written["javascript_package_name"] {
		t.Error("javascript_package_name is no longer written")
	}
}

func TestLanguageOutputKey(t *testing.T) {
	tests := []struct {
		language, key, want string
	}{
		{"javascript", "node_matrix_json", "node_matrix_json"},
		{"javascript", "node_latest", "node_latest"},
		{"javascript", "requires_node", "javascript_requires_node"},
		{"java", "matrix_json", "java_matrix_json"},
		{"java", "node_latest", "java_node_latest"},
		{"go", "matrix_json", "go_matrix_json"},
	}
	for _, tt := range tests {
		if got := languageOutputKey(tt.language, tt.key); got != tt.want {
			t.Errorf("languageOutputKey(%q, %q) = %q, want %q", tt.language, tt.key, got, tt.want)
		}
	}
}

// nodeResolverNow is a clock after the hypothetical Node.js 28 in the
// newer recorded data shipped.
func nodeResolverNow() time.Time {
	return time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
}

// nodeResolverMatrix resolves the unconstrained Node.js matrix through
// a resolver built from cfg, with the clock fixed.
func nodeResolverMatrix(t *testing.T, cfg runConfig, noCache bool) []string {
	t.Helper()
	s := resolverSettings(cfg)
	s.NoCache = noCache
	s.Now = nodeResolverNow
	r, err := javascript.NewResolver(s)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	defer func() { _ = r.Close() }()
	rep, err := r.MatrixFor(context.Background(), "nodejs", supported.MatrixRequest{})
	if err != nil {
		t.Fatalf("MatrixFor: %v", err)
	}
	return rep.Matrix.Versions
}

// TestNodeResolverSettingsOfflineIgnoresCache checks that offline mode
// resolves from the embedded snapshot even when an earlier online step
// cached newer data: the cached data give 24, 26 and 28, the snapshot
// 22, 24 and 26.
func TestNodeResolverSettingsOfflineIgnoresCache(t *testing.T) {
	t.Setenv("SUPPORTED_VERSIONS_CACHE_DIR", t.TempDir())
	supportedtest.CacheResponse(t, supportedtest.DefaultNodeReleasesURL,
		supportedtest.NodeReleasesNewer, nodeResolverNow())
	supportedtest.CacheResponse(t, supportedtest.DefaultNodeManifestURL,
		supportedtest.NodeManifestNewer, nodeResolverNow())
	cfg := runConfig{pythonOffline: true, pythonTimeout: time.Second}

	s := resolverSettings(cfg)
	if !s.Offline || !s.NoCache {
		t.Fatalf("offline settings = %+v, want Offline and NoCache", s)
	}
	if got, want := nodeResolverMatrix(t, cfg, s.NoCache), []string{"22", "24", "26"}; !reflect.DeepEqual(got, want) {
		t.Errorf("offline matrix = %v, want %v", got, want)
	}
	if got, want := nodeResolverMatrix(t, cfg, false), []string{"24", "26", "28"}; !reflect.DeepEqual(got, want) {
		t.Errorf("cache-reading offline matrix = %v, want %v", got, want)
	}
}
