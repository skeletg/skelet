//go:build !vips

package media

import (
	"bytes"
	"image"
	"image/jpeg"
	"image/png"
	"math"

	"golang.org/x/image/draw"
	"golang.org/x/image/webp"
)

// The no-cgo fallback - decodes webp but can only encode JPEG, or PNG for alpha, since pure-Go webp encoding isn't worth the risk yet
const CanEncodeWebP = false

// renderBytes decodes, resamples and encodes one derivative - webp is ignored here, Transform already clamped it on CanEncodeWebP
func renderBytes(raw []byte, w, h int, focalX, focalY float64, _ bool) ([]byte, string, error) {
	src, format, err := decodeImage(raw)
	if err != nil {
		return nil, "", err
	}

	dst := scaleAndCrop(src, w, h, focalX, focalY)

	var buf bytes.Buffer
	ctype := "image/jpeg"
	if format == "png" || format == "gif" {
		// Keep alpha-capable sources as PNG - gif frames flatten
		ctype = "image/png"
		if err := png.Encode(&buf, dst); err != nil {
			return nil, "", err
		}
	} else {
		if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 82}); err != nil {
			return nil, "", err
		}
	}
	return buf.Bytes(), ctype, nil
}

func decodeImage(raw []byte) (image.Image, string, error) {
	if img, err := webp.Decode(bytes.NewReader(raw)); err == nil {
		return img, "webp", nil
	}
	img, format, err := image.Decode(bytes.NewReader(raw))
	return img, format, err
}

// Fit when h == 0 and focal cover-crop when h > 0, never upscaling
func scaleAndCrop(src image.Image, w, h int, focalX, focalY float64) image.Image {
	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	if sw <= 0 || sh <= 0 {
		return src
	}

	if h <= 0 {
		// Fit to width, keeping aspect
		if w >= sw {
			return src
		}
		th := max(int(math.Round(float64(sh)*float64(w)/float64(sw))), 1)
		dst := image.NewRGBA(image.Rect(0, 0, w, th))
		draw.CatmullRom.Scale(dst, dst.Bounds(), src, sb, draw.Over, nil)
		return dst
	}

	scale, outW, outH := coverBox(sw, sh, w, h)
	scaledW := int(math.Round(float64(sw) * scale))
	scaledH := int(math.Round(float64(sh) * scale))
	scaled := image.NewRGBA(image.Rect(0, 0, scaledW, scaledH))
	draw.CatmullRom.Scale(scaled, scaled.Bounds(), src, sb, draw.Over, nil)

	x, y := cropOrigin(scaledW, scaledH, outW, outH, focalX, focalY)
	dst := image.NewRGBA(image.Rect(0, 0, outW, outH))
	draw.Draw(dst, dst.Bounds(), scaled, image.Pt(x, y), draw.Src)
	return dst
}
