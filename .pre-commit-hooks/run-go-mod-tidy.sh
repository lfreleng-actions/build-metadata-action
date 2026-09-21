#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# SPDX-FileCopyrightText: 2026 The Linux Foundation
#
# Run `go mod tidy` and fail when it changed go.mod or go.sum, so the
# hook reports the modification the way pre-commit expects rather than
# silently rewriting the files under a passing status.

set -euo pipefail

# Snapshot both files, recording an absent file as a distinct marker
# rather than as an empty read. go.sum is legitimately absent for a
# module with no external dependencies, and `go mod tidy` may create or
# remove it; each of those is a change to detect, not a reason to abort.
snapshot() {
  local file
  for file in go.mod go.sum; do
    if [ -f "$file" ]; then
      shasum "$file"
    else
      echo "absent  $file"
    fi
  done
}

before=$(snapshot)
go mod tidy
after=$(snapshot)

if [ "$before" != "$after" ]; then
  echo "go mod tidy modified go.mod/go.sum; stage the changes and retry"
  exit 1
fi
