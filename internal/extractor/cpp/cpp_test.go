// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package cpp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewExtractor(t *testing.T) {
	e := NewExtractor()
	assert.NotNil(t, e)
	assert.Equal(t, "cpp", e.Name())
	assert.Equal(t, 1, e.Priority())
}

func TestDetect(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		expected bool
	}{
		{
			name: "CMakeLists.txt",
			files: map[string]string{
				"CMakeLists.txt": "project(test)",
			},
			expected: true,
		},
		{
			name: ".qmake.conf",
			files: map[string]string{
				".qmake.conf": "MODULE_VERSION = 1.0.0",
			},
			expected: true,
		},
		{
			name: "Makefile",
			files: map[string]string{
				"Makefile": "all:\n\tgcc main.c",
			},
			expected: true,
		},
		{
			name: "configure.ac",
			files: map[string]string{
				"configure.ac": "AC_INIT([test], [1.0])",
			},
			expected: true,
		},
		{
			name: "meson.build",
			files: map[string]string{
				"meson.build": "project('test')",
			},
			expected: true,
		},
		{
			name: "cpp files in root",
			files: map[string]string{
				"main.cpp": "int main() {}",
			},
			expected: true,
		},
		{
			name: "cpp files in src",
			files: map[string]string{
				"src/main.cpp": "int main() {}",
			},
			expected: true,
		},
		{
			name: "header files only",
			files: map[string]string{
				"include/test.hpp": "#pragma once",
			},
			expected: false,
		},
		{
			name:     "no C++ indicators",
			files:    map[string]string{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			for path, content := range tt.files {
				fullPath := filepath.Join(tmpDir, path)
				err := os.MkdirAll(filepath.Dir(fullPath), 0755)
				require.NoError(t, err)
				err = os.WriteFile(fullPath, []byte(content), 0644)
				require.NoError(t, err)
			}

			e := NewExtractor()
			result := e.Detect(tmpDir)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractFromCMake(t *testing.T) {
	tests := []struct {
		name              string
		cmakeContent      string
		expectedName      string
		expectedVersion   string
		expectedDesc      string
		checkLangSpecific func(t *testing.T, ls map[string]interface{})
	}{
		{
			name: "basic project",
			cmakeContent: `cmake_minimum_required(VERSION 3.10)
project(MyProject VERSION 1.2.3 DESCRIPTION "A test project")
`,
			expectedName:    "MyProject",
			expectedVersion: "1.2.3",
			expectedDesc:    "A test project",
		},
		{
			name: "with C++ standard",
			cmakeContent: `project(TestApp VERSION 2.0.0)
set(CMAKE_CXX_STANDARD 17)
set(CMAKE_C_STANDARD 11)
`,
			expectedName:    "TestApp",
			expectedVersion: "2.0.0",
			checkLangSpecific: func(t *testing.T, ls map[string]interface{}) {
				assert.Equal(t, "17", ls["cxx_standard"])
				assert.Equal(t, "11", ls["c_standard"])
			},
		},
		{
			name: "with executables and libraries",
			cmakeContent: `project(ComplexProject VERSION 3.1.4)
add_executable(app main.cpp)
add_executable(tool tool.cpp)
add_library(mylib STATIC lib.cpp)
add_library(shared SHARED shared.cpp)
`,
			expectedName:    "ComplexProject",
			expectedVersion: "3.1.4",
			checkLangSpecific: func(t *testing.T, ls map[string]interface{}) {
				execs := ls["executables"].([]string)
				assert.Len(t, execs, 2)
				assert.Contains(t, execs, "app")
				assert.Contains(t, execs, "tool")

				libs := ls["libraries"].([]string)
				assert.Len(t, libs, 2)
				assert.Contains(t, libs, "mylib")
				assert.Contains(t, libs, "shared")
			},
		},
		{
			name: "with dependencies",
			cmakeContent: `project(DependentProject)
find_package(Boost REQUIRED)
find_package(OpenCV)
find_package(Qt5 COMPONENTS Core Widgets)
`,
			expectedName: "DependentProject",
			checkLangSpecific: func(t *testing.T, ls map[string]interface{}) {
				deps := ls["dependencies"].([]string)
				assert.Len(t, deps, 3)
				assert.Contains(t, deps, "Boost")
				assert.Contains(t, deps, "OpenCV")
				assert.Contains(t, deps, "Qt5")
				assert.Equal(t, 3, ls["dependency_count"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			cmakePath := filepath.Join(tmpDir, "CMakeLists.txt")
			err := os.WriteFile(cmakePath, []byte(tt.cmakeContent), 0644)
			require.NoError(t, err)

			e := NewExtractor()
			metadata, err := e.Extract(tmpDir)
			require.NoError(t, err)
			require.NotNil(t, metadata)

			assert.Equal(t, tt.expectedName, metadata.Name)
			if tt.expectedVersion != "" {
				assert.Equal(t, tt.expectedVersion, metadata.Version)
				assert.Equal(t, "CMakeLists.txt", metadata.VersionSource)
			}
			if tt.expectedDesc != "" {
				assert.Equal(t, tt.expectedDesc, metadata.Description)
			}

			assert.Equal(t, "CMake", metadata.LanguageSpecific["build_system"])

			if tt.checkLangSpecific != nil {
				tt.checkLangSpecific(t, metadata.LanguageSpecific)
			}
		})
	}
}

func TestExtractFromMeson(t *testing.T) {
	mesonContent := `project('myapp', 'cpp',
  version: '1.5.0',
  default_options: ['warning_level=3', 'cpp_std=c++17'])

executable('myapp', 'main.cpp')
shared_library('mylib', 'lib.cpp')
dependency('gtk+-3.0')
dependency('libcurl')
`

	tmpDir := t.TempDir()
	mesonPath := filepath.Join(tmpDir, "meson.build")
	err := os.WriteFile(mesonPath, []byte(mesonContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, metadata)

	assert.Equal(t, "myapp", metadata.Name)
	assert.Equal(t, "1.5.0", metadata.Version)
	assert.Equal(t, "meson.build", metadata.VersionSource)
	assert.Equal(t, "Meson", metadata.LanguageSpecific["build_system"])

	execs := metadata.LanguageSpecific["executables"].([]string)
	assert.Contains(t, execs, "myapp")

	libs := metadata.LanguageSpecific["libraries"].([]string)
	assert.Contains(t, libs, "mylib")

	deps := metadata.LanguageSpecific["dependencies"].([]string)
	assert.Len(t, deps, 2)
	assert.Contains(t, deps, "gtk+-3.0")
	assert.Contains(t, deps, "libcurl")
}

func TestExtractFromAutotools(t *testing.T) {
	autotoolsContent := `AC_INIT([mytool], [2.3.1])
AC_CONFIG_SRCDIR([src/main.c])
AM_INIT_AUTOMAKE

PKG_CHECK_MODULES([GLIB], [glib-2.0 >= 2.40])
PKG_CHECK_MODULES([XML], [libxml-2.0])

AC_OUTPUT
`

	tmpDir := t.TempDir()
	configurePath := filepath.Join(tmpDir, "configure.ac")
	err := os.WriteFile(configurePath, []byte(autotoolsContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, metadata)

	assert.Equal(t, "mytool", metadata.Name)
	assert.Equal(t, "2.3.1", metadata.Version)
	assert.Equal(t, "configure.ac", metadata.VersionSource)
	assert.Equal(t, "Autotools", metadata.LanguageSpecific["build_system"])

	deps := metadata.LanguageSpecific["dependencies"].([]string)
	assert.Len(t, deps, 2)
	assert.Contains(t, deps, "glib-2.0")
	assert.Contains(t, deps, "libxml-2.0")
}

func TestExtractFromQmake(t *testing.T) {
	tests := []struct {
		name            string
		qmakeContent    string
		expectedVersion string
	}{
		{
			name: "MODULE_VERSION",
			qmakeContent: `load(qt_build_config)

CONFIG += warning_clean exceptions

MODULE_VERSION = 0.5.0
`,
			expectedVersion: "0.5.0",
		},
		{
			name: "VERSION",
			qmakeContent: `CONFIG += qt
VERSION = 1.2.3
`,
			expectedVersion: "1.2.3",
		},
		{
			name: "both MODULE_VERSION and VERSION",
			qmakeContent: `load(qt_build_config)
MODULE_VERSION = 2.1.0
VERSION = 1.0.0
`,
			expectedVersion: "2.1.0", // MODULE_VERSION takes precedence
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			qmakePath := filepath.Join(tmpDir, ".qmake.conf")
			err := os.WriteFile(qmakePath, []byte(tt.qmakeContent), 0644)
			require.NoError(t, err)

			e := NewExtractor()
			metadata, err := e.Extract(tmpDir)
			require.NoError(t, err)
			require.NotNil(t, metadata)

			assert.Equal(t, tt.expectedVersion, metadata.Version)
			assert.Equal(t, ".qmake.conf", metadata.VersionSource)
			assert.Equal(t, "qmake", metadata.LanguageSpecific["build_system"])
		})
	}
}

func TestExtractNoBuildSystem(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a simple Makefile
	makefilePath := filepath.Join(tmpDir, "Makefile")
	err := os.WriteFile(makefilePath, []byte("all:\n\tgcc main.c -o app\n"), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, metadata)

	// Should fall back to Makefile
	assert.Equal(t, "Makefile", metadata.LanguageSpecific["build_system"])
}
