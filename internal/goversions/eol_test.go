// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package goversions

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewEOLClient(t *testing.T) {
	t.Run("with default values", func(t *testing.T) {
		// Sentinel values: timeout <= 0 and maxRetries < 0 select the
		// library defaults. (maxRetries == 0 explicitly means "no
		// retries" so callers can opt out of retry behaviour.)
		client := NewEOLClient(0, -1)
		assert.NotNil(t, client)
		assert.Equal(t, DefaultTimeout, client.timeout)
		assert.Equal(t, DefaultMaxRetries, client.maxRetries)
		assert.NotNil(t, client.httpClient)
	})

	t.Run("maxRetries=0 means no retries", func(t *testing.T) {
		client := NewEOLClient(0, 0)
		assert.NotNil(t, client)
		assert.Equal(t, 0, client.maxRetries,
			"zero must NOT be remapped to DefaultMaxRetries; "+
				"callers explicitly opt out of retry behaviour with 0")
	})

	t.Run("with custom values", func(t *testing.T) {
		customTimeout := 10 * time.Second
		customRetries := 5
		client := NewEOLClient(customTimeout, customRetries)
		assert.NotNil(t, client)
		assert.Equal(t, customTimeout, client.timeout)
		assert.Equal(t, customRetries, client.maxRetries)
	})
}

func TestFetchEOLDataCaching(t *testing.T) {
	client := NewEOLClient(5*time.Second, 1)

	// Set cached data
	testData := []EOLData{
		{Cycle: "1.26", EOL: false},
	}
	client.cachedData = testData
	client.cacheTime = time.Now()

	// Fetch should return cached data without touching the network
	data, err := client.FetchEOLData()
	require.NoError(t, err)
	assert.Equal(t, testData, data)
}

func TestGetSupportedVersions(t *testing.T) {
	client := NewEOLClient(5*time.Second, 1)

	t.Run("filters EOL versions", func(t *testing.T) {
		pastDate := time.Now().AddDate(0, 0, -30).Format("2006-01-02")
		futureDate := time.Now().AddDate(1, 0, 0).Format("2006-01-02")

		// pastDate/futureDate are computed relative to time.Now()
		// rather than being pinned to real-world EOL dates; the inline
		// notes below only flag these cycles as "in the past" for
		// filtering purposes.
		client.cachedData = []EOLData{
			{Cycle: "1.26", EOL: futureDate},
			{Cycle: "1.25", EOL: futureDate},
			{Cycle: "1.24", EOL: pastDate}, // EOL (date in the past)
			{Cycle: "1.23", EOL: true},     // EOL (boolean form)
		}
		client.cacheTime = time.Now()

		supported, err := client.GetSupportedVersions()
		require.NoError(t, err)

		assert.Contains(t, supported, "1.26")
		assert.Contains(t, supported, "1.25")
		assert.NotContains(t, supported, "1.24")
		assert.NotContains(t, supported, "1.23")
	})

	t.Run("filters out pre-baseline versions", func(t *testing.T) {
		futureDate := time.Now().AddDate(1, 0, 0).Format("2006-01-02")

		client.cachedData = []EOLData{
			{Cycle: "1.26", EOL: futureDate},
			{Cycle: "1.25", EOL: futureDate},
			{Cycle: "1.24", EOL: futureDate}, // Should be filtered (1.25 floor)
			{Cycle: "1.22", EOL: futureDate}, // Should be filtered
		}
		client.cacheTime = time.Now()

		supported, err := client.GetSupportedVersions()
		require.NoError(t, err)

		assert.Contains(t, supported, "1.26")
		assert.Contains(t, supported, "1.25")
		assert.NotContains(t, supported, "1.24")
		assert.NotContains(t, supported, "1.22")
	})
}

