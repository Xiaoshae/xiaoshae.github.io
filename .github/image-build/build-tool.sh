#!/usr/bin/env bash
set -euo pipefail

here=$(cd -- "$(dirname -- "$0")" && pwd)
output=$(realpath -m -- "${1:?Provide an output path for the Linux amd64 tool}")
tmp=$(mktemp -d)
trap 'rm -rf -- "$tmp"' EXIT

version=1.27.4
archive="$tmp/go.tar.gz"
curl -fsSL --retry 2 --connect-timeout 5 --max-time 90 -o "$archive" \
  "https://go.dev/dl/go${version}.linux-amd64.tar.gz"
printf '%s  %s\n' 7226736c2339719cda28fbf3a54d71aa63bcefe376e98cef00034268dab24840 "$archive" | sha256sum -c -
tar -xzf "$archive" -C "$tmp"

mkdir -p "$tmp/src" "$(dirname "$output")"
cp -- "$here"/tool/*.go "$here"/tool/go.mod "$here"/tool/go.sum "$tmp/src/"
bash "$here/install-cwebp.sh" "$tmp/src"
cd "$tmp/src"
export GOTOOLCHAIN=local GOOS=linux GOARCH=amd64 GOAMD64=v1 CGO_ENABLED=0
"$tmp/go/bin/go" test -mod=readonly -tags nodynamic ./...
"$tmp/go/bin/go" build -mod=readonly -tags nodynamic -trimpath -buildvcs=false \
  -ldflags '-s -w -buildid=' -o "$output" .
sha256sum "$output"
