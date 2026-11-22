#!/usr/bin/env bash
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"
repo=$PWD
ref=refs/heads/image-cache
encoder_sha=3f7c790f922b9417031180e33ed3ccfab5f2c543829d7915e2953f8419a81422
version=$(sha256sum .github/image-build/convert_images.py .github/image-build/uv.lock | sha256sum)
namespace="${encoder_sha}-${version%% *}"

tmp=$(mktemp -d)
tree="$tmp/cache"
cleanup() {
  git -C "$repo" worktree remove --force "$tree" 2>/dev/null || true
  rm -rf -- "$tmp"
}
trap cleanup EXIT

git fetch --no-tags --depth=1 origin "$ref"
git worktree add --detach "$tree" FETCH_HEAD

encoder="$tree/bin/cwebp"
cache="$tree/webp/$namespace"
if [[ -L "$tree/bin" || -L "$encoder" || ! -f "$encoder" || ! -x "$encoder" ]]; then
  echo 'Missing executable bin/cwebp on image-cache' >&2
  exit 1
fi
printf '%s  %s\n' "$encoder_sha" "$encoder" | sha256sum -c -
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
PATH="${encoder%/*}:$PATH" uv run --project .github/image-build --frozen python .github/image-build/convert_images.py \
  --site public --cache "$cache" \
  --site-url https://blog.xiaoshae.cn --site-url https://xiaoshae.github.io

# Cache publication is best-effort and never uses a force push.
if [[ ${GITHUB_REF:-} == refs/heads/master ]]; then
  publish() {
    git -C "$tree" add -- "webp/$namespace/" || return
    if git -C "$tree" diff --cached --quiet; then
      echo 'Image cache unchanged'
      return 0
    fi
    git -C "$tree" -c user.name='github-actions[bot]' \
      -c user.email='41898282+github-actions[bot]@users.noreply.github.com' \
      -c core.hooksPath=/dev/null commit --quiet -m 'Cache lossless article WebP images' &&
      git push origin "$(git -C "$tree" rev-parse HEAD):$ref"
  }
  if ! publish; then
    echo '::warning::Could not publish the image cache; the generated site will still be deployed.'
  fi
fi
