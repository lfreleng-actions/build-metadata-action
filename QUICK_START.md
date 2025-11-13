<!--
SPDX-License-Identifier: Apache-2.0
SPDX-FileCopyrightText: 2025 The Linux Foundation
-->

# Quick Start - Testing New Extractors

## 🚀 You're Ready to Test

All 6 new extractors are complete. Here's how to get started with testing.

---

## ✅ What's Complete

- ✅ Helm extractor (280 lines)
- ✅ Terraform extractor (423 lines)
- ✅ Docker extractor (449 lines)
- ✅ PHP extractor (408 lines)
- ✅ Swift extractor (575 lines)
- ✅ Dart/Flutter extractor (433 lines)
- ✅ Sample test file (helm_test.go)
- ✅ Sample testdata (Helm Chart.yaml)
- ✅ All documentation

---

## 🏃 Quick Test

### 1. Install Dependencies

```bash
cd build-metadata-action
go mod download
go mod tidy
```

### 2. Run Existing Helm Test

```bash
go test ./internal/extractor/helm -v
```

You should see output like:

```text
=== RUN   TestExtractor_Name
--- PASS: TestExtractor_Name (0.00s)
=== RUN   TestExtractor_Priority
--- PASS: TestExtractor_Priority (0.00s)
...
PASS
ok      github.com/lfreleng-actions/build-metadata-action/internal/extractor/helm
```

### 3. Check Coverage

```bash
go test ./internal/extractor/helm -cover
```

---

## 📝 Create Your First Test

### For Terraform (Example)

**File:** `internal/extractor/terraform/terraform_test.go`

```golang
package terraform

import (
    "os"
    "path/filepath"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestExtractor_Detect(t *testing.T) {
    tests := []struct {
        name     string
        setup    func(t *testing.T) string
        expected bool
    }{
        {
            name: "valid terraform project",
            setup: func(t *testing.T) string {
                dir := t.TempDir()
                tfPath := filepath.Join(dir, "main.tf")
                err := os.WriteFile(tfPath, []byte(`
terraform {
  required_version = ">=1.5.0"
}
`), 0644)
                require.NoError(t, err)
                return dir
            },
            expected: true,
        },
        {
            name: "no tf files",
            setup: func(t *testing.T) string {
                return t.TempDir()
            },
            expected: false,
        },
    }

    e := NewExtractor()
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            path := tt.setup(t)
            result := e.Detect(path)
            assert.Equal(t, tt.expected, result)
        })
    }
}
```

**Run it:**

```bash
go test ./internal/extractor/terraform -v
```

---

## 📚 Reference Files

### Study These Examples

1. `internal/extractor/helm/helm_test.go` - Complete test file
2. `internal/extractor/python/python_test.go` - Mature test suite
3. `internal/extractor/helm/testdata/complete/Chart.yaml` - Sample testdata

### Read These Guides

1. `TESTING_GUIDE.md` - Comprehensive testing guide
2. `NEW_EXTRACTORS_IMPLEMENTATION.md` - Implementation details
3. `IMPLEMENTATION_CHECKLIST.md` - Progress tracking

---

## 🎯 Your Mission

### This Week

1. Create test files for all 5 remaining extractors
2. Create testdata for each extractor (3-5 examples)
3. Achieve ≥85% test coverage
4. Fix any bugs discovered

### Test File Priority

1. ✅ `helm_test.go` - DONE (sample complete)
2. ⏳ `terraform_test.go` - DO NEXT
3. ⏳ `docker_test.go`
4. ⏳ `php_test.go`
5. ⏳ `swift_test.go`
6. ⏳ `dart_test.go`

---

## 💡 Pro Tips

1. **Copy the pattern** - Use helm_test.go as your template
2. **Start small** - One test function at a time
3. **Real testdata** - Extract from actual projects
4. **Test errors** - Missing files, invalid syntax
5. **Run frequently** - `go test` after each change

---

## 🆘 If Tests Fail

```bash
# Run with verbose output
go test ./internal/extractor/terraform -v -run TestSpecificTest

# Add debug logging in your test
t.Logf("metadata: %+v", metadata)

# Check what the code is actually doing
cat internal/extractor/terraform/terraform.go | grep -A10 "func.*Extract"
```

---

## ✅ Daily Checklist

- [ ] Create test file
- [ ] Write 3-5 test functions
- [ ] Create testdata examples
- [ ] Run tests: `go test -v`
- [ ] Check coverage: `go test -cover`
- [ ] Fix bugs
- [ ] Commit progress

---

## 🎉 When Complete

You'll have:

- 6 fully tested extractors
- ~240+ test functions
- 85%+ test coverage
- 14 languages supported in total
- Production-ready code

---

## Getting Started

**Start Here:** Create `terraform_test.go` using `helm_test.go` as a template.

**Questions?** Check `TESTING_GUIDE.md` or the example test files.

Good luck! 🚀