func TestIsVersionEOL(t *testing.T) {
	client := NewEOLClient(5*time.Second, 1)
	pastDate := time.Now().AddDate(0, 0, -30).Format("2006-01-02")
	futureDate := time.Now().AddDate(1, 0, 0).Format("2006-01-02")

	testData := []EOLData{
		{Cycle: "1.26", EOL: futureDate},
		{Cycle: "1.25", EOL: pastDate},
		{Cycle: "1.24", EOL: true},
		{Cycle: "1.23", EOL: false},
	}

	t.Run("version with future EOL date", func(t *testing.T) {
		isEOL, date := client.IsVersionEOL("1.26", testData)
		assert.False(t, isEOL)
		assert.Empty(t, date)
	})

	t.Run("version with past EOL date", func(t *testing.T) {
		isEOL, date := client.IsVersionEOL("1.25", testData)
		assert.True(t, isEOL)
		assert.Equal(t, pastDate, date)
	})

	t.Run("version with boolean EOL true", func(t *testing.T) {
		isEOL, date := client.IsVersionEOL("1.24", testData)
		assert.True(t, isEOL)
		assert.Equal(t, "true", date)
	})

	t.Run("version with boolean EOL false", func(t *testing.T) {
		isEOL, date := client.IsVersionEOL("1.23", testData)
		assert.False(t, isEOL)
		assert.Empty(t, date)
	})

	t.Run("version not in data", func(t *testing.T) {
		isEOL, date := client.IsVersionEOL("1.99", testData)
		assert.False(t, isEOL)
		assert.Empty(t, date)
	})
}

func TestGetFallbackVersions(t *testing.T) {
	versions := GetFallbackVersions()

	assert.NotEmpty(t, versions)
	// Go's support policy keeps the latest two releases supported; as
	// of mid-2026 those are 1.25 and 1.26.
	assert.Equal(t, []string{"1.25", "1.26"}, versions)

	// Should not contain EOL versions. 1.24 dropped out of support
	// when 1.26 shipped.
	assert.NotContains(t, versions, "1.24")
	assert.NotContains(t, versions, "1.23")
	assert.NotContains(t, versions, "1.22")
}

func TestBaselineAndLatest(t *testing.T) {
	versions := GetFallbackVersions()
	require.NotEmpty(t, versions)
	assert.Equal(t, versions[0], Baseline(),
		"Baseline() must match the first fallback version")
	assert.Equal(t, versions[len(versions)-1], Latest(),
		"Latest() must match the last fallback version")
}

func TestIsVersionBaselineOrLater(t *testing.T) {
	tests := []struct {
		version  string
		expected bool
	}{
		{"1.25", true},
		{"1.26", true},
		{"1.27", true},
		{"1.99", true},
		{"2.0", true},
		{"1.24", false},
		{"1.23", false},
		{"1.22", false},
		{"invalid", false},
		{"1", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			result := isVersionBaselineOrLater(tt.version)
			assert.Equal(t, tt.expected, result, "version %s", tt.version)
		})
	}
}

func TestEOLDataUnmarshal(t *testing.T) {
	t.Run("unmarshal with string EOL", func(t *testing.T) {
		jsonData := `{
			"cycle": "1.25",
			"releaseDate": "2025-08-12",
			"eol": "2026-08-01",
			"latest": "1.25.4",
			"latestReleaseDate": "2026-01-15",
			"lts": false,
			"support": "2026-08-01"
		}`

		var data EOLData
		err := json.Unmarshal([]byte(jsonData), &data)
		require.NoError(t, err)

		assert.Equal(t, "1.25", data.Cycle)
		assert.Equal(t, "1.25.4", data.Latest)
		assert.Equal(t, "2026-08-01", data.EOL)
	})

	t.Run("unmarshal with boolean EOL", func(t *testing.T) {
		jsonData := `{
			"cycle": "1.24",
			"releaseDate": "2025-02-11",
			"eol": true,
			"latest": "1.24.10",
			"latestReleaseDate": "2026-01-15",
			"lts": false,
			"support": false
		}`

		var data EOLData
		err := json.Unmarshal([]byte(jsonData), &data)
		require.NoError(t, err)

		assert.Equal(t, "1.24", data.Cycle)
		assert.Equal(t, true, data.EOL)
	})

	t.Run("unmarshal array of EOL data", func(t *testing.T) {
		jsonData := `[
			{"cycle": "1.26", "eol": false, "latest": "1.26.0"},
			{"cycle": "1.25", "eol": false, "latest": "1.25.4"},
			{"cycle": "1.24", "eol": true, "latest": "1.24.10"}
		]`

		var data []EOLData
		err := json.Unmarshal([]byte(jsonData), &data)
		require.NoError(t, err)

		assert.Len(t, data, 3)
		assert.Equal(t, "1.26", data[0].Cycle)
		assert.Equal(t, "1.25", data[1].Cycle)
		assert.Equal(t, "1.24", data[2].Cycle)
	})
}
