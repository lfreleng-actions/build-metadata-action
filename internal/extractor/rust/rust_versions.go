// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package rust

import (
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
)

// rustVersionCache holds cached Rust version information
var rustVersionCache struct {
	sync.RWMutex
	versions  []string
	fetchedAt time.Time
	cacheTTL  time.Duration
}

// rustStableChannelURL is the rustup manifest of the current stable
// release. Tests point it at a local server.
var rustStableChannelURL = "https://static.rust-lang.org/dist/channel-rust-stable.toml"

// now is the clock behind the offline fallback; tests replace it.
var now = time.Now

// The offline fallback extrapolates the current stable release from a
// verified one. Rust has shipped a stable minor release every six weeks
// since 1.1, so the anchor needs updating only if that train changes.
// Verified against static.rust-lang.org: 1.99.0 shipped on 2026-10-01.
const (
	fallbackAnchorMinor = 99
	releaseInterval     = 6 * 7 * 24 * time.Hour
	// releaseGrace delays counting a release until its day has passed,
	// since the artefacts publish during the day, not at midnight UTC.
	releaseGrace = 24 * time.Hour
)

var fallbackAnchorDate = time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)

// fallbackStableVersion estimates the current stable release ("1.N")
// from the release train, for use when the channel manifest is
// unreachable. It never reports a release older than the anchor.
func fallbackStableVersion(at time.Time) string {
	minor := fallbackAnchorMinor
	if elapsed := at.Sub(fallbackAnchorDate) - releaseGrace; elapsed > 0 {
		minor += int(elapsed / releaseInterval)
	}
	return fmt.Sprintf("1.%d", minor)
}

func init() {
	// Rust releases every 6 weeks, so a 72-hour TTL is conservative.
	rustVersionCache.cacheTTL = 72 * time.Hour
}

// fetchRustVersions fetches available Rust versions dynamically from rust-lang.org.
//
// This function queries the official Rust release channel to get the current stable
// version and generates a reasonable testing matrix. It has a 5-second timeout and
// will return an error if the fetch fails, allowing the caller to fall back to
// static version lists.
//
// The function:
// 1. Fetches channel-rust-stable.toml from static.rust-lang.org
// 2. Parses the current stable version (e.g., "1.84.0")
// 3. Generates a range of the last 6 minor versions (roughly 9 months)
// 4. Always includes "stable" for testing against the latest release
//
// This ensures version matrices stay current without manual updates, while the
// 5-second timeout and error handling prevent workflow failures if the API is
// unreachable. The caller should always have a fallback strategy.
func fetchRustVersions() ([]string, error) {
	rustVersionCache.RLock()
	if len(rustVersionCache.versions) > 0 && time.Since(rustVersionCache.fetchedAt) < rustVersionCache.cacheTTL {
		// Return a defensive copy so callers cannot mutate the shared cache.
		versions := cloneVersions(rustVersionCache.versions)
		rustVersionCache.RUnlock()
		return versions, nil
	}
	rustVersionCache.RUnlock()

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	resp, err := client.Get(rustStableChannelURL)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch Rust versions: status %d", resp.StatusCode)
	}

	var data map[string]interface{}
	if _, err := toml.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	if pkg, ok := data["pkg"].(map[string]interface{}); ok {
		if rust, ok := pkg["rust"].(map[string]interface{}); ok {
			if version, ok := rust["version"].(string); ok {
				// Version string format is "1.XX.Y (hash date)".
				re := regexp.MustCompile(`^(\d+\.\d+)`)
				if matches := re.FindStringSubmatch(version); len(matches) > 1 {
					stableVersion := matches[1]
					versions := generateVersionRange(stableVersion)

					// Cache an independent copy so a caller mutating the
					// returned slice cannot corrupt the shared cache.
					rustVersionCache.Lock()
					rustVersionCache.versions = cloneVersions(versions)
					rustVersionCache.fetchedAt = time.Now()
					rustVersionCache.Unlock()

					return versions, nil
				}
			}
		}
	}

	return nil, fmt.Errorf("could not parse version from response")
}

// generateVersionRange generates a range of versions from a stable version.
//
// Given a stable version (e.g., "1.84"), this generates approximately the last
// 6 minor versions plus "stable". This provides ~9 months of version coverage,
// which is a reasonable testing range that balances thoroughness with CI time.
func generateVersionRange(stableVersion string) []string {
	major, minor, ok := parseMajorMinor(stableVersion)
	if !ok {
		// Non-numeric or incomplete version: fall back to the minimal
		// matrix rather than emit a bogus "0.0" entry.
		return []string{stableVersion, "stable"}
	}

	versions := []string{}
	for i := 0; i < 6 && minor-i >= 0; i++ {
		versions = append(versions, fmt.Sprintf("%d.%d", major, minor-i))
	}

	// "stable" tracks whatever release is currently current.
	versions = append(versions, "stable")

	return versions
}

