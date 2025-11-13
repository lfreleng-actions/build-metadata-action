// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package environment

import (
	"os"
	"runtime"
	"testing"
)

func TestCollect(t *testing.T) {
	// Save original environment
	originalEnv := os.Environ()
	defer func() {
		// Restore original environment
		os.Clearenv()
		for _, e := range originalEnv {
			pair := splitEnv(e)
			if len(pair) == 2 {
				os.Setenv(pair[0], pair[1])
			}
		}
	}()

	tests := []struct {
		name     string
		setupEnv func()
		validate func(*testing.T, *Metadata)
	}{
		{
			name: "GitHub Actions environment",
			setupEnv: func() {
				os.Clearenv()
				os.Setenv("GITHUB_ACTIONS", "true")
				os.Setenv("GITHUB_ACTOR", "testuser")
				os.Setenv("GITHUB_REPOSITORY", "test/repo")
				os.Setenv("GITHUB_WORKFLOW", "CI")
				os.Setenv("RUNNER_OS", "Linux")
				os.Setenv("RUNNER_ARCH", "X64")
			},
			validate: func(t *testing.T, m *Metadata) {
				if !m.CI.IsCI {
					t.Error("Expected IsCI to be true")
				}
				if m.CI.Platform != "github" {
					t.Errorf("Platform = %q, want %q", m.CI.Platform, "github")
				}
				if m.CI.GitHubActor != "testuser" {
					t.Errorf("GitHubActor = %q, want %q", m.CI.GitHubActor, "testuser")
				}
				if m.CI.GitHubRepository != "test/repo" {
					t.Errorf("GitHubRepository = %q, want %q", m.CI.GitHubRepository, "test/repo")
				}
				if m.CI.RunnerOS != "Linux" {
					t.Errorf("RunnerOS = %q, want %q", m.CI.RunnerOS, "Linux")
				}
			},
		},
		{
			name: "GitLab CI environment",
			setupEnv: func() {
				os.Clearenv()
				os.Setenv("GITLAB_CI", "true")
				os.Setenv("CI", "true")
			},
			validate: func(t *testing.T, m *Metadata) {
				if !m.CI.IsCI {
					t.Error("Expected IsCI to be true")
				}
				if m.CI.Platform != "gitlab" {
					t.Errorf("Platform = %q, want %q", m.CI.Platform, "gitlab")
				}
			},
		},
		{
			name: "CircleCI environment",
			setupEnv: func() {
				os.Clearenv()
				os.Setenv("CIRCLECI", "true")
			},
			validate: func(t *testing.T, m *Metadata) {
				if !m.CI.IsCI {
					t.Error("Expected IsCI to be true")
				}
				if m.CI.Platform != "circleci" {
					t.Errorf("Platform = %q, want %q", m.CI.Platform, "circleci")
				}
			},
		},
		{
			name: "Travis CI environment",
			setupEnv: func() {
				os.Clearenv()
				os.Setenv("TRAVIS", "true")
			},
			validate: func(t *testing.T, m *Metadata) {
				if !m.CI.IsCI {
					t.Error("Expected IsCI to be true")
				}
				if m.CI.Platform != "travis" {
					t.Errorf("Platform = %q, want %q", m.CI.Platform, "travis")
				}
			},
		},
		{
			name: "Jenkins environment",
			setupEnv: func() {
				os.Clearenv()
				os.Setenv("JENKINS_HOME", "/var/jenkins")
			},
			validate: func(t *testing.T, m *Metadata) {
				if !m.CI.IsCI {
					t.Error("Expected IsCI to be true")
				}
				if m.CI.Platform != "jenkins" {
					t.Errorf("Platform = %q, want %q", m.CI.Platform, "jenkins")
				}
			},
		},
		{
			name: "Generic CI environment",
			setupEnv: func() {
				os.Clearenv()
				os.Setenv("CI", "true")
			},
			validate: func(t *testing.T, m *Metadata) {
				if !m.CI.IsCI {
					t.Error("Expected IsCI to be true")
				}
				if m.CI.Platform != "unknown" {
					t.Errorf("Platform = %q, want %q", m.CI.Platform, "unknown")
				}
			},
		},
		{
			name: "Local environment (no CI)",
			setupEnv: func() {
				os.Clearenv()
			},
			validate: func(t *testing.T, m *Metadata) {
				if m.CI.IsCI {
					t.Error("Expected IsCI to be false")
				}
				if m.CI.Platform != "local" {
					t.Errorf("Platform = %q, want %q", m.CI.Platform, "local")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupEnv()

			metadata, err := Collect()
			if err != nil {
				t.Fatalf("Collect() error = %v", err)
			}

			if metadata == nil {
				t.Fatal("Collect() returned nil metadata")
			}

			tt.validate(t, metadata)
		})
	}
}

func TestCollectCIEnvironment(t *testing.T) {
	// Save original environment
	originalEnv := os.Environ()
	defer func() {
		os.Clearenv()
		for _, e := range originalEnv {
			pair := splitEnv(e)
			if len(pair) == 2 {
				os.Setenv(pair[0], pair[1])
			}
		}
	}()

	tests := []struct {
		name     string
		setupEnv func()
		want     CIEnvironment
	}{
		{
			name: "complete GitHub Actions environment",
			setupEnv: func() {
				os.Clearenv()
				os.Setenv("GITHUB_ACTIONS", "true")
				os.Setenv("GITHUB_ACTION", "test-action")
				os.Setenv("GITHUB_ACTOR", "octocat")
				os.Setenv("GITHUB_REPOSITORY", "owner/repo")
				os.Setenv("GITHUB_EVENT_NAME", "push")
				os.Setenv("GITHUB_WORKFLOW", "Test Workflow")
				os.Setenv("GITHUB_RUN_NUMBER", "42")
				os.Setenv("GITHUB_RUN_ATTEMPT", "1")
				os.Setenv("RUNNER_OS", "Linux")
				os.Setenv("RUNNER_ARCH", "X64")
				os.Setenv("RUNNER_NAME", "GitHub Actions 1")
			},
			want: CIEnvironment{
				Platform:         "github",
				IsCI:             true,
				RunnerOS:         "Linux",
				RunnerArch:       "X64",
				RunnerName:       "GitHub Actions 1",
				GitHubAction:     "test-action",
				GitHubActor:      "octocat",
				GitHubRepository: "owner/repo",
				GitHubEventName:  "push",
				GitHubWorkflow:   "Test Workflow",
				GitHubRunNumber:  "42",
				GitHubRunAttempt: "1",
			},
		},
		{
			name: "minimal local environment",
			setupEnv: func() {
				os.Clearenv()
			},
			want: CIEnvironment{
				Platform: "local",
				IsCI:     false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupEnv()

			got := collectCIEnvironment()

			if got.Platform != tt.want.Platform {
				t.Errorf("Platform = %q, want %q", got.Platform, tt.want.Platform)
			}
			if got.IsCI != tt.want.IsCI {
				t.Errorf("IsCI = %v, want %v", got.IsCI, tt.want.IsCI)
			}
			if got.RunnerOS != tt.want.RunnerOS {
				t.Errorf("RunnerOS = %q, want %q", got.RunnerOS, tt.want.RunnerOS)
			}
			if got.GitHubActor != tt.want.GitHubActor {
				t.Errorf("GitHubActor = %q, want %q", got.GitHubActor, tt.want.GitHubActor)
			}
			if got.GitHubRepository != tt.want.GitHubRepository {
				t.Errorf("GitHubRepository = %q, want %q", got.GitHubRepository, tt.want.GitHubRepository)
			}
		})
	}
}

func TestCollectRuntimeEnvironment(t *testing.T) {
	env := collectRuntimeEnvironment()

	// Test that basic runtime info is collected
	if env.OS != runtime.GOOS {
		t.Errorf("OS = %q, want %q", env.OS, runtime.GOOS)
	}
	if env.Arch != runtime.GOARCH {
		t.Errorf("Arch = %q, want %q", env.Arch, runtime.GOARCH)
	}
	if env.GoVersion != runtime.Version() {
		t.Errorf("GoVersion = %q, want %q", env.GoVersion, runtime.Version())
	}

	// Test that environment map is initialized
	if env.Environment == nil {
		t.Error("Environment map is nil")
	}
}

func TestDetectSetupActions(t *testing.T) {
	// Save original environment
	originalEnv := os.Environ()
	defer func() {
		os.Clearenv()
		for _, e := range originalEnv {
			pair := splitEnv(e)
			if len(pair) == 2 {
				os.Setenv(pair[0], pair[1])
			}
		}
	}()

	tests := []struct {
		name       string
		setupEnv   func()
		expectKeys []string
	}{
		{
			name: "setup-python detected",
			setupEnv: func() {
				os.Clearenv()
				os.Setenv("pythonLocation", "/opt/hostedtoolcache/Python/3.9.0/x64")
			},
			expectKeys: []string{"setup-python"},
		},
		{
			name: "setup-java detected",
			setupEnv: func() {
				os.Clearenv()
				os.Setenv("JAVA_HOME", "/usr/lib/jvm/java-11")
			},
			expectKeys: []string{"setup-java"},
		},
		{
			name: "no setup actions",
			setupEnv: func() {
				os.Clearenv()
			},
			expectKeys: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupEnv()

			metadata := &Metadata{
				SetupActions: make(map[string]SetupActionInfo),
			}

			detectSetupActions(metadata)

			// Check if expected keys are present
			for _, key := range tt.expectKeys {
				if _, ok := metadata.SetupActions[key]; !ok {
					t.Errorf("Expected setup action %q not found", key)
				}
			}

			// Check no unexpected keys if expectKeys is empty
			if len(tt.expectKeys) == 0 && len(metadata.SetupActions) > 0 {
				t.Errorf("Expected no setup actions, got %d", len(metadata.SetupActions))
			}
		})
	}
}

