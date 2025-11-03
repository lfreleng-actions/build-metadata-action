# frozen_string_literal: true

# SPDX-License-Identifier: Apache-2.0
# SPDX-FileCopyrightText: 2025 The Linux Foundation

Gem::Specification.new do |spec|
  spec.name          = "rails-example-app"
  spec.version       = "7.1.0"
  spec.authors       = ["Rails Developer", "Second Author"]
  spec.email         = ["rails@example.com", "second@example.com"]

  spec.summary       = "A Rails application example"
  spec.description   = "This is a comprehensive Rails application for testing metadata extraction"
  spec.homepage      = "https://github.com/example/rails-app"
  spec.license       = "MIT"
  spec.required_ruby_version = ">= 3.0.0"

  spec.metadata["homepage_uri"] = spec.homepage
  spec.metadata["source_code_uri"] = "https://github.com/example/rails-app"
  spec.metadata["changelog_uri"] = "https://github.com/example/rails-app/blob/main/CHANGELOG.md"

  # Runtime dependencies
  spec.add_runtime_dependency "rails", "~> 7.1"
  spec.add_runtime_dependency "pg", ">= 1.1"
  spec.add_runtime_dependency "puma", "~> 6.0"
  spec.add_runtime_dependency "redis", ">= 4.0"
  spec.add_runtime_dependency "bootsnap", ">= 1.4.4"

  # Development dependencies
  spec.add_development_dependency "rspec-rails", "~> 6.0"
  spec.add_development_dependency "factory_bot_rails", "~> 6.2"
  spec.add_development_dependency "faker", "~> 3.0"
  spec.add_development_dependency "rubocop", "~> 1.50"
  spec.add_development_dependency "rubocop-rails", "~> 2.19"

  spec.files = Dir.chdir(File.expand_path(__dir__)) do
    Dir["{app,config,db,lib}/**/*", "Rakefile", "README.md"]
  end
  spec.require_paths = ["lib"]
end
