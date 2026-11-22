#!/usr/bin/env bash
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"
repo=$PWD
delta=${1:?Provide an output directory for new WebP files}
ref=refs/heads/image-cache
namespace=$(<.github/image-build/article-webp.sha256)
if [[ ! $namespace =~ ^[0-9a-f]{64}$ ]]; then
  echo 'Invalid article-webp SHA-256' >&2
  exit 1
fi

tmp=$(mktemp -d)
tree="$tmp/cache"
cleanup() {
  git -C "$repo" worktree remove --force "$tree" 2>/dev/null || true
  rm -rf -- "$tmp"
}
trap cleanup EXIT

git fetch --no-tags --depth=1 origin "$ref"
git worktree add --detach "$tree" FETCH_HEAD

tool="$tree/bin/article-webp-linux-amd64-$namespace"
cache="$tree/webp/$namespace"
if [[ -L "$tree/bin" || -L "$tool" || ! -f "$tool" || ! -x "$tool" ]]; then
  echo 'Missing pinned article-webp executable on image-cache' >&2
  exit 1
fi
printf '%s  %s\n' "$namespace" "$tool" | sha256sum -c -
if [[ -L "$tree/webp" || -L "$cache" ]]; then
  echo 'Unexpected symlink in the image cache directory' >&2
  exit 1
fi
mkdir -p "$cache"
shopt -s nullglob dotglob
for image in "$cache"/*; do
  if [[ -L "$image" || ! -f "$image" || ! -s "$image" || ! ${image##*/} =~ ^[0-9a-f]{64}\.webp$ ]]; then
    echo 'Unexpected entry in the image cache directory' >&2
    exit 1
  fi
done

# Both Pages deployments use a domain root; there is no base path.
"$tool" --site public --cache "$cache" \
  --site-url https://blog.xiaoshae.cn --site-url https://xiaoshae.github.io

# Export only newly generated files; this job never commits or pushes.
git -C "$tree" ls-files --others --exclude-standard -z -- "webp/$namespace/" > "$tmp/delta"
count=0
while IFS= read -r -d '' path; do
  mkdir -p -- "$delta"
  cp -- "$tree/$path" "$delta/${path##*/}"
  count=$((count + 1))
done < "$tmp/delta"
if [[ $count -eq 0 ]]; then
  echo 'Image cache unchanged'
else
  echo "Prepared $count new WebP cache files"
  if [[ -n ${GITHUB_OUTPUT:-} ]]; then
    echo 'changed=true' >> "$GITHUB_OUTPUT"
  fi
fi
