// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

// Package goversions provides EOL-aware Go release information sourced
// from endoflife.date, mirroring the structure and API shape of the
// internal/pyversions package so the two stay easy to maintain in
// tandem.
package goversions

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	// EndOfLifeAPIURL is the API endpoint for Go EOL data
	// aislop-ignore-next-line ai-slop/hardcoded-url -- stable public data API endpoint, defined as a named constant
	EndOfLifeAPIURL = "https://endoflife.date/api/go.json"
	// DefaultTimeout is the default HTTP timeout for API calls
	DefaultTimeout = 6 * time.Second
	// DefaultMaxRetries is the default number of retry attempts
	DefaultMaxRetries = 2
)

// EOLData represents the end-of-life information for a Go release cycle
type EOLData struct {
	Cycle             string      `json:"cycle"`             // Version number (e.g., "1.25")
	ReleaseDate       string      `json:"releaseDate"`       // Release date
	EOL               interface{} `json:"eol"`               // End of life date or boolean
	Latest            string      `json:"latest"`            // Latest patch version
	LatestReleaseDate string      `json:"latestReleaseDate"` // Latest patch release date
	LTS               bool        `json:"lts"`               // Long term support flag
	Support           interface{} `json:"support"`           // Support end date or boolean
}

// EOLClient handles fetching and caching Go EOL data
type EOLClient struct {
	httpClient *http.Client
	timeout    time.Duration
	maxRetries int
	cachedData []EOLData
	cacheTime  time.Time
}

// NewEOLClient creates a new EOL API client.
//
// Semantics for the sentinel values (matching pyversions.NewEOLClient):
//
//   - `timeout <= 0` selects DefaultTimeout. Go's `http.Client{Timeout: 0}`
//     means "no timeout" -- the client would wait indefinitely for a
//     hung peer -- so this overload exists to keep callers from
//     accidentally configuring an unbounded request budget.
//   - `maxRetries < 0` selects DefaultMaxRetries. `maxRetries == 0`
//     means "no retries" -- callers can therefore explicitly opt out of
//     retry behaviour by passing zero.
func NewEOLClient(timeout time.Duration, maxRetries int) *EOLClient {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	if maxRetries < 0 {
		maxRetries = DefaultMaxRetries
	}

	return &EOLClient{
		httpClient: &http.Client{
			Timeout: timeout,
		},
		timeout:    timeout,
		maxRetries: maxRetries,
	}
}

// FetchEOLData fetches Go EOL data from the API with retries
func (c *EOLClient) FetchEOLData() ([]EOLData, error) {
	// Return cached data if still fresh (less than 1 hour old)
	if c.cachedData != nil && time.Since(c.cacheTime) < time.Hour {
		return c.cachedData, nil
	}

	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 1s, 2s, 4s...
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			time.Sleep(backoff)
		}

		data, err := c.fetchOnce()
		if err == nil {
			c.cachedData = data
			c.cacheTime = time.Now()
			return data, nil
		}
		lastErr = err
	}

	return nil, fmt.Errorf("failed to fetch EOL data after %d retries: %w", c.maxRetries, lastErr)
}

// fetchOnce performs a single attempt to fetch EOL data
func (c *EOLClient) fetchOnce() ([]EOLData, error) {
	resp, err := c.httpClient.Get(EndOfLifeAPIURL)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var data []EOLData
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("received empty data array")
	}

	// Verify each entry has a cycle field
	for i, entry := range data {
		if entry.Cycle == "" {
			return nil, fmt.Errorf("entry %d missing cycle field", i)
		}
	}

	return data, nil
}

// GetSupportedVersions extracts currently supported Go versions at or
// above the configured baseline (see baselineMajor/baselineMinor).
// Returns versions that are not end-of-life. The baseline is bumped
// explicitly when a Go release reaches EOL; callers therefore never get
// stuck building against a toolchain that no longer receives security
// updates regardless of how stale the upstream endoflife.date response
// might be.
func (c *EOLClient) GetSupportedVersions() ([]string, error) {
	data, err := c.FetchEOLData()
	if err != nil {
		return nil, err
	}

	var supported []string

	for _, entry := range data {
		// Only include versions at or above the baseline
		if !isVersionBaselineOrLater(entry.Cycle) {
			continue
		}

		// Check if version is EOL
		if isEOL, _ := c.IsVersionEOL(entry.Cycle, data); !isEOL {
			supported = append(supported, entry.Cycle)
		}
	}

	return supported, nil
}

