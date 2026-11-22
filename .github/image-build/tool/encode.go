package main

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	_ "image/jpeg"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/gen2brain/avif"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

var animated = errors.New("animated/multi-frame image must not be flattened")
var encoderArgs = []string{"-lossless", "-exact", "-q", "75", "-m", "4", "-metadata", "icc"}

func encode(binaryPath, source, target string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	decoded, force, err := checkImage(data)
	if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(filepath.Dir(target), ".encode-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	result := filepath.Join(tmp, "encoded.webp")
	run := func(path string) error {
		args := append(append([]string{}, encoderArgs...), path, "-o", result)
		output, err := exec.Command(binaryPath, args...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("cwebp: %w: %s", err, output)
		}
		return nil
	}
	if force || run(source) != nil {
		if decoded == nil {
			decoded, _, err = image.Decode(bytes.NewReader(data))
			if err != nil {
				return fmt.Errorf("cannot decode the source image: %w", err)
			}
		}
		normalized := filepath.Join(tmp, "decoded.png")
		var buf bytes.Buffer
		if err := png.Encode(&buf, decoded); err != nil {
			return err
		}
		payload := buf.Bytes()
		if chunk := colorProfileChunk(data); len(chunk) != 0 {
			payload = bytes.Join([][]byte{payload[:33], chunk, payload[33:]}, nil)
		}
		if err := os.WriteFile(normalized, payload, 0600); err != nil {
			return err
		}
		if err := run(normalized); err != nil {
			return err
		}
	}
	if info, err := os.Stat(result); err != nil || info.Size() == 0 {
		return errors.New("encoder produced no image")
	}
	return os.Rename(result, target)
}

func checkImage(data []byte) (decoded image.Image, force bool, err error) {
	switch {
	case bytes.HasPrefix(data, []byte("GIF87a")) || bytes.HasPrefix(data, []byte("GIF89a")):
		frames, err := gif.DecodeAll(bytes.NewReader(data))
		if err != nil {
			return nil, false, err
		}
		if len(frames.Image) != 1 {
			return nil, false, animated
		}
		frame := frames.Image[0]
		bounds := image.Rect(0, 0, frames.Config.Width, frames.Config.Height)
		if frame.Bounds() == bounds {
			return frame, true, nil
		}
		canvas := image.NewNRGBA(bounds)
		background, _ := frames.Config.ColorModel.(color.Palette)
		if i := int(frames.BackgroundIndex); i < len(background) {
			transparent := false
			for _, entry := range frame.Palette {
				if _, _, _, alpha := entry.RGBA(); alpha == 0 {
					transparent = true
					break
				}
			}
			if !transparent {
				draw.Draw(canvas, bounds, image.NewUniform(background[i]), image.Point{}, draw.Src)
			}
		}
		draw.Draw(canvas, frame.Bounds(), frame, frame.Bounds().Min, draw.Src)
		return canvas, true, nil
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		if frames := pngChunk(data, "acTL"); len(frames) >= 4 && binary.BigEndian.Uint32(frames) > 1 {
			return nil, false, animated
		}
	case len(data) >= 16 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		for offset := 12; offset+8 <= len(data); {
			id := string(data[offset : offset+4])
			size := int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
			if size > len(data)-offset-8 {
				break
			}
			if id == "ANIM" || id == "ANMF" || (id == "VP8X" && size > 0 && data[offset+8]&2 != 0) {
				return nil, false, animated
			}
			offset += 8 + size + size%2
		}
	case avifFile(data):
		frames, err := avif.DecodeAll(bytes.NewReader(data))
		if err != nil {
			return nil, false, err
		}
		if len(frames.Image) != 1 {
			return nil, false, animated
		}
		return frames.Image[0], true, nil
	default:
		_, _, more := tiffDirectory(data)
		if more {
			return nil, false, animated
		}
	}
	return nil, false, nil
}

func avifFile(data []byte) bool {
	if len(data) < 16 || string(data[4:8]) != "ftyp" {
		return false
	}
	end := int(binary.BigEndian.Uint32(data[:4]))
	if end > len(data) {
		return false
	}
	for i := 8; i+4 <= end; i += 4 {
		if i != 12 && (string(data[i:i+4]) == "avif" || string(data[i:i+4]) == "avis") {
			return true
		}
	}
	return false
}

func pngChunk(data []byte, key string) []byte {
	if !bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")) {
		return nil
	}
	for offset := 8; offset+12 <= len(data); {
		size := int(binary.BigEndian.Uint32(data[offset : offset+4]))
		if size > len(data)-offset-12 {
			break
		}
		if string(data[offset+4:offset+8]) == key {
			return data[offset+8 : offset+8+size]
		}
		offset += 12 + size
	}
	return nil
}

