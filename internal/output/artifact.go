// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package output

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lfreleng-actions/build-metadata-action/internal/validator"
	"gopkg.in/yaml.v3"
)

// ArtifactUploader handles uploading metadata as workflow artifacts
type ArtifactUploader struct {
	Enabled        bool
	NamePrefix     string
	Formats        []string
	OutputDir      string
	ValidateOutput bool
	StrictMode     bool
}

// ArtifactResult contains information about the uploaded artifact
type ArtifactResult struct {
	Path   string
	Suffix string
	Name   string
	Files  []string
}

// NewArtifactUploader creates a new artifact uploader
func NewArtifactUploader(enabled bool, namePrefix string, formats []string, outputDir string, validateOutput bool, strictMode bool) *ArtifactUploader {
	if namePrefix == "" {
		namePrefix = "build-metadata"
	}

	if outputDir == "" {
		outputDir = os.TempDir()
	}

	if len(formats) == 0 {
		formats = []string{"json", "yaml"}
	}

	return &ArtifactUploader{
		Enabled:        enabled,
		NamePrefix:     namePrefix,
		Formats:        formats,
		OutputDir:      outputDir,
		ValidateOutput: validateOutput,
		StrictMode:     strictMode,
	}
}

// Upload uploads metadata as artifact files
func (a *ArtifactUploader) Upload(metadata interface{}, jobName string) (*ArtifactResult, error) {
	if !a.Enabled {
		return nil, nil
	}

	// Generate unique suffix
	suffix, err := generateSuffix()
	if err != nil {
		return nil, fmt.Errorf("failed to generate artifact suffix: %w", err)
	}

	// Create artifact name
	artifactName := fmt.Sprintf("%s-%s-%s", a.NamePrefix, jobName, suffix)

	// Create artifact directory
	artifactPath := filepath.Join(a.OutputDir, artifactName)
	if err := os.MkdirAll(artifactPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create artifact directory: %w", err)
	}

	result := &ArtifactResult{
		Path:   artifactPath,
		Suffix: suffix,
		Name:   artifactName,
		Files:  make([]string, 0),
	}

	// Generate and write outputs in requested formats
	for _, format := range a.Formats {
		switch format {
		case "json":
			files, err := a.writeJSON(artifactPath, metadata)
			if err != nil {
				return nil, fmt.Errorf("failed to write JSON artifacts: %w", err)
			}
			result.Files = append(result.Files, files...)

		case "yaml":
			files, err := a.writeYAML(artifactPath, metadata)
			if err != nil {
				return nil, fmt.Errorf("failed to write YAML artifacts: %w", err)
			}
			result.Files = append(result.Files, files...)

		default:
			return nil, fmt.Errorf("unsupported artifact format: %s", format)
		}
	}

	return result, nil
}

// writeJSON writes JSON artifacts (compact and pretty)
func (a *ArtifactUploader) writeJSON(artifactPath string, metadata interface{}) ([]string, error) {
	files := make([]string, 0, 2)

	// Create validator
	jsonValidator := validator.NewJSONValidator(a.StrictMode)

	// Generate compact and pretty JSON
	compact, pretty, err := jsonValidator.ValidateAndPrettify(metadata)
	if err != nil {
		if a.ValidateOutput {
			return nil, fmt.Errorf("JSON validation failed: %w", err)
		}
		// In non-validating mode, try to generate anyway
		compact, _ = json.Marshal(metadata)
		pretty, _ = json.MarshalIndent(metadata, "", "  ")
	}

	// Write compact JSON
	compactPath := filepath.Join(artifactPath, "metadata.json")
	if err := os.WriteFile(compactPath, compact, 0644); err != nil {
		return nil, fmt.Errorf("failed to write compact JSON: %w", err)
	}
	files = append(files, "metadata.json")

	// Write pretty JSON
	prettyPath := filepath.Join(artifactPath, "metadata-pretty.json")
	if err := os.WriteFile(prettyPath, pretty, 0644); err != nil {
		return nil, fmt.Errorf("failed to write pretty JSON: %w", err)
	}
	files = append(files, "metadata-pretty.json")

	return files, nil
}

// writeYAML writes YAML artifact
func (a *ArtifactUploader) writeYAML(artifactPath string, metadata interface{}) ([]string, error) {
	files := make([]string, 0, 1)

	// Create validator
	yamlValidator := validator.NewYAMLValidator(a.StrictMode)

	// Generate YAML
	yamlBytes, err := yamlValidator.MarshalAndValidate(metadata)
	if err != nil {
		if a.ValidateOutput {
			return nil, fmt.Errorf("YAML validation failed: %w", err)
		}
		// In non-validating mode, try to generate anyway
		yamlBytes, _ = yaml.Marshal(metadata)
	}

	// Write YAML
	yamlPath := filepath.Join(artifactPath, "metadata.yaml")
	if err := os.WriteFile(yamlPath, yamlBytes, 0644); err != nil {
		return nil, fmt.Errorf("failed to write YAML: %w", err)
	}
	files = append(files, "metadata.yaml")

	return files, nil
}

// generateSuffix generates a random 4-character alphanumeric suffix
func generateSuffix() (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	const length = 4

	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}

	suffix := make([]byte, length)
	for i := range bytes {
		suffix[i] = charset[int(bytes[i])%len(charset)]
	}

	return string(suffix), nil
}

// contains checks if a string slice contains a specific string
func contains(slice []string, str string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}

// GetMetadataJSON returns metadata as a JSON string
func GetMetadataJSON(metadata interface{}, validateOutput bool) (string, error) {
	jsonValidator := validator.NewJSONValidator(true)

	jsonBytes, err := jsonValidator.MarshalAndValidate(metadata)
	if err != nil {
		if validateOutput {
			return "", fmt.Errorf("failed to generate JSON: %w", err)
		}
		// Fallback to basic marshaling
		jsonBytes, _ = json.Marshal(metadata)
	}

	return string(jsonBytes), nil
}

// GetMetadataYAML returns metadata as a YAML string
func GetMetadataYAML(metadata interface{}, validateOutput bool) (string, error) {
	yamlValidator := validator.NewYAMLValidator(true)

	yamlBytes, err := yamlValidator.MarshalAndValidate(metadata)
	if err != nil {
		if validateOutput {
			return "", fmt.Errorf("failed to generate YAML: %w", err)
		}
		// Fallback to basic marshaling
		yamlBytes, _ = yaml.Marshal(metadata)
	}

	return string(yamlBytes), nil
}
