<!--
SPDX-License-Identifier: Apache-2.0
SPDX-FileCopyrightText: 2025 The Linux Foundation
-->

# Testing Guide for build-metadata-action

This document explains the comprehensive testing strategy for the
`build-metadata-action` GitHub Action.

## Overview

The test suite validates metadata extraction across **all supported
project types** using a three-tier approach:

1. **Self-Test** - Tests the action on itself (Go module)
2. **Real-World Projects** - Tests against actual open-source repositories
3. **Synthetic Projects** - Tests against minimal, generated project structures

All tests run in **parallel** using GitHub Actions matrix strategy for
speed and efficiency.

## Test Architecture

### 1. Self-Test (`test-self`)

**Purpose**: Verify the action works on its own repository (Go module project).

**Validates**:

- Go module detection
- Basic metadata extraction
- Action execution in the simplest case

**Project**: The `build-metadata-action` repository itself

---

### 2. Real-World Project Matrix (`test-matrix`)

**Purpose**: Test against real, production repositories to ensure
compatibility with actual project structures.

**Strategy**: Uses GitHub Actions matrix to run tests in parallel for
each project type.

#### Tested Project Types

<!-- markdownlint-disable MD013 -->

| Language/Type | Repository | Expected Type | Notes |
| --------------- | ---------- | --------------- | ------- |
| Python (pyproject.toml) | `lfreleng-actions/test-python-project` | Python | Modern Python packaging |
| Node.js (package.json) | `lfreleng-actions/test-node-project` | JavaScript | npm package |
| Docker (Dockerfile) | `lfreleng-actions/test-docker-project` | Docker | Container project |
| Java Maven (pom.xml) | `apache/maven` | Java | Maven build system |
| Rust (Cargo.toml) | `rust-lang/rustlings` | Rust | Cargo package |
| Ruby (Gemspec) | `jekyll/jekyll` | Ruby | RubyGems |
| PHP (composer.json) | `composer/composer` | PHP | Composer package |
| C# (.csproj) | `dotnet/runtime` | CSharp | .NET project |
| Swift (Package.swift) | `apple/swift-argument-parser` | Swift | Swift Package Manager |
| Dart (pubspec.yaml) | `flame-engine/flame` | Flutter | Dart/Flutter package |
| Terraform (versions.tf) | `hashicorp/terraform` | Terraform | Infrastructure as Code |
| Helm (Chart.yaml) | `prometheus-community/helm-charts` | Helm | Kubernetes Helm chart |

<!-- markdownlint-enable MD013 -->

**Advantages**:

- Real complexity and edge cases
- Community-maintained projects
- Actual production structures

**Considerations**:

- Requires network access to GitHub
- Subject to upstream changes
- Slower than synthetic tests

---

### 3. Synthetic Project Matrix (`test-synthetic`)

**Purpose**: Test metadata extraction with minimal, controlled project
structures generated on-the-fly.

**Strategy**: Each matrix job generates a minimal valid project structure
for a specific language/ecosystem.

#### Tested Synthetic Projects

<!-- markdownlint-disable MD013 -->

| Language | Files Generated | Expected Version | Expected Type |
| -------- | ---------------- | ---------------- | --------------- |
| Python (modern) | `pyproject.toml`, `__init__.py` | 1.2.3 | Python |
| Python (legacy) | `setup.py` | 2.0.1 | Python |
| JavaScript | `package.json` | 3.4.5 | JavaScript |
| Go | `go.mod`, `main.go` | 1.22 | Go |
| Rust | `Cargo.toml`, `src/main.rs` | 0.5.0 | Rust |
| Java Maven | `pom.xml` | 1.0.0 | Java |
| Java Gradle | `build.gradle` | 2.1.0 | Java |
| PHP | `composer.json` | 4.2.0 | PHP |
| Ruby | `test.gemspec` | 1.5.0 | Ruby |
| Docker | `Dockerfile` | 3.19 | Docker |
| Helm | `Chart.yaml` | 0.1.0 | Helm |
| Terraform | `versions.tf` | 1.5.0 | Terraform |
| C# | `Test.csproj` | 3.0.0 | CSharp |
| Swift | `Package.swift` | 5.9 | Swift |
| Dart | `pubspec.yaml` | 2.3.1 | Flutter |

