<!--
SPDX-License-Identifier: Apache-2.0
SPDX-FileCopyrightText: 2025 The Linux Foundation
-->

# Testing Strategy: Real-World Projects

## Overview

This document explains our comprehensive testing strategy for the
`build-metadata-action`, which tests against **54 real-world open source
projects** across 18 different programming languages and build systems.

## Why Real-World Projects?

### Problems with Synthetic Tests

The original testing approach used synthetic (generated on-the-fly) test
projects. This approach had significant issues:

1. **Incomplete Coverage**: Synthetic tests covered the "happy path" with
   minimal, perfect project structures
2. **Missing Edge Cases**: Real projects have complexity that synthetic tests
   don't capture:
   - Monorepo structures
   - Non-standard configurations
   - Legacy format variations
   - Complex dependency chains
   - Workspace configurations
3. **Maintenance Burden**: Synthetic tests require maintaining inline bash
   scripts to generate test projects
4. **False Confidence**: Tests passing on synthetic projects doesn't mean they
   work on real codebases
5. **Annotation Noise**: Generated warnings and errors for incomplete extractor implementations

### Benefits of Real-World Testing

Real-world testing against actual open source projects provides:

1. **Realistic Validation**: Tests work against the same complexity developers face
2. **Edge Case Discovery**: Automatically tests non-standard configurations
   and patterns
3. **Continuous Improvement**: As upstream projects evolve, our tests catch
   compatibility issues
4. **Quality Assurance**: If the action works on major open source projects,
   it will work for users
5. **No Maintenance**: We don't maintain test fixtures - we use living,
   breathing projects
6. **Trust Building**: Users can see which projects we test against

## Test Matrix

### Current Coverage: 54 Projects Across 18 Languages

| Language/Type | # Projects | Example Repositories |
| --------------- | ---------- | -------------------- |
| Python | 3 | `psf/requests`, `pallets/flask`, `pytest-dev/pytest` |
| JavaScript | 3 | `expressjs/express`, `lodash/lodash`, `facebook/react` |
| TypeScript | 3 | `microsoft/TypeScript`, `vuejs/core`, `angular/angular` |
| Go | 3 | `kubernetes/kubernetes`, `prometheus/prometheus` |
| Rust | 3 | `rust-lang/rustlings`, `denoland/deno`, `tokio-rs/tokio` |
| Java Maven | 3 | `apache/maven`, `spring-projects/spring-boot` |
| Java Gradle | 3 | `gradle/gradle`, `elastic/elasticsearch` |
| Ruby | 3 | `jekyll/jekyll`, `rails/rails`, `rubygems/rubygems` |
| PHP | 3 | `composer/composer`, `laravel/framework`, `symfony/symfony` |
| C# / .NET | 3 | `dotnet/runtime`, `dotnet/aspnetcore`, `dotnet/roslyn` |
| Swift | 3 | `apple/swift-argument-parser`, `apple/swift-log` |
| Dart | 3 | `flame-engine/flame`, `dart-lang/sdk`, `flutter/packages` |
| Docker | 3 | `docker/getting-started`, `nginxinc/docker-nginx` |
| Helm | 3 | `prometheus-community/helm-charts`, `bitnami/charts` |
| Terraform | 3 | `hashicorp/terraform`, `gruntwork-io/terragrunt` |
| C/C++ | 3 | `catchorg/Catch2`, `opencv/opencv`, `google/googletest` |
| Scala | 3 | `apache/spark`, `playframework/playframework`, `akka/akka` |
| Elixir | 3 | `elixir-lang/elixir`, `phoenixframework/phoenix` |
| Haskell | 3 | `haskell/cabal`, `commercialhaskell/stack`, `jgm/pandoc` |
| Julia | 3 | `JuliaLang/julia`, `JuliaData/DataFrames.jl` |

### Total: 54 unique open source projects

## Project Selection Criteria

Each project in our test matrix meets these criteria:

1. **Popularity**: Well-known, widely-used projects in their ecosystem
2. **Maturity**: Active maintenance and stable structure
3. **Representativeness**: Follows community standards and best practices
4. **Diversity**: Mix of small, medium, and large projects
5. **Complexity**: Includes both simple and complex project structures
6. **Availability**: Public repositories accessible via GitHub

### Why 3 Projects Per Language?

- **Statistical Significance**: Tests diverse patterns and configurations
- **Coverage**: Captures different approaches within the same ecosystem
- **Redundancy**: If one project changes drastically, we still have two others
- **Performance**: Balances thoroughness with CI runtime

## Test Architecture

### Workflow Structure

```yaml
jobs:
  test-self:
    # Tests the action on its own Go codebase

  test-matrix:
    # Matrix of 54 real-world projects
    strategy:
      fail-fast: false
      matrix:
        include:
          - project-type: "Python"
            repo: "psf/requests"
            owner: "psf"
            repo-name: "requests"
            expected-type: "Python"
            subdir: ""
          # ... 53 more projects

  test-summary:
    # Aggregates and reports results
```

### Test Flow

For each project:

1. **Checkout**: Clone the real project from GitHub
2. **Run Action**: Execute `build-metadata-action` on the project
3. **Verify**: Check outputs and verify metadata extraction
4. **Report**: Generate detailed results including warnings

### Handling Test Failures

