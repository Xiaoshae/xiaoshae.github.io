from pathlib import Path
import shutil
import struct
import subprocess
import zlib
import tempfile
import threading
import time
import unittest
from unittest.mock import patch

from convert_images import build


class ImageBuildTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.site = self.root / "_site"
        self.cache = self.root / "cache"
        self.site.mkdir()
        (self.site / "index.html").write_text("<h1>Home</h1>")
        self.article = self.site / "blog" / "example" / "index.html"
        self.article.parent.mkdir(parents=True)
        self.images = self.site / "assets" / "img"
        self.images.mkdir(parents=True)
        self.active = self.peak = self.calls = 0
        self.lock = threading.Lock()
        self.addCleanup(patch.stopall)
        patch("convert_images.validate_static_image").start()
        patch("convert_images.subprocess.check_output", return_value=b"test-encoder-1").start()
        patch("convert_images.subprocess.run", side_effect=self.encode).start()

    def encode(self, args, **kwargs):
        self.assertIn("-lossless", args)
        self.assertIn("-exact", args)
        self.assertNotIn("-mt", args)
        with self.lock:
            self.active += 1
            self.calls += 1
            self.peak = max(self.peak, self.active)
        time.sleep(0.02)
        Path(args[-1]).write_bytes(b"fake-webp:" + Path(args[-3]).read_bytes())
        with self.lock:
            self.active -= 1
        return subprocess.CompletedProcess(args, 0)

    def run_build(self, **kwargs):
        return build(self.site, self.cache, jobs=3, **kwargs)

    def image(self, name, data=b"pixels"):
        (self.images / name).write_bytes(data)

    def test_conversion_references_and_originals(self):
        self.image("shot.png")
        self.image("touring.jpg")
        self.article.write_text('''<img src="/assets/img/touring.jpg">
<div class="post"><div><a href="/assets/img/shot.png?download=1#zoom"><img src='/assets/img/shot.png'></a></div>
<pre>&lt;img src="/assets/img/shot.png"&gt;</pre>
<script>const example = '<img src="/assets/img/shot.png">';</script></div>''')
        self.site.joinpath("index.html").write_text('<img src="/assets/img/shot.png">')
        self.site.joinpath("feed.xml").write_text('<rss><description>&lt;p&gt;&lt;img src=&quot;/assets/img/shot.png&quot;&gt;&lt;/p&gt;</description></rss>')
        self.assertEqual(self.run_build(), {"converted": 1, "cached": 0})
        output = self.article.read_text()
        self.assertIn('href="/assets/img/shot.webp?download=1#zoom"', output)
        self.assertIn('src="/assets/img/shot.webp"', output)
        self.assertIn('src="/assets/img/touring.jpg"', output)
        self.assertIn('&lt;img src="/assets/img/shot.png"&gt;', output)
        self.assertIn("const example = '<img src=\"/assets/img/shot.png\">';", output)
        self.assertFalse((self.images / "shot.png").exists())
        self.assertTrue((self.images / "shot.webp").exists())
        self.assertTrue((self.images / "touring.jpg").exists())
        self.assertFalse((self.images / "touring.webp").exists())
        self.assertIn("shot.webp", self.site.joinpath("index.html").read_text())
        import xml.etree.ElementTree as ET
        feed = ET.parse(self.site / "feed.xml")
        self.assertIn('src="/assets/img/shot.webp"', feed.find("description").text)

    def test_srcset_relative_encoded_paths_and_external_images(self):
        self.image("screen shot.PNG")
        self.image("screen.jpg", b"other pixels")
        self.article.write_text('''<div class="post"><picture><source srcset="../../assets/img/screen%20shot.PNG 1x, /assets/img/screen.jpg 2x"><img src="https://blog.example/assets/img/screen.jpg?x=1&amp;y=2"></picture><img src="https://external.example/remote.png"><img src="data:image/png;base64,AAAA"></div>''')
        self.run_build(site_urls=["https://blog.example"])
        output = self.article.read_text()
        self.assertIn("screen%20shot.webp 1x", output)
        self.assertIn("screen.webp 2x", output)
        self.assertIn("screen.webp?x=1&amp;y=2", output)
        self.assertIn("https://external.example/remote.png", output)

    def test_parallel_conversion_cache_and_invalidation(self):
        for index in range(5):
            self.image(f"{index}.png", str(index).encode())
        markup = '<div class="post">' + ''.join(f'<img src="/assets/img/{i}.png">' for i in range(5)) + '</div>'
        self.article.write_text(markup)
        self.run_build()
        self.assertGreater(self.peak, 1)
        self.assertLessEqual(self.peak, 3)
        for image in self.images.glob("*.webp"):
            image.unlink()
        for index in range(5):
            self.image(f"{index}.png", str(index).encode())
        self.article.write_text(markup)  # A clean Jekyll rebuild restores originals and URLs.
        self.assertEqual(self.run_build(), {"converted": 0, "cached": 5})
        self.assertEqual(list(self.images.glob("*.png")), [])
        for image in self.images.glob("*.webp"):
            image.unlink()
        for index in range(5):
            self.image(f"{index}.png", b"changed pixels" if index == 0 else str(index).encode())
        self.article.write_text(markup)
        self.assertEqual(self.run_build(), {"converted": 1, "cached": 4})

    def test_same_basename_does_not_collide_and_heading_only_pages_are_untouched(self):
        self.image("same.png", b"PNG")
        self.image("same.jpg", b"JPEG")
        self.article.write_text('<div class="post"><img src="/assets/img/same.png"><img src="/assets/img/same.jpg"></div>')
        self.run_build()
        outputs = list(self.images.glob("same-*.webp"))
        self.assertEqual(len(outputs), 2)
        self.assertNotEqual(outputs[0].read_bytes(), outputs[1].read_bytes())
        output = self.article.read_text()
        for image in outputs:
            self.assertIn(image.name, output)
        self.assertNotIn(".png.webp", output)
        self.assertNotIn(".jpg.webp", output)

    def test_existing_webp_is_not_overwritten_by_same_stem_png(self):
        self.image("same.png", b"PNG")
        self.image("same.webp", b"existing WebP")
        self.article.write_text('<div class="post"><img src="/assets/img/same.png"></div>')
        self.run_build()
        self.assertEqual((self.images / "same.webp").read_bytes(), b"existing WebP")
        self.assertEqual(len(list(self.images.glob("same-*.webp"))), 1)

    def test_webp_input_keeps_its_output_file(self):
        self.image("existing.webp")
        self.article.write_text('<div class="post"><img src="/assets/img/existing.webp"></div>')
        self.run_build()
        self.assertTrue((self.images / "existing.webp").is_file())
        self.assertIn('src="/assets/img/existing.webp"', self.article.read_text())
        self.assertEqual(len(list(self.images.glob("*.webp"))), 1)

    def test_baseurl_and_vector_image(self):
        self.image("shot.png")
        self.article.write_text('<div class="post"><img src="/blog-prefix/assets/img/shot.png"><img src="/blog-prefix/assets/img/diagram.svg"></div>')
        self.run_build(baseurl="/blog-prefix/")
        output = self.article.read_text()
        self.assertIn("/blog-prefix/assets/img/shot.webp", output)
        self.assertIn("diagram.svg", output)

    def test_missing_image_and_failed_encoder_do_not_rewrite_html(self):
        markup = '<div class="post"><img src="/assets/img/missing.png"></div>'
        self.article.write_text(markup)
        with self.assertRaises(FileNotFoundError):
            self.run_build()
        self.assertEqual(self.article.read_text(), markup)
        self.image("missing.png")
        with patch("convert_images.Image.open"), patch("convert_images.subprocess.run", side_effect=subprocess.CalledProcessError(1, "cwebp")):
            with self.assertRaises(subprocess.CalledProcessError):
                self.run_build()
        self.assertEqual(self.article.read_text(), markup)
        self.assertTrue((self.images / "missing.png").exists())
        self.assertEqual(list(self.cache.glob("*.webp")), [])

    def test_unsupported_raster_is_not_silently_published_as_webp(self):
        self.article.write_text('<div class="post"><img src="/assets/img/photo.heic"></div>')
        with self.assertRaisesRegex(ValueError, "Unsupported article image"):
            self.run_build()


