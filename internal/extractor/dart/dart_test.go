// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package dart

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractor_Name(t *testing.T) {
	e := NewExtractor()
	assert.Equal(t, "dart", e.Name())
}

func TestExtractor_Priority(t *testing.T) {
	e := NewExtractor()
	assert.Equal(t, 1, e.Priority())
}

func TestExtractor_Detect(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(t *testing.T) string
		cleanup  func(string)
		expected bool
	}{
		{
			name: "valid pubspec.yaml",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				pubspecPath := filepath.Join(dir, "pubspec.yaml")
				err := os.WriteFile(pubspecPath, []byte(`name: my_package
version: 1.0.0
`), 0644)
				require.NoError(t, err)
				return dir
			},
			cleanup:  func(s string) {},
			expected: true,
		},
		{
			name: "missing pubspec.yaml",
			setup: func(t *testing.T) string {
				return t.TempDir()
			},
			cleanup:  func(s string) {},
			expected: false,
		},
		{
			name: "non-existent directory",
			setup: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "nonexistent")
			},
			cleanup:  func(s string) {},
			expected: false,
		},
	}

	e := NewExtractor()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup(t)
			defer tt.cleanup(path)
			result := e.Detect(path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractor_Extract_BasicDart(t *testing.T) {
	dir := t.TempDir()
	pubspecPath := filepath.Join(dir, "pubspec.yaml")

	pubspecContent := `name: my_dart_package
description: A Dart package
version: 1.2.3
homepage: https://example.com
repository: https://github.com/example/package

environment:
  sdk: '>=3.0.0 <4.0.0'

dependencies:
  http: ^1.1.0
  json_annotation: ^4.8.0

dev_dependencies:
  test: ^1.24.0
  build_runner: ^2.4.0
`

	err := os.WriteFile(pubspecPath, []byte(pubspecContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)
	require.NotNil(t, metadata)

	// Common metadata
	assert.Equal(t, "my_dart_package", metadata.Name)
	assert.Equal(t, "1.2.3", metadata.Version)
	assert.Equal(t, "A Dart package", metadata.Description)
	assert.Equal(t, "https://example.com", metadata.Homepage)
	assert.Equal(t, "https://github.com/example/package", metadata.Repository)
	assert.Equal(t, "pubspec.yaml", metadata.VersionSource)

	// Dart-specific metadata
	assert.Equal(t, "my_dart_package", metadata.LanguageSpecific["package_name"])
	assert.Equal(t, false, metadata.LanguageSpecific["is_flutter"])
	assert.Equal(t, "Dart", metadata.LanguageSpecific["framework"])
	assert.Equal(t, ">=3.0.0 <4.0.0", metadata.LanguageSpecific["dart_sdk"])
}

func TestExtractor_Extract_FlutterApp(t *testing.T) {
	dir := t.TempDir()
	pubspecPath := filepath.Join(dir, "pubspec.yaml")

	pubspecContent := `name: my_flutter_app
description: A Flutter application
version: 2.0.0

environment:
  sdk: '>=3.1.0 <4.0.0'

dependencies:
  flutter:
    sdk: flutter
  cupertino_icons: ^1.0.2
  provider: ^6.0.0

dev_dependencies:
  flutter_test:
    sdk: flutter
  flutter_lints: ^2.0.0

flutter:
  uses-material-design: true
  assets:
    - assets/images/
    - assets/icons/
  fonts:
    - family: Roboto
      fonts:
        - asset: fonts/Roboto-Regular.ttf
        - asset: fonts/Roboto-Bold.ttf
          weight: 700
`

	err := os.WriteFile(pubspecPath, []byte(pubspecContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)

	assert.Equal(t, "my_flutter_app", metadata.Name)
	assert.Equal(t, "2.0.0", metadata.Version)
	assert.Equal(t, true, metadata.LanguageSpecific["is_flutter"])
	assert.Equal(t, "Flutter", metadata.LanguageSpecific["framework"])
	assert.Equal(t, true, metadata.LanguageSpecific["uses_material_design"])
}

func TestExtractor_Extract_Dependencies(t *testing.T) {
	dir := t.TempDir()
	pubspecPath := filepath.Join(dir, "pubspec.yaml")

	pubspecContent := `name: test_package
version: 1.0.0

environment:
  sdk: '>=3.0.0 <4.0.0'

dependencies:
  http: ^1.1.0
  json_annotation: ^4.8.0
  provider: ^6.0.0

dev_dependencies:
  test: ^1.24.0
  build_runner: ^2.4.0
  mockito: ^5.4.0
`

	err := os.WriteFile(pubspecPath, []byte(pubspecContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)

	// Check dependencies
	deps := metadata.LanguageSpecific["dependencies"]
	require.NotNil(t, deps)
	depsMap, ok := deps.(map[string]string)
	require.True(t, ok)
	assert.Equal(t, "^1.1.0", depsMap["http"])
	assert.Equal(t, "^4.8.0", depsMap["json_annotation"])
	assert.Equal(t, 3, metadata.LanguageSpecific["dependency_count"])

	// Check dev dependencies
	devDeps := metadata.LanguageSpecific["dev_dependencies"]
	require.NotNil(t, devDeps)
	devDepsMap, ok := devDeps.(map[string]string)
	require.True(t, ok)
	assert.Equal(t, "^1.24.0", devDepsMap["test"])
	assert.Equal(t, 3, metadata.LanguageSpecific["dev_dependency_count"])
}

func TestExtractor_Extract_DartVersionMatrix(t *testing.T) {
	dir := t.TempDir()
	pubspecPath := filepath.Join(dir, "pubspec.yaml")

	pubspecContent := `name: test_package
version: 1.0.0

environment:
  sdk: '>=3.1.0 <4.0.0'
`

	err := os.WriteFile(pubspecPath, []byte(pubspecContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)

	// Check version matrix
	matrix := metadata.LanguageSpecific["dart_version_matrix"]
	require.NotNil(t, matrix)

	matrixList, ok := matrix.([]string)
	require.True(t, ok)
	assert.Contains(t, matrixList, "3.1")
	assert.Contains(t, matrixList, "3.2")

	// Check matrix JSON
	matrixJSON := metadata.LanguageSpecific["matrix_json"]
	require.NotNil(t, matrixJSON)
	assert.Contains(t, matrixJSON, "dart-version")
	assert.Contains(t, matrixJSON, "3.1")
}

func TestExtractor_Extract_FlutterPlugin(t *testing.T) {
	dir := t.TempDir()
	pubspecPath := filepath.Join(dir, "pubspec.yaml")

	pubspecContent := `name: my_plugin
description: A Flutter plugin
version: 1.0.0

environment:
  sdk: '>=3.0.0 <4.0.0'
  flutter: '>=3.0.0'

dependencies:
  flutter:
    sdk: flutter

flutter:
  plugin:
    platforms:
      android:
        package: com.example.myplugin
        pluginClass: MyPlugin
      ios:
        pluginClass: MyPlugin
      macos:
        pluginClass: MyPlugin
      web:
        pluginClass: MyPlugin
        fileName: my_plugin.dart
`

	err := os.WriteFile(pubspecPath, []byte(pubspecContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)

	assert.Equal(t, true, metadata.LanguageSpecific["is_flutter_plugin"])
	assert.Equal(t, "plugin", metadata.LanguageSpecific["package_type"])

	platforms := metadata.LanguageSpecific["plugin_platforms"]
	require.NotNil(t, platforms)
	platformList, ok := platforms.([]string)
	require.True(t, ok)
	assert.Contains(t, platformList, "android")
	assert.Contains(t, platformList, "ios")
	assert.Contains(t, platformList, "web")
	assert.Equal(t, 4, metadata.LanguageSpecific["plugin_platform_count"])
}

func TestExtractor_Extract_FlutterAssets(t *testing.T) {
	dir := t.TempDir()
	pubspecPath := filepath.Join(dir, "pubspec.yaml")

	pubspecContent := `name: my_app
version: 1.0.0

dependencies:
  flutter:
    sdk: flutter

flutter:
  uses-material-design: true
  assets:
    - assets/images/logo.png
    - assets/icons/
    - assets/data/config.json
  fonts:
    - family: CustomFont
      fonts:
        - asset: fonts/Custom-Regular.ttf
    - family: IconFont
      fonts:
        - asset: fonts/Icons.ttf
`

	err := os.WriteFile(pubspecPath, []byte(pubspecContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)

	// Check assets
	assets := metadata.LanguageSpecific["assets"]
	require.NotNil(t, assets)
	assetList, ok := assets.([]string)
	require.True(t, ok)
	assert.Contains(t, assetList, "assets/images/logo.png")
	assert.Equal(t, 3, metadata.LanguageSpecific["asset_count"])

	// Check fonts
	fonts := metadata.LanguageSpecific["custom_fonts"]
	require.NotNil(t, fonts)
	fontList, ok := fonts.([]string)
	require.True(t, ok)
	assert.Contains(t, fontList, "CustomFont")
	assert.Contains(t, fontList, "IconFont")
	assert.Equal(t, 2, metadata.LanguageSpecific["font_count"])
}

func TestExtractor_Extract_CodeGeneration(t *testing.T) {
	dir := t.TempDir()
	pubspecPath := filepath.Join(dir, "pubspec.yaml")

	pubspecContent := `name: my_app
version: 1.0.0

dependencies:
  flutter:
    sdk: flutter

flutter:
  generate: true
  uses-material-design: true
`

	err := os.WriteFile(pubspecPath, []byte(pubspecContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)

	assert.Equal(t, true, metadata.LanguageSpecific["uses_code_generation"])
}

func TestExtractor_Extract_Executables(t *testing.T) {
	dir := t.TempDir()
	pubspecPath := filepath.Join(dir, "pubspec.yaml")

	pubspecContent := `name: my_cli_tool
version: 1.0.0

environment:
  sdk: '>=3.0.0 <4.0.0'

executables:
  mytool:
  anothertool: another_entry_point
`

	err := os.WriteFile(pubspecPath, []byte(pubspecContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)

	executables := metadata.LanguageSpecific["executables"]
	require.NotNil(t, executables)
	assert.Equal(t, 2, metadata.LanguageSpecific["executable_count"])
}

func TestExtractor_Extract_PublishTo(t *testing.T) {
	dir := t.TempDir()
	pubspecPath := filepath.Join(dir, "pubspec.yaml")

	pubspecContent := `name: private_package
version: 1.0.0
publish_to: none
`

	err := os.WriteFile(pubspecPath, []byte(pubspecContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)

	assert.Equal(t, false, metadata.LanguageSpecific["is_publishable"])
	assert.Equal(t, "none", metadata.LanguageSpecific["publish_to"])
}

func TestExtractor_Extract_Topics(t *testing.T) {
	dir := t.TempDir()
	pubspecPath := filepath.Join(dir, "pubspec.yaml")

	pubspecContent := `name: my_package
version: 1.0.0
topics:
  - dart
  - flutter
  - mobile
  - cross-platform
`

	err := os.WriteFile(pubspecPath, []byte(pubspecContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)

	topics := metadata.LanguageSpecific["topics"]
	require.NotNil(t, topics)
	topicList, ok := topics.([]string)
	require.True(t, ok)
	assert.Contains(t, topicList, "dart")
	assert.Contains(t, topicList, "flutter")
}

func TestExtractor_Extract_MissingFile(t *testing.T) {
	dir := t.TempDir()

	e := NewExtractor()
	_, err := e.Extract(dir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "pubspec.yaml not found")
}

func TestExtractor_Extract_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	pubspecPath := filepath.Join(dir, "pubspec.yaml")

	err := os.WriteFile(pubspecPath, []byte(`invalid: yaml: content:`), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	_, err = e.Extract(dir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse")
}

func TestExtractor_Extract_MinimalPubspec(t *testing.T) {
	dir := t.TempDir()
	pubspecPath := filepath.Join(dir, "pubspec.yaml")

	pubspecContent := `name: minimal_package
version: 0.1.0

environment:
  sdk: '>=3.0.0 <4.0.0'
`

	err := os.WriteFile(pubspecPath, []byte(pubspecContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)

	assert.Equal(t, "minimal_package", metadata.Name)
	assert.Equal(t, "0.1.0", metadata.Version)
}

func TestGenerateDartVersionMatrix(t *testing.T) {
	tests := []struct {
		name          string
		constraint    string
		expectedCount int
		shouldContain []string
	}{
		{
			name:          "greater than or equal 3.1",
			constraint:    ">=3.1.0 <4.0.0",
			expectedCount: 3,
			shouldContain: []string{"3.1", "3.2", "3.3"},
		},
		{
			name:          "greater than or equal 3.0",
			constraint:    ">=3.0.0",
			expectedCount: 4,
			shouldContain: []string{"3.0", "3.1", "3.2", "3.3"},
		},
		{
			name:          "caret constraint 3.2",
			constraint:    "^3.2.0",
			expectedCount: 2,
			shouldContain: []string{"3.2", "3.3"},
		},
		{
			name:          "legacy 2.19",
			constraint:    ">=2.19.0 <3.0.0",
			expectedCount: 4,
			shouldContain: []string{"2.19", "3.0", "3.1"},
		},
		{
			name:          "unknown version defaults",
			constraint:    ">=99.0.0",
			expectedCount: 3,
			shouldContain: []string{"3.1", "3.2", "3.3"},
		},
		{
			name:          "empty constraint defaults",
			constraint:    "",
			expectedCount: 3,
			shouldContain: []string{"3.1", "3.2", "3.3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateDartVersionMatrix(tt.constraint)
			assert.Len(t, result, tt.expectedCount)
			for _, version := range tt.shouldContain {
				assert.Contains(t, result, version)
			}
		})
	}
}

func TestQuoteStrings(t *testing.T) {
	input := []string{"3.1", "3.2", "3.3"}
	expected := []string{`"3.1"`, `"3.2"`, `"3.3"`}

	result := quoteStrings(input)
	assert.Equal(t, expected, result)
}

func TestExtractor_Extract_ComplexFlutterApp(t *testing.T) {
	dir := t.TempDir()
	pubspecPath := filepath.Join(dir, "pubspec.yaml")

	pubspecContent := `name: complex_flutter_app
description: A comprehensive Flutter application
version: 3.2.1
homepage: https://example.com
repository: https://github.com/example/app
issue_tracker: https://github.com/example/app/issues
documentation: https://docs.example.com

environment:
  sdk: '>=3.1.0 <4.0.0'
  flutter: '>=3.10.0'

dependencies:
  flutter:
    sdk: flutter
  cupertino_icons: ^1.0.2
  provider: ^6.0.0
  http: ^1.1.0
  shared_preferences: ^2.2.0

dev_dependencies:
  flutter_test:
    sdk: flutter
  flutter_lints: ^2.0.0
  mockito: ^5.4.0

flutter:
  uses-material-design: true
  generate: true
  assets:
    - assets/images/
    - assets/icons/
  fonts:
    - family: Roboto
      fonts:
        - asset: fonts/Roboto-Regular.ttf
        - asset: fonts/Roboto-Bold.ttf
          weight: 700

topics:
  - flutter
  - mobile
  - cross-platform
`

	err := os.WriteFile(pubspecPath, []byte(pubspecContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)

	// Verify comprehensive extraction
	assert.Equal(t, "complex_flutter_app", metadata.Name)
	assert.Equal(t, "3.2.1", metadata.Version)
	assert.Equal(t, "A comprehensive Flutter application", metadata.Description)
	assert.Equal(t, "https://github.com/example/app", metadata.Repository)
	assert.Equal(t, true, metadata.LanguageSpecific["is_flutter"])
	assert.Equal(t, "Flutter", metadata.LanguageSpecific["framework"])
	assert.Equal(t, true, metadata.LanguageSpecific["uses_material_design"])
	assert.Equal(t, true, metadata.LanguageSpecific["uses_code_generation"])
	assert.NotNil(t, metadata.LanguageSpecific["dependencies"])
	assert.NotNil(t, metadata.LanguageSpecific["dev_dependencies"])
	assert.NotNil(t, metadata.LanguageSpecific["assets"])
	assert.NotNil(t, metadata.LanguageSpecific["custom_fonts"])
	assert.NotNil(t, metadata.LanguageSpecific["topics"])
}