Tests use `continue-on-error: true` because:

- Some extractors are not yet fully implemented
- Complex projects may have edge cases we're still handling
- We want to see all test results, not fail fast
- Warnings help us track implementation progress

## Understanding Test Results

### Expected Outcomes

✅ **Success**: Extractor working as expected

- Project type detected
- Metadata extracted
- No errors in logs

⚠️ **Warning**: Extractor partially implemented or edge case encountered

- Project detected but extraction incomplete
- Known limitation (documented in annotations)
- Future enhancement opportunity

❌ **Failure**: Action encountered an error

- Bug in extractor logic
- Unexpected project structure
- Requires investigation

### Annotation Messages

Common warnings you might see:

```text
No specific extractor for project type X: no extractor found for type: X
```text

- **Meaning**: Detector found the project type, but extractor not implemented yet
- **Action**: Create the extractor for that language

```text
Failed to detect project type: could not detect project type in /path
```

- **Meaning**: Project structure doesn't match our detection patterns
- **Action**: Improve detection logic or add support for variant structures

```text
Failed to extract version: could not determine version
```

- **Meaning**: Version information not found or in unexpected format
- **Action**: Enhance version extraction logic

## Implementation Roadmap

### Phase 1: Core Languages (Complete)

- ✅ Python
- ✅ JavaScript/TypeScript
- ✅ Go
- ✅ Java (Maven/Gradle)
- ✅ Rust

### Phase 2: Extended Languages (In Progress)

- 🚧 Ruby (extractor needed)
- 🚧 PHP (extractor needed)
- 🚧 C# / .NET (extractor needed)
- 🚧 Swift (extractor needed)
- 🚧 Dart/Flutter (extractor needed)

### Phase 3: Infrastructure (In Progress)

- 🚧 Docker (extractor needed)
- 🚧 Helm (extractor needed)
- 🚧 Terraform (extractor needed)

### Phase 4: More Languages (Planned)

- ⏳ C/C++
- ⏳ Scala
- ⏳ Elixir
- ⏳ Haskell
- ⏳ Julia

## Running Tests Locally

### Full Test Suite

```bash
# Run all tests
act -j test-matrix

# Or use GitHub CLI
gh workflow run testing.yaml --ref your-branch
```

### Test Specific Language

```bash
# Filter by project type in the matrix
act -j test-matrix --matrix project-type:Python
```

### Manual Testing Against Real Project

```bash
# Clone a test project
git clone https://github.com/psf/requests.git
cd requests

# Run the action locally
../build-metadata-action/build-metadata .

# Check outputs
echo $?
```

## Adding New Test Projects

To add a new project to the test matrix:

1. **Identify the Project**: Find a suitable open source project
2. **Verify Access**: Ensure it's publicly accessible on GitHub
3. **Add to Matrix**: Add entry to `.github/workflows/testing.yaml`

   ```yaml
   - project-type: "NewLanguage"
     repo: "org/project"
     owner: "org"
     repo-name: "project"
     expected-type: "NewLanguage"
     subdir: ""  # or path to subproject
   ```

4. **Document**: Update this file with the new language/project

## Comparison with version-extract-action

Our approach improves upon `version-extract-action`:

### Similarities

- Tests against real-world projects
- Matrix-based testing strategy
- Three projects per language
- Comprehensive language coverage

### Improvements

1. More languages (18 vs ~15)
2. Better validation logic
3. Richer output metadata
4. Integrated environment capture
5. Better error handling and reporting

## CI/CD Integration

### Test Triggers

Tests run on:

- Every push to `main`
- Every pull request to `main`
- Manual workflow dispatch

### Test Duration

- **Self Test**: ~1 minute
- **Matrix Tests**: ~15-20 minutes (parallel execution)
- **Total**: ~20 minutes for full suite

### Resource Usage

- All tests run on `ubuntu-latest`
- Parallel execution across matrix
- Timeout: 15 minutes per job

## Metrics and Monitoring

### Success Metrics

- **Detection Rate**: % of projects with type detection
- **Extraction Rate**: % of projects with metadata extraction
- **Error Rate**: % of projects with unexpected failures
- **Coverage**: Number of languages with full extractor support

### Current Status (Example)

```text
Detection Rate:   95% (51/54 projects)
Extraction Rate:  70% (38/54 projects)
Error Rate:       5%  (3/54 projects)
Full Support:     60% (11/18 languages)
```

## Future Enhancements

### Potential Additions

1. **Expand Projects**: Add 50 more projects (3x current)
2. **Version Testing**: Test across different versions of projects
3. **Monorepo Testing**: Add more complex monorepo examples
4. **Performance Testing**: Measure extraction time per project
5. **Regression Testing**: Store snapshots of extracted metadata

### Language Expansion

- Kotlin (JVM)
- Apple's Obj-C
- Zig
- Nim
- Crystal
- Lua
- Perl
- Clojure

## Contributing

### Adding a New Extractor

1. Create extractor in `internal/extractor/<language>/`
2. Add unit tests with `testdata/` fixtures
3. Add 3 real-world projects to test matrix
4. Update this documentation
5. Verify all tests pass

### Improving Detection

1. Analyze failures in test logs
2. Identify patterns in failed projects
3. Update detection logic
4. Verify improvement across all projects

## Resources

- [Testing Guide
