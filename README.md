<!--
# SPDX-License-Identifier: Apache-2.0
# SPDX-FileCopyrightText: 2025 The Linux Foundation
-->

# 🔧 Build Metadata Action

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go)](https://golang.org)

Universal GitHub Action to capture and display comprehensive metadata related to
software builds across 15+ languages and build systems.

## Overview

The `build-metadata-action` is a unified solution for extracting, processing, and
reporting build metadata for projects written in Python, Java,
JavaScript/TypeScript, Go, .NET, Rust, Ruby, and other languages. It consolidates
functionality from language-specific metadata actions while providing standardized
outputs and rich CI/CD integration.

### Key Features

- 🌐 **Multi-Language Support**: Python, Java (Maven/Gradle), Node.js, Go, .NET,
  Rust, Ruby, and more
- 📊 **Rich Reporting**: Generates beautiful GitHub Step Summary outputs with
  project and build information
- 🔍 **Version Detection**: Integrates with `version-extract-action` for
  comprehensive version extraction
- 🛠️ **Environment Capture**: Reports CI environment, tool versions, and runtime
  configuration
- 📦 **Standardized Outputs**: Consistent, namespaced outputs for downstream
  build actions
- 🎯 **Dynamic Versioning**: Detects and handles dynamic versioning strategies
- 🔗 **Monorepo Support**: Handles multi-language and multi-project repositories

## Supported Languages & Build Systems

<!-- markdownlint-disable MD013 -->

| Language | Build Systems | Version Files |
| -------- | ------------- | ------------- |
| Python | setuptools, poetry, flit, hatch | `pyproject.toml`, `setup.py`, `setup.cfg` |
| JavaScript/TypeScript | npm, yarn, pnpm | `package.json`, `tsconfig.json` |
| Java | Maven, Gradle (Groovy/Kotlin) | `pom.xml`, `build.gradle`, `build.gradle.kts` |
| .NET/C# | MSBuild, dotnet CLI | `*.csproj`, `*.sln`, `*.props` |
| Go | Go modules | `go.mod` |
| Rust | Cargo | `Cargo.toml` |
| Ruby | Bundler, RubyGems | `*.gemspec`, `Gemfile` |
| PHP | Composer | `composer.json` |
| Swift | Swift Package Manager | `Package.swift` |
| Dart/Flutter | pub | `pubspec.yaml` |
| Terraform/OpenTofu | Terraform, OpenTofu | `*.tf`, `versions.tf` |
| C/C++ | CMake, Autoconf, Meson | `CMakeLists.txt`, `configure.ac` |
| Scala | SBT | `build.sbt` |
| Elixir | Mix | `mix.exs` |
| Haskell | Cabal | `*.cabal` |
| Julia | Pkg | `Project.toml` |

<!-- markdownlint-enable MD013 -->

## Usage

### Basic Example

```yaml
- name: Extract Build Metadata
  id: metadata
  uses: lfreleng-actions/build-metadata-action@v1
  with:
    path_prefix: .
```

### Full Example

```yaml
name: Build and Deploy

on: [push, pull_request]

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Python
        uses: actions/setup-python@v5
        with:
          python-version: '3.13'

      - name: Extract Build Metadata
        id: metadata
        uses: lfreleng-actions/build-metadata-action@v1
        with:
          path_prefix: .
          output_format: summary
          include_environment: true
          use_version_extract: true
          verbose: false
          artifact_upload: true
          artifact_formats: json

      - name: Use Metadata in Build
        run: |
          echo "Building ${{ steps.metadata.outputs.project_name }} \
            v${{ steps.metadata.outputs.project_version }}"
          echo "Project Type: ${{ steps.metadata.outputs.project_type }}"
```

### Multi-Language Monorepo Example

```yaml
- name: Extract Python Metadata
  id: python-metadata
  uses: lfreleng-actions/build-metadata-action@v1
  with:
    path_prefix: ./python-service

- name: Extract Node.js Metadata
  id: node-metadata
  uses: lfreleng-actions/build-metadata-action@v1
  with:
    path_prefix: ./web-frontend

- name: Build Services
  run: |
    echo "Python: ${{ steps.python-metadata.outputs.project_version }}"
    echo "Node.js: ${{ steps.node-metadata.outputs.project_version }}"
```

### Export Environment Variables Example

Use `export_env_vars: true` to make metadata available as environment variables
in later steps:

```yaml
- name: Extract Build Metadata
  id: metadata
  uses: lfreleng-actions/build-metadata-action@v1
  with:
    path_prefix: .
    export_env_vars: true

- name: Use Environment Variables
  run: |
    echo "Project: $PROJECT_NAME"
    echo "Version: $PROJECT_VERSION"
    echo "Type: $PROJECT_TYPE"
    # All outputs become uppercase environment variables
    # e.g., project_name -> PROJECT_NAME
    #       python_build_version -> PYTHON_BUILD_VERSION
```

### Output Formats Example

Generate output in one or more formats simultaneously (comma, space, or newline-separated):

```yaml
- name: Extract Build Metadata
  id: metadata
  uses: lfreleng-actions/build-metadata-action@v1
  with:
    path_prefix: .
    # You can specify one or more formats
    output_format: summary,json,markdown
    # Or with spaces: "summary json markdown"
    # Or with newlines:
    # output_format: |
    #   summary
    #   json
    #   markdown

- name: Artifact Formats
  uses: lfreleng-actions/build-metadata-action@v1
  with:
    artifact_upload: true
    artifact_formats: json,yaml
    # Uploads both JSON and YAML artifacts
```

## Inputs

<!-- markdownlint-disable MD013 -->
| Name | Required | Default | Description |
| ---- | -------- | ------- | ----------- |
| `path_prefix` | No | `.` | Path to the project root |
| `output_format` | No | `summary` | Output format(s): `summary`, `json`, `markdown`, `yaml`. Accepts comma-separated, space-separated, or newline-separated values. Set to empty string to disable output. |
| `include_environment` | No | `true` | Include environment metadata |
| `use_version_extract` | No | `true` | Use version-extract-action for version detection |
| `verbose` | No | `false` | Enable verbose output |
| `artifact_upload` | No | `true` | Upload gathered metadata as workflow artifacts |
| `artifact_name_prefix` | No | `build-metadata` | Custom prefix for artifact names |
| `artifact_formats` | No | `json` | Formats to upload as artifacts. Can be comma-separated, space-separated, or newline-separated (e.g., `json`, `yaml`, or `json,yaml`). |
| `validate_output` | No | `true` | Check JSON/YAML output before uploading |
| `strict_validation` | No | `true` | Use strict validation mode (round-trip testing) |
| `export_env_vars` | No | `false` | Export all outputs as environment variables (uppercase with underscores) for use in later steps |
<!-- markdownlint-enable MD013 -->

## Outputs

### Common Outputs

All project types provide these standardized outputs:

<!-- markdownlint-disable MD013 -->
| Output | Description | Example |
| -------- | ------------ | ---------- |
| `project_type` | Detected project type | `python-modern` |
| `project_name` | Project/package name | `myproject` |
| `project_version` | Current version | `1.2.3` |
| `project_path` | Absolute project path | `/workspace/myproject` |
| `version_source` | Source of version info | `pyproject.toml` |
| `versioning_type` | Versioning type: `static` or `dynamic` | `static` |
| `build_timestamp` | ISO 8601 build timestamp | `2025-11-03T12:00:00Z` |
| `git_sha` | Current git commit SHA | `abc123...` |
| `git_branch` | Current git branch | `main` |
| `git_tag` | Current git tag | `v1.2.3` |
| `ci_platform` | CI platform | `github` |
| `ci_run_id` | CI run identifier | `12345678` |
| `ci_run_url` | URL to CI run | `https://github.com/...` |
| `runner_os` | Runner OS | `Linux` |
| `runner_arch` | Runner architecture | `X64` |
| `metadata_json` | Complete metadata as JSON | `{...}` |
| `success` | Extraction success indicator | `true` |
<!-- markdownlint-enable MD013 -->

### Language-Specific Outputs

#### Python

| Output | Description |
| -------- | ------------ |
| `python_version` | Python interpreter version |
| `python_package_name` | Distribution package name |
| `python_requires_python` | Required Python version range |
| `python_build_backend` | Build backend (setuptools, poetry, etc.) |
| `python_metadata_source` | Source file (pyproject.toml, etc.) |
| `python_matrix_json` | CI matrix configuration as JSON |
| `python_dependencies` | Runtime dependencies |

#### Java (Maven)

| Output | Description |
| -------- | ------------ |
| `java_version` | JDK version |
| `maven_version` | Maven version |
| `maven_group_id` | Maven groupId |
| `maven_artifact_id` | Maven artifactId |
| `maven_packaging` | Packaging type (jar, war, etc.) |
| `maven_modules` | Multi-module project modules |

#### Java (Gradle)

| Output | Description |
| -------- | ------------ |
| `java_version` | JDK version |
| `gradle_version` | Gradle version |
| `gradle_group` | Project group |
| `gradle_name` | Project name |
| `gradle_build_file` | Build file type |

#### Node.js/JavaScript

| Output | Description |
| -------- | ------------ |
| `node_version` | Node.js version |
| `npm_version` | npm version |
| `node_package_manager` | Detected package manager (npm, yarn, pnpm) |
| `node_engines` | Required node/npm versions |
| `node_workspaces` | Workspace packages (monorepo) |

#### .NET/C\#

| Output | Description |
| -------- | ------------ |
| `dotnet_version` | .NET SDK version |
| `dotnet_framework` | Target framework(s) |
| `dotnet_assembly_name` | Assembly name |
| `dotnet_package_id` | NuGet package ID |

#### Go

| Output | Description |
| -------- | ------------ |
| `go_version` | Go version |
| `go_module` | Module path |
| `go_module_version` | Module version |

#### Rust

| Output | Description |
| -------- | ------------ |
| `rust_version` | Rust compiler version |
| `cargo_version` | Cargo version |
| `rust_edition` | Rust edition |
| `rust_workspace_members` | Workspace members |

## Example Output

When used in a GitHub Actions workflow, the action generates a rich step summary:

```text
# 🔧 Build Metadata

## Project Information

| Key | Value |
|-----|-------|
| Project Type | Python (Modern) |
| Project Name | dependamerge |
| Project Version | 1.2.3 |
| Version Source | pyproject.toml |
| Dynamic Versioning | No |
| Build Timestamp | 2025-11-03T12:00:00Z |
| Git SHA | `abc1234` |
| Git Branch | `main` |

## CI Environment

| Component | Value |
|-----------|-------|
| Platform | github |
| Runner OS | Linux |
| Runner Arch | X64 |
| Workflow | Build and Test |
| Run Number | 42 |

## Tool Versions

| Tool | Version |
|------|---------|
| python | 3.13.0 |
| pip | 24.0 |
| setuptools | 75.0.0 |

## Language-Specific Metadata

### Python Project Details

| Key | Value |
|-----|-------|
| Package Name | `dependamerge` |
| Requires Python | >=3.9 |
| Build Backend | setuptools |
| Metadata Source | pyproject.toml |

### Build Matrix

```json
{
  "python-version": ["3.9", "3.10", "3.11", "3.12", "3.13"]
}
```

✅ Metadata extraction successful

## Integration with Other Actions

### With Version Extract Action

```yaml
- name: Extract Metadata
  uses: lfreleng-actions/build-metadata-action@v1
  with:
    use_version_extract: true
  env:
    VERSION_EXTRACT_ACTION_PATH: /path/to/version-extract-action
```

### With Build Actions

```yaml
- name: Extract Metadata
  id: metadata
  uses: lfreleng-actions/build-metadata-action@v1

- name: Build Python Package
  uses: lfreleng-actions/python-build-action@v1
  with:
    version: ${{ steps.metadata.outputs.project_version }}
    python_version: ${{ steps.metadata.outputs.python_version }}
```

## Advanced Features

### Dynamic Versioning Support

The action detects and reports when projects use dynamic versioning:

<!-- markdownlint-disable MD013 -->

- Python: `setuptools_scm`, `versioneer`, PEP 621 dynamic versions
- Node.js: `semantic-release`, version `0.0.0-development`
- Java: Maven properties, Gradle project version
- Rust: `0.0.0`, `0.1.0-dev` versions

<!-- markdownlint-enable MD013 -->

### Monorepo Support

Automatically detects and handles monorepo structures:

- Node.js workspaces
- Python multi-package projects
- Rust workspaces
- Maven multi-module projects
- Gradle multi-project builds

## Implementation Details

Built with Go using design patterns from `version-extract-action`:

<!-- markdownlint-disable MD013 -->

- **Strategy Pattern**: Language-specific extractors
- **Chain of Responsibility**: Sequential project type detection
- **Factory Pattern**: Dynamic extractor selection
- **Configuration-Driven**: YAML-based pattern definitions
- **Dynamic Version Fetching**: Automatically updates version matrices from
  upstream sources with static fallbacks

<!-- markdownlint-enable MD013 -->

### Dynamic Version Management

To keep pace with fast-evolving language ecosystems, the action uses a
**dynamic + fallback strategy** for version matrices:

#### Rust Version Detection

- **Primary**: Fetches current stable version from `rust-lang.org`
  - Generates ~6 recent versions (9 months of releases)
  - Adapts to Rust's 6-week release cycle automatically
  - 5-second timeout prevents workflow delays
- **Fallback**: Static version list (updated monthly)
  - Ensures CI/CD reliability during network issues or API downtime
  - Prevents build failures from temporary connectivity problems
  - Provides reasonable version coverage even offline

#### Why This Approach?

Languages like Rust, Swift, and PHP release frequently (every 6-8 weeks). Static
version lists become outdated within weeks, leading to:

- Missing security updates and new features in CI tests
- Manual maintenance burden to keep lists current
- Stale testing that doesn't catch real-world compatibility issues

**Dynamic fetching solves this** while the fallback ensures **reliability**.

See [IMPLEMENTATION_PLAN.md](docs/IMPLEMENTATION_PLAN.md) for detailed
architecture and design decisions.

## Development

### Prerequisites

- Go 1.24 or higher
- Git
- (Optional) Language toolchains for testing

### Building

```bash
make build
```

### Testing

The project includes a comprehensive test suite that validates metadata
extraction across all supported languages and project types.

**Quick Start:**

```bash
make test
```

**Comprehensive Testing:**

The GitHub Actions workflow tests the action against:

- **Real-world projects**: 12+ actual open-source repositories
- **Synthetic projects**: 15+ minimal generated project structures
- **All major languages**: Python, JavaScript, Go, Rust, Java, PHP, Ruby,
  C#, Swift, Dart, Docker, Helm, Terraform, and more

Tests run in parallel using GitHub Actions matrix strategy for speed.

📚 **See [Testing Guide](docs/TESTING.md) for detailed information** about:

- Test architecture and strategy
- How to add new test cases
- Coverage across 50+ project types
- Performance and troubleshooting

### Running Locally

```bash
./build-metadata --path /path/to/project --output-format summary
```

## Contributing

Contributions are welcome! Please see our contributing guidelines and code of conduct.

## License

Apache License 2.0 - see [LICENSE](LICENSE) for details.

## Related Projects

<!-- markdownlint-disable MD013 -->
- [version-extract-action](https://github.com/lfreleng-actions/version-extract-action) - Universal version extraction
- [python-project-metadata-action](https://github.com/lfreleng-actions/python-project-metadata-action) - Python-specific metadata
- [python-build-action](https://github.com/lfreleng-actions/python-build-action) - Python build automation
<!-- markdownlint-enable MD013 -->

## Support

For questions, issues, or feature requests, please open an issue on GitHub.
