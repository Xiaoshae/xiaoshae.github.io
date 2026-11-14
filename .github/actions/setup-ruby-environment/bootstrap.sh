#!/usr/bin/env bash
set -euo pipefail
cd "$GITHUB_WORKSPACE"

# This runs only on an exact cache miss. No release API or latest-version lookup.
archive="ruby-${RUBY_ENV_VERSION}-${RUBY_ENV_PLATFORM}.tar.gz"
url="https://github.com/ruby/ruby-builder/releases/download/ruby-${RUBY_ENV_VERSION}/${archive}"
tmp=$(mktemp -d "$RUNNER_TEMP/pinned-ruby.XXXXXX")
trap 'rm -rf "$tmp"' EXIT
curl --fail --location --silent --show-error --retry 3 --max-time 180 "$url" -o "$tmp/$archive"
printf '%s  %s\n' "$RUBY_ENV_ARCHIVE_SHA256" "$tmp/$archive" | sha256sum --check --strict -
mkdir -p "$(dirname "$RUBY_ENV_PREFIX")"
tar -xzf "$tmp/$archive" -C "$(dirname "$RUBY_ENV_PREFIX")"
touch "${RUBY_ENV_PREFIX}.complete"

ruby -rrubygems -e 'abort "Unexpected Ruby/RubyGems version" unless RUBY_VERSION == ENV.fetch("RUBY_ENV_VERSION") && Gem::VERSION == ENV.fetch("RUBY_ENV_GEMS_VERSION")'
gem install bundler --version "$RUBY_ENV_BUNDLER_VERSION" --no-document
bundle "_${RUBY_ENV_BUNDLER_VERSION}_" install --jobs 4 --retry 3