func TestExtractVersion(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{
			name:   "python version",
			output: "Python 3.9.7",
			want:   "3.9.7",
		},
		{
			name:   "node version",
			output: "v18.16.0",
			want:   "v18.16.0",
		},
		{
			name:   "go version",
			output: "go version go1.21.0 linux/amd64",
			want:   "go1.21.0",
		},
		{
			name:   "version keyword",
			output: "tool version 2.5.1",
			want:   "2.5.1",
		},
		{
			name:   "multiline output",
			output: "Ruby 3.2.0\nCopyright (c) 2023",
			want:   "3.2.0",
		},
		{
			name:   "no version pattern",
			output: "some random text",
			want:   "some random text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractVersion(tt.output)
			if got != tt.want {
				t.Errorf("extractVersion(%q) = %q, want %q", tt.output, got, tt.want)
			}
		})
	}
}

func TestIsVersionLike(t *testing.T) {
	tests := []struct {
		name string
		s    string
		want bool
	}{
		{
			name: "valid version",
			s:    "3.9.7",
			want: true,
		},
		{
			name: "valid version with v prefix",
			s:    "v1.2.3",
			want: true,
		},
		{
			name: "valid version with V prefix",
			s:    "V2.0.0",
			want: true,
		},
		{
			name: "no dots",
			s:    "123",
			want: false,
		},
		{
			name: "starts with letter",
			s:    "abc.def",
			want: false,
		},
		{
			name: "empty string",
			s:    "",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isVersionLike(tt.s)
			if got != tt.want {
				t.Errorf("isVersionLike(%q) = %v, want %v", tt.s, got, tt.want)
			}
		})
	}
}

