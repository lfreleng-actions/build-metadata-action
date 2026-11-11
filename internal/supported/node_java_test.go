// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package supported

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/lfreleng-actions/build-metadata-action/internal/supported/supportedtest"
)

// nodeSnapshotMatrix and javaSnapshotMatrix are the matrices the
// recorded data and the embedded snapshot both give at fixedNow.
var (
	nodeSnapshotMatrix = []string{"22", "24", "26"}
	javaSnapshotMatrix = []string{"17", "21", "25"}
)

// TestNodeMatrixRecordedLive checks that the Node.js matrix reads
// endoflife.date and the actions/node-versions manifest from the test
// server, live and without a warning.
func TestNodeMatrixRecordedLive(t *testing.T) {
	warned := isolate(t)
	up := supportedtest.NewNodeUpstream(t)
	r := newResolver(t, Settings{Timeout: time.Second, EndOfLifeBaseURL: up.URL, ManifestBaseURL: up.URL})

	rep, err := r.Matrix(context.Background(), "node", "")
	if err != nil {
		t.Fatalf("Matrix: %v", err)
	}
	if !reflect.DeepEqual(rep.Matrix.Versions, nodeSnapshotMatrix) {
		t.Errorf("matrix = %v, want %v", rep.Matrix.Versions, nodeSnapshotMatrix)
	}
	if rep.Matrix.Key != "node-version" || rep.Latest != "26" {
		t.Errorf("key, latest = %q, %q; want node-version, 26", rep.Matrix.Key, rep.Latest)
	}
	if want := []Kind{KindLive, KindLive}; !reflect.DeepEqual(sourceKinds(rep), want) {
		t.Errorf("source kinds = %v, want %v", sourceKinds(rep), want)
	}
	if n := up.Requests(); n != 2 {
		t.Errorf("upstream saw %d requests, want 2: endoflife.date and the manifest", n)
	}
	if warned.Len() != 0 {
		t.Errorf("warnings = %q, want none", warned.String())
	}
}

// TestNodeMatrixNewerThanSnapshot checks that a line the embedded
// snapshot does not know reaches the matrix when upstream lists it,
// and that a line upstream has retired leaves it.
func TestNodeMatrixNewerThanSnapshot(t *testing.T) {
	isolate(t)
	up := supportedtest.NewNodeUpstream(t)
	up.ServeReleases(supportedtest.NodeReleasesNewer)
	up.ServeManifest(supportedtest.NodeManifestNewer)
	r := newResolver(t, Settings{Timeout: time.Second, EndOfLifeBaseURL: up.URL, ManifestBaseURL: up.URL})

	rep, err := r.Matrix(context.Background(), "nodejs", "")
	if err != nil {
		t.Fatalf("Matrix: %v", err)
	}
	if want := []string{"24", "26", "28"}; !reflect.DeepEqual(rep.Matrix.Versions, want) {
		t.Errorf("matrix = %v, want %v", rep.Matrix.Versions, want)
	}
	if rep.Latest != "28" {
		t.Errorf("latest = %q, want 28", rep.Latest)
	}
}

