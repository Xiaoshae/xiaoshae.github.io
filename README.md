# Image cache

The production workflow on `master` maintains this branch. It does not deploy it.

- `bin/article-webp-linux-amd64-<sha256>`: static Linux x86-64 article image processor. Source and reproducible build instructions live in `.github/image-build/` on `master`.
- `webp/<sha256>/`: generated images, namespaced by processor digest.
- `bin/cwebp` and the original cache namespace remain for deployments pinned to the previous workflow. The standalone processor embeds this upstream WebP 1.5.0 encoder; see `COPYING`, `PATENTS`, and `licenses/article-webp/` for bundled component notices.
