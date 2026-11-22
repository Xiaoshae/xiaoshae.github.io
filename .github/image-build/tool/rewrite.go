package main

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"io/fs"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"

	xhtml "golang.org/x/net/html"
)

type transform func(value, attr string, post bool) (string, error)

// Match complete attributes rather than substring matches such as data-src.
var attribute = regexp.MustCompile(`([^\s"'<>/=]+)(\s*=\s*)(?:"([^"]*)"|'([^']*)'|([^\s>]+))`)

func addOrigin(origins map[string]bool, value string) error {
	u, err := url.Parse(value)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("invalid site URL: %s", value)
	}
	origins[strings.ToLower(u.Host)] = true
	return nil
}

func (o options) resolve(value, page, root string) (string, error) {
	u, err := url.Parse(value)
	if err != nil {
		return "", fmt.Errorf("invalid image URL %q: %w", value, err)
	}
	if (u.Scheme != "" && !strings.EqualFold(u.Scheme, "http") && !strings.EqualFold(u.Scheme, "https")) ||
		(u.Host != "" && !o.origins[strings.ToLower(u.Host)]) || u.Path == "" {
		return "", nil
	}
	var name string
	if strings.HasPrefix(u.Path, "/") {
		path := strings.TrimPrefix(u.Path, "/")
		if o.base != "" {
			var ok bool
			path, ok = strings.CutPrefix(path, o.base+"/")
			if !ok {
				return "", nil
			}
		}
		name = filepath.Join(root, filepath.FromSlash(path))
	} else {
		name = filepath.Join(filepath.Dir(page), filepath.FromSlash(u.Path))
	}
	if !within(name, root) {
		return "", fmt.Errorf("image escapes site directory: %s", value)
	}
	if resolved, err := filepath.EvalSymlinks(name); err == nil {
		if !within(resolved, root) {
			return "", fmt.Errorf("image escapes site directory: %s", value)
		}
		name = resolved
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	return name, nil
}

func (o options) rewriter(page, root string, out map[string]string) transform {
	return func(value, _ string, _ bool) (string, error) {
		source, err := o.resolve(value, page, root)
		target, ok := out[source]
		if err != nil || !ok || source == target {
			return value, err
		}
		u, _ := url.Parse(value)
		escaped := u.EscapedPath()
		u.RawPath = escaped[:strings.LastIndexByte(escaped, '/')+1] + url.PathEscape(filepath.Base(target))
		u.Path, err = url.PathUnescape(u.RawPath)
		if err != nil {
			return value, err
		}
		return u.String(), nil
	}
}

func mapSrcset(value string, change func(string) (string, error)) (string, error) {
	// The original pipeline leaves data-URL sets alone, as commas are ambiguous.
	if strings.Contains(strings.ToLower(value), "data:") {
		return value, nil
	}
	parts := strings.Split(value, ",")
	changed := false
	for i, part := range parts {
		fields := strings.Fields(part)
		if len(fields) == 0 {
			continue
		}
		updated, err := change(fields[0])
		if err != nil {
			return value, err
		}
		changed = changed || updated != fields[0]
		fields[0] = updated
		parts[i] = strings.Join(fields, " ")
	}
	if !changed {
		return value, nil
	}
	return strings.Join(parts, ", "), nil
}

func editHTML(data []byte, change transform) ([]byte, error) {
	z := xhtml.NewTokenizer(bytes.NewReader(data))
	var out bytes.Buffer
	depth := 0
	for {
		kind := z.Next()
		if kind == xhtml.ErrorToken {
			if errors.Is(z.Err(), io.EOF) {
				out.Write(z.Raw())
				return out.Bytes(), nil
			}
			return nil, z.Err()
		}
		// Token() normalizes the tokenizer's buffer, so copy Raw() first for tags.
		switch kind {
		case xhtml.StartTagToken, xhtml.SelfClosingTagToken, xhtml.EndTagToken:
			raw := bytes.Clone(z.Raw())
			token := z.Token()
			if token.Data == "div" {
				if kind == xhtml.EndTagToken && depth > 0 {
					depth--
				}
				if kind == xhtml.StartTagToken {
					if depth > 0 {
						depth++
					} else {
						for _, a := range token.Attr {
							if a.Key == "class" && strings.Contains(" "+strings.Join(strings.Fields(a.Val), " ")+" ", " post ") {
								depth = 1
								break
							}
						}
					}
				}
			}
			if kind == xhtml.EndTagToken || (token.Data != "a" && token.Data != "img" && token.Data != "source") {
				out.Write(raw)
				continue
			}
			cursor, base := 0, len(token.Data)+1
			for _, m := range attribute.FindAllSubmatchIndex(raw[base:], -1) {
				key := strings.ToLower(string(raw[base+m[2] : base+m[3]]))
				if !((token.Data == "img" && (key == "src" || key == "srcset")) || (token.Data == "source" && key == "srcset") || (token.Data == "a" && key == "href")) {
					continue
				}
				var value string
				for group := 6; group <= 10; group += 2 {
					if m[group] >= 0 {
						value = html.UnescapeString(string(raw[base+m[group] : base+m[group+1]]))
						break
					}
				}
				apply := func(v string) (string, error) { return change(v, key, depth > 0) }
				var updated string
				var err error
				if key == "srcset" {
					updated, err = mapSrcset(value, apply)
				} else {
					updated, err = apply(value)
				}
				if err != nil {
					return nil, err
				}
				if updated == value {
					continue
				}
				out.Write(raw[cursor : base+m[5]])
				out.WriteString(`"` + html.EscapeString(updated) + `"`)
				cursor = base + m[1]
			}
			out.Write(raw[cursor:])
		default:
			out.Write(z.Raw())
		}
	}
}

func editFeed(data []byte, change transform) ([]byte, error) {
	var out bytes.Buffer
	decoder := xml.NewDecoder(bytes.NewReader(data))
	cursor, depth, target := 0, 0, 0
	for {
		start := int(decoder.InputOffset())
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		end := int(decoder.InputOffset())
		switch tok := token.(type) {
		case xml.StartElement:
			depth++
			if target == 0 && (tok.Name.Local == "description" || tok.Name.Local == "summary" || tok.Name.Local == "content" || tok.Name.Local == "encoded") {
				target = depth
			}
		case xml.EndElement:
			if depth == target {
				target = 0
			}
			depth--
		case xml.CharData:
			if target == 0 || target != depth || !bytes.Contains(tok, []byte("<")) {
				continue
			}
			updated, err := editHTML(tok, change)
			if err != nil {
				return nil, err
			}
			if bytes.Equal(tok, updated) {
				continue
			}
			out.Write(data[cursor:start])
			if bytes.HasPrefix(data[start:end], []byte("<![CDATA[")) {
				out.WriteString("<![CDATA[" + strings.ReplaceAll(string(updated), "]]>", "]]]]><![CDATA[>") + "]]>")
			} else if err := xml.EscapeText(&out, updated); err != nil {
				return nil, err
			}
			cursor = end
		}
	}
	out.Write(data[cursor:])
	return out.Bytes(), nil
}