// TestMatrixForDetectsProjectConstraint checks that MatrixFor passes
// the project directory to the module, which reads .nvmrc there, and
// that an explicit constraint wins over the file.
func TestMatrixForDetectsProjectConstraint(t *testing.T) {
	isolate(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".nvmrc"), []byte("24\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := newResolver(t, Settings{Offline: true, NoCache: true})

	rep, err := r.MatrixFor(context.Background(), "nodejs", MatrixRequest{ProjectDir: dir})
	if err != nil {
		t.Fatalf("MatrixFor: %v", err)
	}
	if want := []string{"24"}; !reflect.DeepEqual(rep.Matrix.Versions, want) {
		t.Errorf("matrix = %v, want %v", rep.Matrix.Versions, want)
	}
	if rep.Constraint == nil || rep.Constraint.Origin != ".nvmrc" {
		t.Errorf("constraint = %+v, want one detected in .nvmrc", rep.Constraint)
	}

	rep, err = r.MatrixFor(context.Background(), "nodejs", MatrixRequest{Constraint: ">=22", ProjectDir: dir})
	if err != nil {
		t.Fatalf("MatrixFor: %v", err)
	}
	if !reflect.DeepEqual(rep.Matrix.Versions, nodeSnapshotMatrix) {
		t.Errorf("explicit matrix = %v, want %v", rep.Matrix.Versions, nodeSnapshotMatrix)
	}
	if rep.Constraint == nil || rep.Constraint.Origin != "" {
		t.Errorf("constraint = %+v, want the explicit one", rep.Constraint)
	}
}

// TestJavaMatrixRecordedLive checks that AdoptiumBaseURL reaches the
// module: the available releases are read from the test server, and
// both sources answer live without a warning.
func TestJavaMatrixRecordedLive(t *testing.T) {
	warned := isolate(t)
	up := supportedtest.NewJavaUpstream(t)
	r := newResolver(t, Settings{Timeout: time.Second, EndOfLifeBaseURL: up.URL, AdoptiumBaseURL: up.URL})

	rep, err := r.Matrix(context.Background(), "java", "")
	if err != nil {
		t.Fatalf("Matrix: %v", err)
	}
	if !reflect.DeepEqual(rep.Matrix.Versions, javaSnapshotMatrix) {
		t.Errorf("matrix = %v, want %v", rep.Matrix.Versions, javaSnapshotMatrix)
	}
	if rep.Matrix.Key != "java-version" || rep.Latest != "25" {
		t.Errorf("key, latest = %q, %q; want java-version, 25", rep.Matrix.Key, rep.Latest)
	}
	if want := []Kind{KindLive, KindLive}; !reflect.DeepEqual(sourceKinds(rep), want) {
		t.Errorf("source kinds = %v, want %v", sourceKinds(rep), want)
	}
	if n := up.Requests(); n != 2 {
		t.Errorf("upstream saw %d requests, want 2: endoflife.date and Adoptium", n)
	}
	if warned.Len() != 0 {
		t.Errorf("warnings = %q, want none", warned.String())
	}
}

// TestJavaMatrixNewerThanSnapshot checks that a line the embedded
// snapshot does not know reaches the matrix when upstream lists it.
func TestJavaMatrixNewerThanSnapshot(t *testing.T) {
	isolate(t)
	up := supportedtest.NewJavaUpstream(t)
	up.ServeReleases(supportedtest.JavaReleasesNewer)
	up.ServeManifest(supportedtest.AdoptiumReleasesNewer)
	r := newResolver(t, Settings{Timeout: time.Second, EndOfLifeBaseURL: up.URL, AdoptiumBaseURL: up.URL})

	rep, err := r.Matrix(context.Background(), "java", "21")
	if err != nil {
		t.Fatalf("Matrix: %v", err)
	}
	if want := []string{"21", "25", "29"}; !reflect.DeepEqual(rep.Matrix.Versions, want) {
		t.Errorf("newer matrix = %v, want %v", rep.Matrix.Versions, want)
	}
}

// fakeResolver answers MatrixFor with a fixed report and error, and
// records the requests it saw.
type fakeResolver struct {
	rep    Report
	err    error
	seen   []MatrixRequest
	closed bool
}

func (f *fakeResolver) MatrixFor(_ context.Context, _ string, req MatrixRequest) (Report, error) {
	f.seen = append(f.seen, req)
	return f.rep, f.err
}

func (f *fakeResolver) Close() error {
	f.closed = true
	return nil
}

func reportOf(versions ...string) Report {
	var rep Report
	rep.Matrix.Versions = versions
	return rep
}

func TestResolveWithFallback(t *testing.T) {
	syntaxErr := errors.Join(errors.New("bad"), ErrConstraintSyntax)
	tests := []struct {
		name         string
		primary      *fakeResolver
		snapshot     *fakeResolver
		want         []string
		wantErr      bool
		wantSnapshot bool
		wantWarning  string
	}{
		{"primary answers", &fakeResolver{rep: reportOf("24")}, &fakeResolver{rep: reportOf("22")},
			[]string{"24"}, false, false, ""},
		{"no primary", nil, &fakeResolver{rep: reportOf("22")},
			[]string{"22"}, false, true, ""},
		{"primary fails", &fakeResolver{err: errors.New("down")}, &fakeResolver{rep: reportOf("22")},
			[]string{"22"}, false, true, "Failed to resolve nodejs versions (down)"},
		{"primary empty", &fakeResolver{rep: reportOf()}, &fakeResolver{rep: reportOf("22")},
			[]string{"22"}, false, true, "Resolved no nodejs versions"},
		{"constraint syntax", &fakeResolver{err: syntaxErr}, &fakeResolver{rep: reportOf("22")},
			nil, true, false, ""},
		{"snapshot empty", nil, &fakeResolver{rep: reportOf()},
			nil, true, true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warned := isolate(t)
			var primary ProjectMatrixResolver
			if tt.primary != nil {
				primary = tt.primary
			}
			built := false
			req := MatrixRequest{Constraint: ">=22", ProjectDir: "/project"}
			rep, err := ResolveWithFallback(context.Background(), "nodejs", req, primary,
				func() (ProjectMatrixResolver, error) {
					built = true
					return tt.snapshot, nil
				}, nil)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && !reflect.DeepEqual(rep.Matrix.Versions, tt.want) {
				t.Errorf("matrix = %v, want %v", rep.Matrix.Versions, tt.want)
			}
			if built != tt.wantSnapshot {
				t.Errorf("snapshot built = %v, want %v", built, tt.wantSnapshot)
			}
			if built {
				if !tt.snapshot.closed {
					t.Error("snapshot resolver was not closed")
				}
				if len(tt.snapshot.seen) != 1 || tt.snapshot.seen[0] != req {
					t.Errorf("snapshot saw %+v, want %+v", tt.snapshot.seen, req)
				}
			}
			if tt.wantWarning == "" && warned.Len() != 0 {
				t.Errorf("warnings = %q, want none", warned.String())
			}
			if tt.wantWarning != "" && !strings.Contains(warned.String(), tt.wantWarning) {
				t.Errorf("warnings = %q, want %q", warned.String(), tt.wantWarning)
			}
		})
	}
}

