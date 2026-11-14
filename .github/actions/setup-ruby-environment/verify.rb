# frozen_string_literal: true

# Activating the locked bundle also verifies that every required gem is installed.
# No dependency resolution, registry request, or installation is performed here.
require "bundler/setup"

abort "Unexpected Ruby version" unless RUBY_VERSION == ENV.fetch("RUBY_ENV_VERSION")
abort "Unexpected RubyGems version" unless Gem::VERSION == ENV.fetch("RUBY_ENV_GEMS_VERSION")
abort "Unexpected Bundler version" unless Bundler::VERSION == ENV.fetch("RUBY_ENV_BUNDLER_VERSION")
abort "Unexpected Ruby prefix" unless RbConfig::CONFIG["prefix"] == ENV.fetch("RUBY_ENV_PREFIX")
expected_bundle = File.join(ENV.fetch("GITHUB_WORKSPACE"), "vendor/bundle", Bundler.ruby_scope)
abort "Unexpected dependency directory" unless Bundler.bundle_path.expand_path.to_s == expected_bundle
abort "Bundle must be frozen" unless Bundler.settings[:frozen]
