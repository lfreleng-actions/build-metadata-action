// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package rust

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

// withChannelServer points the stable channel fetch at handler (or at an
// unreachable address when handler is nil), empties the shared cache and
// fixes the fallback clock, restoring everything when the test ends.
func withChannelServer(t *testing.T, handler http.HandlerFunc, at time.Time) {
	t.Helper()

	savedURL, savedNow := rustStableChannelURL, now
	resetCache := func() {
		rustVersionCache.Lock()
		rustVersionCache.versions = nil
		rustVersionCache.fetchedAt = time.Time{}
		rustVersionCache.Unlock()
	}
	t.Cleanup(func() {
		rustStableChannelURL, now = savedURL, savedNow
		resetCache()
	})
	resetCache()
	now = func() time.Time { return at }

	if handler == nil {
		server := httptest.NewServer(http.NotFoundHandler())
		server.Close()
		rustStableChannelURL = server.URL
		return
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	rustStableChannelURL = server.URL
}

func dayAfterAnchor(days int) time.Time {
	return fallbackAnchorDate.Add(time.Duration(days) * 24 * time.Hour)
}

func TestFallbackStableVersionFollowsTheReleaseTrain(t *testing.T) {
	tests := []struct {
		name string
		at   time.Time
		want string
	}{
		{"before the anchor release", fallbackAnchorDate.AddDate(-1, 0, 0), "1.99"},
		{"on the anchor release day", fallbackAnchorDate, "1.99"},
		{"on the next release day", dayAfterAnchor(42), "1.99"},
		{"the day after the next release", dayAfterAnchor(43), "1.100"},
		{"a year after the anchor", dayAfterAnchor(365), "1.107"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fallbackStableVersion(tt.at); got != tt.want {
				t.Errorf("fallbackStableVersion(%s) = %s, want %s", tt.at.Format(time.DateOnly), got, tt.want)
			}
		})
	}
}

func TestRustVersionMatrixFromChannelManifest(t *testing.T) {
	manifest := "manifest-version = \"2\"\ndate = \"2027-01-14\"\n\n" +
		"[pkg.rust]\nversion = \"1.101.0 (0123456789 2027-01-10)\"\n"
	withChannelServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(manifest))
	}, dayAfterAnchor(1))

	got := generateRustVersionMatrix("1.97")
	want := []string{"1.97", "1.98", "1.99", "1.100", "1.101", "stable"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("matrix = %v, want %v", got, want)
	}
}

// An unreachable or failing channel manifest falls back to the release
// train, producing the same shape of matrix as the live path.
func TestRustVersionMatrixOffline(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		msrv    string
		want    []string
	}{
		{
			name: "unreachable, old MSRV",
			msrv: "1.60",
			want: []string{"1.60", "1.94", "1.95", "1.96", "1.97", "1.98", "1.99", "stable"},
		},
		{
			name: "server error, recent MSRV",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			msrv: "1.97",
			want: []string{"1.97", "1.98", "1.99", "stable"},
		},
		{
			name: "unparsable manifest, MSRV newer than stable",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte("[pkg.rust]\nversion = \"unknown\"\n"))
			},
			msrv: "1.120",
			want: []string{"1.120", "stable"},
		},
		{
			name: "MSRV with a patch level appears once",
			msrv: "1.97.0",
			want: []string{"1.97.0", "1.98", "1.99", "stable"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withChannelServer(t, tt.handler, dayAfterAnchor(1))
			if got := generateRustVersionMatrix(tt.msrv); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("matrix = %v, want %v", got, tt.want)
			}
		})
	}
}

// Without an MSRV the matrix starts at the first release supporting the
// edition; edition 2024 was stabilised in 1.85.
func TestRustVersionMatrixFromEachEdition(t *testing.T) {
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
