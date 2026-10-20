// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package version

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// testdata/onap/version.properties is a verbatim copy of
// onap/cps version.properties at commit
// a0bc7132af6eb60fa10c5d2dd9b7bfcf74ced096 (git blob 3a0d9f7b), under its
// own Apache-2.0 SPDX header: the major/minor/patch trio plus
// base_version, release_version and snapshot_version derived through
// ${...}. Keep it byte-identical to upstream; to refresh it, copy the file
// at a new commit and update this comment and the expected version.
func TestExtractVersionPropertiesONAPFixture(t *testing.T) {
	info, ok := ExtractVersionProperties(filepath.Join("testdata", "onap"))
	if !ok {
		t.Fatal("ExtractVersionProperties() ok = false, want true")
	}
	if info.Version != "3.8.6" {
		t.Errorf("Version = %q, want 3.8.6", info.Version)
	}
	if info.Source != "version.properties" {
		t.Errorf("Source = %q, want version.properties", info.Source)
	}
}

// Each fixture mirrors one release-file shape found across ONAP and
// O-RAN-SC repositories; every shape derives base_version from its own
// keys through ${...}.
func TestExtractVersionPropertiesShapes(t *testing.T) {
	tests := []struct {
		dir  string
		want string
	}{
		{dir: "onap", want: "3.8.6"},
		{dir: "release-name", want: "3.3.4"},
		{dir: "major-version", want: "2.0.0"},
		{dir: "sprint-number", want: "1.13.0"},
	}

	for _, tt := range tests {
		t.Run(tt.dir, func(t *testing.T) {
			info, ok := ExtractVersionProperties(filepath.Join("testdata", tt.dir))
			if !ok {
				t.Fatal("ExtractVersionProperties() ok = false, want true")
			}
			if info.Version != tt.want {
				t.Errorf("Version = %q, want %q", info.Version, tt.want)
			}
		})
	}
}

// Resolution uses only the file's own keys, and a key whose references
// cannot be resolved counts as absent rather than leaking ${...}.
func TestExtractVersionPropertiesResolution(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
		wantOK  bool
	}{
		{
			name:    "nested references resolve recursively",
			content: "a=1\nb=2\nc=3\nbase=${a}.${b}.${c}\nrelease_version=${base}\n",
			want:    "1.2.3",
			wantOK:  true,
		},
		{
			name:    "base_version wins over release_version",
			content: "base_version=4.5.6\nrelease_version=9.9.9\n",
			want:    "4.5.6",
			wantOK:  true,
		},
		{
			name:    "trio wins over a literal release_version",
			content: "major=1\nminor=2\npatch=3\nrelease_version=7.7.7\n",
			want:    "1.2.3",
			wantOK:  true,
		},
		{
			name:    "unknown reference falls through to literal version",
			content: "base_version=${missing}.0.0\nversion=0.5.0\n",
			want:    "0.5.0",
			wantOK:  true,
		},
		{
			name:    "self reference is unresolved",
			content: "base_version=${base_version}\n",
			wantOK:  false,
		},
		{
			name:    "cycle is unresolved",
			content: "a=${b}\nb=${a}\nbase_version=${a}\n",
			wantOK:  false,
		},
		{
			name:    "unterminated reference is unresolved",
			content: "major_version=1\nbase_version=${major_version\n",
			wantOK:  false,
		},
		{
			name:    "environment is never read",
			content: "base_version=${HOME}\n",
			wantOK:  false,
		},
		{
			name:    "snapshot_version is not a release version",
			content: "x=1.0.0\nsnapshot_version=${x}-SNAPSHOT\n",
			wantOK:  false,
		},
		{
			name:    "expansion over the value cap is absent",
			content: "a=" + strings.Repeat("x", 200) + "\nbase_version=${a}${a}\nversion=0.5.0\n",
			want:    "0.5.0",
			wantOK:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "version.properties", tt.content)
			info, ok := ExtractVersionProperties(dir)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v (info %+v)", ok, tt.wantOK, info)
			}
			if ok && info.Version != tt.want {
				t.Errorf("Version = %q, want %q", info.Version, tt.want)
			}
		})
	}
}

// repeatedReferenceChain returns keys k0..k16 where each key repeats the
// previous reference four times: within the depth limit, but k16 would
// expand to 4^16 bytes (4 GiB) if expansion were unbounded.
func repeatedReferenceChain() string {
	var sb strings.Builder
	sb.WriteString("k0=x\n")
	for i := 1; i <= maxPropertyDepth; i++ {
		ref := fmt.Sprintf("${k%d}", i-1)
		fmt.Fprintf(&sb, "k%d=%s\n", i, strings.Repeat(ref, 4))
	}
	return sb.String()
}

// A repeated-reference chain must neither exhaust the runner nor block
// a valid version: unused keys are never expanded, and an expansion the
// version depends on is rejected once it passes the value cap.
func TestExtractVersionPropertiesRepeatedReferenceChain(t *testing.T) {
	tests := []struct {
		name   string
		extra  string
		want   string
		wantOK bool
	}{
		{name: "unused chain beside a valid trio", extra: "major=1\nminor=2\npatch=3\n", want: "1.2.3", wantOK: true},
		{name: "base_version depends on the chain", extra: "base_version=${k16}\n", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "version.properties", repeatedReferenceChain()+tt.extra)

			type result struct {
				info *VersionInfo
				ok   bool
			}
			done := make(chan result, 1)
			go func() {
				info, ok := ExtractVersionProperties(dir)
				done <- result{info, ok}
			}()
			select {
			case got := <-done:
				if got.ok != tt.wantOK {
					t.Fatalf("ok = %v, want %v (info %+v)", got.ok, tt.wantOK, got.info)
				}
				if got.ok && got.info.Version != tt.want {
					t.Errorf("Version = %q, want %q", got.info.Version, tt.want)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("ExtractVersionProperties did not finish on a repeated-reference chain")
			}
		})
	}
}

