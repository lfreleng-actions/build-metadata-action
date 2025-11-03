// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package docker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractor_Name(t *testing.T) {
	e := NewExtractor()
	assert.Equal(t, "docker", e.Name())
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
			name: "valid dockerfile",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				dockerfilePath := filepath.Join(dir, "Dockerfile")
				err := os.WriteFile(dockerfilePath, []byte(`FROM node:18-alpine
RUN npm install
`), 0644)
				require.NoError(t, err)
				return dir
			},
			cleanup:  func(s string) {},
			expected: true,
		},
		{
			name: "missing dockerfile",
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

func TestExtractor_Extract_Basic(t *testing.T) {
	dir := t.TempDir()
	dockerfilePath := filepath.Join(dir, "Dockerfile")

	dockerfileContent := `FROM node:18-alpine

LABEL org.opencontainers.image.version="1.0.0"
LABEL org.opencontainers.image.description="Test application"
LABEL org.opencontainers.image.authors="test@example.com"
LABEL org.opencontainers.image.source="https://github.com/example/app"
LABEL org.opencontainers.image.url="https://example.com"

WORKDIR /app

COPY package*.json ./
RUN npm ci

EXPOSE 3000

USER node

CMD ["node", "index.js"]`

	err := os.WriteFile(dockerfilePath, []byte(dockerfileContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)
	require.NotNil(t, metadata)

	// Common metadata extracted from labels
	assert.Equal(t, "1.0.0", metadata.Version)
	assert.Equal(t, "Test application", metadata.Description)
	assert.Equal(t, "https://github.com/example/app", metadata.Repository)
	assert.Equal(t, "https://example.com", metadata.Homepage)
	assert.Contains(t, metadata.Authors, "test@example.com")
	assert.Equal(t, "Dockerfile LABEL org.opencontainers.image.version", metadata.VersionSource)

	// Docker-specific metadata
	assert.Equal(t, "Dockerfile", metadata.LanguageSpecific["metadata_source"])
	assert.Equal(t, "node:18-alpine", metadata.LanguageSpecific["primary_base_image"])
	assert.Equal(t, "/app", metadata.LanguageSpecific["workdir"])
	assert.Equal(t, "node", metadata.LanguageSpecific["user"])
	assert.Equal(t, false, metadata.LanguageSpecific["is_multistage"])
}

func TestExtractor_Extract_MultiStage(t *testing.T) {
	dir := t.TempDir()
	dockerfilePath := filepath.Join(dir, "Dockerfile")

	dockerfileContent := `FROM node:18-alpine AS builder

WORKDIR /build
COPY package*.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM node:18-alpine AS runtime

WORKDIR /app
COPY --from=builder /build/dist ./dist

EXPOSE 8080
USER node

CMD ["node", "dist/index.js"]`

	err := os.WriteFile(dockerfilePath, []byte(dockerfileContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)

	// Check multi-stage build detection
	assert.Equal(t, true, metadata.LanguageSpecific["is_multistage"])
	assert.Equal(t, 2, metadata.LanguageSpecific["stage_count"])

	stages := metadata.LanguageSpecific["build_stages"]
	require.NotNil(t, stages)
	stageList, ok := stages.([]string)
	require.True(t, ok)
	assert.Contains(t, stageList, "builder")
	assert.Contains(t, stageList, "runtime")

	// Check COPY --from references
	copyFrom := metadata.LanguageSpecific["copy_from_stages"]
	require.NotNil(t, copyFrom)
	copyFromList, ok := copyFrom.([]string)
	require.True(t, ok)
	assert.Contains(t, copyFromList, "builder")

	// Check base images
	baseImages := metadata.LanguageSpecific["base_images"]
	require.NotNil(t, baseImages)
	baseImageList, ok := baseImages.([]string)
	require.True(t, ok)
	assert.Len(t, baseImageList, 2)
}

func TestExtractor_Extract_Labels(t *testing.T) {
	dir := t.TempDir()
	dockerfilePath := filepath.Join(dir, "Dockerfile")

	dockerfileContent := `FROM alpine:3.18

LABEL version="2.0.0"
LABEL description="Test container"
LABEL maintainer="admin@example.com"
LABEL custom.label="custom-value"
LABEL "key.with.dots"="value with spaces"`

	err := os.WriteFile(dockerfilePath, []byte(dockerfileContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)

	labels := metadata.LanguageSpecific["labels"]
	require.NotNil(t, labels)

	labelsMap, ok := labels.(map[string]string)
	require.True(t, ok)
	assert.Equal(t, "2.0.0", labelsMap["version"])
	assert.Equal(t, "Test container", labelsMap["description"])
	assert.Equal(t, "admin@example.com", labelsMap["maintainer"])
	assert.Equal(t, "custom-value", labelsMap["custom.label"])
}

func TestExtractor_Extract_ExposedPorts(t *testing.T) {
	dir := t.TempDir()
	dockerfilePath := filepath.Join(dir, "Dockerfile")

	dockerfileContent := `FROM nginx:alpine

EXPOSE 80
EXPOSE 443
EXPOSE 8080/tcp
EXPOSE 9090/udp`

	err := os.WriteFile(dockerfilePath, []byte(dockerfileContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)

	ports := metadata.LanguageSpecific["exposed_ports"]
	require.NotNil(t, ports)

	portsList, ok := ports.([]string)
	require.True(t, ok)
	assert.Contains(t, portsList, "80")
	assert.Contains(t, portsList, "443")
	assert.Contains(t, portsList, "8080/tcp")
	assert.Contains(t, portsList, "9090/udp")
}

func TestExtractor_Extract_Volumes(t *testing.T) {
	dir := t.TempDir()
	dockerfilePath := filepath.Join(dir, "Dockerfile")

	dockerfileContent := `FROM postgres:15

VOLUME /var/lib/postgresql/data
VOLUME ["/var/log", "/var/cache"]`

	err := os.WriteFile(dockerfilePath, []byte(dockerfileContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)

	volumes := metadata.LanguageSpecific["volumes"]
	require.NotNil(t, volumes)

	volumesList, ok := volumes.([]string)
	require.True(t, ok)
	assert.Contains(t, volumesList, "/var/lib/postgresql/data")
	assert.Contains(t, volumesList, "/var/log")
	assert.Contains(t, volumesList, "/var/cache")
}

func TestExtractor_Extract_EnvVars(t *testing.T) {
	dir := t.TempDir()
	dockerfilePath := filepath.Join(dir, "Dockerfile")

	dockerfileContent := `FROM node:18

ENV NODE_ENV=production
ENV PORT=3000
ENV API_KEY "secret-key"
ENV LOG_LEVEL=info DEBUG=false`

	err := os.WriteFile(dockerfilePath, []byte(dockerfileContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)

	env := metadata.LanguageSpecific["env"]
	require.NotNil(t, env)

	envMap, ok := env.(map[string]string)
	require.True(t, ok)
	assert.Equal(t, "production", envMap["NODE_ENV"])
	assert.Equal(t, "3000", envMap["PORT"])
}

func TestExtractor_Extract_BuildArgs(t *testing.T) {
	dir := t.TempDir()
	dockerfilePath := filepath.Join(dir, "Dockerfile")

	dockerfileContent := `FROM ubuntu:22.04

ARG VERSION=1.0.0
ARG BUILD_DATE
ARG USER=appuser`

	err := os.WriteFile(dockerfilePath, []byte(dockerfileContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)

	buildArgs := metadata.LanguageSpecific["build_args"]
	require.NotNil(t, buildArgs)

	argsMap, ok := buildArgs.(map[string]string)
	require.True(t, ok)
	assert.Equal(t, "1.0.0", argsMap["VERSION"])
	assert.Equal(t, "", argsMap["BUILD_DATE"])
	assert.Equal(t, "appuser", argsMap["USER"])
}

func TestExtractor_Extract_Healthcheck(t *testing.T) {
	dir := t.TempDir()
	dockerfilePath := filepath.Join(dir, "Dockerfile")

	dockerfileContent := `FROM nginx:alpine

HEALTHCHECK --interval=30s --timeout=3s CMD curl -f http://localhost/ || exit 1`

	err := os.WriteFile(dockerfilePath, []byte(dockerfileContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)

	healthcheck := metadata.LanguageSpecific["healthcheck"]
	require.NotNil(t, healthcheck)
	assert.Contains(t, healthcheck, "curl")
}

func TestExtractor_Extract_EntrypointAndCmd(t *testing.T) {
	dir := t.TempDir()
	dockerfilePath := filepath.Join(dir, "Dockerfile")

	dockerfileContent := `FROM python:3.11-slim

ENTRYPOINT ["python", "app.py"]
CMD ["--port", "8000"]`

	err := os.WriteFile(dockerfilePath, []byte(dockerfileContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)

	entrypoint := metadata.LanguageSpecific["entrypoint"]
	require.NotNil(t, entrypoint)
	entrypointList, ok := entrypoint.([]string)
	require.True(t, ok)
	assert.Equal(t, []string{"python", "app.py"}, entrypointList)

	cmd := metadata.LanguageSpecific["cmd"]
	require.NotNil(t, cmd)
	cmdList, ok := cmd.([]string)
	require.True(t, ok)
	assert.Equal(t, []string{"--port", "8000"}, cmdList)
}

func TestExtractor_Extract_OCICompliance(t *testing.T) {
	dir := t.TempDir()
	dockerfilePath := filepath.Join(dir, "Dockerfile")

	dockerfileContent := `FROM alpine:3.18

LABEL org.opencontainers.image.created="2025-01-15T12:00:00Z"
LABEL org.opencontainers.image.version="1.0.0"
LABEL org.opencontainers.image.title="My App"
LABEL org.opencontainers.image.description="A test application"
LABEL org.opencontainers.image.source="https://github.com/example/app"`

	err := os.WriteFile(dockerfilePath, []byte(dockerfileContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)

	ociCompliant := metadata.LanguageSpecific["oci_compliant"]
	assert.Equal(t, true, ociCompliant)
}

func TestExtractor_Extract_NonOCICompliance(t *testing.T) {
	dir := t.TempDir()
	dockerfilePath := filepath.Join(dir, "Dockerfile")

	dockerfileContent := `FROM alpine:3.18

LABEL version="1.0.0"
LABEL description="Missing OCI labels"`

	err := os.WriteFile(dockerfilePath, []byte(dockerfileContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)

	ociCompliant := metadata.LanguageSpecific["oci_compliant"]
	assert.Equal(t, false, ociCompliant)
}

func TestExtractor_Extract_MissingFile(t *testing.T) {
	dir := t.TempDir()

	e := NewExtractor()
	_, err := e.Extract(dir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Dockerfile not found")
}

func TestExtractor_Extract_MinimalDockerfile(t *testing.T) {
	dir := t.TempDir()
	dockerfilePath := filepath.Join(dir, "Dockerfile")

	dockerfileContent := `FROM scratch
COPY app /app
CMD ["/app"]`

	err := os.WriteFile(dockerfilePath, []byte(dockerfileContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)

	baseImages := metadata.LanguageSpecific["base_images"]
	require.NotNil(t, baseImages)
	baseImageList, ok := baseImages.([]string)
	require.True(t, ok)
	assert.Contains(t, baseImageList, "scratch")
}

func TestExtractor_Extract_LineContinuation(t *testing.T) {
	dir := t.TempDir()
	dockerfilePath := filepath.Join(dir, "Dockerfile")

	dockerfileContent := `FROM ubuntu:22.04

RUN apt-get update && \
    apt-get install -y \
        curl \
        wget \
        git && \
    apt-get clean

LABEL version="1.0.0" \
      description="Multi-line label"`

	err := os.WriteFile(dockerfilePath, []byte(dockerfileContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)

	// Should successfully parse despite line continuations
	assert.NotNil(t, metadata)
}

func TestExtractor_Extract_Comments(t *testing.T) {
	dir := t.TempDir()
	dockerfilePath := filepath.Join(dir, "Dockerfile")

	dockerfileContent := `# This is a comment
FROM node:18-alpine

# Another comment
# Multiple lines
WORKDIR /app

# Comment before command
RUN npm install`

	err := os.WriteFile(dockerfilePath, []byte(dockerfileContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)

	// Comments should be ignored
	assert.NotNil(t, metadata)
	assert.Equal(t, "/app", metadata.LanguageSpecific["workdir"])
}

func TestExtractor_Extract_MultipleBaseImages(t *testing.T) {
	dir := t.TempDir()
	dockerfilePath := filepath.Join(dir, "Dockerfile")

	dockerfileContent := `FROM node:18 AS node-builder
FROM python:3.11 AS python-builder
FROM golang:1.21 AS go-builder
FROM alpine:3.18

COPY --from=node-builder /app /node-app
COPY --from=python-builder /app /python-app
COPY --from=go-builder /app /go-app`

	err := os.WriteFile(dockerfilePath, []byte(dockerfileContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)

	baseImages := metadata.LanguageSpecific["base_images"]
	require.NotNil(t, baseImages)
	baseImageList, ok := baseImages.([]string)
	require.True(t, ok)
	assert.Equal(t, 4, len(baseImageList))
	assert.Equal(t, 4, metadata.LanguageSpecific["base_image_count"])
}

func TestParseCommand(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "json array format",
			input:    `["node", "index.js"]`,
			expected: []string{"node", "index.js"},
		},
		{
			name:     "json array with spaces",
			input:    `["python", "-m", "app"]`,
			expected: []string{"python", "-m", "app"},
		},
		{
			name:     "shell form",
			input:    `node index.js`,
			expected: []string{"node index.js"},
		},
		{
			name:     "single command",
			input:    `nginx`,
			expected: []string{"nginx"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseCommand(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