func TestGetEnvironmentVariable(t *testing.T) {
	// Save original environment
	originalValue := os.Getenv("TEST_VAR")
	defer func() {
		if originalValue != "" {
			os.Setenv("TEST_VAR", originalValue)
		} else {
			os.Unsetenv("TEST_VAR")
		}
	}()

	testValue := "test_value_12345"
	os.Setenv("TEST_VAR", testValue)

	got := GetEnvironmentVariable("TEST_VAR")
	if got != testValue {
		t.Errorf("GetEnvironmentVariable(\"TEST_VAR\") = %q, want %q", got, testValue)
	}

	// Test non-existent variable
	got = GetEnvironmentVariable("NON_EXISTENT_VAR_XYZ")
	if got != "" {
		t.Errorf("GetEnvironmentVariable(\"NON_EXISTENT_VAR_XYZ\") = %q, want empty string", got)
	}
}

func TestGetAllEnvironmentVariables(t *testing.T) {
	// Save original environment
	originalValue := os.Getenv("TEST_VAR_ALL")
	defer func() {
		if originalValue != "" {
			os.Setenv("TEST_VAR_ALL", originalValue)
		} else {
			os.Unsetenv("TEST_VAR_ALL")
		}
	}()

	testKey := "TEST_VAR_ALL"
	testValue := "test_value_all"
	os.Setenv(testKey, testValue)

	env := GetAllEnvironmentVariables()

	if env == nil {
		t.Fatal("GetAllEnvironmentVariables() returned nil")
	}

	if got, ok := env[testKey]; !ok {
		t.Errorf("Expected key %q not found in environment", testKey)
	} else if got != testValue {
		t.Errorf("env[%q] = %q, want %q", testKey, got, testValue)
	}
}

