<!--
SPDX-License-Identifier: Apache-2.0
SPDX-FileCopyrightText: 2025 The Linux Foundation
-->

# Rust Version Resolution and Caching

## Overview

The Rust extractor resolves the release lines behind
`rust_rust_version_matrix` and `rust_matrix_json` through the
[supported-versions-action](https://github.com/lfreleng-actions/supported-versions-action)
Go module, by way of `internal/supported`, as the Python and Go
extractors do. The module holds the Rust policy as catalog data: the
six newest release lines, then the `stable` label. It keeps the older
five lines although upstream supports the newest alone
(`eol_behaviour: allow`), so they draw no end-of-life warning.

The action asks the module for the unconstrained matrix and applies
the MSRV itself, with `filterVersionsFromMSRV`: the MSRV first, the
newer lines ascending, then `stable`. A patch-level MSRV such as
`1.97.0` does not repeat its own line, an MSRV newer than every line
yields the MSRV and `stable`, and a malformed MSRV leaves the lines
unfiltered, newest first as the previous resolver listed them, then
`stable`. Without an MSRV, the edition picks the first release
supporting it; that mapping does not consult the module.

## Sources

The module reads two upstreams for Rust:

- [endoflife.date](https://endoflife.date/rust), through its v1 API at
  `https://endoflife.date/api/v1/products/rust/`, for the release
  lines, their dates and their newest patches
- The stable channel manifest at
  `https://static.rust-lang.org/dist/channel-rust-stable.toml`, for
  the release rustup installs as `stable`; every line up to its line
  is installable

The `python_eol_timeout` and `python_eol_max_retries` inputs bound each
request, as they do for Python and Go.

## Cache

Responses go to the runner cache the module shares between every
resolver in a job, a directory chosen in this order:

1. `$SUPPORTED_VERSIONS_CACHE_DIR`
2. `$RUNNER_TEMP/supported-versions` under GitHub Actions
3. `supported-versions` in the user cache directory

A cached channel manifest serves for up to 72 hours without network,
as it changes once per release; endoflife.date responses serve for up
to six hours. The cache outlives the process, so later steps of the
same job reuse what an earlier step fetched. When the cache will not
open, the action warns and resolves without one.

## Fallbacks

- **Upstream failure**: When a source is unreachable, times out or
  answers with an error status, the module serves an older cached
  response, else the snapshot embedded in the module, for that source
  alone, and the action warns that it could not fetch live Rust
  release data. A report can mix sources: live lifecycle data with the
  snapshot's stable release, for one.
- **Unusable data**: When the module fails the report (a body that
  does not parse, cached or fresh) or the report holds no release line
  (an empty release list), the action warns and resolves again from
  the embedded snapshot alone, without the cache, so a malformed cached
  response cannot defeat the fallback.
- **Offline**: When `python_offline_mode` is `true`, the resolver reads
  the embedded snapshot alone and skips the cache, so release data an
  earlier online step cached cannot change the matrix.
- **No data at all**: When even the snapshot yields no release line,
  the matrix is the MSRV and `stable`. The action never estimates a
  release the data does not list.

## Differences From the Previous Resolver

The previous resolver read the stable channel alone, kept it in a
72-hour in-process cache, and when the fetch failed estimated the
current release from Rust's six-week release train. The module
changes the numeric lines in two cases, by design:

- **Offline or during an outage**, the lines are the snapshot's
  recorded facts, not a prediction. A runner offline six weeks after
  the module's snapshot sees the snapshot's newest line, where the
  release-train estimate would have named one more.
- **A new release the channel serves before endoflife.date lists it**
  stays out of the numeric lines until endoflife.date catches up;
  `stable` covers it meanwhile. With 1.100.0 on the channel and
  endoflife.date still at 1.99, an MSRV of 1.97 gives
  `["1.97", "1.98", "1.99", "stable"]` where the previous resolver gave
  `["1.97", "1.98", "1.99", "1.100", "stable"]`.

## Testing

Tests never reach the real upstreams. `internal/supported/supportedtest`
serves recorded endoflife.date and channel responses from `httptest`,
pointed at through `supported.Settings.EndOfLifeBaseURL` and
`RustBaseURL`, and can seed the runner cache in a temporary directory.

- `internal/supported/rust_test.go` covers a live report, a channel
  ahead of the lifecycle data, and an outage, a failing channel and a
  stalled channel with their warnings.
- `internal/extractor/rust/rust_versions_test.go` checks the emitted
  `matrix_json` for each MSRV form and edition, from the snapshot, from
  recorded live data, from a new line, and under outage, malformed or
  empty data and a malformed cache, with fixed clocks and injected
  resolvers for the last-resort paths.
- `cmd/build-metadata/rust_resolver_test.go` checks that offline mode
  ignores a seeded cache.

## Related Files

- Implementation: `internal/extractor/rust/rust_versions.go`
- Shared resolver: `internal/supported/supported.go`
- Wiring: `configureRustResolver` in `cmd/build-metadata/run.go`
