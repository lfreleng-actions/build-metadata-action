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
var DefaultGoReleasesURL = defaultReleasesURL(goReleasesPath)

// PythonReleases is an endoflife.date v1 response for the python
// product, modelled on the module's 2026-11-06 snapshot: 3.11 through
// 3.15 are supported and 3.10 has reached end-of-life.
const PythonReleases = `{"schema_version":"1.2.0","result":{"name":"python","releases":[
{"name":"3.15","releaseDate":"2026-10-09","isLts":false,"isEoas":false,"eoasFrom":"2028-10-09","isEol":false,"eolFrom":"2031-10-31","latest":{"name":"3.15.2","date":"2026-11-04"}},
{"name":"3.14","releaseDate":"2025-10-07","isLts":false,"isEoas":false,"eoasFrom":"2027-10-01","isEol":false,"eolFrom":"2030-10-31","latest":{"name":"3.14.10","date":"2026-11-04"}},
{"name":"3.13","releaseDate":"2024-10-07","isLts":false,"isEoas":true,"eoasFrom":"2026-10-01","isEol":false,"eolFrom":"2029-10-31","latest":{"name":"3.13.18","date":"2026-11-04"}},
{"name":"3.12","releaseDate":"2023-10-02","isLts":false,"isEoas":true,"eoasFrom":"2025-04-02","isEol":false,"eolFrom":"2028-10-31","latest":{"name":"3.12.16","date":"2026-11-04"}},
{"name":"3.11","releaseDate":"2022-10-24","isLts":false,"isEoas":true,"eoasFrom":"2024-04-01","isEol":false,"eolFrom":"2027-10-31","latest":{"name":"3.11.18","date":"2026-11-04"}},
{"name":"3.10","releaseDate":"2021-10-04","isLts":false,"isEoas":true,"eoasFrom":"2023-04-05","isEol":true,"eolFrom":"2026-10-01","latest":{"name":"3.10.22","date":"2026-10-01"}}
]}}`

// PythonManifest is an actions/python-versions manifest with one
// stable build of each line PythonReleases lists.
const PythonManifest = `[
{"version":"3.15.2","stable":true,"files":[{"platform":"linux","arch":"x64"}]},
{"version":"3.14.10","stable":true,"files":[{"platform":"linux","arch":"x64"}]},
{"version":"3.13.18","stable":true,"files":[{"platform":"linux","arch":"x64"}]},
{"version":"3.12.16","stable":true,"files":[{"platform":"linux","arch":"x64"}]},
{"version":"3.11.18","stable":true,"files":[{"platform":"linux","arch":"x64"}]},
{"version":"3.10.22","stable":true,"files":[{"platform":"linux","arch":"x64"}]}
]`

// PythonReleasesNewer is PythonReleases as endoflife.date might
// answer it once the embedded snapshot lags: 3.16 is released.
const PythonReleasesNewer = `{"schema_version":"1.2.0","result":{"name":"python","releases":[
{"name":"3.16","releaseDate":"2026-11-03","isLts":false,"isEoas":false,"eoasFrom":"2028-11-03","isEol":false,"eolFrom":"2032-10-31","latest":{"name":"3.16.0","date":"2026-11-03"}},
{"name":"3.15","releaseDate":"2026-10-09","isLts":false,"isEoas":false,"eoasFrom":"2028-10-09","isEol":false,"eolFrom":"2031-10-31","latest":{"name":"3.15.2","date":"2026-11-04"}},
{"name":"3.14","releaseDate":"2025-10-07","isLts":false,"isEoas":false,"eoasFrom":"2027-10-01","isEol":false,"eolFrom":"2030-10-31","latest":{"name":"3.14.10","date":"2026-11-04"}},
{"name":"3.13","releaseDate":"2024-10-07","isLts":false,"isEoas":true,"eoasFrom":"2026-10-01","isEol":false,"eolFrom":"2029-10-31","latest":{"name":"3.13.18","date":"2026-11-04"}},
{"name":"3.12","releaseDate":"2023-10-02","isLts":false,"isEoas":true,"eoasFrom":"2025-04-02","isEol":false,"eolFrom":"2028-10-31","latest":{"name":"3.12.16","date":"2026-11-04"}},
{"name":"3.11","releaseDate":"2022-10-24","isLts":false,"isEoas":true,"eoasFrom":"2024-04-01","isEol":false,"eolFrom":"2027-10-31","latest":{"name":"3.11.18","date":"2026-11-04"}}
]}}`

