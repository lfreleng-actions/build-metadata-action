<!--
SPDX-License-Identifier: Apache-2.0
SPDX-FileCopyrightText: 2025 The Linux Foundation
-->

# Rust Version Caching Implementation

## Overview

The Rust version fetching mechanism now includes an in-memory cache with a
72-hour TTL to avoid unnecessary network calls and improve performance.

## Why 72 Hours?

- **Rust Release Cycle**: Rust releases a new stable version every 6 weeks
  (42 days)
- **Cache Duration**: 72 hours (3 days) is conservative and ensures:
  - CI/CD pipeline runs reuse the same cached data
  - The cache refreshes well before new Rust versions become available
  - Network issues don't impact build reliability for 3 days
  - Performance improvement for projects with 10+ Rust packages

## Implementation Details

### Cache Structure

```go
var rustVersionCache struct {
    sync.RWMutex
    versions   []string
    fetchedAt  time.Time
    cacheTTL   time.Duration
}
```

### Thread Safety

The cache uses `sync.RWMutex` to ensure thread-safe access:

- **Read operations** (`RLock`): Concurrent goroutines can read simultaneously
- **Write operations** (`Lock`): Exclusive access for cache updates

### Cache Flow

1. **Cache Hit** (within TTL):
   - Return cached versions
   - No network call made

2. **Cache Miss** (expired or empty):
   - Fetch from `https://static.rust-lang.org/dist/channel-rust-stable.toml`
   - Parse stable version
   - Generate version range (last 6 minor versions + "stable")
   - Store in cache with current timestamp
   - Return versions

3. **Network Failure**:
   - Fall back to static version map
   - Don't update cache (preserve old data if available)

## Benefits

### Performance

- **First call**: 5-second network timeout (same as before)
- **Later calls**: <1ms (in-memory cache)
- **Monorepo benefit**: Analyzing 10 Rust packages makes 1 network call
  instead of 10

### Reliability

- 72-hour window protects against temporary network issues
- Static fallback ensures builds never fail due to network problems
- Cache persists for the lifetime of the process

### Freshness

- Cache expires every 3 days
- Well within Rust's 6-week release cycle
- Ensures CI/CD tests against current Rust versions

## Testing

Comprehensive test coverage includes:

1. **TestRustVersionCaching**: Verifies cache population and reuse
2. **TestRustVersionCacheExpiration**: Verifies TTL behavior
3. **TestRustVersionCacheConcurrency**: Verifies thread-safe access

All tests pass with both successful and failed network scenarios.

## Cache Lifetime

The cache exists during the GitHub Action execution:

- **Single project**: Cache helps when the action runs more than once
- **Monorepo**: Cache benefits all Rust packages in the repository
- **Workflow**: Cache doesn't persist across workflow runs (intentional -
  keeps data fresh)

## Comparison with Other Approaches

| Approach | Duration | Pros | Cons |
| -------- | -------- | ---- | ---- |
| No cache | N/A | Always fresh | Slow, network dependent |
| 1-hour cache | 1 hour | Fresh | Less performance benefit |
| **72-hour cache** | **3 days** | **Fresh & fast** | **Recommended** |
| 1-week cache | 7 days | Fast | May miss new releases |
| Persistent cache | Until cleared | Fast | Requires cache management |

## Future Enhancements

Potential improvements (not yet implemented):

1. **Persistent disk cache**: Store in `~/.cache/build-metadata-action/`
2. **Environment override**: `RUST_VERSION_CACHE_TTL=24h`
3. **Cache warming**: Pre-fetch on action startup
4. **Metrics**: Log cache hit/miss rates

## Related Files

- Implementation: `internal/extractor/rust/rust.go`
- Tests: `internal/extractor/rust/rust_test.go`
- Fallback data: Static version map in `generateRustVersionMatrix()`
