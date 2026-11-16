package documents

import (
	"bytes"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"

	"github.com/go-pdf/fpdf"
	"golang.org/x/image/draw"
	"golang.org/x/image/webp"
)

const (
	logoHeight    = 14.0 // mm
	logoMaxWidth  = 60.0
	logoMaxPx     = 800
	logoMaxPixels = 40_000_000 // a few kilobytes can claim a picture that decodes to gigabytes, so the size is read first
)

// logoPNG decodes the logo and hands fpdf a PNG it can embed, scaled down so a camera-sized upload doesn't bloat every
// document - false when it isn't an image fpdf could print, an SVG say
func logoPNG(raw []byte) ([]byte, image.Point, bool) {
	if len(raw) == 0 {
		return nil, image.Point{}, false
	}
	cfg, err := webp.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		cfg, _, err = image.DecodeConfig(bytes.NewReader(raw))
	}
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > logoMaxPixels {
		return nil, image.Point{}, false
	}
	img, err := webp.Decode(bytes.NewReader(raw))
	if err != nil {
		if img, _, err = image.Decode(bytes.NewReader(raw)); err != nil {
			return nil, image.Point{}, false
		}
	}
	size := img.Bounds().Size()
	if size.X <= 0 || size.Y <= 0 {
		return nil, image.Point{}, false
	}
	if size.X > logoMaxPx {
		scaled := image.NewRGBA(image.Rect(0, 0, logoMaxPx, max(size.Y*logoMaxPx/size.X, 1)))
		draw.CatmullRom.Scale(scaled, scaled.Bounds(), img, img.Bounds(), draw.Over, nil)
		img, size = scaled, scaled.Bounds().Size()
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, image.Point{}, false
	}
	return buf.Bytes(), size, true
}

func (d doc) logo() {
	raw, size, ok := d.in.Store.Logo, d.in.Store.LogoSize, len(d.in.Store.Logo) > 0
	if size == (image.Point{}) {
		raw, size, ok = logoPNG(raw)
	}
	if !ok {
		return
	}
	h := logoHeight
	w := h * float64(size.X) / float64(size.Y)
	if w > logoMaxWidth {
		w, h = logoMaxWidth, logoMaxWidth*float64(size.Y)/float64(size.X)
	}
	opts := fpdf.ImageOptions{ImageType: "PNG"}
	d.pdf.RegisterImageOptionsReader("logo", opts, bytes.NewReader(raw))
	x, y := d.pdf.GetXY()
	d.pdf.ImageOptions("logo", x, y, w, h, false, opts, 0, "")
	d.pdf.SetY(y + h + 3)
}