// IsVersionEOL checks if a specific Go version is end-of-life
// Returns (isEOL bool, eolDate string)
func (c *EOLClient) IsVersionEOL(version string, data []EOLData) (bool, string) {
	now := time.Now()

	for _, entry := range data {
		if entry.Cycle != version {
			continue
		}

		switch eol := entry.EOL.(type) {
		case string:
			// Parse the date string (format: YYYY-MM-DD)
			eolDate, err := time.Parse("2006-01-02", eol)
			if err != nil {
				// If we can't parse, assume not EOL
				return false, ""
			}
			// Check if current date is past EOL date
			if now.After(eolDate) || now.Equal(eolDate) {
				return true, eol
			}
			return false, ""

		case bool:
			// If EOL is boolean true, it's EOL (no specific date)
			if eol {
				return true, "true"
			}
			return false, ""

		default:
			// Unknown type, assume not EOL
			return false, ""
		}
	}

	// Version not found in data, assume not EOL
	return false, ""
}

// baselineMajor, baselineMinor define the OLDEST Go release this action
// will recognise as a supportable toolchain. latestMajor, latestMinor
// define the NEWEST. They are the only version literals in this package
// controlling the supported range. Go's release support policy keeps
// exactly the two most recent releases supported (each release is
// supported until two newer major releases ship), so the fallback range
// is deliberately narrow: as of mid-2026 the supported releases are
// 1.25 and 1.26. Bumping either bound when a new Go release ships (or
// an old one drops out of support) is a one-line change here; every
// consumer in this repository derives from these constants -- see
// `GetFallbackVersions`, `Baseline`, `Latest`, and the Go extractor's
// supported-version resolution in `internal/extractor/golang`.
//
// The naming is intentionally version-agnostic so the surrounding
// helpers (`isVersionBaselineOrLater`, `GetFallbackVersions`, ...) do
// not need to be renamed on every release cadence.
const (
	baselineMajor = 1
	baselineMinor = 25
	latestMajor   = 1
	latestMinor   = 26
)

// Baseline returns the oldest supported Go version as a major.minor
// string (e.g. "1.25"). It is the single exported representation of
// `baselineMajor.baselineMinor` for callers outside the goversions
// package, so the baseline floor stays a one-line change here.
func Baseline() string {
	return fmt.Sprintf("%d.%d", baselineMajor, baselineMinor)
}

// Latest returns the newest supported Go version as a major.minor
// string (e.g. "1.26"). The companion to Baseline; bumping the upper
// bound is a one-line change to `latestMinor` (or `latestMajor`).
func Latest() string {
	return fmt.Sprintf("%d.%d", latestMajor, latestMinor)
}

// GetFallbackVersions returns the canonical list of supported Go
// versions at or above the baseline and at or below the latest bound,
// in ascending order. Used when the live endoflife.date API is
// unavailable, and as the initialiser for the Go extractor's
// supported-version set.
//
// The list is computed dynamically from `baselineMajor`, `baselineMinor`,
// `latestMajor`, and `latestMinor` so that a Go release cadence bump
// requires a single-line constant change rather than hand-editing the
// returned slice. For now the action only supports the Go 1.x line; the
// loop intentionally treats `latestMajor != baselineMajor` as a future
// extension and panics rather than silently returning a wrong set.
func GetFallbackVersions() []string {
	if baselineMajor != latestMajor {
		// Crossing a major boundary requires bespoke iteration logic
		// (Go 1 minors are open-ended, a Go 2 line would restart at
		// 2.0). The action does not currently support a non-1 major;
		// bump this helper if/when Go 2 ships and is supportable.
		panic(fmt.Sprintf(
			"goversions: cross-major fallback range %d.%d..%d.%d not implemented",
			baselineMajor, baselineMinor, latestMajor, latestMinor))
	}
	versions := make([]string, 0, latestMinor-baselineMinor+1)
	for minor := baselineMinor; minor <= latestMinor; minor++ {
		versions = append(versions, fmt.Sprintf("%d.%d", baselineMajor, minor))
	}
	return versions
}

// isVersionBaselineOrLater checks whether a version string is at or
// above the configured baseline (baselineMajor.baselineMinor). The name
// is intentionally version-agnostic so the function does not need to be
// renamed when the baseline floor moves; bump the constants instead.
func isVersionBaselineOrLater(version string) bool {
	// Match pattern: X.Y where X is the major and Y is the minor.
	var major, minor int
	n, err := fmt.Sscanf(version, "%d.%d", &major, &minor)
	if err != nil || n != 2 {
		return false
	}

	// Major version strictly above the baseline major: accept any minor.
	if major > baselineMajor {
		return true
	}

	// Same major: accept when minor is at or above the baseline minor.
	if major == baselineMajor && minor >= baselineMinor {
		return true
	}

	return false
}
