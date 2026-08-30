package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"math"
	"runtime"
	"sort"
	"time"

	// decodeConfig sniffs headers through image.DecodeConfig, so the formats must be registered in every build
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"golang.org/x/image/webp"
	"golang.org/x/sync/singleflight"
)

// Image derivatives - fit (resized) and cover (focal-cropped) variants built on first request and cached back under derived/<spec>/<key>, so later hits are a plain storage read
// The decode/resample/encode core is build-selected (render_vips.go behind -tags vips, render_purego.go otherwise) - libvips adds webp output and SIMD speed but costs cgo, so host tooling and untagged CI keep the pure path

// The fixed ladder - requests snap UP to the nearest step so arbitrary ?w= values can't create unbounded cache entries
var AllowedWidths = []int{160, 320, 480, 640, 960, 1280, 1600, 2048}

func SnapWidth(w int) int {
	if w <= 0 {
		return AllowedWidths[0]
	}
	i := sort.SearchInts(AllowedWidths, w)
	if i >= len(AllowedWidths) {
		return AllowedWidths[len(AllowedWidths)-1]
	}
	return AllowedWidths[i]
}

// Uploads reach storage through presigned PUT, so they skip the server's body cap - the source can be any size, and decode allocates roughly w*h*4 bytes regardless of output size
// Without a cap, one 10000x10000 upload across the width ladder can OOM the process
const (
	defaultMaxSourcePixels = 40_000_000 // ~40 MP, i.e. 8000x5000
	defaultRenderWait      = 3 * time.Second
)

// The original is over the pixel cap, so there's no derivative - callers 422 rather than retry
var ErrSourceTooLarge = errors.New("media: source image exceeds the pixel cap")

// Every render slot taken and the wait elapsed - callers 503 with Retry-After
var ErrBusy = errors.New("media: image render capacity saturated")

type Transformer struct {
	storage         *Storage
	maxSourcePixels int
	// Bounds concurrent decode and resample, the only CPU-heavy work here - callers WAIT for a slot and only fail once renderWait elapses, so a burst queues instead of stampeding
	sem        chan struct{}
	renderWait time.Duration
	// Folds concurrent misses on the same derivative into one render - a cache purge or crawler sweep would otherwise resample it per request
	inflight singleflight.Group
}

func NewTransformer(s *Storage) *Transformer {
	// Half the CPUs - renders share the box with request serving, and CatmullRom eats whatever it's given
	slots := max(runtime.GOMAXPROCS(0)/2, 1)
	return &Transformer{
		storage:         s,
		maxSourcePixels: defaultMaxSourcePixels,
		sem:             make(chan struct{}, slots),
		renderWait:      defaultRenderWait,
	}
}

func (t *Transformer) SetMaxSourcePixels(n int) {
	if n > 0 {
		t.maxSourcePixels = n
	}
}

// How many decodes run at once and how long a caller queues before ErrBusy - zeros keep the defaults
func (t *Transformer) SetRenderConcurrency(slots int, wait time.Duration) {
	if slots > 0 {
		t.sem = make(chan struct{}, slots)
	}
	if wait > 0 {
		t.renderWait = wait
	}
}

// The object key a (spec, original) pair caches under - exported so a caller can build a warm derivative's public URL without a render
// webp derivatives live in their own keyspace so a jpeg cached before the vips rollout can't mask the upgrade, and both formats can coexist for Accept-negotiated serving
func DerivedKey(key string, w, h int, webp bool) string {
	spec := fmt.Sprintf("w%d", w)
	if h > 0 {
		spec = fmt.Sprintf("w%dh%d", w, h)
	}
	if webp {
		spec += "-webp"
	}
	return "derived/" + spec + "/" + key
}

func SnapSpec(w, h int) (int, int) {
	w = SnapWidth(w)
	if h > 0 {
		h = SnapWidth(h)
	}
	return w, h
}

// A ready derivative's public URL, so a caller can redirect instead of proxying bytes - empty with no public base, since the internal endpoint isn't browser-reachable, or when the derivative isn't built yet
func (t *Transformer) WarmURL(ctx context.Context, key string, w, h int, webp bool) string {
	if !t.storage.HasPublicBase() {
		return ""
	}
	w, h = SnapSpec(w, h)
	dkey := DerivedKey(key, w, h, webp && CanEncodeWebP)
	if _, _, ok, _ := t.storage.Head(ctx, dkey); !ok {
		return ""
	}
	return t.storage.PublicURL(dkey)
}

func (t *Transformer) acquire(ctx context.Context) error {
	timer := time.NewTimer(t.renderWait)
	defer timer.Stop()
	select {
	case t.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return ErrBusy
	}
}

func (t *Transformer) release() { <-t.sem }

