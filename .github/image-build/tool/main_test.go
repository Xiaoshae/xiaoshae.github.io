package main

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gen2brain/avif"
	"golang.org/x/image/webp"
)

func fixture(t *testing.T) options {
	t.Helper()
	root := t.TempDir()
	site := filepath.Join(root, "site")
	if err := os.MkdirAll(site, 0755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(site, "index.html"), nil)
	return options{site: site, cache: filepath.Join(root, "cache"), jobs: 2, origins: map[string]bool{"blog.example": true}}
}

func write(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func raster(t *testing.T, r uint8) (*image.NRGBA, []byte) {
	t.Helper()
	m := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	copy(m.Pix, []byte{r, 0, 0, 255, 12, 34, 56, 0, 0, 255, 0, 128, 0, 0, 255, 255})
	var b bytes.Buffer
	if err := png.Encode(&b, m); err != nil {
		t.Fatal(err)
	}
	return m, b.Bytes()
}

func TestConversionAndWarmCache(t *testing.T) {
	o := fixture(t)
	o.base = "prefix"
	index := filepath.Join(o.site, "index.html")
	post := filepath.Join(o.site, "posts", "one", "index.html")
	feed := filepath.Join(o.site, "feed.xml")
	picture := filepath.Join(o.site, "photos", "screen shot.PNG")
	markup := []byte(`<div class="entry post"><picture><source srcset="../../photos/screen%20shot.PNG 1x, https://external.example/else.png 2x"><img data-src="placeholder.png" SRC="https://blog.example/prefix/photos/screen%20shot.PNG?a=1&amp;b=2"></picture><a href="/prefix/photos/screen%20shot.PNG#zoom">full</a><script>const x = '<img src="../../photos/screen%20shot.PNG">';</script><pre>&lt;img src="sample.png"&gt;</pre></div>`)
	feedBody := []byte(`<rss><description>&lt;img src=&quot;/prefix/photos/screen%20shot.PNG&quot;&gt;</description><item><description><![CDATA[<img src="/prefix/photos/screen%20shot.PNG">]]></description></item></rss>`)
	for i, r := range []uint8{255, 255, 99} {
		original, data := raster(t, r)
		write(t, picture, data)
		write(t, post, markup)
		write(t, index, []byte(`<a href="/">Home</a><img src="/prefix/photos/screen%20shot.PNG"><img src="/prefix/photos/missing.png">`))
		write(t, feed, feedBody)
		if err := process(o); err != nil {
			t.Fatal(err)
		}
		entries, err := os.ReadDir(o.cache)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1+i/2 {
			t.Fatalf("build %d: %d entries", i, len(entries))
		}
		wantIndex := `<a href="/">Home</a><img src="/prefix/photos/screen%20shot.webp"><img src="/prefix/photos/missing.png">`
		if got := string(read(t, index)); got != wantIndex {
			t.Fatalf("listing markup changed unexpectedly: %s", got)
		}
		body := string(read(t, post))
		for _, want := range []string{`screen%20shot.webp 1x`, `screen%20shot.webp?a=1&amp;b=2`, `screen%20shot.webp#zoom`, `data-src="placeholder.png"`, `https://external.example/else.png`, `<img src="../../photos/screen%20shot.PNG">`, `&lt;img src="sample.png"&gt;`} {
			if !strings.Contains(body, want) {
				t.Errorf("missing %q in %s", want, body)
			}
		}
		if strings.Count(string(read(t, feed)), "screen%20shot.webp") != 2 {
			t.Fatal("feed references not rewritten")
		}
		out := filepath.Join(filepath.Dir(picture), "screen shot.webp")
		decoded, err := webp.Decode(bytes.NewReader(read(t, out)))
		if err != nil {
			t.Fatal(err)
		}
		if m, ok := decoded.(*image.NRGBA); !ok || !bytes.Equal(m.Pix, original.Pix) {
			t.Fatalf("lossless pixels differ: %v vs %v", decoded, original)
		}
		if _, err := os.Stat(picture); !os.IsNotExist(err) {
			t.Fatalf("original still present: %v", err)
		}
		if err := os.Remove(out); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCollisionAndMissingImage(t *testing.T) {
	o := fixture(t)
	index := filepath.Join(o.site, "index.html")
	_, data := raster(t, 10)
	write(t, filepath.Join(o.site, "same.png"), data)
	write(t, filepath.Join(o.site, "same.webp"), []byte("unreferenced existing asset"))
	original := []byte(`<div class="post"><img src="same.png"><img src="missing.png"></div>`)
	write(t, index, original)
	if err := process(o); !os.IsNotExist(err) && (err == nil || !strings.Contains(err.Error(), "missing.png")) {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(read(t, index), original) {
		t.Fatal("rewrote page after failure")
	}
	write(t, index, []byte(`<div class="post"><img src="same.png"><img src="vector.svg"></div>`))
	if err := process(o); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(read(t, index)), `src="same-`) || string(read(t, filepath.Join(o.site, "same.webp"))) != "unreferenced existing asset" {
		t.Fatal("existing WebP was not respected")
	}
}

func TestMislabeledAVIFAndAnimatedGIF(t *testing.T) {
	o := fixture(t)
	index := filepath.Join(o.site, "index.html")
	m, _ := raster(t, 10)
	var b bytes.Buffer
	if err := avif.Encode(&b, m, avif.Options{Lossless: true}); err != nil {
		t.Fatal(err)
	}
	original, err := avif.Decode(bytes.NewReader(b.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(o.site, "avif.jpg"), b.Bytes())
	write(t, index, []byte(`<div class="post"><img src="avif.jpg"></div>`))
	if err := process(o); err != nil {
		t.Fatal(err)
	}
	converted, err := webp.Decode(bytes.NewReader(read(t, filepath.Join(o.site, "avif.webp"))))
	if err != nil {
		t.Fatal(err)
	}
	for y := range 2 {
		for x := range 2 {
			if color.NRGBA64Model.Convert(original.At(x, y)) != color.NRGBA64Model.Convert(converted.At(x, y)) {
				t.Errorf("pixel %d,%d differs", x, y)
			}
		}
	}
	b.Reset()
	frame := image.NewPaletted(image.Rect(0, 0, 2, 2), color.Palette{color.Black, color.White})
	if err := gif.EncodeAll(&b, &gif.GIF{Image: []*image.Paletted{frame, frame}, Delay: []int{1, 1}}); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(o.site, "animated.gif"), b.Bytes())
	markup := []byte(`<div class="post"><img src="animated.gif"></div>`)
	write(t, index, markup)
	if err := process(o); err == nil || !strings.Contains(err.Error(), "must not be flattened") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(read(t, index), markup) {
		t.Fatal("rewrote page after failure")
	}
}

func TestInvalidInputs(t *testing.T) {
	o := fixture(t)
	for _, tc := range []struct{ path, message string }{
		{"/image.heic", "unsupported article image"},
		{"../outside.png", "escapes site directory"},
	} {
		write(t, filepath.Join(o.site, "index.html"), []byte(`<div class="post"><img src="`+tc.path+`"></div>`))
		if err := process(o); err == nil || !strings.Contains(err.Error(), tc.message) {
			t.Errorf("%s: %v", tc.path, err)
		}
	}
}
