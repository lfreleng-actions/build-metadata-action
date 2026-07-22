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

		if strings.HasSuffix(line, "\\") {
			currentLine += strings.TrimSuffix(line, "\\") + " "
			continue
		}

		// Complete the line
		if currentLine != "" {
			line = currentLine + line
			currentLine = ""
		}

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

// firstLabel returns the value of the first label key that is present.
func firstLabel(labels map[string]string, keys ...string) (string, bool) {
	for _, k := range keys {
		if v, ok := labels[k]; ok {
			return v, true
		}
	}
	return "", false
}

// applyDockerLabelMetadata maps Dockerfile labels onto core project fields.
// Precedence differs per field to preserve historical behaviour: version,
// description, license and maintainer prefer the plain label key over its
// OCI equivalent, whereas repository (source) and homepage (url) prefer the
// org.opencontainers.image.* key first.
func applyDockerLabelMetadata(dockerMeta *DockerfileMetadata, metadata *extractor.ProjectMetadata) {
	labels := dockerMeta.Labels

	if version, ok := labels["version"]; ok {
		metadata.Version = version
		metadata.VersionSource = "Dockerfile LABEL version"
	} else if version, ok := labels["org.opencontainers.image.version"]; ok {
		metadata.Version = version
		metadata.VersionSource = "Dockerfile LABEL org.opencontainers.image.version"
	}

	if v, ok := firstLabel(labels, "description", "org.opencontainers.image.description"); ok {
		metadata.Description = v
	}
	if v, ok := firstLabel(labels, "license", "org.opencontainers.image.licenses"); ok {
		metadata.License = v
	}
	if v, ok := firstLabel(labels, "maintainer", "org.opencontainers.image.authors"); ok {
		metadata.Authors = []string{v}
	}
	if v, ok := firstLabel(labels, "org.opencontainers.image.source", "source"); ok {
		metadata.Repository = v
	}
	if v, ok := firstLabel(labels, "org.opencontainers.image.url", "url"); ok {
		metadata.Homepage = v
	}
}

// applyDockerRuntimeMetadata records base images, labels, and runtime
// instruction data under LanguageSpecific.
func applyDockerRuntimeMetadata(dockerMeta *DockerfileMetadata, metadata *extractor.ProjectMetadata) {
	ls := metadata.LanguageSpecific
	ls["metadata_source"] = "Dockerfile"
	ls["base_images"] = dockerMeta.BaseImages

	if len(dockerMeta.BaseImages) > 0 {
		ls["primary_base_image"] = dockerMeta.BaseImages[0]
		ls["base_image_count"] = len(dockerMeta.BaseImages)
	}
	if len(dockerMeta.Labels) > 0 {
		ls["labels"] = dockerMeta.Labels
		ls["label_count"] = len(dockerMeta.Labels)
	}
	if len(dockerMeta.ExposedPorts) > 0 {
		ls["exposed_ports"] = dockerMeta.ExposedPorts
	}
	if len(dockerMeta.Volumes) > 0 {
		ls["volumes"] = dockerMeta.Volumes
	}
	if len(dockerMeta.Entrypoint) > 0 {
		ls["entrypoint"] = dockerMeta.Entrypoint
	}
	if len(dockerMeta.Cmd) > 0 {
		ls["cmd"] = dockerMeta.Cmd
	}
	if dockerMeta.WorkDir != "" {
		ls["workdir"] = dockerMeta.WorkDir
	}
	if dockerMeta.User != "" {
		ls["user"] = dockerMeta.User
	}
	if len(dockerMeta.Env) > 0 {
		ls["env"] = dockerMeta.Env
	}
	if len(dockerMeta.Args) > 0 {
		ls["build_args"] = dockerMeta.Args
	}
	if dockerMeta.HealthCheck != "" {
		ls["healthcheck"] = dockerMeta.HealthCheck
	}

	if len(dockerMeta.Stages) > 0 {
		ls["build_stages"] = dockerMeta.Stages
		ls["is_multistage"] = true
		ls["stage_count"] = len(dockerMeta.Stages)
	} else {
		ls["is_multistage"] = false
	}

	if len(dockerMeta.CopyFrom) > 0 {
		ls["copy_from_stages"] = dockerMeta.CopyFrom
	}
}

// applyDockerOCICompliance flags whether the required OCI image labels are all present.
func applyDockerOCICompliance(dockerMeta *DockerfileMetadata, metadata *extractor.ProjectMetadata) {
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

// populateMetadata converts DockerfileMetadata to ProjectMetadata
func (e *Extractor) populateMetadata(dockerMeta *DockerfileMetadata, metadata *extractor.ProjectMetadata, projectPath string) {
	metadata.Name = filepath.Base(projectPath)
	applyDockerLabelMetadata(dockerMeta, metadata)
	applyDockerRuntimeMetadata(dockerMeta, metadata)
	applyDockerOCICompliance(dockerMeta, metadata)
}

// Detect checks if this extractor can handle the project
func (e *Extractor) Detect(projectPath string) bool {
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
