// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package pyversions

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

const (
	// endOfLifeAPIURLEnv overrides the default endoflife.date endpoint
	// (for mirrors, proxies, or tests) when set to a non-empty value.
	endOfLifeAPIURLEnv = "BUILD_METADATA_PYTHON_EOL_API_URL"
	// endOfLifeAPIHost is the endoflife.date host serving public release
	// data; combined with the per-language path to form the endpoint.
	endOfLifeAPIHost = "endoflife.date"
	// defaultEndOfLifeAPIURL is the public endoflife.date endpoint for
	// Python release data, used when endOfLifeAPIURLEnv is unset.
	defaultEndOfLifeAPIURL = "https://" + endOfLifeAPIHost + "/api/python.json"
	// DefaultTimeout is the default HTTP timeout for API calls
	DefaultTimeout = 6 * time.Second
	// DefaultMaxRetries is the default number of retry attempts
	DefaultMaxRetries = 2
)

// endOfLifeAPIURL returns the endoflife.date endpoint to query, honoring
// the endOfLifeAPIURLEnv override and falling back to the public default.
// The override is only used when it parses as a well-formed http/https
// URL; a malformed value is rejected in favour of the default. Note this
// validates only the URL shape, not the target host.
func endOfLifeAPIURL() string {
	override := os.Getenv(endOfLifeAPIURLEnv)
	if override == "" {
		return defaultEndOfLifeAPIURL
	}
	if isValidHTTPURL(override) {
		return override
	}
	return defaultEndOfLifeAPIURL
}

// isValidHTTPURL reports whether raw parses as an absolute http/https URL
// with a host, the minimum shape safe to hand to http.Client.Get. It does
// not restrict which host may be contacted.
func isValidHTTPURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

// EOLData represents the end-of-life information for a Python version
type EOLData struct {
	Cycle             string      `json:"cycle"`             // Version number (e.g., "3.11")
	ReleaseDate       string      `json:"releaseDate"`       // Release date
	EOL               interface{} `json:"eol"`               // End of life date or boolean
	Latest            string      `json:"latest"`            // Latest patch version
	LatestReleaseDate string      `json:"latestReleaseDate"` // Latest patch release date
	LTS               bool        `json:"lts"`               // Long term support flag
	Support           interface{} `json:"support"`           // Support end date or boolean
}

// EOLClient handles fetching and caching Python EOL data
type EOLClient struct {
	httpClient *http.Client
	timeout    time.Duration
	maxRetries int
	cachedData []EOLData
	cacheTime  time.Time
}

// NewEOLClient creates a new EOL API client.
//
// Semantics for the sentinel values:
//
//   - `timeout <= 0` selects DefaultTimeout. Go's `http.Client{Timeout: 0}`
//     means "no timeout" -- the client would wait indefinitely for a
//     hung peer -- so this overload exists to keep callers from
//     accidentally configuring an unbounded request budget.
//   - `maxRetries < 0` selects DefaultMaxRetries. `maxRetries == 0`
//     means "no retries" -- callers can therefore explicitly opt out of
//     retry behaviour by passing zero (previously zero was ambiguous
//     with "use the default").
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

// FetchEOLData fetches Python EOL data from the API with retries
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
	resp, err := c.httpClient.Get(endOfLifeAPIURL())
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

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

// GetSupportedVersions extracts currently supported Python versions at or
// above the configured baseline (see baselineMajor/baselineMinor). Returns
// versions that are not end-of-life. The baseline is bumped explicitly
// when a Python release reaches EOL; callers therefore never get stuck
// building against an interpreter that no longer receives security
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

// IsVersionEOL checks if a specific Python version is end-of-life
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

// baselineMajor, baselineMinor define the OLDEST Python release this
// action will recognise as a supportable interpreter. latestMajor,
// latestMinor define the NEWEST. They are the only version literals
// in this package controlling the supported range: bumping either bound
// when Python's release cadence changes (a release reaching EOL, or a
// new minor shipping) is a one-line change here. Every other consumer
// in this repository derives from these constants -- see
// `GetFallbackVersions`, `Baseline`, `Latest`, and the Python extractor's
// `supportedPythonVersions` slice, which is initialised from
// `GetFallbackVersions()`.
//
// The naming is intentionally version-agnostic so the surrounding
// helpers (`isVersionBaselineOrLater`, `GetFallbackVersions`, ...) do
// not need to be renamed on every annual cadence.
const (
	baselineMajor = 3
	baselineMinor = 10
	latestMajor   = 3
	latestMinor   = 14
)

// Baseline returns the oldest supported Python version as a major.minor
// string (e.g. "3.10"). It is the single exported representation of
// `baselineMajor.baselineMinor` for callers outside the pyversions
// package, so the baseline floor stays a one-line change here.
func Baseline() string {
	return fmt.Sprintf("%d.%d", baselineMajor, baselineMinor)
}

// Latest returns the newest supported Python version as a major.minor
// string (e.g. "3.14"). The companion to Baseline; bumping the upper
// bound is a one-line change to `latestMinor` (or `latestMajor`).
func Latest() string {
	return fmt.Sprintf("%d.%d", latestMajor, latestMinor)
}

// GetFallbackVersions returns the canonical list of supported Python
// versions at or above the baseline and at or below the latest bound,
// in ascending order. Used when the live endoflife.date API is
// unavailable or in offline mode, and as the initialiser for the Python
// extractor's `supportedPythonVersions` slice.
//
// The list is computed dynamically from `baselineMajor`, `baselineMinor`,
// `latestMajor`, and `latestMinor` so that a Python release cadence bump
// requires a single-line constant change rather than hand-editing the
// returned slice. For now the action only supports the Python 3.x line;
// the loop intentionally treats `latestMajor != baselineMajor` as a
// future extension and panics rather than silently returning a wrong set.
func GetFallbackVersions() []string {
	if baselineMajor != latestMajor {
		// Crossing a major boundary requires bespoke iteration logic
		// (Python 3 ended at 3.x, Python 4 would restart at 4.0). The
		// action does not currently support a non-3 major; bump this
		// helper if/when Python 4 ships and is supportable.
		panic(fmt.Sprintf(
			"pyversions: cross-major fallback range %d.%d..%d.%d not implemented",
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
