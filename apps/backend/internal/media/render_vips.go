//go:build vips

package media

import (
	"log/slog"
	"sync"

	"github.com/davidbyttow/govips/v2/vips"
)

// The cgo path - libvips resamples with SIMD and shrink-on-load, and unlocks webp output for Accept-negotiated serving
const CanEncodeWebP = true

// Taller than any real image at ladder widths - the fit path bounds only width, and libvips thumbnail wants a finite box
const fitMaxHeight = 100_000

var vipsOnce sync.Once

// Startup is process-lifetime, so no Shutdown - ConcurrencyLevel 1 keeps threads-per-render flat because the Transformer semaphore already owns parallelism
func startVips() {
	vipsOnce.Do(func() {
		vips.LoggingSettings(func(domain string, _ vips.LogLevel, msg string) {
			slog.Warn("vips: "+msg, "domain", domain)
		}, vips.LogLevelWarning)
		vips.Startup(&vips.Config{ConcurrencyLevel: 1})
	})
}

// renderBytes decodes, resamples and encodes one derivative - unlike the pure path it applies EXIF orientation, so phone photos stop serving sideways
func renderBytes(raw []byte, w, h int, focalX, focalY float64, webp bool) ([]byte, string, error) {
	startVips()

	var img *vips.ImageRef
	var err error
	if h <= 0 {
		// Fit - thumbnail gets shrink-on-load and auto-rotates, SizeDown keeps the never-upscale contract
		img, err = vips.NewThumbnailWithSizeFromBuffer(raw, w, fitMaxHeight, vips.InterestingNone, vips.SizeDown)
		if err != nil {
			return nil, "", err
		}
	} else {
		// Cover - vips's built-in crops can't honour a focal point, so scale then extract by the shared geometry
		img, err = vips.NewImageFromBuffer(raw)
		if err != nil {
			return nil, "", err
		}
		if err := img.AutoRotate(); err != nil {
			img.Close()
			return nil, "", err
		}
		scale, outW, outH := coverBox(img.Width(), img.Height(), w, h)
		if scale < 1 {
			if err := img.Resize(scale, vips.KernelLanczos3); err != nil {
				img.Close()
				return nil, "", err
			}
		}
		// Resize rounds on its own terms, so re-read the dims and give up a pixel rather than extract out of bounds
		aw, ah := img.Width(), img.Height()
		outW = min(outW, aw)
		outH = min(outH, ah)
		x, y := cropOrigin(aw, ah, outW, outH, focalX, focalY)
		if err := img.ExtractArea(x, y, outW, outH); err != nil {
			img.Close()
			return nil, "", err
		}
	}
	defer img.Close()

	if webp {
		params := vips.NewWebpExportParams()
		params.Quality = 80
		params.StripMetadata = true
		data, _, err := img.ExportWebp(params)
		return data, "image/webp", err
	}
	// The non-webp variant mirrors the pure path's format split, with HasAlpha as a backstop so a transparent source can't flatten into jpeg
	if img.HasAlpha() || img.Format() == vips.ImageTypePNG || img.Format() == vips.ImageTypeGIF {
		params := vips.NewPngExportParams()
		params.StripMetadata = true
		data, _, err := img.ExportPng(params)
		return data, "image/png", err
	}
	params := vips.NewJpegExportParams()
	params.Quality = 82
	params.StripMetadata = true
	data, _, err := img.ExportJpeg(params)
	return data, "image/jpeg", err
}