<!-- markdownlint-enable MD013 -->

**Advantages**:

- Fast execution (no external dependencies)
- Full control over test data
- Predictable results
- Easy to add new test cases

**Example Generation Script** (Python modern):

```bash
mkdir -p test-project/src/test_pkg
cat > test-project/pyproject.toml << 'EOF'
[project]
name = "test-python-project"
version = "1.2.3"
description = "Synthetic test project"
EOF
```

---

## Test Execution

### Matrix Parallelization

Both `test-matrix` and `test-synthetic` jobs use GitHub Actions matrix strategy:

```yaml
strategy:
  fail-fast: false
  matrix:
    include:
      - project-type: "Python (pyproject.toml)"
        sample-repo: "lfreleng-actions/test-python-project"
        expected-type: "Python"
      # ... more entries
```

**Benefits**:

- Tests run **concurrently** across runners
- Faster test execution
- Independent failure isolation (`fail-fast: false`)
- Clear per-language test results

### Test Flow

Each matrix job follows this pattern:

1. **Checkout**: Get the build-metadata-action code
2. **Setup**: Either checkout real repo OR generate synthetic project
3. **Execute**: Run the action with the test project path
4. **Verify**: Check that we found the expected metadata

### Validation

Current validation is basic but we can enhance it:

```bash
echo "🔍 Validating metadata extraction for $PROJECT_TYPE"
echo "Expected type: $EXPECTED_TYPE"
echo "✅ Tested on $PROJECT_TYPE project"
```

**Future enhancements could include**:

- JSON output validation
- Version format verification
- Metadata completeness checks
- Output schema validation

---

## Test Summary Job

The `test-summary` job aggregates results from all test jobs:

- **Dependencies**: Waits for all test jobs to complete
- **Always runs**: Uses `if: always()` to run even if tests fail
- **Reports**:
  - Individual job status (✅/❌)
  - List of all tested project types
  - Pass/fail status
- **Fails workflow**: If any test job failed

**GitHub Actions Summary Output**:

```markdown
# 🧪 Test Execution Summary

✅ **Self Test (Go Module)**: Passed
✅ **Matrix Tests (Real Projects)**: Passed
✅ **Synthetic Tests (Generated Projects)**: Passed

## Tested Project Types

### Real-World Projects
- Python (pyproject.toml)
- Node.js (package.json)
- ...

### Synthetic Projects
- Python (modern & legacy)
- JavaScript
- ...

✅ **All tests passed!**
```

---

## Adding New Test Cases

### Adding a Real-World Project

Add to the `test-matrix` job's matrix include:

```yaml
- project-type: "New Language (config-file)"
  sample-repo: "org/repo-name"
  expected-type: "NewLanguage"
  sample-subdir: "optional/subdirectory"  # if needed
```

### Adding a Synthetic Project

Add to the `test-synthetic` job's matrix include:

```yaml
- language: "New Language"
  setup-script: |
    mkdir -p test-project
    cat > test-project/config.ext << 'EOF'
    # Your minimal project configuration
    EOF
  expected-version: "1.0.0"
  expected-type: "NewLanguage"
```

### Testing Locally

You can manually test the generation scripts:

```bash
# Generate a synthetic Python project
mkdir -p test-project/src/test_pkg
cat > test-project/pyproject.toml << 'EOF'
[project]
name = "test-python-project"
version = "1.2.3"
description = "Synthetic test project"
EOF

# Run the action locally (requires act or similar)
act -j test-synthetic
```

---

## Coverage

### Supported Languages Tested

