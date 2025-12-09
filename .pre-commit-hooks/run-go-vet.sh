#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# SPDX-FileCopyrightText: 2025 The Linux Foundation

set -e

# If no arguments provided, default to current package
if [ $# -eq 0 ]; then
  go vet .
else
  # Run go vet on all provided package paths
  go vet "$@"
fi
