#!/usr/bin/env bash
set -euo pipefail

if [[ ${GITHUB_REF:-} != refs/heads/master ]]; then
  echo 'Only the master workflow may publish the image cache' >&2
  exit 1
fi
delta=$(realpath -- "${1:?Provide the downloaded WebP delta directory}")
cd "$(git rev-parse --show-toplevel)"
repo=$PWD
ref=refs/heads/image-cache
namespace=$(<.github/image-build/article-webp.sha256)
if [[ ! $namespace =~ ^[0-9a-f]{64}$ ]]; then
  echo 'Invalid article-webp SHA-256' >&2
  exit 1
fi

shopt -s nullglob dotglob
files=("$delta"/*)
if [[ ${#files[@]} -eq 0 ]]; then
  echo 'No new cache files in the artifact' >&2
  exit 1
fi
for image in "${files[@]}"; do
  if [[ -L "$image" || ! -f "$image" || ! -s "$image" || ! ${image##*/} =~ ^[0-9a-f]{64}\.webp$ ]]; then
    echo 'Invalid entry in the WebP delta artifact' >&2
    exit 1
  fi
done

tmp=$(mktemp -d)
tree="$tmp/cache"
cleanup() {
  git -C "$repo" worktree remove --force "$tree" 2>/dev/null || true
  rm -rf -- "$tmp"
}
trap cleanup EXIT

# Merge into the latest cache tip, not the snapshot the build read.
git fetch --no-tags --depth=1 origin "$ref"
git worktree add --detach "$tree" FETCH_HEAD
cache="$tree/webp/$namespace"
if [[ -L "$tree/webp" || -L "$cache" ]]; then
  echo 'Unexpected symlink in the cache destination' >&2
  exit 1
fi
mkdir -p "$cache"
for image in "${files[@]}"; do
  target="$cache/${image##*/}"
  if [[ -L "$target" || ( -e "$target" && ! -f "$target" ) ]]; then
    echo "Invalid cache destination: ${image##*/}" >&2
    exit 1
  fi
  if [[ -e "$target" ]]; then
    if ! cmp -s -- "$image" "$target"; then
      echo "Conflicting cache entry: ${image##*/}" >&2
      exit 1
    fi
    continue
  fi
  install -m 0644 -- "$image" "$target"
  git -C "$tree" add -- "webp/$namespace/${image##*/}"
done
if git -C "$tree" diff --cached --quiet; then
  echo 'WebP delta already published'
  exit 0
fi

git -C "$tree" -c user.name='github-actions[bot]' \
  -c user.email='41898282+github-actions[bot]@users.noreply.github.com' \
  -c core.hooksPath=/dev/null commit --quiet -m 'Cache lossless article WebP images'
if ! git push origin "$(git -C "$tree" rev-parse HEAD):$ref"; then
  echo '::warning::Could not publish the WebP delta; deployment is independent of this job.'
  exit 1
fi
