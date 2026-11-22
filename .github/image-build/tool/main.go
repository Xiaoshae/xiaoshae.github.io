package main

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
)

//go:embed cwebp
var encoder []byte

type options struct {
	site, cache, base string
	origins           map[string]bool
	jobs              int
}

func within(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func process(o options) error {
	root, err := filepath.Abs(o.site)
	if err != nil {
		return err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(root, "index.html")); err != nil {
		return fmt.Errorf("not a built site: %w", err)
	}
	cache, err := filepath.Abs(o.cache)
	if err != nil {
		return err
	}
	if within(cache, root) {
		return errors.New("cache must be outside the published site")
	}
	if err := os.MkdirAll(cache, 0755); err != nil {
		return err
	}
	cache, err = filepath.EvalSymlinks(cache)
	if err != nil {
		return err
	}
	if within(cache, root) {
		return errors.New("cache must be outside the published site")
	}

	pages := map[string][]byte{}
	sources := map[string]string{}
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".html" {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("not a regular HTML file: %s", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		pages[path] = data
		_, err = editHTML(data, func(value, attr string, post bool) (string, error) {
			if !post || attr == "href" {
				return value, nil
			}
			source, err := o.resolve(value, path, root)
			if err != nil || source == "" {
				return value, err
			}
			switch strings.ToLower(filepath.Ext(source)) {
			case ".heic", ".heif":
				return value, fmt.Errorf("unsupported article image: %s", source)
			case ".png", ".jpg", ".jpeg", ".tif", ".tiff", ".webp", ".gif", ".bmp", ".avif":
				if _, exists := sources[source]; exists {
					return value, nil
				}
				info, err := os.Stat(source)
				if err != nil {
					return value, fmt.Errorf("article image in %s: %w", path, err)
				}
				if !info.Mode().IsRegular() {
					return value, fmt.Errorf("not a regular image: %s", source)
				}
				data, err := os.ReadFile(source)
				if err != nil {
					return value, err
				}
				sources[source] = filepath.Join(cache, fmt.Sprintf("%x.webp", sha256.Sum256(data)))
			}
			return value, nil
		})
		return err
	})
	if err != nil {
		return err
	}
	if len(sources) == 0 {
		fmt.Println("No local article raster images found.")
		return nil
	}

	paths := make([]string, 0, len(sources))
	for path := range sources {
		paths = append(paths, path)
	}
	slices.Sort(paths)
	out, err := destinations(paths, root)
	if err != nil {
		return err
	}
	missing := map[string]string{}
	for _, source := range paths {
		target := sources[source]
		info, err := os.Lstat(target)
		if errors.Is(err, fs.ErrNotExist) {
			missing[target] = source
			continue
		}
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Size() == 0 {
			return fmt.Errorf("invalid cache entry: %s", target)
		}
	}
	if len(missing) > 0 {
		temp, err := os.MkdirTemp("", "article-webp-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(temp)
		binary := filepath.Join(temp, "cwebp")
		if err := os.WriteFile(binary, encoder, 0700); err != nil {
			return err
		}
		if err := convertAll(binary, missing, min(o.jobs, len(missing))); err != nil {
			return err
		}
	}

	// Prepare all reference updates before changing the published files.
	changed := map[string][]byte{}
	for page, data := range pages {
		updated, err := editHTML(data, o.rewriter(page, root, out))
		if err != nil {
			return err
		}
		if !bytes.Equal(data, updated) {
			changed[page] = updated
		}
	}
	feed := filepath.Join(root, "feed.xml")
	if info, err := os.Lstat(feed); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("not a regular feed file: %s", feed)
		}
		data, err := os.ReadFile(feed)
		if err != nil {
			return err
		}
		updated, err := editFeed(data, o.rewriter(feed, root, out))
		if err != nil {
			return err
		}
		if !bytes.Equal(data, updated) {
			changed[feed] = updated
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	for source, dest := range out {
		data, err := os.ReadFile(sources[source])
		if err != nil {
			return err
		}
		if err := os.WriteFile(dest, data, 0644); err != nil {
			return err
		}
	}
	for path, data := range changed {
		if err := os.WriteFile(path, data, 0644); err != nil {
			return err
		}
	}
	for source, dest := range out {
		if source != dest {
			if err := os.Remove(source); err != nil {
				return err
			}
		}
	}
	fmt.Printf("Article images: %d, cached: %d, encoded: %d\n", len(sources), len(sources)-len(missing), len(missing))
	return nil
}

func destinations(paths []string, root string) (map[string]string, error) {
	counts, reserved := map[string]int{}, map[string]bool{}
	for _, source := range paths {
		target := strings.TrimSuffix(source, filepath.Ext(source)) + ".webp"
		counts[target]++
		reserved[source], reserved[target] = true, true
	}
	out := map[string]string{}
	for _, source := range paths {
		stem := strings.TrimSuffix(source, filepath.Ext(source))
		target := stem + ".webp"
		if target != source {
			_, err := os.Lstat(target)
			if err != nil && !errors.Is(err, fs.ErrNotExist) {
				return nil, err
			}
			if counts[target] > 1 || err == nil {
				rel, err := filepath.Rel(root, source)
				if err != nil {
					return nil, err
				}
				sum := sha256.Sum256([]byte(rel))
				prefix := fmt.Sprintf("%s-%x", stem, sum[:6])
				for i := 0; ; i++ {
					target = prefix + ".webp"
					if i != 0 {
						target = fmt.Sprintf("%s-%d.webp", prefix, i)
					}
					if reserved[target] {
						continue
					}
					if _, err := os.Lstat(target); errors.Is(err, fs.ErrNotExist) {
						break
					} else if err != nil {
						return nil, err
					}
				}
			}
		}
		reserved[target], out[source] = true, target
	}
	return out, nil
}

func convertAll(binary string, missing map[string]string, workers int) error {
	type job struct{ source, target string }
	queue := make(chan job, len(missing))
	for target, source := range missing {
		queue <- job{source, target}
	}
	close(queue)
	var wg sync.WaitGroup
	var once sync.Once
	var first error
	for range workers {
		wg.Go(func() {
			for j := range queue {
				if err := encode(binary, j.source, j.target); err != nil {
					once.Do(func() { first = fmt.Errorf("%s: %w", j.source, err) })
					return
				}
			}
		})
	}
	wg.Wait()
	return first
}

func main() {
	o := options{origins: map[string]bool{}}
	flag.StringVar(&o.site, "site", "public", "rendered site directory")
	flag.StringVar(&o.cache, "cache", ".cache/article-webp", "cache directory (dedicated to this version of the tool)")
	flag.StringVar(&o.base, "baseurl", "", "base path for absolute site URLs")
	flag.IntVar(&o.jobs, "jobs", min(runtime.GOMAXPROCS(0), 8), "concurrent encoders")
	flag.Func("site-url", "local site origin; may be repeated", func(origin string) error { return addOrigin(o.origins, origin) })
	flag.Parse()
	o.base = strings.Trim(o.base, "/")
	if flag.NArg() != 0 || o.jobs < 1 {
		flag.Usage()
		os.Exit(2)
	}
	if err := process(o); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
