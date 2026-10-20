package search

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
)

type stubEngine struct {
	name  string
	err   error
	hits  []ProductHit
	total int64
	calls int
}

func (s *stubEngine) Name() string { return s.name }
func (s *stubEngine) SearchProducts(context.Context, string, uuid.UUID, string, int, int, map[uuid.UUID]float64) ([]ProductHit, int64, error) {
	s.calls++
	return s.hits, s.total, s.err
}
func (s *stubEngine) BrowseProducts(context.Context, uuid.UUID, string, string, BrowseFilter, string, int, int, map[uuid.UUID]float64) (BrowseResult, error) {
	s.calls++
	return BrowseResult{Total: s.total}, s.err
}
func (s *stubEngine) ListSortOptions(context.Context) []SortOption {
	return []SortOption{{Slug: s.name}}
}

func TestTieredEngine(t *testing.T) {
	boom := errors.New("cluster down")

	t.Run("primary answers and the backup is never called", func(t *testing.T) {
		primary := &stubEngine{name: "primary", total: 7}
		backup := &stubEngine{name: "backup", total: 1}
		var degraded bool
		e := NewTieredEngine(primary, backup, func(context.Context, string, error) { degraded = true })

		if _, total, err := e.SearchProducts(context.Background(), "q", uuid.New(), "en", 10, 0, nil); err != nil || total != 7 {
			t.Fatalf("got total=%d err=%v, want 7/nil", total, err)
		}
		if backup.calls != 0 {
			t.Fatal("backup was called while the primary was healthy")
		}
		if degraded {
			t.Fatal("reported degraded on a healthy primary")
		}
	})

	t.Run("primary failure falls back and reports degraded", func(t *testing.T) {
		primary := &stubEngine{name: "primary", err: boom}
		backup := &stubEngine{name: "backup", total: 3}
		var op string
		e := NewTieredEngine(primary, backup, func(_ context.Context, o string, _ error) { op = o })

		res, err := e.BrowseProducts(context.Background(), uuid.New(), "en", "", BrowseFilter{}, "", 10, 0, nil)
		if err != nil || res.Total != 3 {
			t.Fatalf("got total=%d err=%v, want 3/nil", res.Total, err)
		}
		if backup.calls != 1 {
			t.Fatalf("backup calls = %d, want 1", backup.calls)
		}
		if op != "browse" {
			t.Fatalf("degraded op = %q, want browse", op)
		}
	})

	// The shipped production default: no cluster configured at all
	t.Run("nil primary runs entirely on the backup", func(t *testing.T) {
		backup := &stubEngine{name: "backup", total: 5}
		var degraded bool
		e := NewTieredEngine(nil, backup, func(context.Context, string, error) { degraded = true })

		if _, total, err := e.SearchProducts(context.Background(), "q", uuid.New(), "en", 10, 0, nil); err != nil || total != 5 {
			t.Fatalf("got total=%d err=%v, want 5/nil", total, err)
		}
		if _, err := e.BrowseProducts(context.Background(), uuid.New(), "en", "", BrowseFilter{}, "", 10, 0, nil); err != nil {
			t.Fatalf("browse: %v", err)
		}
		// Never configured is a steady state, not a degradation - reporting it marked every browse and search response
		// uncacheable forever, which disabled the front response cache on the two hottest paths for the shipped default
		if degraded {
			t.Fatal("a tier with no primary reported degraded, which keeps every response out of the cache")
		}
		if got := e.Name(); got != "backup" {
			t.Fatalf("Name() = %q, want backup", got)
		}
		if opts := e.ListSortOptions(context.Background()); len(opts) != 1 || opts[0].Slug != "backup" {
			t.Fatalf("sort options came from the wrong engine: %+v", opts)
		}
	})

	// Both down is a real error, not a silent empty page
	t.Run("both failing surfaces the error", func(t *testing.T) {
		e := NewTieredEngine(&stubEngine{name: "primary", err: boom}, &stubEngine{name: "backup", err: boom}, nil)
		if _, _, err := e.SearchProducts(context.Background(), "q", uuid.New(), "en", 10, 0, nil); err == nil {
			t.Fatal("want an error when every tier fails")
		}
	})

	t.Run("a cancelled caller is not a primary outage", func(t *testing.T) {
		primary := &stubEngine{name: "primary", err: context.Canceled}
		backup := &stubEngine{name: "backup", total: 3}
		var degraded bool
		e := NewTieredEngine(primary, backup, func(context.Context, string, error) { degraded = true })
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		if _, _, err := e.SearchProducts(ctx, "q", uuid.New(), "en", 10, 0, nil); !errors.Is(err, context.Canceled) {
			t.Fatalf("search err = %v, want context.Canceled", err)
		}
		if _, err := e.BrowseProducts(ctx, uuid.New(), "en", "", BrowseFilter{}, "", 10, 0, nil); !errors.Is(err, context.Canceled) {
			t.Fatalf("browse err = %v, want context.Canceled", err)
		}
		if backup.calls != 0 || degraded {
			t.Fatalf("backup called %d times, degraded=%v - want neither", backup.calls, degraded)
		}
	})
}

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

