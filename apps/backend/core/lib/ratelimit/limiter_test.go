package ratelimit

import (
	"context"
	"testing"
	"testing/synctest"
	"time"
)

func TestMemoryLimiter_AllowsRateThenBlocks(t *testing.T) {
	l := NewMemory(3, time.Minute)
	ctx := context.Background()
	for i := 1; i <= 3; i++ {
		if !l.Allow(ctx, "k") {
			t.Fatalf("call %d: Allow = false, want true (within rate)", i)
		}
	}
	if l.Allow(ctx, "k") {
		t.Error("4th call: Allow = true, want false (over rate)")
	}
	if l.Allow(ctx, "k") {
		t.Error("5th call: still over rate, want false")
	}
}

func TestMemoryLimiter_Take(t *testing.T) {
	l := NewMemory(100, time.Minute) // constructor rate irrelevant to Take
	ctx := context.Background()
	for want := int64(1); want <= 3; want++ {
		n, ok := l.Take(ctx, "k")
		if !ok || n != want {
			t.Fatalf("Take #%d = (%d, %v), want (%d, true)", want, n, ok, want)
		}
	}
	// Independent key starts its own count
	if n, _ := l.Take(ctx, "other"); n != 1 {
		t.Errorf("independent key Take = %d, want 1", n)
	}
}

func TestMemoryLimiter_AllowN(t *testing.T) {
	l := NewMemory(100, time.Minute) // constructor rate ignored by AllowN
	ctx := context.Background()
	// rate <= 0 disables the check entirely (the "throttle off" config value)
	// and must not consume budget
	for range 5 {
		if !l.AllowN(ctx, "off", 0) {
			t.Fatal("rate 0 must always allow")
		}
	}
	// Per-call rate override wins over the constructor rate
	for i := 1; i <= 2; i++ {
		if !l.AllowN(ctx, "k", 2) {
			t.Fatalf("call %d: AllowN = false, want true (within override rate)", i)
		}
	}
	if l.AllowN(ctx, "k", 2) {
		t.Error("3rd call: AllowN = true, want false (over override rate)")
	}
	// A raised rate mid-window honors the counter already accrued
	if !l.AllowN(ctx, "k", 5) {
		t.Error("raised rate should re-admit the same key mid-window")
	}
}

func TestMemoryLimiter_KeysAreIndependent(t *testing.T) {
	l := NewMemory(1, time.Minute)
	ctx := context.Background()
	if !l.Allow(ctx, "a") {
		t.Fatal("first call for a should be allowed")
	}
	if l.Allow(ctx, "a") {
		t.Fatal("second call for a should be blocked")
	}
	// b has its own bucket - a's exhaustion must not affect it
	if !l.Allow(ctx, "b") {
		t.Error("first call for b should be allowed (independent bucket)")
	}
}

// A production-shaped window on synctest's fake clock - the old version used a 10ms window and a
// 25ms real sleep, which is both slower and a flake waiting for a loaded CI box
func TestMemoryLimiter_WindowResets(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l := NewMemory(1, time.Hour)
		ctx := context.Background()
		if !l.Allow(ctx, "k") {
			t.Fatal("first call should be allowed")
		}
		if l.Allow(ctx, "k") {
			t.Fatal("second call within window should be blocked")
		}
		synctest.Sleep(time.Hour + time.Minute) // past the window, instantly
		if !l.Allow(ctx, "k") {
			t.Error("after window elapsed, key should reset to allowed")
		}
	})
}

// TestMemoryLimiter_ForcedExpiryResets is the deterministic complement to the sleep-based test: reach into the
// (internal) bucket and stamp resetAt in the past, then confirm the next Allow rolls a fresh window
func TestMemoryLimiter_ForcedExpiryResets(t *testing.T) {
	l := NewMemory(1, time.Hour)
	ctx := context.Background()
	if !l.Allow(ctx, "k") {
		t.Fatal("first call should be allowed")
	}
	if l.Allow(ctx, "k") {
		t.Fatal("second call should be blocked before forced expiry")
	}
	l.mu.Lock()
	l.buckets["k"].resetAt = time.Now().Add(-time.Minute)
	l.mu.Unlock()
	if !l.Allow(ctx, "k") {
		t.Error("after forced expiry the window should reset and allow")
	}
}

// Runs the real Sweep goroutine on its real ticker rather than replicating the delete condition here, which would leave the loop itself unexercised
func TestMemoryLimiter_SweepDropsExpiredBuckets(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l := NewMemory(5, time.Hour)
		ctx := context.Background()
		l.Allow(ctx, "dead")
		l.mu.Lock()
		l.buckets["dead"].resetAt = time.Now().Add(-time.Minute)
		l.mu.Unlock()

		stop := make(chan struct{})
		defer close(stop)
		go l.Sweep(stop)

		// Just shy of the first tick at 2h, so this bucket's window outlives it
		synctest.Sleep(time.Hour + 59*time.Minute)
		l.Allow(ctx, "live")
		synctest.Sleep(2 * time.Minute) // crosses the tick

		l.mu.Lock()
		remaining := len(l.buckets)
		_, deadStillThere := l.buckets["dead"]
		_, liveStillThere := l.buckets["live"]
		l.mu.Unlock()

		if deadStillThere {
			t.Error("expired bucket should have been dropped")
		}
		if !liveStillThere {
			t.Error("bucket touched within the last window should survive the tick")
		}
		if remaining != 1 {
			t.Errorf("remaining buckets = %d, want 1", remaining)
		}
	})
}

func TestMemoryLimiter_SweepStopsOnClose(t *testing.T) {
	l := NewMemory(1, time.Millisecond)
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		l.Sweep(stop)
		close(done)
	}()
	close(stop)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Sweep did not return after stop was closed")
	}
}

// The whole layer ships off by wiring rate 0, so this is the property that keeps a disabled limiter disabled
func TestZeroRateAdmitsEverythingWithoutBackend(t *testing.T) {
	for _, l := range []Limiter{NewRedis(nil, 0, time.Minute), NewMemory(0, time.Minute)} {
		for i := range 500 {
			if !l.Allow(context.Background(), "k") {
				t.Fatalf("%T refused request %d at rate 0", l, i)
			}
		}
	}
}

// Take stays live at rate 0 - the catalog throttle reads its budget from config and must keep working while
// per-IP limiting is off, so a "disabled" limiter still counts when asked directly
func TestZeroRateStillCounts(t *testing.T) {
	l := NewMemory(0, time.Minute)
	if n, ok := l.Take(context.Background(), "k"); !ok || n != 1 {
		t.Fatalf("Take = (%d, %v), want (1, true)", n, ok)
	}
	if n, _ := l.Take(context.Background(), "k"); n != 2 {
		t.Fatalf("second Take = %d, want 2", n)
	}
}
