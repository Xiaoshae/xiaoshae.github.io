"""Convert local article images in a built Jekyll site, without editing sources."""

import argparse
from concurrent.futures import ThreadPoolExecutor
import hashlib
import html
from html.parser import HTMLParser
import os
from pathlib import Path
import re
import shutil
import subprocess
import tempfile
from urllib.parse import unquote, urlsplit, urlunsplit

from PIL import Image, __version__ as PILLOW_VERSION

RASTER = {".png", ".jpg", ".jpeg", ".tif", ".tiff", ".webp", ".gif", ".bmp", ".avif"}
UNSUPPORTED = {".heic", ".heif"}
# No -mt: parallelism is across images, not nested encoder thread pools.
ENCODER_ARGS = ("-lossless", "-exact", "-q", "75", "-m", "4", "-metadata", "icc")
ATTR = re.compile(r'''(\b(src|srcset|href)\s*=\s*)(?:"([^"]*)"|'([^']*)'|([^\s>]+))''', re.I)


def image_attributes(tag):
    return {"img": {"src", "srcset"}, "source": {"srcset"}, "a": {"href"}}.get(tag, set())


def map_srcset(value, transform):
    # Data URLs can contain commas; leave them intact.
    if "data:" in value.lower():
        return value
    candidates = []
    for candidate in value.split(","):
        parts = candidate.strip().split()
        if parts:
            parts[0] = transform(parts[0])
        candidates.append(" ".join(parts))
    return ", ".join(candidates)


class Page(HTMLParser):
    """Record real HTML tags only; preserve all markup and code samples verbatim."""

    def __init__(self, path, text):
        super().__init__(convert_charrefs=False)
        self.path = path
        self.text = text
        self.tags = []
        self.post_depth = 0
        self.line_offsets = [0]
        self.line_offsets.extend(m.end() for m in re.finditer("\n", text))
        self.feed(text)
        self.close()

    def handle_starttag(self, tag, attrs):
        attrs = dict(attrs)
        if tag == "div":
            if self.post_depth:
                self.post_depth += 1
            elif "post" in (attrs.get("class") or "").split():
                self.post_depth = 1
        if image_attributes(tag):
            line, column = self.getpos()
            self.tags.append((self.line_offsets[line - 1] + column,
                              self.get_starttag_text(), tag, attrs, bool(self.post_depth)))

    def handle_endtag(self, tag):
        if tag == "div" and self.post_depth:
            self.post_depth -= 1

    def handle_startendtag(self, tag, attrs):
        self.handle_starttag(tag, attrs)
        self.handle_endtag(tag)


def resolve_image(url, page, root, baseurl, origins):
    parsed = urlsplit(url)
    if parsed.scheme not in ("", "http", "https"):
        return None
    if parsed.netloc and parsed.netloc.lower() not in origins:
        return None
    path = unquote(parsed.path)
    if not path:
        return None
    if path.startswith("/"):
        if baseurl:
            if not path.startswith(baseurl + "/"):
                return None
            path = path[len(baseurl):]
        source = root / path.lstrip("/")
    else:
        source = page.path.parent / path
    source = source.resolve()
    if not source.is_relative_to(root):
        raise ValueError(f"Image escapes site directory: {url}")
    return source


def validate_static_image(source):
    with Image.open(source) as image:
        if getattr(image, "n_frames", 1) > 1:
            raise ValueError(f"Animated/multi-frame image must not be flattened: {source}")


def convert(source, cache, encoder, version):
    digest = hashlib.sha256(version + repr(ENCODER_ARGS).encode() + source.read_bytes()).hexdigest()
    cached = cache / f"{digest}.webp"
    destination = source.with_name(source.name + ".webp")
    if cached.exists():
        shutil.copyfile(cached, destination)
        return True
    validate_static_image(source)
    # Temporary files and atomic rename prevent partially written cache entries.
    with tempfile.TemporaryDirectory(dir=cache) as temporary:
        output = Path(temporary) / "image.webp"
        try:
            subprocess.run([encoder, *ENCODER_ARGS, str(source), "-o", str(output)],
                           check=True, capture_output=True)
        except subprocess.CalledProcessError:
            # Decode by file contents, not extension (some existing .jpg files are AVIF).
            # This also supports static GIF/BMP/AVIF without a second lossy encoding.
            normalized = Path(temporary) / "decoded.png"
            with Image.open(source) as image:
                image.convert("RGBA").save(normalized, icc_profile=image.info.get("icc_profile"))
            subprocess.run([encoder, *ENCODER_ARGS, str(normalized), "-o", str(output)],
                           check=True, capture_output=True)
        if not output.is_file() or output.stat().st_size == 0:
            raise RuntimeError(f"Encoder produced no image: {source}")
        shutil.copyfile(output, destination)
        os.replace(output, cached)
    return False