func TestBreaker(t *testing.T) {
	newTestBreaker := func() (*breaker, *fakeClock) {
		clock := &fakeClock{t: time.Unix(1_000_000, 0)}
		b := newBreaker()
		b.now = clock.now
		return b, clock
	}
	fail := func(b *breaker, n int) (tripped bool) {
		for range n {
			tr, _ := b.record(false, true)
			tripped = tripped || tr
		}
		return tripped
	}

	t.Run("trips at the failure floor", func(t *testing.T) {
		b, _ := newTestBreaker()
		if fail(b, breakerMinFailures-1) {
			t.Fatal("tripped below the floor")
		}
		if !fail(b, 1) {
			t.Fatal("did not trip at the floor")
		}
		if try, _ := b.allow(); try {
			t.Fatal("open breaker let a request through")
		}
	})

	t.Run("successes outnumbering failures keep it closed", func(t *testing.T) {
		b, _ := newTestBreaker()
		for range 10 {
			b.record(false, false)
		}
		if fail(b, 9) {
			t.Fatal("tripped while successes outnumbered failures")
		}
	})

	t.Run("failures spread across windows don't add up", func(t *testing.T) {
		b, clock := newTestBreaker()
		fail(b, breakerMinFailures-1)
		clock.advance(breakerWindow + time.Second)
		if fail(b, breakerMinFailures-1) {
			t.Fatal("counted failures from an expired window")
		}
	})

	t.Run("one probe after the cooldown decides", func(t *testing.T) {
		b, clock := newTestBreaker()
		fail(b, breakerMinFailures)
		clock.advance(breakerCooldown)
		try, probe := b.allow()
		if !try || !probe {
			t.Fatalf("allow() = %v, %v after the cooldown, want a probe", try, probe)
		}
		if try, _ := b.allow(); try {
			t.Fatal("a second request went through while the probe was out")
		}
		b.record(true, true)
		if try, _ := b.allow(); try {
			t.Fatal("a failed probe didn't reopen the breaker")
		}
		clock.advance(breakerCooldown)
		_, probe = b.allow()
		if _, recovered := b.record(probe, false); !recovered {
			t.Fatal("a healthy probe didn't close the breaker")
		}
		if try, probe := b.allow(); !try || probe {
			t.Fatalf("allow() = %v, %v after recovery, want a plain request", try, probe)
		}
	})

	t.Run("a released probe frees the slot", func(t *testing.T) {
		b, clock := newTestBreaker()
		fail(b, breakerMinFailures)
		clock.advance(breakerCooldown)
		_, probe := b.allow()
		b.release(probe)
		if try, probe := b.allow(); !try || !probe {
			t.Fatal("a released probe left the breaker stuck open")
		}
	})
}

func TestTieredEngineCircuit(t *testing.T) {
	primary := &stubEngine{name: "primary", err: errors.New("timeout")}
	backup := &stubEngine{name: "backup", total: 2}
	var causes []error
	e := NewTieredEngine(primary, backup, func(_ context.Context, _ string, cause error) { causes = append(causes, cause) })
	clock := &fakeClock{t: time.Unix(1_000_000, 0)}
	e.breaker.now = clock.now
	browse := func() (BrowseResult, error) {
		return e.BrowseProducts(context.Background(), uuid.New(), "en", "", BrowseFilter{}, "", 10, 0, nil)
	}

	for range breakerMinFailures + 3 {
		if res, err := browse(); err != nil || res.Total != 2 {
			t.Fatalf("got total=%d err=%v, want the backup's 2/nil", res.Total, err)
		}
	}
	if primary.calls != breakerMinFailures {
		t.Fatalf("primary called %d times, want %d before the circuit opened", primary.calls, breakerMinFailures)
	}
	if len(causes) != breakerMinFailures+3 || !errors.Is(causes[len(causes)-1], errPrimaryOpen) {
		t.Fatalf("skipped requests must still report degraded, got %v", causes)
	}

	primary.err = nil
	primary.total = 9
	clock.advance(breakerCooldown)
	for range 2 {
		if res, err := browse(); err != nil || res.Total != 9 {
			t.Fatalf("got total=%d err=%v, want the recovered primary's 9/nil", res.Total, err)
		}
	}
}