// Derivative bytes and content type for an original key - w snaps to the ladder, h is optional, and when set the output is a w x h cover-crop around the focal point, where 0.5,0.5 is centre - Never upscales
// webp asks for webp output - honored only in vips builds, so a pure build quietly serves the jpeg/png variant instead
func (t *Transformer) Transform(ctx context.Context, key string, w, h int, focalX, focalY float64, webp bool) ([]byte, string, error) {
	webp = webp && CanEncodeWebP
	w, h = SnapSpec(w, h)
	dkey := DerivedKey(key, w, h, webp)
	if data, ctype, ok := t.cached(ctx, dkey); ok {
		return data, ctype, nil
	}

	// One render per derivative no matter how many raced in - waiters share the leader's result, error included, and a cancelled leader makes them retry rather than pile on
	type rendered struct {
		data  []byte
		ctype string
	}
	v, err, _ := t.inflight.Do(dkey, func() (any, error) {
		if data, ctype, ok := t.cached(ctx, dkey); ok {
			return rendered{data, ctype}, nil
		}
		if err := t.acquire(ctx); err != nil {
			return nil, err
		}
		defer t.release()
		data, ctype, err := t.render(ctx, key, dkey, w, h, focalX, focalY, webp)
		if err != nil {
			return nil, err
		}
		return rendered{data, ctype}, nil
	})
	if err != nil {
		return nil, "", err
	}
	out := v.(rendered)
	return out.data, out.ctype, nil
}

// Fetches a ready derivative head-then-get, so a miss costs one metadata call rather than a failed body read
func (t *Transformer) cached(ctx context.Context, dkey string) ([]byte, string, bool) {
	_, ctype, ok, _ := t.storage.Head(ctx, dkey)
	if !ok {
		return nil, "", false
	}
	data, err := t.storage.GetBytes(ctx, dkey)
	if err != nil {
		return nil, "", false
	}
	return data, ctype, true
}

// The fetch, decode/resample/encode and cache-back, holding a slot throughout - the pixel work is renderBytes, selected by build tag
func (t *Transformer) render(ctx context.Context, key, dkey string, w, h int, focalX, focalY float64, webp bool) ([]byte, string, error) {
	raw, err := t.storage.GetBytes(ctx, key)
	if err != nil {
		return nil, "", fmt.Errorf("load original: %w", err)
	}
	// Header-only size check - the allocation guard, so it runs before any decode
	if err := checkSourceSize(raw, t.maxSourcePixels); err != nil {
		return nil, "", fmt.Errorf("%s: %w", key, err)
	}
	data, ctype, err := renderBytes(raw, w, h, focalX, focalY, webp)
	if err != nil {
		return nil, "", fmt.Errorf("render %s: %w", key, err)
	}
	// Cache-back is best-effort - a write hiccup shouldn't fail the response, and the next request just regenerates
	_ = t.storage.PutBytes(ctx, dkey, data, ctype)
	return data, ctype, nil
}

// Rejects an original whose pixel count would blow the budget, reading dimensions from the header - wraps ErrSourceTooLarge so callers can 422
func checkSourceSize(raw []byte, maxPixels int) error {
	cfg, _, err := decodeConfig(raw)
	if err != nil {
		return fmt.Errorf("decode config: %w", err)
	}
	if px := cfg.Width * cfg.Height; px > maxPixels {
		return fmt.Errorf("%w: %dx%d = %d px, cap %d", ErrSourceTooLarge, cfg.Width, cfg.Height, px, maxPixels)
	}
	return nil
}

// Reads just the header - dimensions without the pixels
func decodeConfig(raw []byte) (image.Config, string, error) {
	if cfg, err := webp.DecodeConfig(bytes.NewReader(raw)); err == nil {
		return cfg, "webp", nil
	}
	return image.DecodeConfig(bytes.NewReader(raw))
}

// coverBox solves the cover-crop target: shrink the w x h box instead of upscaling, then the scale that covers it - shared by both render paths so a build switch can't change geometry
func coverBox(sw, sh, w, h int) (scale float64, outW, outH int) {
	scale = math.Max(float64(w)/float64(sw), float64(h)/float64(sh))
	if scale > 1 {
		ratio := 1 / scale
		w = max(int(math.Round(float64(w)*ratio)), 1)
		h = max(int(math.Round(float64(h)*ratio)), 1)
		scale = math.Max(float64(w)/float64(sw), float64(h)/float64(sh))
	}
	return scale, w, h
}

// cropOrigin centres the outW x outH window on the focal point, clamped inside the scaled aw x ah image - 0,0 focal means centre
func cropOrigin(aw, ah, outW, outH int, focalX, focalY float64) (x, y int) {
	if focalX <= 0 && focalY <= 0 {
		focalX, focalY = 0.5, 0.5
	}
	focalX = math.Min(1, math.Max(0, focalX))
	focalY = math.Min(1, math.Max(0, focalY))
	x = int(math.Round(focalX*float64(aw))) - outW/2
	y = int(math.Round(focalY*float64(ah))) - outH/2
	return clampInt(x, 0, aw-outW), clampInt(y, 0, ah-outH)
}

func clampInt(v, lo, hi int) int {
	if hi < lo {
		hi = lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
