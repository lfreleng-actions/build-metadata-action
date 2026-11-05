// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Linux Foundation

package environment

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// getToolVersion must not mistake stderr noise for a version, yet must
// still read a version that a tool prints only on stderr.
func TestGetToolVersionSeparatesStreams(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake tools are POSIX shell scripts")
	}

	tests := []struct {
		name   string
		script string
		want   string
	}{
		{
			name: "java with JAVA_TOOL_OPTIONS notice",
			script: `echo 'Picked up JAVA_TOOL_OPTIONS: -Xmx512m' >&2
echo 'openjdk version "21.0.5" 2024-10-15' >&2
echo 'OpenJDK Runtime Environment (build 21.0.5+11)' >&2
`,
			want: `"21.0.5"`,
		},
		{
			name: "version on stdout, noise on stderr",
			script: `echo 'warning: config 1.0 is deprecated' >&2
echo 'tool 4.5.6'
`,
			want: "4.5.6",
		},
		{
			name: "no version anywhere",
			script: `echo 'unknown build'
echo 'nor here' >&2
`,
			want: "unknown build",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			binDir := t.TempDir()
			script := "#!/bin/sh\n" + tt.script
			if err := os.WriteFile(filepath.Join(binDir, "faketool"), []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", binDir)
			if got := getToolVersion("faketool", "-version"); got != tt.want {
				t.Errorf("getToolVersion = %q, want %q", got, tt.want)
			}
		})
	}
}
