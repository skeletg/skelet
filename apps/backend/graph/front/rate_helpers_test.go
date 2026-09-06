package front

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"testing/synctest"
	"time"

	"github.com/skeletg/skelet/apps/backend/internal/auth"
	"github.com/skeletg/skelet/apps/backend/internal/ratelimit"
)

// In-memory limiter plus a pre-seeded config memo (no channel in ctx resolves to the "default" memo key), so count, delay and refusal all run exactly as in production minus the DB read
func throttleTestResolver(soft int) *Resolver {
	r := &Resolver{RateLimiter: ratelimit.NewMemory(1000, time.Minute)}
	r.catalogRate.Store("default", catalogRateEntry{rate: soft, expires: time.Now().Add(time.Hour)})
	return r
}

func throttleTestCtx() context.Context {
	return auth.WithRequest(context.Background(), httptest.NewRequest("POST", "/api/front/graphql", nil)) // httptest sets RemoteAddr 192.0.2.1:1234 and tests trust no proxies, so ClientIP lands on a stable per-test IP
}

// MEASURES each sequential call to assert the three zones - free inside the budget, linear slowdown
// past it, refusal past 3x - on synctest's clock each elapsed time is EXACT, so this needs no slack
// for timer overshoot the way it did against the wall clock
func TestCheckCatalogRate_ThrottleShape(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const soft = 2
		r := throttleTestResolver(soft)
		ctx := throttleTestCtx()

		type zone struct {
			call     int
			minDelay time.Duration
			reject   bool
		}
		zones := []zone{
			{1, 0, false},                     // within budget: free
			{2, 0, false},                     // at budget: free
			{3, 1 * throttleDelayStep, false}, // +1 over: first step
			{4, 2 * throttleDelayStep, false}, // +2 over: ramp
			{5, 3 * throttleDelayStep, false}, // +3 over: ramp
			{6, 4 * throttleDelayStep, false}, // 3x boundary: slowest, still served
			{7, 0, true},                      // past 3x: refused, instantly
			{8, 0, true},                      // stays refused
		}
		for _, z := range zones {
			start := time.Now()
			err := r.checkCatalogRate(ctx)
			elapsed := time.Since(start)
			if z.reject {
				if !errors.Is(err, ErrRateLimited) {
					t.Fatalf("call %d: err = %v, want ErrRateLimited", z.call, err)
				}
				if elapsed != 0 {
					t.Errorf("call %d: refusal took %v, want instant (no delay before reject)", z.call, elapsed)
				}
				continue
			}
			if err != nil {
				t.Fatalf("call %d: err = %v, want nil (served)", z.call, err)
			}
			if elapsed != z.minDelay {
				t.Errorf("call %d: elapsed %v, want exactly %v", z.call, elapsed, z.minDelay)
			}
		}
	})
}

// The zero knob is the default, so the hot path must come out untouched - no delay, no refusal
func TestCheckCatalogRate_Off(t *testing.T) {
	r := throttleTestResolver(0)
	ctx := throttleTestCtx()
	for i := 1; i <= 50; i++ {
		start := time.Now()
		if err := r.checkCatalogRate(ctx); err != nil {
			t.Fatalf("call %d: err = %v, want nil with throttle off", i, err)
		}
		if elapsed := time.Since(start); elapsed > 20*time.Millisecond {
			t.Fatalf("call %d: took %v, throttle-off must not delay", i, elapsed)
		}
	}
}

// A client hang-up mid-delay has to release the goroutine promptly instead of sleeping out the full delay
func TestCheckCatalogRate_CancelledContext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const soft = 1
		r := throttleTestResolver(soft)
		base := throttleTestCtx()

		if err := r.checkCatalogRate(base); err != nil { // burn the budget
			t.Fatalf("budget call: %v", err)
		}
		ctx, cancel := context.WithCancel(base)
		go func() {
			time.Sleep(30 * time.Millisecond)
			cancel()
		}()
		start := time.Now()
		err := r.checkCatalogRate(ctx) // n=2: a full delay step is scheduled, cancelled at 30ms
		elapsed := time.Since(start)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
		// Exact on the fake clock: it returns the moment cancel lands, not "some time under the step"
		if elapsed != 30*time.Millisecond {
			t.Errorf("cancellation took %v, want exactly 30ms - it waited past the cancel", elapsed)
		}
	})
}

func TestThrottleDelay(t *testing.T) {
	const soft = 60
	cases := []struct {
		name   string
		n      int64
		delay  time.Duration
		reject bool
	}{
		{"under budget", 1, 0, false},
		{"at budget", soft, 0, false},
		{"one over: first step", soft + 1, throttleDelayStep, false},
		{"five over: linear growth", soft + 5, 5 * throttleDelayStep, false},
		{"far over: capped", soft + 100, throttleMaxDelay, false},
		{"at reject boundary: still served (slowly)", soft * throttleRejectMult, throttleMaxDelay, false},
		{"past reject boundary: refused", soft*throttleRejectMult + 1, 0, true},
	}
	for _, c := range cases {
		d, rej := throttleDelay(c.n, soft)
		if d != c.delay || rej != c.reject {
			t.Errorf("%s: throttleDelay(%d, %d) = (%v, %v), want (%v, %v)", c.name, c.n, soft, d, rej, c.delay, c.reject)
		}
	}
}