func tiffDirectory(data []byte) (binary.ByteOrder, []byte, bool) {
	if len(data) < 8 {
		return nil, nil, false
	}
	var order binary.ByteOrder
	switch string(data[:4]) {
	case "II\x2a\x00":
		order = binary.LittleEndian
	case "MM\x00\x2a":
		order = binary.BigEndian
	default:
		return nil, nil, false
	}
	offset := int(order.Uint32(data[4:8]))
	if offset > len(data)-2 {
		return nil, nil, false
	}
	next := offset + 2 + int(order.Uint16(data[offset:offset+2]))*12
	if next > len(data)-4 {
		return nil, nil, false
	}
	return order, data[offset+2 : next], order.Uint32(data[next:next+4]) != 0
}

func colorProfileChunk(data []byte) []byte {
	if chunk := pngChunk(data, "iCCP"); len(chunk) != 0 {
		return makeICCChunk(chunk)
	}
	var profile []byte
	if avifFile(data) {
		// ICC color properties are encoded as a colr box with type prof or rICC.
		for start := 4; start < len(data); {
			index := bytes.Index(data[start:], []byte("colr"))
			if index < 0 {
				break
			}
			i := start + index
			start = i + 4
			if i+8 > len(data) {
				break
			}
			size := int(binary.BigEndian.Uint32(data[i-4 : i]))
			if size >= 12 && size <= len(data)-i+4 && (string(data[i+4:i+8]) == "prof" || string(data[i+4:i+8]) == "rICC") {
				profile = data[i+8 : i-4+size]
				break
			}
		}
	} else if order, entries, _ := tiffDirectory(data); order != nil {
		for entry := range len(entries) / 12 {
			e := entries[entry*12 : entry*12+12]
			if order.Uint16(e[:2]) != 34675 {
				continue
			}
			size, offset := int(order.Uint32(e[4:8])), int(order.Uint32(e[8:12]))
			if offset <= len(data) && size <= len(data)-offset {
				profile = data[offset : offset+size]
			}
			break
		}
	} else if len(data) >= 138 && string(data[:2]) == "BM" && binary.LittleEndian.Uint32(data[14:18]) >= 124 &&
		binary.LittleEndian.Uint32(data[70:74]) == 0x4d424544 {
		offset, size := 14+int(binary.LittleEndian.Uint32(data[126:130])), int(binary.LittleEndian.Uint32(data[130:134]))
		if offset <= len(data) && size <= len(data)-offset {
			profile = data[offset : offset+size]
		}
	}
	if len(profile) < 128 || string(profile[36:40]) != "acsp" {
		return nil
	}
	var encoded bytes.Buffer
	encoded.WriteString("ICC\x00\x00")
	w := zlib.NewWriter(&encoded)
	_, _ = w.Write(profile)
	_ = w.Close()
	return makeICCChunk(encoded.Bytes())
}

func makeICCChunk(payload []byte) []byte {
	chunk := binary.BigEndian.AppendUint32(nil, uint32(len(payload)))
	chunk = append(chunk, "iCCP"...)
	chunk = append(chunk, payload...)
	return binary.BigEndian.AppendUint32(chunk, crc32.ChecksumIEEE(chunk[4:]))
}
