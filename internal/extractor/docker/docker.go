// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package docker

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/lfreleng-actions/build-metadata-action/internal/extractor"
)

// Extractor extracts metadata from Docker projects
type Extractor struct {
	extractor.BaseExtractor
}

// NewExtractor creates a new Docker extractor
func NewExtractor() *Extractor {
	return &Extractor{
		BaseExtractor: extractor.NewBaseExtractor("docker", 1),
	}
}

// DockerfileMetadata represents parsed Dockerfile metadata
type DockerfileMetadata struct {
	BaseImages   []string
	Labels       map[string]string
	ExposedPorts []string
	Volumes      []string
	Entrypoint   []string
	Cmd          []string
	WorkDir      string
	User         string
	Env          map[string]string
	Args         map[string]string
	HealthCheck  string
	Stages       []string
	CopyFrom     []string
}

// Extract retrieves metadata from a Docker project
func (e *Extractor) Extract(projectPath string) (*extractor.ProjectMetadata, error) {
	metadata := &extractor.ProjectMetadata{
		LanguageSpecific: make(map[string]interface{}),
	}

	// Look for Dockerfile
	dockerfilePath := filepath.Join(projectPath, "Dockerfile")
	if _, err := os.Stat(dockerfilePath); err != nil {
		return nil, fmt.Errorf("Dockerfile not found in %s", projectPath)
	}

	dockerMeta, err := e.parseDockerfile(dockerfilePath)
	if err != nil {
		return nil, err
	}

	e.populateMetadata(dockerMeta, metadata, projectPath)

	return metadata, nil
}

// parseDockerfile parses a Dockerfile and extracts metadata
func (e *Extractor) parseDockerfile(path string) (*DockerfileMetadata, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open Dockerfile: %w", err)
	}
	defer file.Close()

	dockerMeta := &DockerfileMetadata{
		BaseImages:   make([]string, 0),
		Labels:       make(map[string]string),
		ExposedPorts: make([]string, 0),
		Volumes:      make([]string, 0),
		Entrypoint:   make([]string, 0),
		Cmd:          make([]string, 0),
		Env:          make(map[string]string),
		Args:         make(map[string]string),
		Stages:       make([]string, 0),
		CopyFrom:     make([]string, 0),
	}

	scanner := bufio.NewScanner(file)
	var currentLine string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Handle line continuation
		if strings.HasSuffix(line, "\\") {
			currentLine += strings.TrimSuffix(line, "\\") + " "
			continue
		}

		// Complete the line
		if currentLine != "" {
			line = currentLine + line
			currentLine = ""
		}

		// Parse the instruction
		e.parseInstruction(line, dockerMeta)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading Dockerfile: %w", err)
	}

	return dockerMeta, nil
}

// parseInstruction parses a single Dockerfile instruction
func (e *Extractor) parseInstruction(line string, meta *DockerfileMetadata) {
	// Split into instruction and arguments
	parts := strings.SplitN(line, " ", 2)
	if len(parts) < 1 {
		return
	}

	instruction := strings.ToUpper(parts[0])
	args := ""
	if len(parts) > 1 {
		args = strings.TrimSpace(parts[1])
	}

	switch instruction {
	case "FROM":
		e.parseFrom(args, meta)

	case "LABEL":
		e.parseLabel(args, meta)

	case "EXPOSE":
		ports := strings.Fields(args)
		meta.ExposedPorts = append(meta.ExposedPorts, ports...)

	case "VOLUME":
		// Remove brackets if present
		args = strings.Trim(args, "[]")
		volumes := strings.Split(args, ",")
		for _, v := range volumes {
			v = strings.TrimSpace(v)
			v = strings.Trim(v, `"`)
			if v != "" {
				meta.Volumes = append(meta.Volumes, v)
			}
		}

	case "ENTRYPOINT":
		meta.Entrypoint = parseCommand(args)

	case "CMD":
		meta.Cmd = parseCommand(args)

	case "WORKDIR":
		meta.WorkDir = args

	case "USER":
		meta.User = args

	case "ENV":
		e.parseEnv(args, meta)

	case "ARG":
		e.parseArg(args, meta)

	case "HEALTHCHECK":
		meta.HealthCheck = args

	case "COPY":
		e.parseCopy(args, meta)
	}
}