@unittest.skipUnless(shutil.which("cwebp") and shutil.which("dwebp"), "WebP tools not installed")
class RealEncoderTests(unittest.TestCase):
    def test_mislabeled_avif_and_animation_safety(self):
        from PIL import Image

        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            site = root / "site"
            site.mkdir()
            (site / "index.html").write_text('<div class="post"><img src="/shot.jpg"></div>')
            Image.new("RGB", (4, 4), (12, 34, 56)).save(site / "shot.jpg", format="AVIF")
            with Image.open(site / "shot.jpg") as original:
                original_pixels = original.convert("RGBA").tobytes()
            build(site, root / "cache", jobs=2)
            self.assertFalse((site / "shot.jpg").exists())
            with Image.open(site / "shot.webp") as converted:
                self.assertEqual(original_pixels, converted.convert("RGBA").tobytes())
            Image.new("RGB", (4, 4), "red").save(site / "animation.gif", save_all=True,
                append_images=[Image.new("RGB", (4, 4), "blue")])
            (site / "index.html").write_text('<div class="post"><img src="/animation.gif"></div>')
            with self.assertRaisesRegex(ValueError, "must not be flattened"):
                build(site, root / "cache", jobs=2)

    def test_lossless_rgba_pixels_including_transparent_rgb(self):
        def chunk(kind, data):
            return struct.pack(">I", len(data)) + kind + data + struct.pack(">I", zlib.crc32(kind + data))

        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            site = root / "site"
            site.mkdir()
            (site / "index.html").write_text('<div class="post"><img src="/shot.png"></div>')
            pixels = bytes([255, 0, 0, 255, 12, 34, 56, 0, 0, 255, 0, 128, 0, 0, 255, 255])
            png = b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", 2, 2, 8, 6, 0, 0, 0))
            png += chunk(b"IDAT", zlib.compress(b"\0" + pixels[:8] + b"\0" + pixels[8:])) + chunk(b"IEND", b"")
            (site / "shot.png").write_bytes(png)
            result = build(site, root / "cache", jobs=2)
            self.assertEqual(result["converted"], 1)
            decoded = root / "decoded.pam"
            subprocess.run(["dwebp", str(site / "shot.webp"), "-pam", "-o", str(decoded)],
                           check=True, capture_output=True)
            self.assertEqual(decoded.read_bytes().split(b"ENDHDR\n", 1)[1], pixels)


if __name__ == "__main__":
    unittest.main()