def rewrite(page, sources, root, baseurl, origins):
    def update_url(url):
        source = resolve_image(url, page, root, baseurl, origins)
        if source not in sources:
            return url
        parsed = urlsplit(url)
        # Appending rather than replacing the extension avoids foo.png/foo.jpg collisions.
        return urlunsplit(parsed._replace(path=parsed.path + ".webp"))

    text = page.text
    for offset, raw, tag, _, _ in reversed(page.tags):
        def replace(match):
            if match[2].lower() not in image_attributes(tag):
                return match[0]
            value = html.unescape(next(v for v in match.groups()[2:] if v is not None))
            updated = map_srcset(value, update_url) if match[2].lower() == "srcset" else update_url(value)
            if value == updated:
                return match[0]
            return match[1] + '"' + html.escape(updated, quote=True) + '"'
        updated = ATTR.sub(replace, raw)
        text = text[:offset] + updated + text[offset + len(raw):]
    return text


def rewrite_feed(path, sources, root, baseurl, origins):
    # Jekyll's feed.xml embeds article HTML as XML-escaped description text.
    original = path.read_text(encoding="utf-8")
    def replace(match):
        payload = html.unescape(match[2])
        if "<img" not in payload and "<source" not in payload:
            return match[0]
        updated = rewrite(Page(path, payload), sources, root, baseurl, origins)
        if updated == payload:
            return match[0]
        return match[1] + html.escape(updated, quote=False) + match[3]
    updated = re.sub(r"(<description\b[^>]*>)(.*?)(</description>)", replace, original, flags=re.S)
    if updated != original:
        path.write_text(updated, encoding="utf-8")


def build(site, cache, jobs, encoder="cwebp", baseurl="", site_urls=()):
    root = Path(site).resolve()
    cache = Path(cache).resolve()
    if not (root / "index.html").is_file():
        raise ValueError(f"Not a built site: {root}")
    if cache.is_relative_to(root):
        raise ValueError("Cache must be outside the published site")
    baseurl = "/" + baseurl.strip("/") if baseurl.strip("/") else ""
    origins = {urlsplit(url).netloc.lower() for url in site_urls}
    pages = [Page(path, path.read_text(encoding="utf-8")) for path in sorted(root.rglob("*.html"))]
    sources = set()
    for page in pages:
        for _, _, tag, attrs, in_post in page.tags:
            if not in_post:
                continue
            # Links are rewritten later, but do not trigger conversion by themselves.
            for attr in image_attributes(tag) - {"href"}:
                def collect(url):
                    source = resolve_image(url, page, root, baseurl, origins)
                    if source is None:
                        return url
                    if source.suffix.lower() in UNSUPPORTED:
                        raise ValueError(f"Unsupported article image; export HEIC/HEIF as PNG first: {url}")
                    if source.suffix.lower() in RASTER:
                        if not source.is_file():
                            raise FileNotFoundError(f"Missing article image in {page.path}: {url}")
                        sources.add(source)
                    return url
                if attrs.get(attr):
                    if attr == "srcset":
                        map_srcset(attrs[attr], collect)
                    else:
                        collect(attrs[attr])
    if not sources:
        print("No local article raster images found.")
        return {"converted": 0, "cached": 0}
    cache.mkdir(parents=True, exist_ok=True)
    version = subprocess.check_output([encoder, "-version"]) + PILLOW_VERSION.encode()
    ordered = sorted(sources)
    with ThreadPoolExecutor(max_workers=jobs) as pool:
        # Complete every conversion successfully before changing any HTML references.
        hits = list(pool.map(lambda source: convert(source, cache, encoder, version), ordered))
    for page in pages:
        updated = rewrite(page, sources, root, baseurl, origins)
        if updated != page.text:
            page.path.write_text(updated, encoding="utf-8")
    feed = root / "feed.xml"
    if feed.is_file():
        rewrite_feed(feed, sources, root, baseurl, origins)
    before = sum(source.stat().st_size for source in sources)
    after = sum(source.with_name(source.name + ".webp").stat().st_size for source in sources)
    print(f"Article images: {len(sources)}, cached: {sum(hits)}, workers: {jobs}")
    print(f"Referenced image bytes: {before} -> {after} (lossless; savings are not guaranteed)")
    return {"converted": len(sources) - sum(hits), "cached": sum(hits)}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--site", default="_site")
    parser.add_argument("--cache", default=".cache/article-webp")
    parser.add_argument("--jobs", type=int, default=min(8, os.cpu_count() or 2))
    parser.add_argument("--baseurl", default="")
    parser.add_argument("--site-url", action="append", default=[])
    args = parser.parse_args()
    if args.jobs < 1:
        parser.error("--jobs must be positive")
    build(args.site, args.cache, args.jobs, baseurl=args.baseurl, site_urls=args.site_url)


if __name__ == "__main__":
    main()