// parseFrom extracts base image and stage information
func (e *Extractor) parseFrom(args string, meta *DockerfileMetadata) {
	// Pattern: FROM image[:tag] [AS stage]
	parts := strings.Fields(args)
	if len(parts) > 0 {
		baseImage := parts[0]
		meta.BaseImages = append(meta.BaseImages, baseImage)

		// Check for stage name
		for i, part := range parts {
			if strings.ToUpper(part) == "AS" && i+1 < len(parts) {
				meta.Stages = append(meta.Stages, parts[i+1])
				break
			}
		}
	}
}

// parseLabel extracts label key-value pairs
func (e *Extractor) parseLabel(args string, meta *DockerfileMetadata) {
	// Handle multiple formats:
	// LABEL key=value
	// LABEL key="value"
	// LABEL key1=value1 key2=value2
	// LABEL "key"="value"

	// Simple regex to match key=value pairs
	re := regexp.MustCompile(`([^\s=]+)\s*=\s*"([^"]*)"|([^\s=]+)\s*=\s*([^\s]+)`)
	matches := re.FindAllStringSubmatch(args, -1)

	for _, match := range matches {
		var key, value string
		if match[1] != "" {
			// Quoted value
			key = strings.Trim(match[1], `"`)
			value = match[2]
		} else if match[3] != "" {
			// Unquoted value
			key = strings.Trim(match[3], `"`)
			value = strings.Trim(match[4], `"`)
		}

		if key != "" {
			meta.Labels[key] = value
		}
	}
}

// parseEnv extracts environment variables
func (e *Extractor) parseEnv(args string, meta *DockerfileMetadata) {
	// Handle: ENV KEY=value or ENV KEY value
	if strings.Contains(args, "=") {
		// KEY=value format
		re := regexp.MustCompile(`([^\s=]+)\s*=\s*"([^"]*)"|([^\s=]+)\s*=\s*([^\s]+)`)
		matches := re.FindAllStringSubmatch(args, -1)

		for _, match := range matches {
			var key, value string
			if match[1] != "" {
				key = match[1]
				value = match[2]
			} else if match[3] != "" {
				key = match[3]
				value = match[4]
			}
			if key != "" {
				meta.Env[key] = value
			}
		}
	} else {
		// KEY value format (single variable)
		parts := strings.SplitN(args, " ", 2)
		if len(parts) == 2 {
			meta.Env[parts[0]] = strings.Trim(parts[1], `"`)
		}
	}
}

// parseArg extracts build arguments
func (e *Extractor) parseArg(args string, meta *DockerfileMetadata) {
	// Handle: ARG NAME[=default]
	parts := strings.SplitN(args, "=", 2)
	key := parts[0]
	value := ""
	if len(parts) > 1 {
		value = parts[1]
	}
	meta.Args[key] = value
}

// parseCopy extracts COPY --from references
func (e *Extractor) parseCopy(args string, meta *DockerfileMetadata) {
	// Check for --from flag
	re := regexp.MustCompile(`--from=(\S+)`)
	if matches := re.FindStringSubmatch(args); len(matches) > 1 {
		meta.CopyFrom = append(meta.CopyFrom, matches[1])
	}
}

// parseCommand parses CMD or ENTRYPOINT arguments
func parseCommand(args string) []string {
	// Handle JSON array format: ["executable", "param1", "param2"]
	if strings.HasPrefix(args, "[") && strings.HasSuffix(args, "]") {
		args = strings.Trim(args, "[]")
		parts := strings.Split(args, ",")
		result := make([]string, 0, len(parts))
		for _, part := range parts {
			part = strings.TrimSpace(part)
			part = strings.Trim(part, `"`)
			if part != "" {
				result = append(result, part)
			}
		}
		return result
	}

	// Shell form: just return as single element
	return []string{args}
}

