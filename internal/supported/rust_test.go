// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package supported

import (
	"context"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/lfreleng-actions/build-metadata-action/internal/supported/supportedtest"
)

// rustSnapshotMatrix is the Rust matrix the recorded data and the
// embedded snapshot both give: the six newest lines, then stable.
var rustSnapshotMatrix = []string{"1.94", "1.95", "1.96", "1.97", "1.98", "1.99", "stable"}

// rustReport resolves the unconstrained Rust matrix from up.
func rustReport(t *testing.T, up *supportedtest.Upstream, timeout time.Duration) Report {
	t.Helper()
	r := newResolver(t, Settings{
		Timeout:          timeout,
		EndOfLifeBaseURL: up.URL,
		RustBaseURL:      up.URL,
	})
	rep, err := r.Matrix(context.Background(), "rust", "")
	if err != nil {
		t.Fatalf("Matrix: %v", err)
	}
	return rep
}

// sourceKinds returns the kind of each source behind rep, in catalog
// order: endoflife.date, then the stable channel.
func sourceKinds(rep Report) []Kind {
	kinds := make([]Kind, 0, len(rep.Source.Sources))
	for _, s := range rep.Source.Sources {
		kinds = append(kinds, s.Kind)
	}
	return kinds
}

// TestRustMatrixRecordedLive checks that RustBaseURL reaches the
// module: the channel manifest is read from the test server, and both
// sources answer live without a warning.
func TestRustMatrixRecordedLive(t *testing.T) {
	warned := isolate(t)
	up := supportedtest.NewRustUpstream(t)
	rep := rustReport(t, up, time.Second)

	if !reflect.DeepEqual(rep.Matrix.Versions, rustSnapshotMatrix) {
		t.Errorf("matrix = %v, want %v", rep.Matrix.Versions, rustSnapshotMatrix)
	}
	if want := []Kind{KindLive, KindLive}; !reflect.DeepEqual(sourceKinds(rep), want) {
		t.Errorf("source kinds = %v, want %v", sourceKinds(rep), want)
	}
	if n := up.Requests(); n != 2 {
		t.Errorf("upstream saw %d requests, want 2: endoflife.date and the channel", n)
	}
	if warned.Len() != 0 {
		t.Errorf("warnings = %q, want none", warned.String())
	}
}

// TestRustMatrixChannelAhead checks that a stable release the
// lifecycle data does not list yet stays out of the numeric lines: the
// stable label covers it until endoflife.date catches up.
func TestRustMatrixChannelAhead(t *testing.T) {
	isolate(t)
	up := supportedtest.NewRustUpstream(t)
	up.ServeManifest(supportedtest.RustChannelNewer)
	rep := rustReport(t, up, time.Second)

	if !reflect.DeepEqual(rep.Matrix.Versions, rustSnapshotMatrix) {
		t.Errorf("matrix = %v, want %v", rep.Matrix.Versions, rustSnapshotMatrix)
	}
}

// TestRustMatrixDegraded checks the reports the module gives when one
// or both Rust sources fail, and that each draws a live-fetch warning.
func TestRustMatrixDegraded(t *testing.T) {
	for _, tt := range []struct {
		name    string
		breakIt func(*supportedtest.Upstream)
		want    []Kind
	}{
		{"outage", func(u *supportedtest.Upstream) { u.Fail(http.StatusServiceUnavailable) },
			[]Kind{KindSnapshot, KindSnapshot}},
		{"channel 503", func(u *supportedtest.Upstream) { u.FailManifest(http.StatusServiceUnavailable) },
			[]Kind{KindLive, KindSnapshot}},
		{"channel timeout", func(u *supportedtest.Upstream) { u.StallManifest(5 * time.Second) },
			[]Kind{KindLive, KindSnapshot}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			warned := isolate(t)
			up := supportedtest.NewRustUpstream(t)
			tt.breakIt(up)
			rep := rustReport(t, up, 200*time.Millisecond)

			if !reflect.DeepEqual(rep.Matrix.Versions, rustSnapshotMatrix) {
				t.Errorf("matrix = %v, want %v", rep.Matrix.Versions, rustSnapshotMatrix)
			}
			if !reflect.DeepEqual(sourceKinds(rep), tt.want) {
				t.Errorf("source kinds = %v, want %v", sourceKinds(rep), tt.want)
			}
			if !strings.Contains(warned.String(), "Could not fetch live Rust release data") {
				t.Errorf("warnings = %q, want a live-fetch warning", warned.String())
			}
		})
	}
}