// PythonManifestNewer is PythonManifest once a stable 3.16 build has
// landed.
const PythonManifestNewer = `[
{"version":"3.16.0","stable":true,"files":[{"platform":"linux","arch":"x64"}]},
{"version":"3.15.2","stable":true,"files":[{"platform":"linux","arch":"x64"}]},
{"version":"3.14.10","stable":true,"files":[{"platform":"linux","arch":"x64"}]},
{"version":"3.13.18","stable":true,"files":[{"platform":"linux","arch":"x64"}]},
{"version":"3.12.16","stable":true,"files":[{"platform":"linux","arch":"x64"}]},
{"version":"3.11.18","stable":true,"files":[{"platform":"linux","arch":"x64"}]}
]`

// PythonReleasesEmpty is an endoflife.date v1 response for the python
// product that lists no release, as a degraded upstream might answer.
const PythonReleasesEmpty = `{"schema_version":"1.2.0","result":{"name":"python","releases":[]}}`

// DefaultPythonReleasesURL is the endoflife.date URL the module reads
// the python product from when no base URL is overridden.
var DefaultPythonReleasesURL = defaultReleasesURL(pythonReleasesPath)

// DefaultPythonManifestURL is the URL the module reads the
// actions/python-versions manifest from when no base URL is
// overridden.
var DefaultPythonManifestURL = (&url.URL{Scheme: "https", Host: "raw.githubusercontent.com", Path: pythonManifestPath}).String()

// RustReleases is an endoflife.date v1 response for the rust product,
// built from the release facts the module's snapshot recorded on
// 2026-11-06 and trimmed to the newest seven lines.
const RustReleases = `{"schema_version":"1.2.0","result":{"name":"rust","releases":[
{"name":"1.99","releaseDate":"2026-10-01","isLts":false,"isEoas":false,"isEol":false,"latest":{"name":"1.99.1","date":"2026-10-15"}},
{"name":"1.98","releaseDate":"2026-08-20","isLts":false,"isEoas":false,"isEol":true,"eolFrom":"2026-10-01","latest":{"name":"1.98.1","date":"2026-09-03"}},
{"name":"1.97","releaseDate":"2026-07-09","isLts":false,"isEoas":false,"isEol":true,"eolFrom":"2026-08-20","latest":{"name":"1.97.1","date":"2026-07-16"}},
{"name":"1.96","releaseDate":"2026-05-28","isLts":false,"isEoas":false,"isEol":true,"eolFrom":"2026-07-09","latest":{"name":"1.96.1","date":"2026-06-30"}},
{"name":"1.95","releaseDate":"2026-04-16","isLts":false,"isEoas":false,"isEol":true,"eolFrom":"2026-05-28","latest":{"name":"1.95.0","date":"2026-04-16"}},
{"name":"1.94","releaseDate":"2026-03-06","isLts":false,"isEoas":false,"isEol":true,"eolFrom":"2026-04-16","latest":{"name":"1.94.1","date":"2026-03-26"}},
{"name":"1.93","releaseDate":"2026-01-22","isLts":false,"isEoas":false,"isEol":true,"eolFrom":"2026-03-06","latest":{"name":"1.93.1","date":"2026-02-12"}}
]}}`

// RustReleasesNewer is RustReleases as endoflife.date might answer it
// once the embedded snapshot lags: 1.100 is released and 1.99 has
// reached end-of-life.
const RustReleasesNewer = `{"schema_version":"1.2.0","result":{"name":"rust","releases":[
{"name":"1.100","releaseDate":"2026-11-12","isLts":false,"isEoas":false,"isEol":false,"latest":{"name":"1.100.0","date":"2026-11-12"}},
{"name":"1.99","releaseDate":"2026-10-01","isLts":false,"isEoas":false,"isEol":true,"eolFrom":"2026-11-12","latest":{"name":"1.99.1","date":"2026-10-15"}},
{"name":"1.98","releaseDate":"2026-08-20","isLts":false,"isEoas":false,"isEol":true,"eolFrom":"2026-10-01","latest":{"name":"1.98.1","date":"2026-09-03"}},
{"name":"1.97","releaseDate":"2026-07-09","isLts":false,"isEoas":false,"isEol":true,"eolFrom":"2026-08-20","latest":{"name":"1.97.1","date":"2026-07-16"}},
{"name":"1.96","releaseDate":"2026-05-28","isLts":false,"isEoas":false,"isEol":true,"eolFrom":"2026-07-09","latest":{"name":"1.96.1","date":"2026-06-30"}},
{"name":"1.95","releaseDate":"2026-04-16","isLts":false,"isEoas":false,"isEol":true,"eolFrom":"2026-05-28","latest":{"name":"1.95.0","date":"2026-04-16"}}
]}}`