// TestResolveWithFallbackWarnsThroughCaller checks that the retry
// warning reaches the caller's warn function, not the package writer.
func TestResolveWithFallbackWarnsThroughCaller(t *testing.T) {
	warned := isolate(t)
	var got []string
	warn := func(format string, args ...any) { got = append(got, fmt.Sprintf(format, args...)) }
	_, err := ResolveWithFallback(context.Background(), "java", MatrixRequest{},
		&fakeResolver{err: errors.New("down")},
		func() (ProjectMatrixResolver, error) { return &fakeResolver{rep: reportOf("21")}, nil }, warn)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !strings.Contains(got[0], "Failed to resolve java versions (down)") {
		t.Errorf("caller warnings = %q, want the retry warning", got)
	}
	if warned.Len() != 0 {
		t.Errorf("package warnings = %q, want none", warned.String())
	}
}

func TestResolveWithFallbackSnapshotBuildFails(t *testing.T) {
	isolate(t)
	want := errors.New("no snapshot")
	_, err := ResolveWithFallback(context.Background(), "java", MatrixRequest{}, nil,
		func() (ProjectMatrixResolver, error) { return nil, want }, nil)
	if !errors.Is(err, want) {
		t.Errorf("err = %v, want %v", err, want)
	}
}

func TestMatrixJSON(t *testing.T) {
	rep := reportOf("17", "21")
	rep.Matrix.Key = "java-version"
	if got, want := MatrixJSON(rep), `{"java-version": ["17", "21"]}`; got != want {
		t.Errorf("MatrixJSON = %s, want %s", got, want)
	}
	rep = reportOf()
	rep.Matrix.Key = "node-version"
	if got, want := MatrixJSON(rep), `{"node-version": []}`; got != want {
		t.Errorf("MatrixJSON = %s, want %s", got, want)
	}
}
