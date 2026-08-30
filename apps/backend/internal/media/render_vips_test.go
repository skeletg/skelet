//go:build vips

package media

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	"golang.org/x/image/webp"
)

// Left half red, right half blue as PNG bytes - crop position verifiable by sampling the decoded output
func redBluePNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 64, 32))
	for y := range 32 {
		for x := range 64 {
			c := color.RGBA{R: 255, A: 255}
			if x >= 32 {
				c = color.RGBA{B: 255, A: 255}
			}
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode: %v", err)
	}
	return buf.Bytes()
}

func jpegBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h)), nil); err != nil {
		t.Fatalf("encode: %v", err)
	}
	return buf.Bytes()
}

func TestVipsRenderFitWebP(t *testing.T) {
	data, ctype, err := renderBytes(jpegBytes(t, 64, 32), 32, 0, 0.5, 0.5, true)
	if err != nil {
		t.Fatalf("renderBytes: %v", err)
	}
	if ctype != "image/webp" {
		t.Fatalf("ctype = %q, want image/webp", ctype)
	}
	cfg, err := webp.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("output isn't webp: %v", err)
	}
	if cfg.Width != 32 || cfg.Height != 16 {
		t.Fatalf("fit dims %dx%d, want 32x16", cfg.Width, cfg.Height)
	}
}

func TestVipsRenderNeverUpscales(t *testing.T) {
	data, _, err := renderBytes(jpegBytes(t, 64, 32), 160, 0, 0.5, 0.5, true)
	if err != nil {
		t.Fatalf("renderBytes: %v", err)
	}
	cfg, err := webp.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if cfg.Width != 64 {
		t.Fatalf("fit upscaled to %d, want 64", cfg.Width)
	}
}

func TestVipsRenderCoverFocal(t *testing.T) {
	src := redBluePNG(t)

	sample := func(raw []byte) (r, g, b uint32) {
		img, err := webp.Decode(bytes.NewReader(raw))
		if err != nil {
			t.Fatalf("decode webp: %v", err)
		}
		bo := img.Bounds()
		if bo.Dx() != 32 || bo.Dy() != 32 {
			t.Fatalf("cover dims %v, want 32x32", bo)
		}
		r, g, b, _ = img.At(bo.Min.X+16, bo.Min.Y+16).RGBA()
		return r >> 8, g >> 8, b >> 8
	}

	left, _, err := renderBytes(src, 32, 32, 0.1, 0.5, true)
	if err != nil {
		t.Fatalf("renderBytes: %v", err)
	}
	if r, _, b := sample(left); r < 200 || b > 50 {
		t.Fatalf("left-focal crop center = r%d b%d, want red", r, b)
	}

	right, _, err := renderBytes(src, 32, 32, 0.9, 0.5, true)
	if err != nil {
		t.Fatalf("renderBytes: %v", err)
	}
	if r, _, b := sample(right); b < 200 || r > 50 {
		t.Fatalf("right-focal crop center = r%d b%d, want blue", r, b)
	}
}

// The non-webp variant keeps the pure path's format split so non-webp clients see no change
func TestVipsRenderLegacyFormats(t *testing.T) {
	if _, ctype, err := renderBytes(jpegBytes(t, 64, 32), 32, 0, 0.5, 0.5, false); err != nil || ctype != "image/jpeg" {
		t.Fatalf("jpeg source = %q, %v, want image/jpeg", ctype, err)
	}
	if _, ctype, err := renderBytes(redBluePNG(t), 32, 0, 0.5, 0.5, false); err != nil || ctype != "image/png" {
		t.Fatalf("png source = %q, %v, want image/png", ctype, err)
	}
}
