// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package terraform

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractor_Name(t *testing.T) {
	e := NewExtractor()
	assert.Equal(t, "terraform", e.Name())
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
			name: "valid terraform project",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				tfPath := filepath.Join(dir, "main.tf")
				err := os.WriteFile(tfPath, []byte(`
terraform {
  required_version = ">=1.5.0"
}
`), 0644)
				require.NoError(t, err)
				return dir
			},
			cleanup:  func(s string) {},
			expected: true,
		},
		{
			name: "multiple tf files",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				mainPath := filepath.Join(dir, "main.tf")
				versionsPath := filepath.Join(dir, "versions.tf")
				err := os.WriteFile(mainPath, []byte(`resource "aws_instance" "example" {}`), 0644)
				require.NoError(t, err)
				err = os.WriteFile(versionsPath, []byte(`terraform { required_version = ">=1.0" }`), 0644)
				require.NoError(t, err)
				return dir
			},
			cleanup:  func(s string) {},
			expected: true,
		},
		{
			name: "no tf files",
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
	versionsPath := filepath.Join(dir, "versions.tf")

	tfContent := `terraform {
  required_version = ">= 1.5.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }

  backend "s3" {
    bucket = "my-terraform-state"
    key    = "terraform.tfstate"
    region = "us-east-1"
  }
}`

	err := os.WriteFile(versionsPath, []byte(tfContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)
	require.NotNil(t, metadata)

	// Common metadata
	assert.Equal(t, filepath.Base(dir), metadata.Name)
	assert.Equal(t, ">= 1.5.0", metadata.Version)
	assert.Equal(t, "terraform.required_version", metadata.VersionSource)

	// Terraform-specific metadata
	assert.Equal(t, ">= 1.5.0", metadata.LanguageSpecific["terraform_version"])
	assert.Equal(t, "versions.tf", metadata.LanguageSpecific["metadata_source"])
	assert.Equal(t, "s3", metadata.LanguageSpecific["backend"])
}

func TestExtractor_Extract_Providers(t *testing.T) {
	dir := t.TempDir()
	tfPath := filepath.Join(dir, "main.tf")

	tfContent := `terraform {
  required_version = ">= 1.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
    google = {
      source  = "hashicorp/google"
      version = ">= 4.0"
    }
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 3.0"
    }
  }
}`

	err := os.WriteFile(tfPath, []byte(tfContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)

	// Check providers
	providers := metadata.LanguageSpecific["providers"]
	require.NotNil(t, providers)

	providersList, ok := providers.([]map[string]string)
	require.True(t, ok)
	assert.Len(t, providersList, 3)

	// Check provider count
	assert.Equal(t, 3, metadata.LanguageSpecific["provider_count"])

	// Verify at least one provider has expected fields
	found := false
	for _, p := range providersList {
		if p["name"] == "aws" {
			found = true
			break
		}
	}
	assert.True(t, found, "aws provider should be present")
}

func TestExtractor_Extract_Modules(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.tf")

	tfContent := `module "vpc" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "5.0.0"

  name = "my-vpc"
  cidr = "10.0.0.0/16"
}

module "security_group" {
  source  = "terraform-aws-modules/security-group/aws"
  version = "~> 5.0"

  name        = "my-sg"
  description = "Security group"
  vpc_id      = module.vpc.vpc_id
}`

	err := os.WriteFile(mainPath, []byte(tfContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)

	// Check modules
	modules := metadata.LanguageSpecific["modules"]
	require.NotNil(t, modules)

	modulesList, ok := modules.([]map[string]string)
	require.True(t, ok)
	assert.Len(t, modulesList, 2)

	// Check module count
	assert.Equal(t, 2, metadata.LanguageSpecific["module_count"])

	// Verify module details
	found := false
	for _, m := range modulesList {
		if m["name"] == "vpc" {
			assert.Equal(t, "terraform-aws-modules/vpc/aws", m["source"])
			assert.Equal(t, "5.0.0", m["version"])
			found = true
		}
	}
	assert.True(t, found, "vpc module should be present")
}

func TestExtractor_Extract_Resources(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.tf")

	tfContent := `resource "aws_instance" "web" {
  ami           = "ami-0c55b159cbfafe1f0"
  instance_type = "t2.micro"
}

resource "aws_instance" "db" {
  ami           = "ami-0c55b159cbfafe1f0"
  instance_type = "t2.small"
}

resource "aws_s3_bucket" "data" {
  bucket = "my-data-bucket"
}

resource "aws_vpc" "main" {
  cidr_block = "10.0.0.0/16"
}`

	err := os.WriteFile(mainPath, []byte(tfContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)

	// Check resources
	resourceCount := metadata.LanguageSpecific["resource_count"]
	assert.Equal(t, 4, resourceCount)

	resourceTypes := metadata.LanguageSpecific["resource_types"]
	require.NotNil(t, resourceTypes)

	typesMap, ok := resourceTypes.(map[string]int)
	require.True(t, ok)
	assert.Equal(t, 2, typesMap["aws_instance"])
	assert.Equal(t, 1, typesMap["aws_s3_bucket"])
	assert.Equal(t, 1, typesMap["aws_vpc"])
}

func TestExtractor_Extract_VersionMatrix(t *testing.T) {
	dir := t.TempDir()
	tfPath := filepath.Join(dir, "versions.tf")

	tfContent := `terraform {
  required_version = ">= 1.5.0"
}`

	err := os.WriteFile(tfPath, []byte(tfContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)

	// Check version matrix
	matrix := metadata.LanguageSpecific["terraform_version_matrix"]
	require.NotNil(t, matrix)

	matrixList, ok := matrix.([]string)
	require.True(t, ok)
	assert.Contains(t, matrixList, "1.5")
	assert.Contains(t, matrixList, "1.6")

	// Check matrix JSON
	matrixJSON := metadata.LanguageSpecific["matrix_json"]
	require.NotNil(t, matrixJSON)
	assert.Contains(t, matrixJSON, "terraform-version")
	assert.Contains(t, matrixJSON, "1.5")
}

func TestExtractor_Extract_MissingFiles(t *testing.T) {
	dir := t.TempDir()

	e := NewExtractor()
	_, err := e.Extract(dir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no Terraform files found")
}

func TestExtractor_Extract_MinimalConfig(t *testing.T) {
	dir := t.TempDir()
	tfPath := filepath.Join(dir, "main.tf")

	// Minimal valid Terraform file
	tfContent := `terraform {
  required_version = ">= 1.0"
}`

	err := os.WriteFile(tfPath, []byte(tfContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)

	assert.Equal(t, ">= 1.0", metadata.LanguageSpecific["terraform_version"])
}

func TestExtractor_Extract_MultipleFiles(t *testing.T) {
	dir := t.TempDir()

	// Create multiple .tf files
	versionsContent := `terraform {
  required_version = ">= 1.5.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}`

	mainContent := `resource "aws_instance" "example" {
  ami           = "ami-12345"
  instance_type = "t2.micro"
}`

	modulesContent := `module "vpc" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "5.0.0"
}`

	err := os.WriteFile(filepath.Join(dir, "versions.tf"), []byte(versionsContent), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(dir, "main.tf"), []byte(mainContent), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(dir, "modules.tf"), []byte(modulesContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)

	// Should aggregate data from all files
	assert.Equal(t, ">= 1.5.0", metadata.LanguageSpecific["terraform_version"])
	assert.NotNil(t, metadata.LanguageSpecific["providers"])
	assert.NotNil(t, metadata.LanguageSpecific["modules"])
	assert.NotNil(t, metadata.LanguageSpecific["resource_count"])
}

func TestExtractor_Extract_RegexFallback(t *testing.T) {
	dir := t.TempDir()
	tfPath := filepath.Join(dir, "main.tf")

	// Content that might fail HCL parsing but regex can handle
	tfContent := `terraform {
  required_version = ">= 1.5.0"

  required_providers {
    aws = "~> 5.0"
  }

  backend "s3" {
    bucket = "my-bucket"
  }
}`

	err := os.WriteFile(tfPath, []byte(tfContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)

	// Should extract basic info even with simplified syntax
	assert.Equal(t, ">= 1.5.0", metadata.LanguageSpecific["terraform_version"])
}

func TestGenerateTerraformVersionMatrix(t *testing.T) {
	tests := []struct {
		name          string
		constraint    string
		expectedCount int
		shouldContain []string
	}{
		{
			name:          "greater than or equal 1.5",
			constraint:    ">= 1.5.0",
			expectedCount: 3,
			shouldContain: []string{"1.5", "1.6", "1.7"},
		},
		{
			name:          "greater than or equal 1.0",
			constraint:    ">= 1.0.0",
			expectedCount: 8,
			shouldContain: []string{"1.0", "1.1", "1.7"},
		},
		{
			name:          "pessimistic constraint 1.5",
			constraint:    "~> 1.5.0",
			expectedCount: 3,
			shouldContain: []string{"1.5", "1.6", "1.7"},
		},
		{
			name:          "pessimistic constraint 1.3",
			constraint:    "~> 1.3",
			expectedCount: 5,
			shouldContain: []string{"1.3", "1.4", "1.5"},
		},
		{
			name:          "legacy version 0.15",
			constraint:    ">= 0.15.0",
			expectedCount: 4,
			shouldContain: []string{"0.15", "1.0", "1.1"},
		},
		{
			name:          "unknown version defaults",
			constraint:    ">= 99.0",
			expectedCount: 3,
			shouldContain: []string{"1.5", "1.6", "1.7"},
		},
		{
			name:          "empty constraint defaults",
			constraint:    "",
			expectedCount: 3,
			shouldContain: []string{"1.5", "1.6", "1.7"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateTerraformVersionMatrix(tt.constraint)
			assert.Len(t, result, tt.expectedCount)
			for _, version := range tt.shouldContain {
				assert.Contains(t, result, version)
			}
		})
	}
}

func TestQuoteStrings(t *testing.T) {
	input := []string{"1.5", "1.6", "1.7"}
	expected := []string{`"1.5"`, `"1.6"`, `"1.7"`}

	result := quoteStrings(input)
	assert.Equal(t, expected, result)
}

func TestExtractor_Extract_ComplexProviders(t *testing.T) {
	dir := t.TempDir()
	tfPath := filepath.Join(dir, "providers.tf")

	tfContent := `terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = ">= 2.0"
    }
    helm = {
      source  = "hashicorp/helm"
      version = "~> 2.9"
    }
  }
}`

	err := os.WriteFile(tfPath, []byte(tfContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)

	providers := metadata.LanguageSpecific["providers"]
	require.NotNil(t, providers)

	providersList, ok := providers.([]map[string]string)
	require.True(t, ok)
	assert.GreaterOrEqual(t, len(providersList), 3)

	// Check that we have the expected providers
	providerNames := make(map[string]bool)
	for _, p := range providersList {
		providerNames[p["name"]] = true
	}

	// At least one of these should be present (HCL or regex parsing)
	assert.True(t, providerNames["aws"] || providerNames["kubernetes"] || providerNames["helm"])
}

func TestExtractor_Extract_EmptyTerraformBlock(t *testing.T) {
	dir := t.TempDir()
	tfPath := filepath.Join(dir, "main.tf")

	tfContent := `resource "aws_instance" "example" {
  ami           = "ami-12345"
  instance_type = "t2.micro"
}`

	err := os.WriteFile(tfPath, []byte(tfContent), 0644)
	require.NoError(t, err)

	e := NewExtractor()
	metadata, err := e.Extract(dir)
	require.NoError(t, err)

	// Should still succeed with resources but no terraform block
	assert.Equal(t, 1, metadata.LanguageSpecific["resource_count"])
}