// RustReleasesEmpty is an endoflife.date v1 response for the rust
// product that lists no release, as a degraded upstream might answer.
const RustReleasesEmpty = `{"schema_version":"1.2.0","result":{"name":"rust","releases":[]}}`

// RustChannel is the part of the stable channel manifest the module
// reads, as static.rust-lang.org served it for 1.99.1.
const RustChannel = "manifest-version = \"2\"\ndate = \"2026-10-15\"\n\n" +
	"[pkg.rust]\nversion = \"1.99.1 (0123456789 2026-10-15)\"\n"

// RustChannelNewer is RustChannel once 1.100.0 ships: the channel
// serves a release endoflife.date may not list yet.
const RustChannelNewer = "manifest-version = \"2\"\ndate = \"2026-11-12\"\n\n" +
	"[pkg.rust]\nversion = \"1.100.0 (0123456789 2026-11-12)\"\n"

// DefaultRustReleasesURL is the endoflife.date URL the module reads
// the rust product from when no base URL is overridden.
var DefaultRustReleasesURL = defaultReleasesURL(rustReleasesPath)

// DefaultRustChannelURL is the URL the module reads the Rust stable
// channel manifest from when no base URL is overridden.
var DefaultRustChannelURL = (&url.URL{Scheme: "https", Host: "static.rust-lang.org", Path: rustChannelPath}).String()

const (
	rustReleasesPath   = "/products/rust/"
	rustChannelPath    = "/dist/channel-rust-stable.toml"
	goReleasesPath     = "/products/go/"
	goManifestPath     = "/actions/go-versions/main/versions-manifest.json"
	pythonReleasesPath = "/products/python/"
	pythonManifestPath = "/actions/python-versions/main/versions-manifest.json"
)

func defaultReleasesURL(path string) string {
	return (&url.URL{Scheme: "https", Host: "endoflife.date", Path: "/api/v1" + path}).String()
}

// Upstream is a test server standing in for endoflife.date and one
// more source of a tool, serving that tool's release facts. For Go and
// Python the second source is the actions versions manifest: use URL
// as both Settings.EndOfLifeBaseURL and Settings.ManifestBaseURL. For
// Rust it is the stable channel manifest: use URL as both
// Settings.EndOfLifeBaseURL and Settings.RustBaseURL. The manifest
// methods act on the second source either way.
type Upstream struct {
	URL            string
	requests       atomic.Int64
	status         atomic.Int64
	manifestStatus atomic.Int64
	manifestStall  atomic.Int64

	releasesPath string
	manifestPath string

	mu       sync.Mutex
	releases string
	manifest string
}

// GoReleasesEmpty is an endoflife.date v1 response for the go product
// that lists no release, as a degraded upstream might answer.
const GoReleasesEmpty = `{"schema_version":"1.2.0","result":{"name":"go","releases":[]}}`

// ServeReleases makes later requests for the tool's endoflife.date
// product answer body in place of the recorded releases.
func (u *Upstream) ServeReleases(body string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.releases = body
}