// The resolver charges every byte it copies and stays within budget on
// the chain, without retaining an expansion larger than the value cap.
func TestPropertyResolverBudgets(t *testing.T) {
	r := newPropertyResolver(parseVersionProperties(repeatedReferenceChain()))
	if _, ok := r.resolve(fmt.Sprintf("k%d", maxPropertyDepth), 0); ok {
		t.Fatal("resolve(k16) ok = true, want false")
	}
	if r.work > maxPropertyWorkBytes {
		t.Errorf("work = %d, want <= %d", r.work, maxPropertyWorkBytes)
	}
	for key, cached := range r.memo {
		if len(cached.value) > maxPropertyValueBytes {
			t.Errorf("memo[%s] holds %d bytes, want <= %d", key, len(cached.value), maxPropertyValueBytes)
		}
	}

	r = newPropertyResolver(nil)
	chunk := strings.Repeat("x", maxPropertyValueBytes)
	for r.work+len(chunk) <= maxPropertyWorkBytes {
		var sb strings.Builder
		if !r.write(&sb, chunk) {
			t.Fatalf("write refused at work = %d, under the budget", r.work)
		}
	}
	var sb strings.Builder
	before := r.work
	if r.write(&sb, chunk) || sb.Len() != 0 || r.work != before {
		t.Errorf("write over the work budget copied %d bytes, work %d -> %d", sb.Len(), before, r.work)
	}
}

// depthChain returns k0=1 and k1..k15 each referencing the previous key,
// so ${k15} needs 16 reference levels: exactly the nesting limit when
// referenced from a top-level key, and one too many through an alias.
func depthChain() string {
	var sb strings.Builder
	sb.WriteString("k0=1\n")
	for i := 1; i < maxPropertyDepth; i++ {
		fmt.Fprintf(&sb, "k%d=${k%d}\n", i, i-1)
	}
	return sb.String()
}

// A cached expansion must not let a key pass the nesting limit it would
// fail on its own. major resolves the chain first and caches it; minor
// reaches the same chain one level deeper, so it is over the limit, the
// trio is incomplete and the version falls through to release_version.
func TestExtractVersionPropertiesDepthAfterCache(t *testing.T) {
	dir := t.TempDir()
	content := depthChain() +
		"major=${k15}\nminor=${alias}\nalias=${k15}\npatch=0\nrelease_version=9.9.9\n"
	writeFile(t, dir, "version.properties", content)

	info, ok := ExtractVersionProperties(dir)
	if !ok {
		t.Fatal("ExtractVersionProperties() ok = false, want true")
	}
	if info.Version != "9.9.9" {
		t.Errorf("Version = %q, want 9.9.9 (over-depth minor must not complete the trio)", info.Version)
	}
}

// Whether a key resolves must not depend on what was resolved before it.
func TestPropertyResolverDepthIsOrderIndependent(t *testing.T) {
	raw := parseVersionProperties(depthChain() + "major=${k15}\nminor=${alias}\nalias=${k15}\n")

	fresh := newPropertyResolver(raw)
	_, minorFresh := fresh.resolve("minor", 0)

	warm := newPropertyResolver(raw)
	if _, ok := warm.resolve("major", 0); !ok {
		t.Fatal("resolve(major) ok = false, want true at exactly the depth limit")
	}
	_, minorWarm := warm.resolve("minor", 0)

	if minorFresh || minorWarm {
		t.Errorf("resolve(minor) ok = %v fresh, %v after major; want false both ways", minorFresh, minorWarm)
	}
}

// When path_prefix names a sub-project, only that directory's
// version.properties counts. The parent's file is neither read nor
// used as a fallback, so each sub-project reports its own version.
func TestExtractVersionPropertiesPerDirectory(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "version.properties", "major=1\nminor=0\npatch=0\n")

	withOwn := filepath.Join(root, "service-a")
	withoutOwn := filepath.Join(root, "service-b")
	mkdirAll(t, withOwn)
	mkdirAll(t, withoutOwn)
	writeFile(t, withOwn, "version.properties", "major=2\nminor=3\npatch=4\n")

	info, ok := ExtractVersionProperties(withOwn)
	if !ok || info.Version != "2.3.4" {
		t.Errorf("sub-project with its own file: got (%v, %v), want 2.3.4", info, ok)
	}

	if info, ok := ExtractVersionProperties(withoutOwn); ok {
		t.Errorf("sub-project without a file: got %q, want no version (no parent fallback)", info.Version)
	}

	info, ok = ExtractVersionProperties(root)
	if !ok || info.Version != "1.0.0" {
		t.Errorf("repository root: got (%v, %v), want 1.0.0", info, ok)
	}
}

func mkdirAll(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("creating %s: %v", dir, err)
	}
}
