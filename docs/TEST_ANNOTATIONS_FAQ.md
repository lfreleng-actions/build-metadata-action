<!--
SPDX-License-Identifier: Apache-2.0
SPDX-FileCopyrightText: 2025 The Linux Foundation
-->

# Test Annotations FAQ

## Understanding Warnings in Test Runs

This document explains the annotations and warnings you see when running tests
for the `build-metadata-action`.

## Overview

When running the test suite, you may see annotations like:

```text
Annotations
33 warnings
Test: Docker (Dockerfile)
No specific extractor for project type unknown: no extractor found for type:
unknown
```

**This behavior happens by design.** These warnings help us track
implementation progress across 18+ programming languages and build systems.

## Why Do We Have Annotations?

### The Good News

✅ **Tests work as expected** - The action detects projects and attempts extraction
✅ **Real-world complexity** - We test against 54 open source projects
✅ **Progress tracking** - Warnings show which extractors need implementation
✅ **No failures** - Warnings don't mean tests failed, incomplete features

### The Reality

Our test suite runs against **54 real-world projects across 18 languages**.
Some extractors are:

- ✅ **Fully Implemented**: Python, JavaScript, Go, Rust, Java
- 🚧 **Partially Implemented**: Ruby, PHP, C#, Swift, Dart, Docker, Helm, Terraform
- ⏳ **Planned**: C/C++, Scala, Elixir, Haskell, Julia

## Common Annotation Messages

### 1. "No specific extractor for project type X"

```text
No specific extractor for project type ruby-gemspec: no extractor found for
type: ruby
```

**What it means:**

- The action detected the project type
- But the extractor for that language is not yet implemented
- Detection works ✅, Extraction pending 🚧

**What happens:**

- The action returns basic metadata
- Some fields may be empty or have placeholder values
- The test continues and completes

**Action needed:**

- Create the extractor in `internal/extractor/<language>/`
- Add unit tests with real project examples
- Re-run tests to verify

**Example languages with this warning:**

- Ruby (`ruby-gemspec`, `ruby`)
- PHP (`php-composer`, `php`)
- Swift (`swift-package`, `swift`)
- Dart (`dart-flutter`, `dart`)
- C# (`csharp-project`, `csharp-props`, `dotnet`)

---

### 2. "Failed to detect project type"

```text
Failed to detect project type: could not detect project type in
/path/to/project
```

**What it means:**

- The detector couldn't identify the project type
- The project structure doesn't match our detection patterns
- Could be a complex monorepo or non-standard layout

**What happens:**

- The action reports "Unknown" as the project type
- Extraction may still work for some metadata
- Test continues with limited data

**Action needed:**

- Review the project structure
- Update detector logic in `internal/detector/`
- Add support for variant project layouts
- Consider subdirectory scanning for monorepos

**Common causes:**

- Monorepo without clear language markers in root
- Project using non-standard file names
- More than one language indicator (polyglot projects)
- Build system files in subdirectories

---

### 3. "Failed to extract version"

```text
Failed to extract version: could not determine version
```

**What it means:**

- Project type detected
- But the action couldn't extract version information
- Version might be dynamic, missing, or in unexpected format

**What happens:**

- Metadata extraction continues
- Version field will be empty or "unknown"
- Other metadata (name, description, etc.) extraction may still succeed

**Action needed:**

- Check if the project uses dynamic versioning
- Look for version in alternative locations
- Update version extraction logic
- Consider integrating with `version-extract-action`

**Common scenarios:**

- Dynamic versioning from git tags
- Version in non-standard location
- Version requires compilation/build to determine
- Multi-module project with per-module versions

---

### 4. "Failed to extract project metadata"

```text
Failed to extract project metadata: no Python project files found in /path
```

**What it means:**

- Extractor couldn't find expected project files
- Path might be incorrect (wrong subdirectory)
- Project structure doesn't match extractor expectations

**What happens:**

- Extraction stops prematurely
- Minimal or no metadata returned
- Test completes but with warnings

**Action needed:**

- Verify the correct project path
- Check if project uses non-standard structure
- Update extractor to handle variations
- Add support for more file patterns

**Example cases:**

- Looking for `pyproject.toml` but project has `setup.py`
- Expecting files in root, but they're in subdirectory
- Project using legacy or deprecated file formats

---

### 5. "Failed to parse configuration file"

```text
Failed to parse Cargo.toml: toml: incompatible types: TOML value has type
map[string]interface {}; destination has type string
```

**What it means:**

- File found but the action couldn't parse it
- Structure doesn't match expected schema
- Real-world projects use features our parser doesn't support yet

**What happens:**

- Parsing fails with detailed error
- Partial data may be available
- Test marked with warning

**Action needed:**

- Review the actual file in the test project
- Update parser to handle the structure
- Add test case for this specific format
- Consider more flexible parsing logic

**Common causes:**

- Advanced TOML/YAML features
- Nested structures we don't expect
- Comments or formatting that breaks parser
- Version-specific syntax

---

## Why We Test Against Real Projects

### Old Approach: Synthetic Tests ❌

```yaml
# Generate a minimal test project
setup-script: |
  mkdir -p test-project
  cat > test-project/package.json << 'EOF'
  {
    "name": "test-project",
    "version": "1.0.0"
  }
  EOF
```

**Problems:**

- Tests "happy path" with perfect, minimal files
- Misses real-world complexity
- False confidence - tests pass but real projects fail
- Maintenance burden maintaining inline scripts

### New Approach: Real Projects ✅

