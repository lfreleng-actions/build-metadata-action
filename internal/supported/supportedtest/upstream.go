// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

// Package supportedtest serves recorded upstream release data for
// tests of package supported and its callers, so they never touch the
// network.
package supportedtest

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lfreleng-actions/supported-versions-action/pkg/cache"
)

// GoReleases is an endoflife.date v1 response for the go product,
// built from the release facts the module's snapshot recorded on
// 2026-11-06 and trimmed to the newest four lines.
const GoReleases = `{"schema_version":"1.2.0","result":{"name":"go","releases":[
{"name":"1.27","releaseDate":"2026-08-11","isLts":false,"isEoas":false,"isEol":false,"latest":{"name":"1.27.3","date":"2026-10-07"}},
{"name":"1.26","releaseDate":"2026-02-10","isLts":false,"isEoas":false,"isEol":false,"latest":{"name":"1.26.10","date":"2026-10-07"}},
{"name":"1.25","releaseDate":"2025-08-12","isLts":false,"isEoas":false,"isEol":true,"eolFrom":"2026-08-19","latest":{"name":"1.25.14","date":"2026-08-19"}},
{"name":"1.24","releaseDate":"2025-02-11","isLts":false,"isEoas":false,"isEol":true,"eolFrom":"2026-02-10","latest":{"name":"1.24.13","date":"2026-02-04"}}
]}}`

// GoManifest is the actions/go-versions manifest, built from the same
// snapshot and trimmed to the newest stable build of each line above.
const GoManifest = `[
{"version":"1.27.3","stable":true,"files":[{"platform":"linux","arch":"x64"}]},
{"version":"1.26.10","stable":true,"files":[{"platform":"linux","arch":"x64"}]},
{"version":"1.25.14","stable":true,"files":[{"platform":"linux","arch":"x64"}]},
{"version":"1.24.13","stable":true,"files":[{"platform":"linux","arch":"x64"}]}
]`

// DefaultGoReleasesURL is the endoflife.date URL the module reads the
// go product from when no base URL is overridden.
var DefaultGoReleasesURL = (&url.URL{Scheme: "https", Host: "endoflife.date", Path: "/api/v1" + goReleasesPath}).String()

const (
	goReleasesPath = "/products/go/"
	goManifestPath = "/actions/go-versions/main/versions-manifest.json"
)

// Upstream is a test server standing in for endoflife.date and the
// actions versions manifests. Use URL as both Settings.EndOfLifeBaseURL
// and Settings.ManifestBaseURL.
type Upstream struct {
	URL            string
	requests       atomic.Int64
	status         atomic.Int64
	manifestStatus atomic.Int64

	mu       sync.Mutex
	releases string
	manifest string
}

// GoReleasesEmpty is an endoflife.date v1 response for the go product
// that lists no release, as a degraded upstream might answer.
const GoReleasesEmpty = `{"schema_version":"1.2.0","result":{"name":"go","releases":[]}}`

// ServeGoReleases makes later requests for the go product answer body
// in place of GoReleases.
func (u *Upstream) ServeGoReleases(body string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.releases = body
}

func (u *Upstream) goReleases() string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.releases
}

// GoReleasesStale is GoReleases as an upstream that lags a release
// might still answer it: 1.25 is not yet marked end-of-life and
// carries no end-of-life date.
const GoReleasesStale = `{"schema_version":"1.2.0","result":{"name":"go","releases":[
{"name":"1.27","releaseDate":"2026-08-11","isLts":false,"isEoas":false,"isEol":false,"latest":{"name":"1.27.3","date":"2026-10-07"}},
{"name":"1.26","releaseDate":"2026-02-10","isLts":false,"isEoas":false,"isEol":false,"latest":{"name":"1.26.10","date":"2026-10-07"}},
{"name":"1.25","releaseDate":"2025-08-12","isLts":false,"isEoas":false,"isEol":false,"latest":{"name":"1.25.14","date":"2026-08-19"}},
{"name":"1.24","releaseDate":"2025-02-11","isLts":false,"isEoas":false,"isEol":true,"eolFrom":"2026-02-10","latest":{"name":"1.24.13","date":"2026-02-04"}}
]}}`