// populateMetadata converts DockerfileMetadata to ProjectMetadata
func (e *Extractor) populateMetadata(dockerMeta *DockerfileMetadata, metadata *extractor.ProjectMetadata, projectPath string) {
	// Extract name from directory
	metadata.Name = filepath.Base(projectPath)

	// Extract version from labels
	if version, ok := dockerMeta.Labels["version"]; ok {
		metadata.Version = version
		metadata.VersionSource = "Dockerfile LABEL version"
	} else if version, ok := dockerMeta.Labels["org.opencontainers.image.version"]; ok {
		metadata.Version = version
		metadata.VersionSource = "Dockerfile LABEL org.opencontainers.image.version"
	}

	// Extract description
	if desc, ok := dockerMeta.Labels["description"]; ok {
		metadata.Description = desc
	} else if desc, ok := dockerMeta.Labels["org.opencontainers.image.description"]; ok {
		metadata.Description = desc
	}

	// Extract license
	if license, ok := dockerMeta.Labels["license"]; ok {
		metadata.License = license
	} else if license, ok := dockerMeta.Labels["org.opencontainers.image.licenses"]; ok {
		metadata.License = license
	}

	// Extract authors
	if authors, ok := dockerMeta.Labels["maintainer"]; ok {
		metadata.Authors = []string{authors}
	} else if authors, ok := dockerMeta.Labels["org.opencontainers.image.authors"]; ok {
		metadata.Authors = []string{authors}
	}

	// Extract repository
	if url, ok := dockerMeta.Labels["org.opencontainers.image.source"]; ok {
		metadata.Repository = url
	} else if url, ok := dockerMeta.Labels["source"]; ok {
		metadata.Repository = url
	}

	// Extract homepage
	if url, ok := dockerMeta.Labels["org.opencontainers.image.url"]; ok {
		metadata.Homepage = url
	} else if url, ok := dockerMeta.Labels["url"]; ok {
		metadata.Homepage = url
	}

	// Docker-specific metadata
	metadata.LanguageSpecific["metadata_source"] = "Dockerfile"
	metadata.LanguageSpecific["base_images"] = dockerMeta.BaseImages

	if len(dockerMeta.BaseImages) > 0 {
		metadata.LanguageSpecific["primary_base_image"] = dockerMeta.BaseImages[0]
		metadata.LanguageSpecific["base_image_count"] = len(dockerMeta.BaseImages)
	}

	if len(dockerMeta.Labels) > 0 {
		metadata.LanguageSpecific["labels"] = dockerMeta.Labels
		metadata.LanguageSpecific["label_count"] = len(dockerMeta.Labels)
	}

	if len(dockerMeta.ExposedPorts) > 0 {
		metadata.LanguageSpecific["exposed_ports"] = dockerMeta.ExposedPorts
	}

	if len(dockerMeta.Volumes) > 0 {
		metadata.LanguageSpecific["volumes"] = dockerMeta.Volumes
	}

	if len(dockerMeta.Entrypoint) > 0 {
		metadata.LanguageSpecific["entrypoint"] = dockerMeta.Entrypoint
	}

	if len(dockerMeta.Cmd) > 0 {
		metadata.LanguageSpecific["cmd"] = dockerMeta.Cmd
	}

	if dockerMeta.WorkDir != "" {
		metadata.LanguageSpecific["workdir"] = dockerMeta.WorkDir
	}

	if dockerMeta.User != "" {
		metadata.LanguageSpecific["user"] = dockerMeta.User
	}

	if len(dockerMeta.Env) > 0 {
		metadata.LanguageSpecific["env"] = dockerMeta.Env
	}

	if len(dockerMeta.Args) > 0 {
		metadata.LanguageSpecific["build_args"] = dockerMeta.Args
	}

	if dockerMeta.HealthCheck != "" {
		metadata.LanguageSpecific["healthcheck"] = dockerMeta.HealthCheck
	}

	if len(dockerMeta.Stages) > 0 {
		metadata.LanguageSpecific["build_stages"] = dockerMeta.Stages
		metadata.LanguageSpecific["is_multistage"] = true
		metadata.LanguageSpecific["stage_count"] = len(dockerMeta.Stages)
	} else {
		metadata.LanguageSpecific["is_multistage"] = false
	}

	if len(dockerMeta.CopyFrom) > 0 {
		metadata.LanguageSpecific["copy_from_stages"] = dockerMeta.CopyFrom
	}

	// Check for OCI image spec compliance
	ociLabels := []string{
		"org.opencontainers.image.created",
		"org.opencontainers.image.version",
		"org.opencontainers.image.title",
		"org.opencontainers.image.description",
		"org.opencontainers.image.source",
	}
	ociCompliant := true
	for _, label := range ociLabels {
		if _, ok := dockerMeta.Labels[label]; !ok {
			ociCompliant = false
			break
		}
	}
	metadata.LanguageSpecific["oci_compliant"] = ociCompliant
}

// Detect checks if this extractor can handle the project
func (e *Extractor) Detect(projectPath string) bool {
	// Check for Dockerfile
	dockerfilePath := filepath.Join(projectPath, "Dockerfile")
	if _, err := os.Stat(dockerfilePath); err == nil {
		return true
	}

	return false
}

// init registers the Docker extractor
func init() {
	extractor.RegisterExtractor(NewExtractor())
}