- ✅ Python (modern pyproject.toml + legacy setup.py)
- ✅ JavaScript/Node.js (package.json)
- ✅ Go (go.mod)
- ✅ Rust (Cargo.toml)
- ✅ Java (Maven pom.xml + Gradle build.gradle)
- ✅ PHP (composer.json)
- ✅ Ruby (gemspec)
- ✅ C# / .NET (csproj)
- ✅ Swift (Package.swift)
- ✅ Dart/Flutter (pubspec.yaml)
- ✅ Docker (Dockerfile)
- ✅ Helm (Chart.yaml)
- ✅ Terraform (versions.tf)

### Languages with Extractors Not Yet in Test Matrix

These languages have extractors but aren't explicitly tested (yet):

- C/C++ (CMakeLists.txt, configure.ac, header files)
- Kotlin (build.gradle.kts)
- Scala (build.sbt)
- Elixir (mix.exs)
- Haskell (cabal)
- Julia (Project.toml)
- R (DESCRIPTION)
- Perl (META.yml, Makefile.PL)
- Lua (rockspec)
- Ansible (galaxy.yml)
- OpenAPI (openapi.yaml)
- And more...

**Future work**: Add synthetic tests for these as well.

---

## Performance

### Typical Test Execution Times

<!-- markdownlint-disable MD013 -->

| Job Type | Duration | Concurrency |
| -------- | -------- | ----------- |
| Self Test | ~30s | 1 job |
| Matrix Tests (12 projects) | ~2-3 min | 12 parallel jobs |
| Synthetic Tests (15 projects) | ~1-2 min | 15 parallel jobs |
| **Total** | **~3-4 min** | **28 parallel jobs** |

<!-- markdownlint-enable MD013 -->

The matrix strategy provides **significant speedup** compared to sequential
testing, which would take ~30-45 minutes.

---

## Continuous Integration

The testing workflow runs automatically on:

- **Push to `main`** - Validates production branch
- **Pull Requests** - Validates changes before merge
- **Manual Dispatch** - On-demand testing via GitHub UI

### Concurrency Control

```yaml
concurrency:
  group: "${{ github.workflow }}-${{ github.ref }}"
  cancel-in-progress: true
```

Prevents concurrent test runs from the same branch, saving resources.

---

## Troubleshooting

### Test Failures

1. **Check the job summary** in GitHub Actions for high-level status
2. **Expand failed jobs** to see detailed logs
3. **Look for**:
   - Network issues (for real-world projects)
   - Syntax errors in generation scripts (for synthetic projects)
   - Unexpected metadata extraction results

### Common Issues

**Real-world project tests fail**:

- Upstream repository may have changed structure
- Network connectivity issues
- Repository name or location changed

**Synthetic tests fail**:

- Syntax error in heredoc generation script
- Missing directory creation
- Invalid project configuration

### Debugging Tips

Add debug output to validation steps:

```bash
- name: "Validation"
  run: |
    echo "🔍 Debug: Listing test project contents"
    find test-project -type f
    echo ""
    echo "📝 Debug: Action outputs"
    echo "Type detected: ${{ steps.test.outputs.project_type }}"
    echo "Version: ${{ steps.test.outputs.version }}"
```

---

## Future Enhancements

### Planned Improvements

1. **Enhanced Validation**
   - Check JSON output structure
   - Check version format compliance
   - Verify all expected fields are present

2. **Coverage Expansion**
   - Add tests for all 50+ supported project types
   - Add multi-language monorepo tests
   - Add edge case tests (malformed files, etc.)

3. **Performance Metrics**
   - Track test execution time trends
   - Measure metadata extraction performance

4. **Test Artifacts**
   - Save metadata extraction results
   - Generate comparison reports
   - Track compatibility over time

### Contributing

To add new test cases:

1. Choose real-world project OR synthetic approach
2. Add matrix entry to appropriate job
3. Set expected type/version
4. Update this documentation
5. Submit pull request

---

## References

- [GitHub Actions Matrix Strategy]
(<https://docs.github.com/en/actions/using-jobs/using-a-matrix-for-your-jobs>)
- [build-metadata-action README](../README.md)
- [Supported Project Types](../SUPPORTED_TYPES.md)

---

**Last Updated**: 2025-01-20
