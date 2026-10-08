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

| Language              | Build Systems                   | Version Files                                 |
| --------------------- | ------------------------------- | --------------------------------------------- |
| Python                | setuptools, poetry, flit, hatch | `pyproject.toml`, `setup.py`, `setup.cfg`     |
| JavaScript/TypeScript | npm, yarn, pnpm                 | `package.json`, `tsconfig.json`               |
| Java                  | Maven, Gradle (Groovy/Kotlin)   | `pom.xml`, `build.gradle`, `build.gradle.kts` |
| .NET/C#               | MSBuild, dotnet CLI             | `*.csproj`, `*.sln`, `*.props`                |
| Go                    | Go modules                      | `go.mod`                                      |
| Rust                  | Cargo                           | `Cargo.toml`                                  |
| Ruby                  | Bundler, RubyGems               | `*.gemspec`, `Gemfile`                        |
| PHP                   | Composer                        | `composer.json`                               |
| Swift                 | Swift Package Manager           | `Package.swift`                               |
| Dart/Flutter          | pub                             | `pubspec.yaml`                                |
| Terraform/OpenTofu    | Terraform, OpenTofu             | `*.tf`, `versions.tf`                         |
| C/C++                 | CMake, Autoconf, Meson          | `CMakeLists.txt`, `configure.ac`              |
| Scala                 | SBT                             | `build.sbt`                                   |
| Elixir                | Mix                             | `mix.exs`                                     |
| Haskell               | Cabal                           | `*.cabal`                                     |
| Julia                 | Pkg                             | `Project.toml`                                |

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
| Name                   | Required | Default          | Description                                                                                                                                                            |
| ---------------------- | -------- | ---------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `path_prefix`          | No       | `.`              | Path to the project root                                                                                                                                               |
| `project_type`         | No       | `""`             | Declare the project type instead of detecting it (e.g. `java-maven`, `java-gradle`). See [Declaring the project type](#declaring-the-project-type).                    |
| `output_format`        | No       | `summary`        | Output format(s): `summary`, `json`, `markdown`, `yaml`. Accepts comma-separated, space-separated, or newline-separated values. Set to empty string to disable output. |
| `include_environment`  | No       | `true`           | Include environment metadata                                                                                                                                           |
| `use_version_extract`  | No       | `true`           | Use version-extract-action for version detection                                                                                                                       |
| `verbose`              | No       | `false`          | Enable verbose output                                                                                                                                                  |
| `artifact_upload`      | No       | `true`           | Upload gathered metadata as workflow artifacts                                                                                                                         |
| `artifact_name_prefix` | No       | `build-metadata` | Custom prefix for artifact names                                                                                                                                       |
| `artifact_formats`     | No       | `json`           | Formats to upload as artifacts. Can be comma-separated, space-separated, or newline-separated (e.g., `json`, `yaml`, or `json,yaml`).                                  |
| `validate_output`      | No       | `true`           | Check JSON/YAML output before uploading                                                                                                                                |
| `strict_validation`    | No       | `true`           | Use strict validation mode (round-trip testing)                                                                                                                        |
| `export_env_vars`      | No       | `false`          | Export all outputs as environment variables (uppercase with underscores) for use in later steps                                                                        |
<!-- markdownlint-enable MD013 -->

### Declaring the project type

Detection resolves the **first** rule that matches in priority order, and
those priorities are global rather than per language. A repository
carrying more than one marker file resolves to whichever ranks highest,
which is not always the thing the workflow builds.

The common case is a Maven project that also has a `package.json`, the
shape `frontend-maven-plugin` produces. `javascript-npm` outranks
`java-maven`, so:

| Output         | Plain Maven  | Maven plus `package.json` |
| -------------- | ------------ | ------------------------- |
| `project_type` | `java-maven` | `javascript-npm`          |
| `build_tool`   | `maven`      | `npm`                     |
| `java_version` | `17`         | *(empty)*                 |

A consumer reading `java_version` gets nothing and falls back to its own
default, choosing a JDK the project never asked for.

Where the caller already knows, say so:

```yaml
- uses: lfreleng-actions/build-metadata-action@<sha>
  with:
    project_type: java-maven
```

A reusable workflow dedicated to one build tool always has better
information than a detector, because the caller chose that workflow.

The action reports an unrecognised value and discards it, running
detection instead: detection still produces a real answer, whereas a type
matching no extractor produces none at all.

## Outputs

### Common Outputs

All project types provide these standardized outputs:

<!-- markdownlint-disable MD013 -->
| Output                       | Description                                                                                                            | Example                  |
| ---------------------------- | ---------------------------------------------------------------------------------------------------------------------- | ------------------------ |
| `project_type`               | Resolved project type, detected or supplied via the `project_type` input                                               | `python-modern`          |
| `build_tool`                 | Build tool the project type implies; empty when not identified                                                         | `maven`                  |
| `project_name`               | Project/package name                                                                                                   | `myproject`              |
| `project_version`            | Current version                                                                                                        | `1.2.3`                  |
| `project_path`               | Absolute project path                                                                                                  | `/workspace/myproject`   |
| `version_source`             | Source of version info                                                                                                 | `pyproject.toml`         |
| `versioning_type`            | Versioning type: `static` or `dynamic`                                                                                 | `static`                 |
| `version_properties_version` | Version from version.properties (LF/ONAP convention); empty when absent                                                | `1.1.0`                  |
| `version_properties_match`   | Whether version.properties matches `project_version` (empty when not comparable)                                       | `true`                   |
| `snapshot_version`           | Synthesized interim/development version (`X.Y.Z-SNAPSHOT` convention)                                                  | `1.1.0-SNAPSHOT`         |
| `release_files`              | Comma-separated release request files under `releases/` (global-jjb/LF convention); empty when none                    | `releases/3.8.2.yaml`    |
| `release_file_count`         | Number of release request files found under `releases/`                                                                | `1`                      |
| `is_release_ready`           | True when at least one release request file is present under `releases/` (describes the tree, not the commit)          | `true`                   |
| `release_version`            | Version parsed from a lone release file (`container_release_tag` for container files); empty when more than one exists | `3.8.2`                  |
| `release_ref`                | Git ref parsed from a lone release file; empty when more than one exists                                               | `abc123...`              |
| `release_distribution_type`  | `distribution_type` of a lone release file, lowercased; empty when the file sets none or more than one file exists     | `container`              |
| `build_timestamp`            | ISO 8601 build timestamp                                                                                               | `2025-11-03T12:00:00Z`   |
| `git_sha`                    | Current git commit SHA                                                                                                 | `abc123...`              |
| `git_branch`                 | Current git branch                                                                                                     | `main`                   |
| `git_tag`                    | Current git tag                                                                                                        | `v1.2.3`                 |
| `ci_platform`                | CI platform                                                                                                            | `github`                 |
| `ci_run_id`                  | CI run identifier                                                                                                      | `12345678`               |
| `ci_run_url`                 | URL to CI run                                                                                                          | `https://github.com/...` |
| `runner_os`                  | Runner OS                                                                                                              | `Linux`                  |
| `runner_arch`                | Runner architecture                                                                                                    | `X64`                    |
| `metadata_json`              | Complete metadata as JSON                                                                                              | `{...}`                  |
| `success`                    | Extraction success indicator                                                                                           | `true`                   |
<!-- markdownlint-enable MD013 -->

### Language-Specific Outputs

#### Python

| Output                   | Description                              |
| ------------------------ | ---------------------------------------- |
| `python_version`         | Python interpreter version               |
| `python_package_name`    | Distribution package name                |
| `python_requires_python` | Required Python version range            |
| `python_build_backend`   | Build backend (setuptools, poetry, etc.) |
| `python_metadata_source` | Source file (pyproject.toml, etc.)       |
| `python_matrix_json`     | CI matrix configuration as JSON          |
| `python_dependencies`    | Runtime dependencies                     |

#### Java (Maven)

| Output                       | Description                       |
| ---------------------------- | --------------------------------- |
| `java_version`               | JDK version                       |
| `java_version_source`        | JDK version source                |
| `java_group_id`              | Maven groupId                     |
| `java_artifact_id`           | Maven artifactId                  |
| `java_packaging`             | Packaging type (jar, war, etc.)   |
| `java_has_parent`            | Whether the POM declares a parent |
| `java_is_multi_module`       | Multi-module (reactor) project    |
| `java_module_count`          | Number of reactor modules         |
| `java_frameworks`            | Detected frameworks               |
| `java_source_dirs`           | Main source directories           |
| `java_test_source_dirs`      | Test source directories           |
| `java_coverage_tool`         | Coverage tool, when configured    |
| `java_coverage_report_paths` | Coverage report locations         |
| `java_generated_source_dirs` | Inferred generated source dirs    |
| `java_generated_sources`     | Derivation of each generated dir  |

The layout outputs describe where a scanner should look. They are
module-relative, so a consumer applies them per module across a reactor,
and they fall back to Maven's conventions (`src/main/java`,
`src/test/java`) when `<build>` is silent, because that is what the build
itself uses.

