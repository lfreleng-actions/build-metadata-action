# frozen_string_literal: true

# SPDX-License-Identifier: Apache-2.0
# SPDX-FileCopyrightText: 2025 The Linux Foundation

Gem::Specification.new do |s|
  s.name        = "simple-gem"
  s.version     = "0.1.0"
  s.authors     = ["Simple Developer"]
  s.email       = ["simple@example.com"]
  s.summary     = "A simple Ruby gem"
  s.description = "A minimal Ruby gem for testing purposes"
  s.homepage    = "https://github.com/example/simple-gem"
  s.license     = "Apache-2.0"

  s.required_ruby_version = ">= 2.7.0"
  s.platform              = "ruby"

  s.add_dependency "bundler", "~> 2.0"
  s.add_dependency "rake", "~> 13.0"

  s.add_development_dependency "rspec", "~> 3.0"
  s.add_development_dependency "rubocop"

  s.files         = Dir["lib/**/*.rb", "README.md", "LICENSE"]
  s.require_paths = ["lib"]
end