func (u *Upstream) currentReleases() string {
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

// ServeManifest makes later requests for the tool's manifest answer
// body in place of the recorded manifest.
func (u *Upstream) ServeManifest(body string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.manifest = body
}

func (u *Upstream) currentManifest() string {
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

// FailManifest makes later requests for the tool's manifest alone
// answer status, as a manifest that moved or broke would; zero restores
// the manifest body.
func (u *Upstream) FailManifest(status int) {
	u.manifestStatus.Store(int64(status))
}

// StallManifest makes later requests for the tool's manifest alone
// wait d, or until the client gives up, before answering, as a manifest
// host that times out would; zero restores prompt answers.
func (u *Upstream) StallManifest(d time.Duration) {
	u.manifestStall.Store(int64(d))
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
// or the bodies ServeReleases and ServeManifest set; any other path
// answers 404. The server stops when the test ends.
func NewGoUpstream(t *testing.T) *Upstream {
	t.Helper()
	return newUpstream(t, &Upstream{
		releasesPath: goReleasesPath,
		manifestPath: goManifestPath,
		releases:     GoReleases,
		manifest:     GoManifest,
	})
}

// NewPythonUpstream starts an Upstream serving PythonReleases and
// PythonManifest, or the bodies ServeReleases and ServeManifest set;
// any other path answers 404. The server stops when the test ends.
func NewPythonUpstream(t *testing.T) *Upstream {
	t.Helper()
	return newUpstream(t, &Upstream{
		releasesPath: pythonReleasesPath,
		manifestPath: pythonManifestPath,
		releases:     PythonReleases,
		manifest:     PythonManifest,
	})
}

// NewRustUpstream starts an Upstream serving RustReleases and
// RustChannel, or the bodies ServeReleases and ServeManifest set; any
// other path answers 404. The server stops when the test ends.
func NewRustUpstream(t *testing.T) *Upstream {
	t.Helper()
	return newUpstream(t, &Upstream{
		releasesPath: rustReleasesPath,
		manifestPath: rustChannelPath,
		releases:     RustReleases,
		manifest:     RustChannel,
	})
}

// CacheReleases stores body as an HTTP 200 answer for the tool's
// endoflife.date product on u in the cache cache.Open resolves,
// fetched at fetchedAt. Point $SUPPORTED_VERSIONS_CACHE_DIR at a test
// directory first.
func (u *Upstream) CacheReleases(t *testing.T, body string, fetchedAt time.Time) {
	t.Helper()
	CacheResponse(t, u.URL+u.releasesPath, body, fetchedAt)
}

func newUpstream(t *testing.T, u *Upstream) *Upstream {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u.requests.Add(1)
		if status := u.status.Load(); status != 0 {
			w.WriteHeader(int(status))
			return
		}
		switch r.URL.Path {
		case u.releasesPath:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(u.currentReleases()))
		case u.manifestPath:
			if stall := time.Duration(u.manifestStall.Load()); stall > 0 {
				select {
				case <-r.Context().Done():
					return
				case <-time.After(stall):
				}
			}
			if status := u.manifestStatus.Load(); status != 0 {
				w.WriteHeader(int(status))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(u.currentManifest()))
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
	CacheResponse(t, DefaultGoReleasesURL, body, fetchedAt)
}

// CacheMalformedPythonReleases stores a malformed HTTP 200 body for
// DefaultPythonReleasesURL in the cache cache.Open resolves, as
// CacheMalformedGoReleases does for Go.
func CacheMalformedPythonReleases(t *testing.T, fetchedAt time.Time) {
	t.Helper()
	CacheResponse(t, DefaultPythonReleasesURL, "<html>not json</html>", fetchedAt)
}

// CacheManifest stores body as an HTTP 200 answer for the tool's
// manifest on u in the cache cache.Open resolves, fetched at
// fetchedAt, as an earlier online step would have left it. Point
// $SUPPORTED_VERSIONS_CACHE_DIR at a test directory first.
func (u *Upstream) CacheManifest(t *testing.T, body string, fetchedAt time.Time) {
	t.Helper()
	CacheResponse(t, u.URL+u.manifestPath, body, fetchedAt)
}

// CacheResponse stores body as an HTTP 200 answer for rawURL in the
// cache cache.Open resolves, fetched at fetchedAt, as an earlier
// online step would have left it. Point $SUPPORTED_VERSIONS_CACHE_DIR
// at a test directory first.
func CacheResponse(t *testing.T, rawURL, body string, fetchedAt time.Time) {
	t.Helper()
	c, err := cache.Open()
	if err != nil {
		t.Fatalf("cache.Open: %v", err)
	}
	defer func() { _ = c.Close() }()
	err = c.Put(cache.Key(http.MethodGet, rawURL, nil), cache.Entry{
		Method:    http.MethodGet,
		URL:       rawURL,
		Status:    http.StatusOK,
		FetchedAt: fetchedAt,
		Body:      []byte(body),
	})
	if err != nil {
		t.Fatalf("cache.Put: %v", err)
	}
}