// parseMajorMinor parses the leading "major.minor" of a version string.
// It returns ok=false when either component is absent or non-numeric so
// callers can choose a safe fallback instead of treating the value as 0.0.
func parseMajorMinor(version string) (int, int, bool) {
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return 0, 0, false
	}
	major, errMajor := strconv.Atoi(parts[0])
	minor, errMinor := strconv.Atoi(parts[1])
	if errMajor != nil || errMinor != nil {
		return 0, 0, false
	}
	return major, minor, true
}

// cloneVersions returns an independent copy of the given slice so that
// the shared version cache cannot be mutated through a returned slice.
func cloneVersions(versions []string) []string {
	if versions == nil {
		return nil
	}
	out := make([]string, len(versions))
	copy(out, versions)
	return out
}

// generateRustVersionMatrix generates a list of Rust versions from MSRV.
//
// The matrix holds the MSRV, the six most recent stable minor releases
// at or above it, and "stable". The recent releases come from the live
// stable channel manifest; when that is unreachable (network issues,
// timeout) they are extrapolated from the six-week release train, so an
// offline runner gets the same shape of matrix rather than a stale list.
func generateRustVersionMatrix(msrv string) []string {
	versions, err := fetchRustVersions()
	if err != nil || len(versions) == 0 {
		versions = generateVersionRange(fallbackStableVersion(now()))
	}
	return filterVersionsFromMSRV(msrv, versions)
}

// filterVersionsFromMSRV filters versions to only include those >= MSRV.
//
// This ensures we don't test against versions older than the project's
// Minimum Supported Rust Version (MSRV), as those tests would fail.
//
// On success the MSRV is included as the first version in the result,
// followed by newer versions in ascending order, with special versions
// (stable, beta, nightly) at the end. If the MSRV cannot be parsed, the
// input slice is returned unchanged (no filtering or reordering).
func filterVersionsFromMSRV(msrv string, allVersions []string) []string {
	filtered := []string{}

	msrvMajor, msrvMinor, ok := parseMajorMinor(msrv)
	if !ok {
		// Malformed MSRV: return the unfiltered set rather than treat MSRV
		// as 0.0 and admit versions that should be filtered out.
		return allVersions
	}

	for _, version := range allVersions {
		if version == "stable" || version == "beta" || version == "nightly" {
			filtered = append(filtered, version)
			continue
		}

		major, minor, ok := parseMajorMinor(version)
		if !ok {
			continue
		}

		if major > msrvMajor || (major == msrvMajor && minor >= msrvMinor) {
			filtered = append(filtered, version)
		}
	}

	numericVersions := []string{}
	specialVersions := []string{}
	for _, v := range filtered {
		if v == "stable" || v == "beta" || v == "nightly" {
			specialVersions = append(specialVersions, v)
		} else {
			numericVersions = append(numericVersions, v)
		}
	}

	sort.Slice(numericVersions, func(i, j int) bool {
		iParts := strings.Split(numericVersions[i], ".")
		jParts := strings.Split(numericVersions[j], ".")

		if len(iParts) < 2 || len(jParts) < 2 {
			return numericVersions[i] < numericVersions[j]
		}

		iMajor, _ := strconv.Atoi(iParts[0])
		jMajor, _ := strconv.Atoi(jParts[0])
		if iMajor != jMajor {
			return iMajor < jMajor
		}

		iMinor, _ := strconv.Atoi(iParts[1])
		jMinor, _ := strconv.Atoi(jParts[1])
		return iMinor < jMinor
	})

	result := []string{msrv}
	for _, v := range numericVersions {
		if v != msrv {
			result = append(result, v)
		}
	}
	result = append(result, specialVersions...)

	return result
}

// generateRustVersionMatrixFromEdition generates versions based on Rust edition
func generateRustVersionMatrixFromEdition(edition string) []string {
	switch edition {
	case "2021":
		return []string{"1.56", "stable"}
	case "2018":
		return []string{"1.31", "stable"}
	case "2015":
		return []string{"1.0", "stable"}
	default:
		return []string{"stable"}
	}
}