`java_coverage_tool` and `java_coverage_report_paths` appear when the
build configures coverage, and are absent otherwise. Absence is a
deliberate answer: pointing a scanner at a report that is never produced
is worse than telling it there is none. The action finds JaCoCo in
`<build><plugins>`, in `<build><pluginManagement>`, and in declared
modules — a reactor root often configures nothing itself and delegates to
a parent module, as ONAP cps does with `cps-parent/pom.xml`.

`java_generated_source_dirs` lists where the build will generate
sources, so a scanner can exclude code nobody wrote. Unlike the other
layout outputs this one is an inference, not a fact: the action runs
before the build, so the directories do not exist yet, and the action
reads generator plugin declarations instead. `java_generated_sources`
gives each entry as a JSON object with its `module`, `path`, `plugin`
and `derivation`. A `derivation` of `configured` means the POM sets the
whole location, in the plugin's `<configuration>` or the properties
backing it; `plugin-default` means some part of the location comes from
the plugin's documented default, such as an openapi-generator output
directory left unset above a configured `sourceFolder`.

The action recognises `openapi-generator-maven-plugin`,
`swagger-codegen-maven-plugin` (v2 and v3), `protobuf-maven-plugin`
(xolstice and ascopes), `jaxb2-maven-plugin`, `antlr4-maven-plugin`,
`jooq-codegen-maven` (every edition) and `jsonschema2pojo-maven-plugin`,
plus annotation processors that write sources (MapStruct, Dagger,
AutoValue, Hibernate's metamodel generator, QueryDSL and Immutables)
whether declared as dependencies, including Maven 4's processor-path
types, or on `annotationProcessorPaths`, per compiler execution and
within any `annotationProcessors` allow-list. It reads configuration
from executions, `<pluginManagement>` and on-disk parent POMs at any
depth, as Maven merges them: inheritance one level at a time on the POMs
as written, with `<inherited>` and `combine.self="override"`, then
plugin coordinates, execution phases and goals interpolated before
deciding what runs. It resolves inherited dependencies nearest first and
walks every nested reactor module. A generator needs an execution
binding its goal to count; one bound to the phase `none` or switched off
by its `skip` parameter does not.

The action follows the default build, a plain `mvn` with no `-P` and no
`-D`, and merges in the profiles active there, as Maven does before
inheritance. A profile counts when its conditions all hold, with file
conditions checked against each module, and one marked
`activeByDefault` counts when no other profile of its POM activates. The
action leaves out a profile whose activation turns on the JDK, the
operating system or a property the command line may set, along with any
`activeByDefault` profile it could displace.

The action never reports a directory that could hold hand-written
sources. It refuses a path that holds or sits inside a source directory
(a generator pointed at `${project.basedir}`, say), counting as source
directories the declared ones, as the properties or build directory of
any profile may set them, and every root `build-helper-maven-plugin`
adds, in any profile, whichever module of the reactor they belong to. It
compares where each directory lies on disk, a root a link moves
included, and refuses a path reached through a link. For a module whose
source directories it cannot all see, because it inherits from a parent
it cannot read from disk, a root will not resolve, or a root rests on
properties two profiles could set together, it reports nothing outside
the build directory, which holds nothing but build output. The same goes
for a path anywhere inside such a module, whichever module generates it,
since a reactor root's unread parent can place a root in any module
below it: the path must lie in some module's build directory, which
`mvn clean` empties. It takes the build directory to be Maven's `target`
unless a POM it can read sets another; if an unread parent moves it, a
reported path under `target` is never created, so excluding it excludes
nothing. Beyond that, when in doubt it leaves an entry out rather than
guess: an unknown plugin, an unresolved property, a path outside the
module, or a default the plugin version decides when that version is
unknown. One assumption remains: javac stops searching the classpath for
annotation processors at JDK 23 unless the build configures processing,
and the action applies that to compilations targeting Java 23 or later,
but cannot see which JDK builds an older one and assumes the search. A
directory reported for a processor that does not run is never created;
excluding it then excludes nothing.

Since the flat list carries no module, `java_generated_source_dirs` also
leaves out a path that overlaps hand-written sources in any module,
modules of every profile included and those a module entry names under
any profile's properties, a path that reaches through a link in any
module, and any path that is not build output in every module whose
source directories it cannot all see. When it cannot read a declared
module at all, or a module entry rests on properties two profiles could
set together, it publishes no flat list. `java_generated_sources` still
records each entry against the module that generates it. Both outputs
are absent when the build configures no recognised generator.

The action resolves the Java level (`java_version`) in Maven's own
precedence: the POM's `maven.compiler.release`, then
`maven.compiler.source`/`target`, then `java.version`, then the
`maven-compiler-plugin` `<configuration>`. When the scanned POM declares
no level, the action inherits it from on-disk parent POMs (via
`relativePath`) and, for aggregator roots, from a reactor module — so an
ONAP-style root whose level lives in a shared `*-parent` module still
resolves. The `java_version_source` output reports where the value
came from (e.g. `maven.compiler.release`, `maven-compiler-plugin/release`,
`module:cps-parent`).

#### Java (Gradle)

| Output                       | Description                         |
| ---------------------------- | ----------------------------------- |
| `java_version`               | JDK version                         |
| `java_version_source`        | JDK version source                  |
| `java_group_id`              | Project group                       |
| `java_artifact_id`           | Project name                        |
| `java_build_dsl`             | Build DSL (groovy or kotlin)        |
| `java_is_multi_project`      | Multi-project build                 |
| `java_frameworks`            | Detected frameworks                 |
| `java_gradle_version`        | Gradle version the wrapper declares |
| `java_gradle_version_source` | Source of that version              |

For Gradle the action reads the level from the build file toolchain
(`JavaLanguageVersion.of(N)`), then `source`/`targetCompatibility`
(`JavaVersion.VERSION_N` or a bare/quoted literal), then
`gradle.properties`; `java_version_source` reports the form detected.

`java_gradle_version` comes from the wrapper's `distributionUrl`. This is
the version the project asks to build with, which is a different fact
from the version a CI step provisioned: `gradle/actions/setup-gradle`
reports what it set up itself, and sets up nothing when a build defers
to the wrapper, so that output is empty for wrapper-driven projects.

The output stays empty when the project has no wrapper, or when
`distributionUrl` names no recognisable version. A consumer comparing it
against a tool's floor needs to tell "too old" from "unknown", so the
action reports nothing rather than guessing.

#### Node.js/JavaScript

| Output                 | Description                                |
| ---------------------- | ------------------------------------------ |
| `node_version`         | Node.js version                            |
| `npm_version`          | npm version                                |
| `node_package_manager` | Detected package manager (npm, yarn, pnpm) |
| `node_engines`         | Required node/npm versions                 |
| `node_workspaces`      | Workspace packages (monorepo)              |

#### .NET/C\#

| Output                 | Description         |
| ---------------------- | ------------------- |
| `dotnet_version`       | .NET SDK version    |
| `dotnet_framework`     | Target framework(s) |
| `dotnet_assembly_name` | Assembly name       |
| `dotnet_package_id`    | NuGet package ID    |

#### Go

<!-- markdownlint-disable MD013 -->

| Output                      | Description                                                |
| --------------------------- | ---------------------------------------------------------- |
| `go_base_name`              | Friendly name from the module path (`/vN` suffix stripped) |
| `go_module_path`            | Go module path declared in `go.mod`                        |
| `go_go_version`             | Go version from the `go` directive in `go.mod`             |
| `go_metadata_source`        | Source of Go metadata (`go.mod`)                           |
| `go_toolchain`              | Toolchain directive from `go.mod` (when present)           |
| `go_dependencies`           | Direct dependencies as `module@version`                    |
| `go_indirect_dependencies`  | Indirect dependencies as `module@version`                  |
| `go_dependency_count`       | Number of direct dependencies                              |
| `go_total_dependency_count` | Total dependencies (direct plus indirect)                  |
| `go_dependency_map`         | JSON object mapping modules to versions                    |
| `go_replace_directives`     | Replace directives as JSON array of `{old, new}`           |
| `go_replace_count`          | Number of replace directives                               |
| `go_exclude_directives`     | Exclude directives (comma-separated)                       |
| `go_exclude_count`          | Number of exclude directives                               |
| `go_retract_directives`     | Retract directives (comma-separated)                       |
| `go_retract_count`          | Number of retract directives                               |
| `go_frameworks`             | Detected Go frameworks/tools (comma-separated)             |
| `go_go_version_matrix`      | Supported (non-EOL) Go versions for testing                |
| `go_matrix_json`            | Go version test matrix as JSON                             |

<!-- markdownlint-enable MD013 -->

The action derives the Go version matrix from live
[endoflife.date](https://endoflife.date/go) data: it selects the
supported (non-EOL) Go releases at or above the version declared in
`go.mod`. When the API is unreachable, a static fallback list of the
supported releases applies instead.

#### Rust

<!-- markdownlint-disable MD013 -->

| Output                           | Description                                                         |
| -------------------------------- | ------------------------------------------------------------------- |
| `rust_package_name`              | Package name from `Cargo.toml`                                      |
| `rust_metadata_source`           | Source of Rust metadata (`Cargo.toml`)                              |
| `rust_edition`                   | Rust edition                                                        |
| `rust_msrv`                      | MSRV: the oldest Rust release the project supports (`rust-version`) |
| `rust_rust_version`              | Same value as `rust_msrv`, named after Cargo's field                |
| `rust_rust_version_matrix`       | Rust versions to test against (comma-separated)                     |
| `rust_matrix_json`               | Rust version test matrix as JSON                                    |
| `rust_documentation`             | Documentation URL                                                   |
| `rust_keywords`                  | Package keywords (comma-separated)                                  |
| `rust_categories`                | crates.io categories (comma-separated)                              |
| `rust_publish`                   | `publish` setting: `true`, `false` or a JSON array of registries    |
| `rust_license_file`              | License file path                                                   |
| `rust_readme`                    | README path                                                         |
| `rust_dependencies`              | Normal dependencies as `name@version` (comma-separated)             |
| `rust_dependency_count`          | Number of normal dependencies                                       |
| `rust_optional_dependencies`     | Names of optional dependencies (comma-separated)                    |
| `rust_dev_dependencies`          | Dev-dependencies as `name@version` (comma-separated)                |
| `rust_dev_dependency_count`      | Number of dev-dependencies                                          |
| `rust_build_dependencies`        | Build-dependencies as `name@version` (comma-separated)              |
| `rust_build_dependency_count`    | Number of build-dependencies                                        |
| `rust_total_dependency_count`    | Normal, dev- and build-dependencies together                        |
| `rust_features`                  | JSON object mapping each feature to what it enables                 |
| `rust_feature_names`             | Feature names, sorted (comma-separated)                             |
| `rust_feature_count`             | Number of features                                                  |
| `rust_is_workspace`              | `true` when `[workspace]` lists members                             |
| `rust_workspace_members`         | `[workspace]` members entries as written, globs unexpanded          |
| `rust_workspace_member_count`    | Number of `[workspace]` members entries                             |
| `rust_workspace_resolver`        | Workspace dependency resolver version                               |
| `rust_binary_targets`            | Declared `[[bin]]` target names (comma-separated)                   |
| `rust_binary_count`              | Number of declared `[[bin]]` targets                                |
| `rust_lib_name`                  | `[lib]` target name                                                 |
| `rust_crate_types`               | `[lib]` crate types (comma-separated)                               |
| `rust_has_build_script`          | `true` when `package.build` names a build script                    |
| `rust_build_script`              | Build script path from `package.build`                              |
| `rust_frameworks`                | Frameworks detected from dependencies (comma-separated)             |
| `rust_toolchain_kind`            | Toolchain the toolchain file selects: `channel`, `path` or `none`   |
| `rust_toolchain_file`            | Toolchain file rustup reads, relative to the project directory      |
| `rust_toolchain_channel`         | Channel or toolchain name from the toolchain file                   |
| `rust_toolchain_components`      | Components from the toolchain file (comma-separated)                |
| `rust_toolchain_targets`         | Targets from the toolchain file (comma-separated)                   |
| `rust_toolchain_profile`         | Profile from the toolchain file                                     |
| `rust_publishable_packages`      | JSON array of the packages `cargo publish` would upload             |
| `rust_publishable_package_count` | Number of entries in `rust_publishable_packages`                    |

<!-- markdownlint-enable MD013 -->

An output is empty when `Cargo.toml` does not set the value. Fields
inherited with `{ workspace = true }` resolve against
`[workspace.package]` in the same manifest.

`rust_msrv` and `rust_rust_version` both carry the MSRV, the oldest
Rust release the project declares support for. Neither reports a
compiler: the action runs no Rust toolchain to produce its outputs.
With `include_environment` enabled, `metadata_json` records under
`environment.tools` the `rustc` and `cargo` versions of the toolchain
the job environment selects: the rustup default, or `RUSTUP_TOOLCHAIN`
when the job sets it. The action probes them from the system temporary
directory, so a `rust-toolchain` file in the repository can neither
select nor run them.

The `rust_toolchain_*` outputs describe the toolchain a project selects
through a rustup toolchain file, so later steps can install it. The
action parses the file itself, the way rustup finds it: the nearest
`rust-toolchain` or `rust-toolchain.toml` in the project directory or
a parent, where the legacy `rust-toolchain` wins when both exist.
Under GitHub Actions the search stops at `GITHUB_WORKSPACE`, and a
symlinked file must resolve inside it. The search compares real paths,
as rustup starts from the real working directory, and skips with a
warning a project directory that resolves outside the workspace. A
file that selects a toolchain by absolute path (a lone `path` key, or
a legacy one-line `rust-toolchain` naming one) reports
`rust_toolchain_kind` as `path` and leaves the path out of every
output. A file rustup rejects (a relative `path`, a `path` beside
other keys, a path in `channel`, the reserved name `none`, a legacy
`rust-toolchain` with a blank line before or after its name, no
`channel`, `path`, `components` or `targets` key, or an unknown
`profile` for anything but a custom toolchain) reports `none` with a
warning annotation. A valid file with `components` or `targets` but
no `channel` or `path` also reports
`none`, without a warning, since rustup then keeps its default
toolchain; `rust_toolchain_file` names the file in both cases and
stays empty when there is none. Names outside `[A-Za-z0-9._+-]` get
dropped with a warning annotation that names the problem without
repeating the content.

`rust_publishable_packages` lists the packages of the workspace that
`cargo publish` would upload, as JSON objects with `name`, `version`,
`manifest_path` and, when `publish` names registries, `registries`.
`manifest_path` is relative to `path_prefix`, the form
`rust-crate-publish-action` takes. The action finds the members the
way cargo does: the root package, the `members` entries expanded with
cargo's glob rules (recursive `**`, `[!...]` negation, absolute
entries), and path dependencies inside the workspace root (relative or
absolute), less anything under an `exclude` entry that no `members`
entry names. Cargo reads `exclude` entries as literal paths, not globs,
and so does the action. A package outside the root joins, as in
cargo, when its `package.workspace` key points back at the root; its
`manifest_path` then starts with `../`. Under GitHub Actions the
action reads no manifest and lists no directory outside
`GITHUB_WORKSPACE`, whatever the globs say, and, like the toolchain
search, skips with a warning a project directory that resolves outside
the workspace, reporting no packages.
A package with `publish = false`, `publish = []` or no `version` stays
out of the list. A name or path outside a conservative character set
(a name must also start with a letter or `_`, as Cargo requires), or a
version Cargo rejects (anything but SemVer `MAJOR.MINOR.PATCH`
without leading zeros, with optional pre-release and build metadata),
leaves the package out, with a warning annotation. A registry name
outside that character set drops that registry, with a warning;
the package stays in the list while one or more of its registries
remain. The action reads `[workspace.package]` from the selected
manifest alone, so point `path_prefix` at the workspace root: a
package that inherits `version` or `publish` from a root above
`path_prefix` stays out of the list, with a warning annotation. The
action reads the manifests and runs no cargo command.

```yaml
- id: metadata
  uses: lfreleng-actions/build-metadata-action@<sha>
- if: steps.metadata.outputs.rust_publishable_package_count != '0'
  run: echo "$PACKAGES" | jq -r '.[].manifest_path'
  env:
    PACKAGES: ${{ steps.metadata.outputs.rust_publishable_packages }}
```

Dependency entries carry `(optional)` and a `[feature, ...]` list where
set. Those lists contain commas too, so parse `metadata_json` when you
need exact values.

## Example Output

When used in a GitHub Actions workflow, the action generates a rich step summary:

```text
# 🔧 Build Metadata

## Project Information

| Key                | Value                |
| ------------------ | -------------------- |
| Project Type       | Python (Modern)      |
| Project Name       | dependamerge         |
| Project Version    | 1.2.3                |
| Version Source     | pyproject.toml       |
| Dynamic Versioning | No                   |
| Build Timestamp    | 2025-11-03T12:00:00Z |
| Git SHA            | `abc1234`            |
| Git Branch         | `main`               |

## CI Environment

| Component   | Value          |
| ----------- | -------------- |
| Platform    | github         |
| Runner OS   | Linux          |
| Runner Arch | X64            |
| Workflow    | Build and Test |
| Run Number  | 42             |

## Tool Versions

| Tool       | Version |
| ---------- | ------- |
| python     | 3.13.0  |
| pip        | 24.0    |
| setuptools | 75.0.0  |

## Language-Specific Metadata

### Python Project Details

| Key             | Value          |
| --------------- | -------------- |
| Package Name    | `dependamerge` |
| Requires Python | >=3.10         |
| Build Backend   | setuptools     |
| Metadata Source | pyproject.toml |

### Build Matrix

```json
{
  "python-version": ["3.10", "3.11", "3.12", "3.13", "3.14"]
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

The Rust matrix (`rust_rust_version_matrix`, `rust_matrix_json`) holds
the MSRV, the six most recent stable minor releases at or above it, and
`stable`. With no MSRV it falls back to the first release supporting the
edition, then `stable`.

- **Primary**: Reads the current stable release from
  `https://static.rust-lang.org/dist/channel-rust-stable.toml`
  - 5-second timeout prevents workflow delays
  - Cached for 72 hours within one run
- **Fallback**: When that fetch fails, estimates the current stable
  release from Rust's six-week release train, counted from a verified
  release (1.99.0, 2026-10-01)
  - Ensures CI/CD reliability during network issues or API downtime
  - Produces the same matrix shape as the live path, without a
    hand-maintained list going stale

See [Rust version caching](docs/RUST_VERSION_CACHE.md) for details.

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
