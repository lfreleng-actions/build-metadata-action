#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# SPDX-FileCopyrightText: 2026 The Linux Foundation
#
# Run gofmt on the given files, rewriting them in place, and fail when
# any file changed. gofmt exits 0 whether or not it reformatted
# anything, so the list it prints is the only signal; pre-commit expects
# a modifying hook to fail so the change is reported and staged.
#
# gofmt ships with the Go toolchain and needs neither module downloads
# nor network access, so this runs on pre-commit.ci where golangci-lint
# (which does need both) is skipped. It is the one Go check that runs
# there, so it stays a hook of its own rather than folding into the
# linter's formatters.

set -euo pipefail

if ! command -v gofmt >/dev/null 2>&1; then
  echo "gofmt not found on PATH; install the Go toolchain" >&2
  exit 1
fi

changed=$(gofmt -l -w "$@")
if [ -n "$changed" ]; then
  echo "gofmt reformatted:"
  echo "$changed"
  exit 1
fi
