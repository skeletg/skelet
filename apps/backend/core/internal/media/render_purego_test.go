//go:build !vips

package media

import (
	"image"
	"image/color"
	"testing"
)

// Left half red, right half blue, so crop position is verifiable by sampling colors
func redBlue() image.Image {
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
	return img
}

func centerColor(img image.Image) (r, g, b uint32) {
	bo := img.Bounds()
	r, g, b, _ = img.At(bo.Min.X+bo.Dx()/2, bo.Min.Y+bo.Dy()/2).RGBA()
	return r >> 8, g >> 8, b >> 8
}

func TestScaleAndCropFocal(t *testing.T) {
	src := redBlue()

	// Fit: never upscale - 160 wide requested on a 64px source stays 64
	fit := scaleAndCrop(src, 160, 0, 0.5, 0.5)
	if fit.Bounds().Dx() != 64 {
		t.Fatalf("fit upscaled: %v", fit.Bounds())
	}

	// Fit downscale keeps aspect
	small := scaleAndCrop(src, 32, 0, 0.5, 0.5)
	if small.Bounds().Dx() != 32 || small.Bounds().Dy() != 16 {
		t.Fatalf("fit 32 = %v, want 32x16", small.Bounds())
	}

	// Cover crop with LEFT focal must be red at center
	left := scaleAndCrop(src, 32, 32, 0.1, 0.5)
	if left.Bounds().Dx() != 32 || left.Bounds().Dy() != 32 {
		t.Fatalf("cover dims %v, want 32x32", left.Bounds())
	}
	if r, _, b := centerColor(left); r < 200 || b > 50 {
		t.Fatalf("left-focal crop center = r%d b%d, want red", r, b)
	}

	// RIGHT focal must be blue
	right := scaleAndCrop(src, 32, 32, 0.9, 0.5)
	if r, _, b := centerColor(right); b < 200 || r > 50 {
		t.Fatalf("right-focal crop center = r%d b%d, want blue", r, b)
	}

	// Default (center) focal straddles the seam - just dimension-check
	center := scaleAndCrop(src, 32, 32, 0, 0)
	if center.Bounds().Dx() != 32 || center.Bounds().Dy() != 32 {
		t.Fatalf("center crop dims %v", center.Bounds())
	}
}

// The pure path must ignore a webp ask rather than emit something it can't encode
func TestRenderBytesIgnoresWebPAsk(t *testing.T) {
	data, ctype, err := renderBytes(pngBytes(t, 64, 32), 32, 0, 0.5, 0.5, true)
	if err != nil {
		t.Fatalf("renderBytes: %v", err)
	}
	if ctype != "image/png" {
		t.Fatalf("ctype = %q, want image/png", ctype)
	}
	if len(data) == 0 {
		t.Fatal("empty output")
	}
}