```yaml
# Test against actual open source project
- project-type: "JavaScript"
  repo: "facebook/react"
  owner: "facebook"
  repo-name: "react"
```

**Benefits:**

- Tests real-world complexity and edge cases
- Discovers issues before users do
- No maintenance - projects evolve naturally
- Builds trust - users see which projects we test against

---

## Test Results Interpretation

### Success Indicators ✅

```text
✅ Tested Python project
   Name: requests
   Detected Type: Python
   Version: 2.31.0
   Test Outcome: success
```

This means:

- Detection worked
- Extraction worked
- All metadata retrieved
- No warnings or errors

### Warning Indicators ⚠️

```text
⚠️ Test completed with issues for PHP
   This may show missing extractor implementation
   Name:
   Detected Type:
   Version:
   Test Outcome: success
```

This means:

- Test didn't fail
- But extractor is not implemented
- Basic detection may work
- Full extraction pending

### What "Success with Warnings" Means

Tests can complete with warnings because:

1. **Progressive Enhancement**: We want to see all results, not fail fast
2. **Implementation Tracking**: Warnings show what needs work
3. **Partial Functionality**: Some features may work even if others don't
4. **Non-Blocking**: Users can still use the action for supported languages

---

## Implementation Status Dashboard

### ✅ Fully Working (No Warnings)

- **Python** - `pyproject.toml`, `setup.py`, `setup.cfg`
- **JavaScript/TypeScript** - `package.json`
- **Go** - `go.mod`
- **Rust** - `Cargo.toml` (with some edge cases)
- **Java** - `pom.xml`, `build.gradle`, `build.gradle.kts`

### 🚧 Partially Working (Some Warnings)

- **Ruby** - Detection works, extraction pending
- **PHP** - Detection works, extraction pending
- **C# / .NET** - Detection works, extraction pending
- **Swift** - Detection works, extraction pending
- **Dart/Flutter** - Detection works, extraction pending
- **Docker** - Basic support, needs enhancement
- **Helm** - Basic support, needs enhancement
- **Terraform** - Basic support, needs enhancement

### ⏳ Planned (Not Yet Started)

- **C/C++** - CMake, Autoconf, Meson
- **Scala** - SBT
- **Elixir** - Mix
- **Haskell** - Cabal
- **Julia** - Pkg

---

## How to Fix Warnings

### For Developers

1. **Pick a Language**: Choose one from the "Partially Working" list
2. **Study Examples**: Look at existing extractors (Python, Go, Rust)
3. **Create Extractor**: Build `internal/extractor/<language>/`
4. **Add Tests**: Create unit tests with `testdata/` fixtures
5. **Run Tests**: Verify warnings disappear for that language
6. **Submit PR**: Include tests and documentation

### For Users

**You don't need to do anything!** Warnings don't affect functionality for
supported languages.

If you need a language that shows warnings:

- Check implementation status above
- Wait for the team to create the extractor
- Or contribute an implementation yourself
- Use language-specific actions in the meantime

---

## Test Matrix Summary

Our test suite includes:

| Category | Count | Status |
| -------- | ----- | ------ |
| Total Projects | 54 | Testing |
| Languages Tested | 18 | Expanding |
| Fully Implemented | 5 | ✅ Working |
| Partially Implemented | 8 | 🚧 In Progress |
| Planned | 5 | ⏳ Future |
| Expected Warnings | 20-40 | Normal |
| Actual Failures | 0-5 | Investigating |

---

## FAQs

### Q: Are the warnings a problem?

**A:** No! Warnings are normal and help track implementation progress. They
don't show test failures.

### Q: Why don't you remove tests that produce warnings?

**A:** Because we want to:

1. Track progress on implementation
2. Test detection even if extraction isn't done
3. See the full picture of what works and what doesn't
4. Catch regressions as we add features

### Q: Will the warnings go away?

**A:** Yes, as the team implements extractors, warnings will decrease. Our goal
is <5 warnings in the final version.

### Q: Can I use the action if my language shows warnings?

**A:** It depends:

- If detection works: You'll get basic metadata
- If extraction works partially: You'll get some fields
- If nothing works: Use a language-specific action instead

### Q: How do I know what will work for my project?

**A:** Check the "Implementation Status Dashboard" section above. Languages
marked ✅ are fully functional.

### Q: Why 54 projects? Why not 100? Or 500?

**A:** Balance between:

- **Coverage**: Test variety of patterns (54 covers major patterns)
- **CI Time**: Tests complete in ~20 minutes
- **Maintenance**: More projects = more potential for upstream changes
- **Value**: Diminishing returns after ~3 projects per language

### Q: Can I suggest a project to add?

**A:** Yes! Open an issue with:

- Project name and GitHub URL
- Why it's representative of that language
- Any unique features it would test

---

## Related Documentation

- [Testing Strategy](./TESTING_STRATEGY.md) - Testing approach
- [Testing Guide](../TESTING_GUIDE.md) - How to write tests
- [Contributing](../CONTRIBUTING.md) - How to contribute extractors

---

## Summary

**TL;DR:**

- 33 warnings = Expected and normal ✅
- Warnings = Implementation tracking, not failures 📊
- Tests work = Detecting real-world complexity 🎯
- No action needed = Unless you want to contribute 🤝

The warnings you see are a **feature, not a bug**. They give us visibility
into which extractors need implementation while still testing detection and
other functionality.

**When in doubt**: Check if your language is in the "✅ Fully Working" list.
If yes, you're good to go!
