// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package scala

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
	assert.Equal(t, "scala", e.Name())
	assert.Equal(t, 1, e.Priority())
}

func TestDetect(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		expected bool
	}{
		{
			name: "build.sbt",
			files: map[string]string{
				"build.sbt": `name := "test"`,
			},
			expected: true,
		},
		{
			name: "build.sc (Mill)",
			files: map[string]string{
				"build.sc": `object test extends ScalaModule`,
			},
			expected: true,
		},
		{
			name: "project/build.properties",
			files: map[string]string{
				"project/build.properties": `sbt.version=1.9.0`,
			},
			expected: true,
		},
		{
			name: "src/main/scala",
			files: map[string]string{
				"src/main/scala/Main.scala": `object Main extends App`,
			},
			expected: true,
		},
		{
			name:     "no Scala indicators",
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

func TestExtractFromBuildSbt(t *testing.T) {
	buildSbtContent := `name := "my-scala-project"

version := "1.0.0"

scalaVersion := "2.13.12"

organization := "com.example"

description := "A sample Scala project"

homepage := Some(url("https://example.com"))

licenses := Seq("Apache-2.0" -> url("http://www.apache.org/licenses/LICENSE-2.0"))

libraryDependencies ++= Seq(
  "org.typelevel" %% "cats-core" % "2.10.0",
  "org.scalatest" %% "scalatest" % "3.2.17" % Test
)
`

	tmpDir := t.TempDir()
	buildSbtPath := filepath.Join(tmpDir, "build.sbt")
	err := os.WriteFile(buildSbtPath, []byte(buildSbtContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, metadata)

	assert.Equal(t, "my-scala-project", metadata.Name)
	assert.Equal(t, "1.0.0", metadata.Version)
	assert.Equal(t, "build.sbt", metadata.VersionSource)
	assert.Equal(t, "A sample Scala project", metadata.Description)
	assert.Equal(t, "https://example.com", metadata.Homepage)
	assert.Equal(t, "Apache-2.0", metadata.License)

	assert.Equal(t, "SBT", metadata.LanguageSpecific["build_tool"])
	assert.Equal(t, "2.13.12", metadata.LanguageSpecific["scala_version"])
	assert.Equal(t, "com.example", metadata.LanguageSpecific["organization"])

	deps := metadata.LanguageSpecific["dependencies"].([]string)
	assert.Len(t, deps, 2)
	assert.Contains(t, deps, "org.typelevel:cats-core:2.10.0")
	assert.Contains(t, deps, "org.scalatest:scalatest:3.2.17")
	assert.Equal(t, 2, metadata.LanguageSpecific["dependency_count"])
}

func TestExtractFromBuildSbtMinimal(t *testing.T) {
	buildSbtContent := `name := "minimal-project"
scalaVersion := "3.3.1"
`

	tmpDir := t.TempDir()
	buildSbtPath := filepath.Join(tmpDir, "build.sbt")
	err := os.WriteFile(buildSbtPath, []byte(buildSbtContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, metadata)

	assert.Equal(t, "minimal-project", metadata.Name)
	assert.Equal(t, "3.3.1", metadata.LanguageSpecific["scala_version"])
	assert.Equal(t, "SBT", metadata.LanguageSpecific["build_tool"])
}

func TestExtractSbtVersion(t *testing.T) {
	buildSbtContent := `name := "test"`
	buildPropsContent := `sbt.version=1.9.7`

	tmpDir := t.TempDir()

	buildSbtPath := filepath.Join(tmpDir, "build.sbt")
	err := os.WriteFile(buildSbtPath, []byte(buildSbtContent), 0644)
	require.NoError(t, err)

	projectDir := filepath.Join(tmpDir, "project")
	err = os.MkdirAll(projectDir, 0755)
	require.NoError(t, err)

	buildPropsPath := filepath.Join(projectDir, "build.properties")
	err = os.WriteFile(buildPropsPath, []byte(buildPropsContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, metadata)

	assert.Equal(t, "1.9.7", metadata.LanguageSpecific["sbt_version"])
}

func TestExtractFromMill(t *testing.T) {
	buildScContent := `import mill._, scalalib._

object myproject extends ScalaModule {
  def scalaVersion = "2.13.12"

  def ivyDeps = Agg(
    ivy"com.lihaoyi::upickle:3.1.3",
    ivy"com.lihaoyi::os-lib:0.9.1"
  )
}
`

	tmpDir := t.TempDir()
	buildScPath := filepath.Join(tmpDir, "build.sc")
	err := os.WriteFile(buildScPath, []byte(buildScContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, metadata)

	assert.Equal(t, "myproject", metadata.Name)
	assert.Equal(t, "Mill", metadata.LanguageSpecific["build_tool"])
	assert.Equal(t, "2.13.12", metadata.LanguageSpecific["scala_version"])

	deps := metadata.LanguageSpecific["dependencies"].([]string)
	assert.Len(t, deps, 2)
	assert.Contains(t, deps, "com.lihaoyi:upickle:3.1.3")
	assert.Contains(t, deps, "com.lihaoyi:os-lib:0.9.1")
}

func TestGenerateScalaVersionMatrix(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		expected []string
	}{
		{
			name:     "Scala 3.3",
			version:  "3.3.1",
			expected: []string{"3.3", "3.4"},
		},
		{
			name:     "Scala 2.13",
			version:  "2.13.12",
			expected: []string{"2.13"},
		},
		{
			name:     "Scala 2.12",
			version:  "2.12.18",
			expected: []string{"2.12", "2.13"},
		},
		{
			name:     "Scala 2.11 (legacy)",
			version:  "2.11.12",
			expected: []string{"2.11", "2.12"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateScalaVersionMatrix(tt.version)
			assert.Equal(t, tt.expected, result)
		})
	}
}
