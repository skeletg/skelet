package media

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"strings"
	"testing"
	"time"
)

// pngBytes encodes a w x h PNG - the header is what the pixel cap reads
func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatalf("encode: %v", err)
	}
	return buf.Bytes()
}

func TestDecodeConfigReadsDimensionsWithoutDecoding(t *testing.T) {
	cfg, _, err := decodeConfig(pngBytes(t, 640, 480))
	if err != nil {
		t.Fatalf("decodeConfig: %v", err)
	}
	if cfg.Width != 640 || cfg.Height != 480 {
		t.Fatalf("got %dx%d, want 640x480", cfg.Width, cfg.Height)
	}
}

func TestDerivedKey(t *testing.T) {
	if got := DerivedKey("p/img.jpg", 640, 0, false); got != "derived/w640/p/img.jpg" {
		t.Errorf("fit key = %q", got)
	}
	if got := DerivedKey("p/img.jpg", 640, 480, false); got != "derived/w640h480/p/img.jpg" {
		t.Errorf("cover key = %q", got)
	}
	// webp lives in its own keyspace - non-webp keys stay untouched so those caches keep serving
	if got := DerivedKey("p/img.jpg", 640, 0, true); got != "derived/w640-webp/p/img.jpg" {
		t.Errorf("webp fit key = %q", got)
	}
	if got := DerivedKey("p/img.jpg", 640, 480, true); got != "derived/w640h480-webp/p/img.jpg" {
		t.Errorf("webp cover key = %q", got)
	}
}

func TestSnapSpec(t *testing.T) {
	// h == 0 stays 0 (fit); a set h snaps onto the same ladder as w
	if w, h := SnapSpec(500, 0); w != 640 || h != 0 {
		t.Errorf("SnapSpec(500,0) = %d,%d want 640,0", w, h)
	}
	if w, h := SnapSpec(500, 500); w != 640 || h != 640 {
		t.Errorf("SnapSpec(500,500) = %d,%d want 640,640", w, h)
	}
}

// checkSourceSize is the allocation guard: it must reject from the header,
// before a full decode allocates w*h*4 for the source
func TestCheckSourceSize(t *testing.T) {
	raw := pngBytes(t, 100, 100) // 10k px

	if err := checkSourceSize(raw, 10_000); err != nil {
		t.Fatalf("exactly at the cap should pass: %v", err)
	}
	if err := checkSourceSize(raw, 20_000); err != nil {
		t.Fatalf("under the cap should pass: %v", err)
	}

	err := checkSourceSize(raw, 9_999)
	if !errors.Is(err, ErrSourceTooLarge) {
		t.Fatalf("over the cap = %v, want ErrSourceTooLarge", err)
	}
	// The message has to name the numbers - this lands in an operator's log
	// when a legitimate upload starts 422ing
	if !strings.Contains(err.Error(), "100x100") || !strings.Contains(err.Error(), "9999") {
		t.Errorf("error lacks dimensions/cap: %v", err)
	}

	if err := checkSourceSize([]byte("not an image"), 10_000); err == nil {
		t.Error("undecodable source should error")
	} else if errors.Is(err, ErrSourceTooLarge) {
		t.Error("undecodable source misreported as too large")
	}
}

func TestAcquireBlocksThenReturnsErrBusy(t *testing.T) {
	tr := &Transformer{sem: make(chan struct{}, 1), renderWait: 50 * time.Millisecond}
	if err := tr.acquire(context.Background()); err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	// Slot taken: the second caller waits out renderWait, then rejects rather
	// than piling on - Throttle first, refuse second
	start := time.Now()
	err := tr.acquire(context.Background())
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("second acquire = %v, want ErrBusy", err)
	}
	if elapsed := time.Since(start); elapsed < 40*time.Millisecond {
		t.Fatalf("returned after %v, should have waited out renderWait", elapsed)
	}
	tr.release()
	if err := tr.acquire(context.Background()); err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
}

func TestAcquireHonorsContextCancellation(t *testing.T) {
	tr := &Transformer{sem: make(chan struct{}, 1), renderWait: time.Hour}
	if err := tr.acquire(context.Background()); err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// A client that has gone away must not hold a queue slot for an hour
	if err := tr.acquire(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("acquire with cancelled ctx = %v, want context.Canceled", err)
	}
}

func TestSetRenderConcurrencyResizesPool(t *testing.T) {
	tr := NewTransformer(nil)
	tr.SetRenderConcurrency(3, 10*time.Millisecond)
	if cap(tr.sem) != 3 {
		t.Fatalf("cap = %d, want 3", cap(tr.sem))
	}
	if tr.renderWait != 10*time.Millisecond {
		t.Fatalf("renderWait = %v", tr.renderWait)
	}
	// Zero values are "keep what you have", so callers can set one knob
	tr.SetRenderConcurrency(0, 0)
	if cap(tr.sem) != 3 || tr.renderWait != 10*time.Millisecond {
		t.Fatalf("zero args changed config: cap=%d wait=%v", cap(tr.sem), tr.renderWait)
	}
}

func TestSetMaxSourcePixelsIgnoresNonPositive(t *testing.T) {
	tr := NewTransformer(nil)
	if tr.maxSourcePixels != defaultMaxSourcePixels {
		t.Fatalf("default = %d", tr.maxSourcePixels)
	}
	tr.SetMaxSourcePixels(0)
	if tr.maxSourcePixels != defaultMaxSourcePixels {
		t.Fatalf("0 overrode the default")
	}
	tr.SetMaxSourcePixels(5)
	if tr.maxSourcePixels != 5 {
		t.Fatalf("got %d, want 5", tr.maxSourcePixels)
	}
}
