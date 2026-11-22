#!/usr/bin/env bash
set -euo pipefail

bin=${1:?Provide a directory for cwebp}
version=1.5.0
sha256=f4bf49f85991f50e86a5404d16f15b72a053bb66768ed5cc0f6d042277cc2bb8
package="libwebp-${version}-linux-x86-64"
archive=$(mktemp)
trap 'rm -f "$archive"' EXIT

curl -fsSL --retry 2 --connect-timeout 5 --max-time 15 -o "$archive" \
  "https://storage.googleapis.com/downloads.webmproject.org/releases/webp/${package}.tar.gz"
printf '%s  %s\n' "$sha256" "$archive" | sha256sum -c -
mkdir -p "$bin"
tar -xzf "$archive" -C "$bin" --strip-components=2 "${package}/bin/cwebp"
"$bin/cwebp" -version