func TestTieredEngineCancelledProbe(t *testing.T) {
	primary := &stubEngine{name: "primary", err: errors.New("timeout")}
	backup := &stubEngine{name: "backup", total: 2}
	e := NewTieredEngine(primary, backup, nil)
	clock := &fakeClock{t: time.Unix(1_000_000, 0)}
	e.breaker.now = clock.now
	for range breakerMinFailures {
		_, _ = e.BrowseProducts(context.Background(), uuid.New(), "en", "", BrowseFilter{}, "", 10, 0, nil)
	}
	clock.advance(breakerCooldown)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	primary.err = context.Canceled
	if _, err := e.BrowseProducts(ctx, uuid.New(), "en", "", BrowseFilter{}, "", 10, 0, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled probe err = %v, want context.Canceled", err)
	}
	primary.err, primary.total = nil, 9
	if res, err := e.BrowseProducts(context.Background(), uuid.New(), "en", "", BrowseFilter{}, "", 10, 0, nil); err != nil || res.Total != 9 {
		t.Fatalf("got total=%d err=%v, want the primary probed again after a cancelled probe", res.Total, err)
	}
}

type gatedEngine struct {
	name    string
	gate    chan struct{}
	entered chan struct{}
	calls   atomic.Int32
}

func (g *gatedEngine) Name() string { return g.name }
func (g *gatedEngine) SearchProducts(context.Context, string, uuid.UUID, string, int, int, map[uuid.UUID]float64) ([]ProductHit, int64, error) {
	g.calls.Add(1)
	g.entered <- struct{}{}
	<-g.gate
	return nil, 1, nil
}
func (g *gatedEngine) BrowseProducts(context.Context, uuid.UUID, string, string, BrowseFilter, string, int, int, map[uuid.UUID]float64) (BrowseResult, error) {
	g.calls.Add(1)
	g.entered <- struct{}{}
	<-g.gate
	return BrowseResult{Total: 1}, nil
}
func (g *gatedEngine) ListSortOptions(context.Context) []SortOption { return nil }

func TestTieredEngineFallbackCap(t *testing.T) {
	newGate := func() *gatedEngine {
		return &gatedEngine{name: "backup", gate: make(chan struct{}), entered: make(chan struct{}, 8)}
	}
	search := func(e *TieredEngine, errs chan<- error) {
		_, _, err := e.SearchProducts(context.Background(), "q", uuid.New(), "en", 10, 0, nil)
		errs <- err
	}

	t.Run("sheds past the cap and frees the slot after", func(t *testing.T) {
		backup := newGate()
		defer close(backup.gate)
		e := NewTieredEngine(&stubEngine{name: "primary", err: errors.New("down")}, backup, nil, WithFallbackLimit(1))
		e.wait = 20 * time.Millisecond
		first, second := make(chan error, 1), make(chan error, 1)
		go search(e, first)
		<-backup.entered

		go search(e, second)
		select {
		case err := <-second:
			if !errors.Is(err, ErrFallbackSaturated) {
				t.Fatalf("second fallback err = %v, want ErrFallbackSaturated", err)
			}
		case <-backup.entered:
			t.Fatal("a second request reached the backup past the cap")
		}
		backup.gate <- struct{}{}
		if err := <-first; err != nil {
			t.Fatalf("first fallback: %v", err)
		}
		third := make(chan error, 1)
		go search(e, third)
		select {
		case <-backup.entered:
			backup.gate <- struct{}{}
		case err := <-third:
			t.Fatalf("the slot wasn't released: %v", err)
		}
		if err := <-third; err != nil {
			t.Fatalf("third fallback: %v", err)
		}
	})

	t.Run("a tier without a primary is never capped", func(t *testing.T) {
		backup := newGate()
		defer close(backup.gate)
		e := NewTieredEngine(nil, backup, nil, WithFallbackLimit(1))
		e.wait = 20 * time.Millisecond
		errs := make(chan error, 2)
		go search(e, errs)
		go search(e, errs)
		for range 2 {
			select {
			case <-backup.entered:
			case err := <-errs:
				t.Fatalf("a primary-less tier refused a request: %v", err)
			}
		}
	})
}
