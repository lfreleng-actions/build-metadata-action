// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package pyversions

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	// EndOfLifeAPIURL is the API endpoint for Python EOL data
	EndOfLifeAPIURL = "https://endoflife.date/api/python.json"
	// DefaultTimeout is the default HTTP timeout for API calls
	DefaultTimeout = 6 * time.Second
	// DefaultMaxRetries is the default number of retry attempts
	DefaultMaxRetries = 2
)

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

// NewEOLClient creates a new EOL API client
func NewEOLClient(timeout time.Duration, maxRetries int) *EOLClient {
	if timeout == 0 {
		timeout = DefaultTimeout
	}
	if maxRetries == 0 {
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
	resp, err := c.httpClient.Get(EndOfLifeAPIURL)
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

	// Validate the data structure
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

// GetSupportedVersions extracts currently supported Python versions (3.9+)
// Returns versions that are not end-of-life
func (c *EOLClient) GetSupportedVersions() ([]string, error) {
	data, err := c.FetchEOLData()
	if err != nil {
		return nil, err
	}

	var supported []string

	for _, entry := range data {
		// Only include Python 3.9 and later
		if !isVersion39OrLater(entry.Cycle) {
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

		// Handle different EOL field types
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

// GetFallbackVersions returns a hardcoded list of supported versions
// Used when the API is unavailable or in offline mode
func GetFallbackVersions() []string {
	// This list should be periodically updated
	// Current as of 2025
	return []string{"3.9", "3.10", "3.11", "3.12", "3.13", "3.14"}
}

// isVersion39OrLater checks if a version string is Python 3.9 or later
func isVersion39OrLater(version string) bool {
	// Match pattern: 3.9, 3.10, 3.11, ... or 4.x, 5.x, etc.
	// We want 3.9+ or 4.0+

	var major, minor int
	n, err := fmt.Sscanf(version, "%d.%d", &major, &minor)
	if err != nil || n != 2 {
		return false
	}

	// Major version 4 or higher
	if major >= 4 {
		return true
	}

	// Python 3.9 or later
	if major == 3 && minor >= 9 {
		return true
	}

	return false
}