// GoManifestLagging is GoManifest before the newest line's builds
// reach it: it lists no 1.27 build.
const GoManifestLagging = `[
{"version":"1.26.10","stable":true,"files":[{"platform":"linux","arch":"x64"}]},
{"version":"1.25.14","stable":true,"files":[{"platform":"linux","arch":"x64"}]},
{"version":"1.24.13","stable":true,"files":[{"platform":"linux","arch":"x64"}]}
]`

// ServeGoManifest makes later requests for the go manifest answer
// body in place of GoManifest.
func (u *Upstream) ServeGoManifest(body string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.manifest = body
}

func (u *Upstream) goManifest() string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.manifest
}

// GoReleasesNewer is GoReleases as endoflife.date might answer it
// once the embedded snapshot lags: 1.28 is released and 1.26 has
// reached end-of-life, so the supported lines are 1.27 and 1.28.
const GoReleasesNewer = `{"schema_version":"1.2.0","result":{"name":"go","releases":[
{"name":"1.28","releaseDate":"2026-11-03","isLts":false,"isEoas":false,"isEol":false,"latest":{"name":"1.28.0","date":"2026-11-03"}},
{"name":"1.27","releaseDate":"2026-08-11","isLts":false,"isEoas":false,"isEol":false,"latest":{"name":"1.27.3","date":"2026-10-07"}},
{"name":"1.26","releaseDate":"2026-02-10","isLts":false,"isEoas":false,"isEol":true,"eolFrom":"2026-11-03","latest":{"name":"1.26.10","date":"2026-10-07"}},
{"name":"1.25","releaseDate":"2025-08-12","isLts":false,"isEoas":false,"isEol":true,"eolFrom":"2026-08-19","latest":{"name":"1.25.14","date":"2026-08-19"}}
]}}`

// FailGoManifest makes later requests for the go manifest alone answer
// status, as a manifest that moved or broke would; zero restores the
// manifest body.
func (u *Upstream) FailGoManifest(status int) {
	u.manifestStatus.Store(int64(status))
}

// Fail makes every later request answer status, as an outage would;
// zero restores the recorded responses.
func (u *Upstream) Fail(status int) {
	u.status.Store(int64(status))
}

// Requests returns how many requests the server has answered.
func (u *Upstream) Requests() int64 {
	return u.requests.Load()
}

// NewGoUpstream starts an Upstream serving GoReleases and GoManifest,
// or the bodies ServeGoReleases and ServeGoManifest set; any other
// path answers 404. The server stops when the test ends.
func NewGoUpstream(t *testing.T) *Upstream {
	t.Helper()
	u := &Upstream{releases: GoReleases, manifest: GoManifest}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u.requests.Add(1)
		if status := u.status.Load(); status != 0 {
			w.WriteHeader(int(status))
			return
		}
		switch r.URL.Path {
		case goReleasesPath:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(u.goReleases()))
		case goManifestPath:
			if status := u.manifestStatus.Load(); status != 0 {
				w.WriteHeader(int(status))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(u.goManifest()))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	u.URL = srv.URL
	return u
}

// CacheMalformedGoReleases stores a malformed HTTP 200 body for
// DefaultGoReleasesURL in the cache cache.Open resolves, fetched at
// fetchedAt, as a broken upstream answer cached before parsing would
// be. Point $SUPPORTED_VERSIONS_CACHE_DIR at a test directory first.
func CacheMalformedGoReleases(t *testing.T, fetchedAt time.Time) {
	t.Helper()
	CacheGoReleases(t, "<html>not json</html>", fetchedAt)
}

// CacheGoReleases stores body as an HTTP 200 answer for
// DefaultGoReleasesURL in the cache cache.Open resolves, fetched at
// fetchedAt, as an earlier online step would have left it. Point
// $SUPPORTED_VERSIONS_CACHE_DIR at a test directory first.
func CacheGoReleases(t *testing.T, body string, fetchedAt time.Time) {
	t.Helper()
	c, err := cache.Open()
	if err != nil {
		t.Fatalf("cache.Open: %v", err)
	}
	defer func() { _ = c.Close() }()
	err = c.Put(cache.Key(http.MethodGet, DefaultGoReleasesURL, nil), cache.Entry{
		Method:    http.MethodGet,
		URL:       DefaultGoReleasesURL,
		Status:    http.StatusOK,
		FetchedAt: fetchedAt,
		Body:      []byte(body),
	})
	if err != nil {
		t.Fatalf("cache.Put: %v", err)
	}
}
