// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2025 The Linux Foundation

package ruby

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lfreleng-actions/build-metadata-action/internal/extractor"
)

func TestNewExtractor(t *testing.T) {
	e := NewExtractor()
	if e == nil {
		t.Fatal("NewExtractor returned nil")
	}
	if e.Name() != "ruby" {
		t.Errorf("Expected name 'ruby', got '%s'", e.Name())
	}
	if e.Priority() != 1 {
		t.Errorf("Expected priority 1, got %d", e.Priority())
	}
}

func TestDetect(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		expected bool
	}{
		{
			name: "gemspec file",
			files: map[string]string{
				"mygem.gemspec": `Gem::Specification.new do |s|
  s.name = "mygem"
end`,
			},
			expected: true,
		},
		{
			name: "Gemfile only",
			files: map[string]string{
				"Gemfile": `source 'https://rubygems.org'
gem 'rails'`,
			},
			expected: true,
		},
		{
			name: "Rakefile only",
			files: map[string]string{
				"Rakefile": `task :default => :test`,
			},
			expected: true,
		},
		{
			name: "config.ru (Rack app)",
			files: map[string]string{
				"config.ru": `run MyApp`,
			},
			expected: true,
		},
		{
			name: ".ruby-version",
			files: map[string]string{
				".ruby-version": "3.2.0",
			},
			expected: true,
		},
		{
			name: "lib directory with Ruby files",
			files: map[string]string{
				"lib/mylib.rb": "class MyLib; end",
			},
			expected: true,
		},
		{
			name: "no Ruby indicators",
			files: map[string]string{
				"README.md": "# My Project",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			for path, content := range tt.files {
				fullPath := filepath.Join(tmpDir, path)
				dir := filepath.Dir(fullPath)
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
					t.Fatal(err)
				}
			}

			e := NewExtractor()
			result := e.Detect(tmpDir)
			if result != tt.expected {
				t.Errorf("Detect() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestExtractFromGemspec(t *testing.T) {
	tests := []struct {
		name     string
		gemspec  string
		validate func(*testing.T, *extractor.ProjectMetadata)
	}{
		{
			name: "basic gemspec",
			gemspec: `Gem::Specification.new do |spec|
  spec.name        = "awesome_gem"
  spec.version     = "1.2.3"
  spec.authors     = ["Alice Developer"]
  spec.email       = ["alice@example.com"]
  spec.summary     = "An awesome Ruby gem"
  spec.description = "This gem does awesome things"
  spec.homepage    = "https://github.com/alice/awesome_gem"
  spec.license     = "MIT"
end`,
			validate: func(t *testing.T, m *extractor.ProjectMetadata) {
				if m.Name != "awesome_gem" {
					t.Errorf("Name = %q, want %q", m.Name, "awesome_gem")
				}
				if m.Version != "1.2.3" {
					t.Errorf("Version = %q, want %q", m.Version, "1.2.3")
				}
				if m.Description != "This gem does awesome things" {
					t.Errorf("Description = %q, want %q", m.Description, "This gem does awesome things")
				}
				if m.Homepage != "https://github.com/alice/awesome_gem" {
					t.Errorf("Homepage = %q, want %q", m.Homepage, "https://github.com/alice/awesome_gem")
				}
				if m.License != "MIT" {
					t.Errorf("License = %q, want %q", m.License, "MIT")
				}
				if len(m.Authors) != 1 || m.Authors[0] != "Alice Developer" {
					t.Errorf("Authors = %v, want [Alice Developer]", m.Authors)
				}
				if summary, ok := m.LanguageSpecific["ruby_summary"].(string); !ok || summary != "An awesome Ruby gem" {
					t.Errorf("ruby_summary = %v, want %q", m.LanguageSpecific["ruby_summary"], "An awesome Ruby gem")
				}
			},
		},
		{
			name: "gemspec with dependencies",
			gemspec: `Gem::Specification.new do |s|
  s.name    = "mygem"
  s.version = "0.1.0"

  s.add_runtime_dependency "rails", ">= 6.0"
  s.add_runtime_dependency "pg"
  s.add_development_dependency "rspec", "~> 3.0"
  s.add_development_dependency "rubocop"
end`,
			validate: func(t *testing.T, m *extractor.ProjectMetadata) {
				runtimeDeps, ok := m.LanguageSpecific["ruby_runtime_dependencies"].([]Dependency)
				if !ok {
					t.Fatal("ruby_runtime_dependencies not found or wrong type")
				}
				if len(runtimeDeps) != 2 {
					t.Errorf("Expected 2 runtime dependencies, got %d", len(runtimeDeps))
				}
				if runtimeDeps[0].Name != "rails" || runtimeDeps[0].Requirement != ">= 6.0" {
					t.Errorf("Unexpected runtime dependency: %+v", runtimeDeps[0])
				}

				devDeps, ok := m.LanguageSpecific["ruby_development_dependencies"].([]Dependency)
				if !ok {
					t.Fatal("ruby_development_dependencies not found or wrong type")
				}
				if len(devDeps) != 2 {
					t.Errorf("Expected 2 development dependencies, got %d", len(devDeps))
				}
			},
		},
		{
			name: "gemspec with required_ruby_version",
			gemspec: `Gem::Specification.new do |s|
  s.name                  = "modern_gem"
  s.version               = "2.0.0"
  s.required_ruby_version = ">= 3.0.0"
  s.platform              = "ruby"
end`,
			validate: func(t *testing.T, m *extractor.ProjectMetadata) {
				if rubyVersion, ok := m.LanguageSpecific["ruby_required_ruby_version"].(string); !ok || rubyVersion != ">= 3.0.0" {
					t.Errorf("ruby_required_ruby_version = %v, want %q", m.LanguageSpecific["ruby_required_ruby_version"], ">= 3.0.0")
				}
				if platform, ok := m.LanguageSpecific["ruby_platform"].(string); !ok || platform != "ruby" {
					t.Errorf("ruby_platform = %v, want %q", m.LanguageSpecific["ruby_platform"], "ruby")
				}
			},
		},
		{
			name: "gemspec with alternative license spelling",
			gemspec: `Gem::Specification.new do |s|
  s.name    = "british_gem"
  s.version = "1.0.0"
  s.licence = "Apache-2.0"
end`,
			validate: func(t *testing.T, m *extractor.ProjectMetadata) {
				if m.License != "Apache-2.0" {
					t.Errorf("License = %q, want %q", m.License, "Apache-2.0")
				}
			},
		},
		{
			name: "gemspec with 's' variable",
			gemspec: `Gem::Specification.new do |s|
  s.name        = "ess_gem"
  s.version     = "0.5.0"
  s.author      = "Bob"
  s.email       = "bob@test.com"
end`,
			validate: func(t *testing.T, m *extractor.ProjectMetadata) {
				if m.Name != "ess_gem" {
					t.Errorf("Name = %q, want %q", m.Name, "ess_gem")
				}
				if m.Version != "0.5.0" {
					t.Errorf("Version = %q, want %q", m.Version, "0.5.0")
				}
			},
		},
		{
			name: "gemspec with add_dependency (no runtime prefix)",
			gemspec: `Gem::Specification.new do |s|
  s.name    = "dep_gem"
  s.version = "1.0.0"
  s.add_dependency "nokogiri", "~> 1.13"
end`,
			validate: func(t *testing.T, m *extractor.ProjectMetadata) {
				runtimeDeps, ok := m.LanguageSpecific["ruby_runtime_dependencies"].([]Dependency)
				if !ok || len(runtimeDeps) == 0 {
					t.Fatal("Expected runtime dependencies")
				}
				if runtimeDeps[0].Name != "nokogiri" {
					t.Errorf("Dependency name = %q, want %q", runtimeDeps[0].Name, "nokogiri")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			gemspecPath := filepath.Join(tmpDir, "test.gemspec")
			if err := os.WriteFile(gemspecPath, []byte(tt.gemspec), 0644); err != nil {
				t.Fatal(err)
			}

			e := NewExtractor()
			metadata := &extractor.ProjectMetadata{
				LanguageSpecific: make(map[string]interface{}),
			}

			if err := e.extractFromGemspec(gemspecPath, metadata); err != nil {
				t.Fatalf("extractFromGemspec() error = %v", err)
			}

			tt.validate(t, metadata)
		})
	}
}

func TestExtractFromGemfile(t *testing.T) {
	tests := []struct {
		name     string
		gemfile  string
		validate func(*testing.T, *extractor.ProjectMetadata)
	}{
		{
			name: "basic Gemfile",
			gemfile: `source 'https://rubygems.org'

ruby '3.2.0'

gem 'rails', '~> 7.0'
gem 'pg', '>= 1.1'
gem 'puma'`,
			validate: func(t *testing.T, m *extractor.ProjectMetadata) {
				if version, ok := m.LanguageSpecific["ruby_version"].(string); !ok || version != "3.2.0" {
					t.Errorf("ruby_version = %v, want %q", m.LanguageSpecific["ruby_version"], "3.2.0")
				}
				if source, ok := m.LanguageSpecific["ruby_gem_source"].(string); !ok || source != "https://rubygems.org" {
					t.Errorf("ruby_gem_source = %v, want %q", m.LanguageSpecific["ruby_gem_source"], "https://rubygems.org")
				}

				deps, ok := m.LanguageSpecific["ruby_gemfile_dependencies"].([]Dependency)
				if !ok {
					t.Fatal("ruby_gemfile_dependencies not found")
				}
				if len(deps) != 3 {
					t.Errorf("Expected 3 dependencies, got %d", len(deps))
				}
			},
		},
		{
			name: "Gemfile with bundler",
			gemfile: `source 'https://rubygems.org'

gem 'bundler', '~> 2.0'
gem 'rake'`,
			validate: func(t *testing.T, m *extractor.ProjectMetadata) {
				if hasBundler, ok := m.LanguageSpecific["ruby_has_bundler"].(bool); !ok || !hasBundler {
					t.Error("Expected ruby_has_bundler to be true")
				}
			},
		},
		{
			name: "Gemfile with platforms",
			gemfile: `source 'https://rubygems.org'

platform :ruby do
  gem 'pg'
end

platform :jruby do
  gem 'activerecord-jdbc-adapter'
end`,
			validate: func(t *testing.T, m *extractor.ProjectMetadata) {
				platforms, ok := m.LanguageSpecific["ruby_platforms"].([]string)
				if !ok {
					t.Fatal("ruby_platforms not found")
				}
				if len(platforms) != 2 {
					t.Errorf("Expected 2 platforms, got %d", len(platforms))
				}
			},
		},
		{
			name: "Gemfile with comments",
			gemfile: `source 'https://rubygems.org'

# Database
gem 'pg'

# Web server
gem 'puma'`,
			validate: func(t *testing.T, m *extractor.ProjectMetadata) {
				deps, ok := m.LanguageSpecific["ruby_gemfile_dependencies"].([]Dependency)
				if !ok || len(deps) != 2 {
					t.Errorf("Expected 2 dependencies (comments ignored), got %d", len(deps))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			gemfilePath := filepath.Join(tmpDir, "Gemfile")
			if err := os.WriteFile(gemfilePath, []byte(tt.gemfile), 0644); err != nil {
				t.Fatal(err)
			}

			e := NewExtractor()
			metadata := &extractor.ProjectMetadata{
				LanguageSpecific: make(map[string]interface{}),
			}

			if err := e.extractFromGemfile(gemfilePath, metadata); err != nil {
				t.Fatalf("extractFromGemfile() error = %v", err)
			}

			tt.validate(t, metadata)
		})
	}
}

func TestExtractRubyVersion(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "simple version",
			content: "3.2.0",
			want:    "3.2.0",
		},
		{
			name:    "version with newline",
			content: "3.1.4\n",
			want:    "3.1.4",
		},
		{
			name:    "version with whitespace",
			content: "  2.7.8  \n",
			want:    "2.7.8",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			versionPath := filepath.Join(tmpDir, ".ruby-version")
			if err := os.WriteFile(versionPath, []byte(tt.content), 0644); err != nil {
				t.Fatal(err)
			}

			e := NewExtractor()
			version, err := e.extractRubyVersion(versionPath)
			if err != nil {
				t.Fatalf("extractRubyVersion() error = %v", err)
			}
			if version != tt.want {
				t.Errorf("extractRubyVersion() = %q, want %q", version, tt.want)
			}
		})
	}
}

func TestDetectFrameworks(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		gemfile  string
		expected []string
	}{
		{
			name: "Rails application",
			files: map[string]string{
				"config/application.rb":    "Rails::Application",
				"app/models/.gitkeep":      "",
				"app/views/.gitkeep":       "",
				"app/controllers/.gitkeep": "",
			},
			expected: []string{"rails", "action_view", "active_record", "action_controller"},
		},
		{
			name: "Rails with Webpacker",
			files: map[string]string{
				"config/application.rb":   "Rails::Application",
				"app/javascript/.gitkeep": "",
			},
			expected: []string{"rails", "webpacker"},
		},
		{
			name: "Sinatra app via config.ru",
			files: map[string]string{
				"config.ru": "require 'sinatra'\nrun Sinatra::Application",
			},
			expected: []string{"sinatra"},
		},
		{
			name: "Sinatra app via Gemfile",
			gemfile: `source 'https://rubygems.org'
gem 'sinatra'`,
			expected: []string{"sinatra"},
		},
		{
			name: "RSpec tests",
			files: map[string]string{
				"spec/spec_helper.rb": "RSpec.configure",
			},
			expected: []string{"rspec"},
		},
		{
			name: "Minitest",
			files: map[string]string{
				"test/test_helper.rb": "require 'minitest'",
			},
			expected: []string{"minitest"},
		},
		{
			name: "Cucumber",
			files: map[string]string{
				"features/example.feature": "Feature: Example",
			},
			expected: []string{"cucumber"},
		},
		{
			name: "Hanami",
			files: map[string]string{
				"config/hanami.rb": "Hanami.configure",
			},
			expected: []string{"hanami"},
		},
		{
			name: "Grape API",
			gemfile: `source 'https://rubygems.org'
gem 'grape'`,
			expected: []string{"grape"},
		},
		{
			name: "Rails with RSpec and Cucumber",
			files: map[string]string{
				"bin/rails":         "#!/usr/bin/env ruby",
				"spec/.gitkeep":     "",
				"features/.gitkeep": "",
			},
			expected: []string{"rails", "rspec", "cucumber"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			// Create files
			for path, content := range tt.files {
				fullPath := filepath.Join(tmpDir, path)
				dir := filepath.Dir(fullPath)
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
					t.Fatal(err)
				}
			}

			// Create Gemfile if specified
			if tt.gemfile != "" {
				gemfilePath := filepath.Join(tmpDir, "Gemfile")
				if err := os.WriteFile(gemfilePath, []byte(tt.gemfile), 0644); err != nil {
					t.Fatal(err)
				}
			}

			e := NewExtractor()
			frameworks := e.detectFrameworks(tmpDir)

			// Check all expected frameworks are present
			for _, expected := range tt.expected {
				found := false
				for _, framework := range frameworks {
					if framework == expected {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected framework %q not found in %v", expected, frameworks)
				}
			}
		})
	}
}

func TestExtract(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		wantErr  bool
		validate func(*testing.T, *extractor.ProjectMetadata)
	}{
		{
			name: "complete Ruby project",
			files: map[string]string{
				"mygem.gemspec": `Gem::Specification.new do |s|
  s.name    = "mygem"
  s.version = "1.0.0"
  s.authors = ["Developer"]
  s.license = "MIT"
end`,
				"Gemfile": `source 'https://rubygems.org'
ruby '3.2.0'
gem 'bundler'`,
				".ruby-version":         "3.2.0",
				"config/application.rb": "Rails::Application",
			},
			wantErr: false,
			validate: func(t *testing.T, m *extractor.ProjectMetadata) {
				if m.Name != "mygem" {
					t.Errorf("Name = %q, want %q", m.Name, "mygem")
				}
				if m.Version != "1.0.0" {
					t.Errorf("Version = %q, want %q", m.Version, "1.0.0")
				}
				if version, ok := m.LanguageSpecific["ruby_version"].(string); !ok || version != "3.2.0" {
					t.Errorf("ruby_version = %v, want %q", m.LanguageSpecific["ruby_version"], "3.2.0")
				}
				frameworks, ok := m.LanguageSpecific["ruby_frameworks"].([]string)
				if !ok || len(frameworks) == 0 {
					t.Error("Expected frameworks to be detected")
				}
			},
		},
		{
			name: "Gemfile only project",
			files: map[string]string{
				"Gemfile": `source 'https://rubygems.org'
gem 'sinatra'`,
			},
			wantErr: false,
			validate: func(t *testing.T, m *extractor.ProjectMetadata) {
				if _, ok := m.LanguageSpecific["ruby_gemfile_dependencies"]; !ok {
					t.Error("Expected ruby_gemfile_dependencies")
				}
			},
		},
		{
			name: "gemspec only project",
			files: map[string]string{
				"test.gemspec": `Gem::Specification.new do |s|
  s.name    = "testgem"
  s.version = "0.1.0"
end`,
			},
			wantErr: false,
			validate: func(t *testing.T, m *extractor.ProjectMetadata) {
				if m.Name != "testgem" {
					t.Errorf("Name = %q, want %q", m.Name, "testgem")
				}
			},
		},
		{
			name: "ruby-version only",
			files: map[string]string{
				".ruby-version": "3.1.0",
			},
			wantErr: false,
			validate: func(t *testing.T, m *extractor.ProjectMetadata) {
				if version, ok := m.LanguageSpecific["ruby_version"].(string); !ok || version != "3.1.0" {
					t.Errorf("ruby_version = %v, want %q", m.LanguageSpecific["ruby_version"], "3.1.0")
				}
			},
		},
		{
			name:    "no Ruby files",
			files:   map[string]string{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			for path, content := range tt.files {
				fullPath := filepath.Join(tmpDir, path)
				dir := filepath.Dir(fullPath)
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
					t.Fatal(err)
				}
			}

			e := NewExtractor()
			metadata, err := e.Extract(tmpDir)

			if tt.wantErr {
				if err == nil {
					t.Error("Extract() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("Extract() unexpected error = %v", err)
			}

			if metadata == nil {
				t.Fatal("Extract() returned nil metadata")
			}

			if tt.validate != nil {
				tt.validate(t, metadata)
			}
		})
	}
}

func TestGenerateVersionMatrix(t *testing.T) {
	tests := []struct {
		name     string
		metadata *extractor.ProjectMetadata
		validate func(*testing.T, map[string]interface{})
	}{
		{
			name: "with required_ruby_version",
			metadata: &extractor.ProjectMetadata{
				LanguageSpecific: map[string]interface{}{
					"ruby_required_ruby_version": ">= 3.0",
				},
			},
			validate: func(t *testing.T, matrix map[string]interface{}) {
				versions, ok := matrix["ruby-version"].([]string)
				if !ok {
					t.Fatal("ruby-version not found in matrix")
				}
				if len(versions) == 0 {
					t.Error("Expected at least one Ruby version")
				}
			},
		},
		{
			name: "with ruby_version",
			metadata: &extractor.ProjectMetadata{
				LanguageSpecific: map[string]interface{}{
					"ruby_version": "3.2.0",
				},
			},
			validate: func(t *testing.T, matrix map[string]interface{}) {
				versions, ok := matrix["ruby-version"].([]string)
				if !ok || len(versions) == 0 {
					t.Fatal("Expected ruby-version in matrix")
				}
				if versions[0] != "3.2.0" {
					t.Errorf("Expected version 3.2.0, got %v", versions[0])
				}
			},
		},
		{
			name: "no version specified (defaults)",
			metadata: &extractor.ProjectMetadata{
				LanguageSpecific: map[string]interface{}{},
			},
			validate: func(t *testing.T, matrix map[string]interface{}) {
				versions, ok := matrix["ruby-version"].([]string)
				if !ok || len(versions) == 0 {
					t.Fatal("Expected default ruby-version in matrix")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewExtractor()
			matrix := e.GenerateVersionMatrix(tt.metadata)

			// Check OS matrix is always present
			if os, ok := matrix["os"].([]string); !ok || len(os) == 0 {
				t.Error("Expected os in matrix")
			}

			if tt.validate != nil {
				tt.validate(t, matrix)
			}
		})
	}
}

func TestParseRubyVersionRequirement(t *testing.T) {
	tests := []struct {
		name        string
		requirement string
		expectMin   int // minimum number of versions expected
	}{
		{
			name:        "exact version",
			requirement: "3.2.0",
			expectMin:   1,
		},
		{
			name:        "greater than or equal",
			requirement: ">= 3.0",
			expectMin:   1,
		},
		{
			name:        "pessimistic version",
			requirement: "~> 3.1",
			expectMin:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewExtractor()
			versions := e.parseRubyVersionRequirement(tt.requirement)

			if len(versions) < tt.expectMin {
				t.Errorf("parseRubyVersionRequirement(%q) returned %d versions, want at least %d",
					tt.requirement, len(versions), tt.expectMin)
			}
		})
	}
}

func TestIsRailsProject(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		expected bool
	}{
		{
			name: "config/application.rb exists",
			files: map[string]string{
				"config/application.rb": "Rails::Application",
			},
			expected: true,
		},
		{
			name: "bin/rails exists",
			files: map[string]string{
				"bin/rails": "#!/usr/bin/env ruby",
			},
			expected: true,
		},
		{
			name: "Gemfile with rails",
			files: map[string]string{
				"Gemfile": "gem 'rails'",
			},
			expected: true,
		},
		{
			name:     "not a Rails project",
			files:    map[string]string{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			for path, content := range tt.files {
				fullPath := filepath.Join(tmpDir, path)
				dir := filepath.Dir(fullPath)
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
					t.Fatal(err)
				}
			}

			e := NewExtractor()
			result := e.isRailsProject(tmpDir)
			if result != tt.expected {
				t.Errorf("isRailsProject() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestIsSinatraProject(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		expected bool
	}{
		{
			name: "config.ru with Sinatra",
			files: map[string]string{
				"config.ru": "require 'sinatra'\nrun Sinatra::Application",
			},
			expected: true,
		},
		{
			name: "Gemfile with sinatra",
			files: map[string]string{
				"Gemfile": "gem 'sinatra'",
			},
			expected: true,
		},
		{
			name:     "not a Sinatra project",
			files:    map[string]string{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			for path, content := range tt.files {
				fullPath := filepath.Join(tmpDir, path)
				dir := filepath.Dir(fullPath)
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
					t.Fatal(err)
				}
			}

			e := NewExtractor()
			result := e.isSinatraProject(tmpDir)
			if result != tt.expected {
				t.Errorf("isSinatraProject() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestHasGemfileDependency(t *testing.T) {
	tests := []struct {
		name     string
		gemfile  string
		gemName  string
		expected bool
	}{
		{
			name:     "gem present",
			gemfile:  "gem 'rails'\ngem 'pg'",
			gemName:  "rails",
			expected: true,
		},
		{
			name:     "gem not present",
			gemfile:  "gem 'sinatra'",
			gemName:  "rails",
			expected: false,
		},
		{
			name:     "gem with version",
			gemfile:  "gem 'rails', '~> 7.0'",
			gemName:  "rails",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			gemfilePath := filepath.Join(tmpDir, "Gemfile")
			if err := os.WriteFile(gemfilePath, []byte(tt.gemfile), 0644); err != nil {
				t.Fatal(err)
			}

			e := NewExtractor()
			result := e.hasGemfileDependency(tmpDir, tt.gemName)
			if result != tt.expected {
				t.Errorf("hasGemfileDependency(%q) = %v, want %v", tt.gemName, result, tt.expected)
			}
		})
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		name  string
		slice []string
		value string
		want  bool
	}{
		{
			name:  "value present",
			slice: []string{"a", "b", "c"},
			value: "b",
			want:  true,
		},
		{
			name:  "value not present",
			slice: []string{"a", "b", "c"},
			value: "d",
			want:  false,
		},
		{
			name:  "empty slice",
			slice: []string{},
			value: "a",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := contains(tt.slice, tt.value)
			if got != tt.want {
				t.Errorf("contains(%v, %q) = %v, want %v", tt.slice, tt.value, got, tt.want)
			}
		})
	}
}

// TestIsVersionCompatible tests numeric version comparison
func TestIsVersionCompatible(t *testing.T) {
	e := NewExtractor()

	tests := []struct {
		name        string
		requirement string
		version     string
		expected    bool
		description string
	}{
		{
			name:        "exact match major.minor",
			requirement: "3.0",
			version:     "3.0",
			expected:    true,
			description: "3.0 satisfies requirement 3.0",
		},
		{
			name:        "version with patch satisfies major.minor",
			requirement: "3.0",
			version:     "3.0.5",
			expected:    true,
			description: "3.0.5 satisfies requirement 3.0 (prefix match)",
		},
		{
			name:        "higher minor version",
			requirement: "3.0",
			version:     "3.1",
			expected:    true,
			description: "3.1 > 3.0",
		},
		{
			name:        "higher major version",
			requirement: "2.7",
			version:     "3.0",
			expected:    true,
			description: "3.0 > 2.7",
		},
		{
			name:        "much higher minor version",
			requirement: "3.0",
			version:     "3.10",
			expected:    true,
			description: "3.10 > 3.0 (numeric comparison, not lexicographic)",
		},
		{
			name:        "much higher minor with patch",
			requirement: "3.0",
			version:     "3.10.1",
			expected:    true,
			description: "3.10.1 > 3.0",
		},
		{
			name:        "lower minor version",
			requirement: "3.1",
			version:     "3.0",
			expected:    false,
			description: "3.0 < 3.1",
		},
		{
			name:        "lower major version",
			requirement: "3.0",
			version:     "2.7",
			expected:    false,
			description: "2.7 < 3.0",
		},
		{
			name:        "version 2.7 vs 3.0",
			requirement: "2.7",
			version:     "3.0",
			expected:    true,
			description: "3.0 > 2.7",
		},
		{
			name:        "version 3.2 vs 3.10",
			requirement: "3.2",
			version:     "3.10",
			expected:    true,
			description: "3.10 > 3.2 (tests proper numeric comparison)",
		},
		{
			name:        "version 3.10 vs 3.2",
			requirement: "3.10",
			version:     "3.2",
			expected:    false,
			description: "3.2 < 3.10",
		},
		{
			name:        "version with three components",
			requirement: "3.0.0",
			version:     "3.0.5",
			expected:    true,
			description: "3.0.5 > 3.0.0",
		},
		{
			name:        "equal three component versions",
			requirement: "3.0.5",
			version:     "3.0.5",
			expected:    true,
			description: "3.0.5 == 3.0.5",
		},
		{
			name:        "version 3.0 vs 3.0.0",
			requirement: "3.0",
			version:     "3.0.0",
			expected:    true,
			description: "3.0.0 satisfies 3.0",
		},
		{
			name:        "common case: 2.7 vs 3.3",
			requirement: "2.7",
			version:     "3.3",
			expected:    true,
			description: "3.3 > 2.7",
		},
		{
			name:        "common case: 3.0 vs 3.3",
			requirement: "3.0",
			version:     "3.3",
			expected:    true,
			description: "3.3 > 3.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := e.isVersionCompatible(tt.requirement, tt.version)
			if result != tt.expected {
				t.Errorf("isVersionCompatible(%q, %q) = %v, expected %v - %s",
					tt.requirement, tt.version, result, tt.expected, tt.description)
			}
		})
	}
}

// TestGetCompatibleVersions tests the version filtering
func TestGetCompatibleVersions(t *testing.T) {
	e := NewExtractor()

	tests := []struct {
		name          string
		baseVersion   string
		expectedMin   int
		shouldInclude []string
		shouldExclude []string
	}{
		{
			name:          "requirement 2.7",
			baseVersion:   "2.7",
			expectedMin:   5, // Should include 2.7, 3.0, 3.1, 3.2, 3.3
			shouldInclude: []string{"2.7", "3.0", "3.1", "3.2", "3.3"},
			shouldExclude: []string{},
		},
		{
			name:          "requirement 3.0",
			baseVersion:   "3.0",
			expectedMin:   4, // Should include 3.0, 3.1, 3.2, 3.3
			shouldInclude: []string{"3.0", "3.1", "3.2", "3.3"},
			shouldExclude: []string{"2.7"},
		},
		{
			name:          "requirement 3.2",
			baseVersion:   "3.2",
			expectedMin:   2, // Should include 3.2, 3.3
			shouldInclude: []string{"3.2", "3.3"},
			shouldExclude: []string{"2.7", "3.0", "3.1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := e.getCompatibleVersions(tt.baseVersion)

			if len(result) < tt.expectedMin {
				t.Errorf("getCompatibleVersions(%q) returned %d versions, expected at least %d",
					tt.baseVersion, len(result), tt.expectedMin)
			}

			// Check that expected versions are included
			for _, expected := range tt.shouldInclude {
				found := false
				for _, version := range result {
					if version == expected {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("getCompatibleVersions(%q) missing expected version %q, got %v",
						tt.baseVersion, expected, result)
				}
			}

			// Check that excluded versions are not included
			for _, excluded := range tt.shouldExclude {
				for _, version := range result {
					if version == excluded {
						t.Errorf("getCompatibleVersions(%q) incorrectly included version %q, got %v",
							tt.baseVersion, excluded, result)
					}
				}
			}
		})
	}
}
