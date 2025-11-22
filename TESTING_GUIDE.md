# Testing Guide for New Extractors

## Quick Start

This guide helps you test the 6 newly implemented extractors: Helm, Terraform,
Docker, PHP, Swift, and Dart/Flutter.

---

## Prerequisites

```bash
# Install Go 1.21+
go version

# Install dependencies
cd build-metadata-action
go mod download
go mod tidy
```

---

## Running Tests

### Run All Tests

```bash
go test ./internal/extractor/... -v -cover
```

### Run Tests for Specific Extractor

```bash
# Helm
go test ./internal/extractor/helm -v -cover

# Terraform
go test ./internal/extractor/terraform -v -cover

# Docker
go test ./internal/extractor/docker -v -cover

# PHP
go test ./internal/extractor/php -v -cover

# Swift
go test ./internal/extractor/swift -v -cover

# Dart
go test ./internal/extractor/dart -v -cover
```

### Run with Coverage Report

```bash
go test ./internal/extractor/helm -coverprofile=coverage.out
go tool cover -html=coverage.out
```

---

## Test File Template

Each extractor needs a `*_test.go` file. Here's a template:

```golang
// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package <extractor_name>

import (
 "path/filepath"
 "testing"

 "github.com/stretchr/testify/assert"
 "github.com/stretchr/testify/require"
)

func TestExtractor_Detect(t *testing.T) {
 tests := []struct {
  name     string
  path     string
  expected bool
 }{
  {
   name:     "valid project",
   path:     "testdata/valid",
   expected: true,
  },
  {
   name:     "missing file",
   path:     "testdata/empty",
   expected: false,
  },
 }

 e := NewExtractor()
 for _, tt := range tests {
  t.Run(tt.name, func(t *testing.T) {
   result := e.Detect(tt.path)
   assert.Equal(t, tt.expected, result)
  })
 }
}

func TestExtractor_Extract(t *testing.T) {
 e := NewExtractor()

 t.Run("basic extraction", func(t *testing.T) {
  metadata, err := e.Extract("testdata/valid")
  require.NoError(t, err)
  require.NotNil(t, metadata)

  assert.Equal(t, "expected-name", metadata.Name)
  assert.Equal(t, "1.0.0", metadata.Version)
  assert.NotEmpty(t, metadata.LanguageSpecific)
 })

 t.Run("missing project", func(t *testing.T) {
  _, err := e.Extract("testdata/nonexistent")
  assert.Error(t, err)
 })
}

func TestExtractor_Name(t *testing.T) {
 e := NewExtractor()
 assert.Equal(t, "expected-name", e.Name())
}

func TestExtractor_Priority(t *testing.T) {
 e := NewExtractor()
 assert.Equal(t, 1, e.Priority())
}
```

---

## Creating Test Data

### Directory Structure

```text
testdata/
├── valid/              # Valid, complete project
├── minimal/            # Minimal valid project
├── complex/            # Complex real-world example
├── invalid/            # Invalid/malformed files
└── empty/              # Empty directory (no project files)
```

### Example: Helm Test Data

**testdata/valid/Chart.yaml:**

```yaml
apiVersion: v2
name: my-chart
description: A test Helm chart
version: 1.0.0
appVersion: "1.0"
type: application
kubeVersion: ">=1.27.0"
keywords:
  - test
  - demo
maintainers:
  - name: Test Maintainer
    email: test@example.com
dependencies:
  - name: redis
    version: "17.0.0"
    repository: "https://charts.bitnami.com/bitnami"
```

### Example: PHP Test Data

**testdata/valid/composer.json:**

```json
{
  "name": "vendor/package",
  "description": "Test PHP package",
  "version": "1.0.0",
  "type": "library",
  "license": "MIT",
  "authors": [
    {
      "name": "Test Author",
      "email": "test@example.com"
    }
  ],
  "require": {
    "php": "^8.1",
    "symfony/console": "^6.0"
  },
  "require-dev": {
    "phpunit/phpunit": "^10.0"
  },
  "autoload": {
    "psr-4": {
      "Vendor\\Package\\": "src/"
    }
  }
}
```

### Example: Docker Test Data

**testdata/valid/Dockerfile:**

```dockerfile
FROM node:18-alpine AS builder

LABEL org.opencontainers.image.version="1.0.0"
LABEL org.opencontainers.image.description="Test application"
LABEL org.opencontainers.image.authors="test@example.com"

WORKDIR /app
COPY package*.json ./
RUN npm ci

COPY . .
RUN npm run build

FROM node:18-alpine

WORKDIR /app
COPY --from=builder /app/dist ./dist

EXPOSE 3000
USER node

CMD ["node", "dist/index.js"]
```

---

## Test Categories

### 1. Detection Tests

- ✅ Detects valid project
- ✅ Returns false for missing files
- ✅ Returns false for wrong project type
- ✅ Handles edge cases (symlinks, permissions)

### 2. Extraction Tests

- ✅ Extracts name
- ✅ Extracts version
- ✅ Extracts description
- ✅ Extracts dependencies
- ✅ Handles missing optional fields
- ✅ Generates version matrix

### 3. Parser Tests