func TestIsCI(t *testing.T) {
	// Save original environment
	originalEnv := os.Environ()
	defer func() {
		os.Clearenv()
		for _, e := range originalEnv {
			pair := splitEnv(e)
			if len(pair) == 2 {
				os.Setenv(pair[0], pair[1])
			}
		}
	}()

	tests := []struct {
		name     string
		setupEnv func()
		want     bool
	}{
		{
			name: "GitHub Actions",
			setupEnv: func() {
				os.Clearenv()
				os.Setenv("GITHUB_ACTIONS", "true")
			},
			want: true,
		},
		{
			name: "GitLab CI",
			setupEnv: func() {
				os.Clearenv()
				os.Setenv("GITLAB_CI", "true")
			},
			want: true,
		},
		{
			name: "CircleCI",
			setupEnv: func() {
				os.Clearenv()
				os.Setenv("CIRCLECI", "true")
			},
			want: true,
		},
		{
			name: "Travis CI",
			setupEnv: func() {
				os.Clearenv()
				os.Setenv("TRAVIS", "true")
			},
			want: true,
		},
		{
			name: "Jenkins",
			setupEnv: func() {
				os.Clearenv()
				os.Setenv("JENKINS_HOME", "/var/jenkins")
			},
			want: true,
		},
		{
			name: "Generic CI",
			setupEnv: func() {
				os.Clearenv()
				os.Setenv("CI", "true")
			},
			want: true,
		},
		{
			name: "Local environment",
			setupEnv: func() {
				os.Clearenv()
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupEnv()

			got := IsCI()
			if got != tt.want {
				t.Errorf("IsCI() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetCIPlatform(t *testing.T) {
	// Save original environment
	originalEnv := os.Environ()
	defer func() {
		os.Clearenv()
		for _, e := range originalEnv {
			pair := splitEnv(e)
			if len(pair) == 2 {
				os.Setenv(pair[0], pair[1])
			}
		}
	}()

	tests := []struct {
		name     string
		setupEnv func()
		want     string
	}{
		{
			name: "GitHub Actions",
			setupEnv: func() {
				os.Clearenv()
				os.Setenv("GITHUB_ACTIONS", "true")
			},
			want: "github",
		},
		{
			name: "GitLab CI",
			setupEnv: func() {
				os.Clearenv()
				os.Setenv("GITLAB_CI", "true")
			},
			want: "gitlab",
		},
		{
			name: "CircleCI",
			setupEnv: func() {
				os.Clearenv()
				os.Setenv("CIRCLECI", "true")
			},
			want: "circleci",
		},
		{
			name: "Travis CI",
			setupEnv: func() {
				os.Clearenv()
				os.Setenv("TRAVIS", "true")
			},
			want: "travis",
		},
		{
			name: "Jenkins",
			setupEnv: func() {
				os.Clearenv()
				os.Setenv("JENKINS_HOME", "/var/jenkins")
			},
			want: "jenkins",
		},
		{
			name: "Generic CI",
			setupEnv: func() {
				os.Clearenv()
				os.Setenv("CI", "true")
			},
			want: "unknown",
		},
		{
			name: "Local environment",
			setupEnv: func() {
				os.Clearenv()
			},
			want: "local",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupEnv()

			got := GetCIPlatform()
			if got != tt.want {
				t.Errorf("GetCIPlatform() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRuntimeEnvironmentFields(t *testing.T) {
	metadata, err := Collect()
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}

	// Verify Runtime environment is populated
	if metadata.Runtime.OS == "" {
		t.Error("Runtime.OS is empty")
	}
	if metadata.Runtime.Arch == "" {
		t.Error("Runtime.Arch is empty")
	}
	if metadata.Runtime.GoVersion == "" {
		t.Error("Runtime.GoVersion is empty")
	}
	if metadata.Runtime.Environment == nil {
		t.Error("Runtime.Environment is nil")
	}
}

func TestMetadataInitialization(t *testing.T) {
	metadata, err := Collect()
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}

	// Verify all maps are initialized
	if metadata.Tools == nil {
		t.Error("Tools map is nil")
	}
	if metadata.SetupActions == nil {
		t.Error("SetupActions map is nil")
	}
}

// Helper function to split environment variable strings
func splitEnv(s string) []string {
	idx := 0
	for i, c := range s {
		if c == '=' {
			idx = i
			break
		}
	}
	if idx == 0 {
		return []string{s}
	}
	return []string{s[:idx], s[idx+1:]}
}
