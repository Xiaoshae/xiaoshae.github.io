package main

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"testing"
)

func TestStaticGIFPreservesCanvas(t *testing.T) {
	palette := color.Palette{color.Black, color.White}
	frame := image.NewPaletted(image.Rect(1, 1, 2, 2), palette)
	frame.SetColorIndex(1, 1, 1)
	var data bytes.Buffer
	if err := gif.EncodeAll(&data, &gif.GIF{
		Image: []*image.Paletted{frame}, Delay: []int{0},
		Config: image.Config{ColorModel: palette, Width: 3, Height: 3},
	}); err != nil {
		t.Fatal(err)
	}
	m, fallback, err := checkImage(data.Bytes())
	if err != nil || !fallback {
		t.Fatalf("GIF decoding failed: %v", err)
	}
	if m.Bounds() != image.Rect(0, 0, 3, 3) || color.GrayModel.Convert(m.At(1, 1)) != (color.Gray{Y: 255}) {
		t.Fatalf("incorrect canvas: %s", m.Bounds())
	}
}