- ✅ Parses valid syntax
- ✅ Handles malformed files
- ✅ Supports all format variants
- ✅ Handles Unicode and special characters

### 4. Integration Tests

- ✅ Works with real-world projects
- ✅ Integrates with extractor registry
- ✅ Output format is consistent

---

## Manual Testing

### Test with Real Projects

```bash
# Helm
git clone https://github.com/bitnami/charts.git
./build-metadata testcharts/bitnami/redis

# PHP
git clone https://github.com/laravel/framework.git
./build-metadata framework

# Docker
git clone https://github.com/docker/getting-started.git
./build-metadata getting-started/app

# Swift
git clone https://github.com/apple/swift-argument-parser.git
./build-metadata swift-argument-parser

# Dart/Flutter
git clone https://github.com/flutter/gallery.git
./build-metadata gallery

# Terraform
git clone https://github.com/hashicorp/terraform-provider-aws.git
./build-metadata terraform-provider-aws
```

---

## Coverage Goals

<!-- markdownlint-disable MD013 -->
| Extractor | Target Coverage | Critical Functions |
| --------- | ---------------- | ----------------- |
| Helm | ≥85% | `extractFromChartYAML`, `generateKubernetesVersionMatrix` |
| Terraform | ≥85% | `parseFile`, `parseTerraformBlock`, `parseWithRegex` |
| Docker | ≥85% | `parseDockerfile`, `parseInstruction`, `parseLabel` |
| PHP | ≥85% | `extractFromComposerJSON`, `detectPHPFramework` |
| Swift | ≥85% | `parsePackageSwift`, `extractDependencies` |
| Dart | ≥85% | `extractFromPubspec`, `extractFlutterMetadata` |
<!-- markdownlint-enable MD013 -->

---

## Common Test Scenarios

### Version Matrix Generation

```go
func TestGenerateVersionMatrix(t *testing.T) {
 tests := []struct {
  constraint string
  expected   []string
 }{
  {">=1.27.0", []string{"1.27", "1.28", "1.29", "1.30"}},
  {"^8.1", []string{"8.1", "8.2", "8.3"}},
  {">=3.0", []string{"3.0", "3.1", "3.2", "3.3"}},
 }

 for _, tt := range tests {
  t.Run(tt.constraint, func(t *testing.T) {
   result := generateVersionMatrix(tt.constraint)
   assert.Equal(t, tt.expected, result)
  })
 }
}
```

### Dependency Extraction

```go
func TestDependencyExtraction(t *testing.T) {
 metadata, err := extractor.Extract("testdata/with-deps")
 require.NoError(t, err)

 deps := metadata.LanguageSpecific["dependencies"]
 assert.NotNil(t, deps)

 depCount := metadata.LanguageSpecific["dependency_count"]
 assert.Greater(t, depCount, 0)
}
```

### Framework Detection

```go
func TestFrameworkDetection(t *testing.T) {
 tests := []struct {
  testdata string
  expected string
 }{
  {"testdata/laravel", "Laravel"},
  {"testdata/symfony", "Symfony"},
  {"testdata/plain", ""},
 }

 e := NewExtractor()
 for _, tt := range tests {
  t.Run(tt.expected, func(t *testing.T) {
   metadata, err := e.Extract(tt.testdata)
   require.NoError(t, err)

   framework := metadata.LanguageSpecific["framework"]
   assert.Equal(t, tt.expected, framework)
  })
 }
}
```

---

## Debugging Failed Tests

### Enable Verbose Output

```bash
go test ./internal/extractor/helm -v -run TestSpecificTest
```

### Print Metadata for Inspection

```go
t.Logf("Metadata: %+v", metadata)
t.Logf("Language Specific: %+v", metadata.LanguageSpecific)
```

### Use Debugger

```bash
# Install delve
go install github.com/go-delve/delve/cmd/dlv@latest

# Debug test
dlv test ./internal/extractor/helm -- -test.run TestExtractor_Extract
```

---

## Continuous Integration

### GitHub Actions Workflow

```yaml
name: Test New Extractors

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.21'

      - name: Run tests
        run: |
          go test ./internal/extractor/helm -v -cover
          go test ./internal/extractor/terraform -v -cover
          go test ./internal/extractor/docker -v -cover
          go test ./internal/extractor/php -v -cover
          go test ./internal/extractor/swift -v -cover
          go test ./internal/extractor/dart -v -cover

      - name: Check coverage
        run: |
          go test ./internal/extractor/... -coverprofile=coverage.out
          go tool cover -func=coverage.out
```

---

## Test Checklist

Before submitting:

- [ ] All tests pass
- [ ] Coverage ≥85% per extractor
- [ ] No race conditions (`go test -race`)
- [ ] No memory leaks (run with `-memprofile`)
- [ ] Documentation updated
- [ ] Testdata examples added
- [ ] Integration tests pass
- [ ] Manual testing completed with real projects

---

## Getting Help

- Check existing tests in `internal/extractor/python/python_test.go`
- Review test patterns in other extractors
- Use `go test -h` for test flags
- Read [Go testing documentation](https://pkg.go.dev/testing)

---

**Last Updated:** 2025-01-XX
**Status:** Ready for Implementation
