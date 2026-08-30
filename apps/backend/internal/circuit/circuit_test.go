package circuit

import (
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

// A hand-driven clock, so the cooldown transitions are exact instead of slept for
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func newTestBreaker(threshold int, cooldown time.Duration) (*Breaker, *fakeClock) {
	clock := &fakeClock{t: time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)}
	b := New(threshold, cooldown)
	b.SetNow(clock.now)
	return b, clock
}

// An unknown target is closed - a fresh process must not gate anything it has never called
func TestUnknownTargetIsClosed(t *testing.T) {
	b, _ := newTestBreaker(3, time.Minute)
	id := uuid.New()
	if !b.Allow(id) {
		t.Error("Allow on an unseen target = false, want true")
	}
	if got := b.Info(id); got.State != Closed || got.ConsecutiveFailures != 0 || got.OpenedAt != nil {
		t.Errorf("Info = %+v, want a zero closed circuit", got)
	}
}

// Failures below the threshold accumulate but must not gate - one flaky call is not an outage
func TestFailuresBelowThresholdStayClosed(t *testing.T) {
	b, _ := newTestBreaker(3, time.Minute)
	id := uuid.New()
	for i := 1; i < 3; i++ {
		b.Record(id, false)
		if !b.Allow(id) {
			t.Fatalf("after %d failures Allow = false, want true (threshold is 3)", i)
		}
		if got := b.Info(id); got.State != Closed || got.ConsecutiveFailures != i {
			t.Errorf("after %d failures Info = %+v, want closed with %d failures", i, got, i)
		}
	}
}

// A success is the only thing that clears the count, so an intermittent endpoint never creeps to the threshold
func TestSuccessResetsTheFailureCount(t *testing.T) {
	b, _ := newTestBreaker(3, time.Minute)
	id := uuid.New()
	b.Record(id, false)
	b.Record(id, false)
	b.Record(id, true)
	if got := b.Info(id); got.State != Closed || got.ConsecutiveFailures != 0 {
		t.Fatalf("Info after a success = %+v, want a fully cleared circuit", got)
	}
	b.Record(id, false)
	b.Record(id, false)
	if !b.Allow(id) {
		t.Error("two failures after a reset already tripped the circuit, want the count to have restarted")
	}
}

func TestTripsAtThresholdAndBlocksForTheCooldown(t *testing.T) {
	b, clock := newTestBreaker(3, time.Minute)
	id := uuid.New()
	for range 3 {
		b.Record(id, false)
	}
	if b.Allow(id) {
		t.Fatal("Allow at the threshold = true, want the circuit open")
	}
	got := b.Info(id)
	if got.State != Open || got.ConsecutiveFailures != 3 {
		t.Errorf("Info = %+v, want open with 3 failures", got)
	}
	if got.OpenedAt == nil || !got.OpenedAt.Equal(clock.now()) {
		t.Errorf("OpenedAt = %v, want the trip time %v", got.OpenedAt, clock.now())
	}

	clock.advance(59 * time.Second)
	if b.Allow(id) {
		t.Error("Allow one second before the cooldown elapses = true, want false")
	}
}

// Exactly one probe crosses the gate - letting the whole backlog through at once is what makes a
// breaker useless against an endpoint that is merely slow
func TestCooldownReleasesASingleProbe(t *testing.T) {
	b, clock := newTestBreaker(2, time.Minute)
	id := uuid.New()
	b.Record(id, false)
	b.Record(id, false)

	clock.advance(time.Minute)
	if !b.Allow(id) {
		t.Fatal("Allow after the cooldown = false, want the probe let through")
	}
	if b.Allow(id) {
		t.Error("a second concurrent caller was let through, want only one probe in flight")
	}
	if got := b.Info(id); got.State != HalfOpen {
		t.Errorf("Info while probing = %s, want %s", got.State, HalfOpen)
	}
}

// The probe's outcome is the whole point - a success has to reopen the target to full traffic
func TestSuccessfulProbeClosesTheCircuit(t *testing.T) {
	b, clock := newTestBreaker(2, time.Minute)
	id := uuid.New()
	b.Record(id, false)
	b.Record(id, false)
	clock.advance(time.Minute)
	if !b.Allow(id) {
		t.Fatal("no probe was released")
	}

	b.Record(id, true)
	if got := b.Info(id); got.State != Closed || got.ConsecutiveFailures != 0 || got.OpenedAt != nil {
		t.Fatalf("Info after a successful probe = %+v, want a fully closed circuit", got)
	}
	for i := range 5 {
		if !b.Allow(id) {
			t.Fatalf("call %d after recovery was blocked, want full traffic restored", i)
		}
	}
}

// A failed probe must re-arm the full cooldown from now, not from the original trip, or a dead
// endpoint gets probed on every single call once its first window has passed
func TestFailedProbeReArmsTheCooldown(t *testing.T) {
	b, clock := newTestBreaker(2, time.Minute)
	id := uuid.New()
	b.Record(id, false)
	b.Record(id, false)
	trippedAt := clock.now()

	clock.advance(time.Minute)
	if !b.Allow(id) {
		t.Fatal("no probe was released")
	}
	b.Record(id, false)

	got := b.Info(id)
	if got.State != Open {
		t.Errorf("Info after a failed probe = %s, want %s", got.State, Open)
	}
	if got.OpenedAt == nil || !got.OpenedAt.After(trippedAt) {
		t.Errorf("OpenedAt = %v, want it moved past the original trip at %v", got.OpenedAt, trippedAt)
	}
	if b.Allow(id) {
		t.Error("Allow right after a failed probe = true, want the new cooldown enforced")
	}
	clock.advance(59 * time.Second)
	if b.Allow(id) {
		t.Error("Allow inside the re-armed cooldown = true, want false")
	}
	clock.advance(time.Second)
	if !b.Allow(id) {
		t.Error("Allow after the re-armed cooldown = false, want the next probe released")
	}
}

// Info is what the admin UI renders, so a circuit whose cooldown has quietly elapsed must read
// half-open rather than open, even though no probe has been claimed yet
func TestInfoReportsHalfOpenOnceTheCooldownElapses(t *testing.T) {
	b, clock := newTestBreaker(1, time.Minute)
	id := uuid.New()
	b.Record(id, false)
	if got := b.Info(id); got.State != Open {
		t.Fatalf("Info = %s, want %s", got.State, Open)
	}
	clock.advance(time.Minute)
	if got := b.Info(id); got.State != HalfOpen {
		t.Errorf("Info after the cooldown = %s, want %s", got.State, HalfOpen)
	}
}

// Admins fix an endpoint and re-save it - sitting out the remaining cooldown would look broken
func TestResetClosesImmediately(t *testing.T) {
	b, _ := newTestBreaker(2, time.Hour)
	id := uuid.New()
	b.Record(id, false)
	b.Record(id, false)
	if b.Allow(id) {
		t.Fatal("circuit did not open")
	}
	b.Reset(id)
	if !b.Allow(id) {
		t.Error("Allow after Reset = false, want the circuit closed")
	}
	if got := b.Info(id); got.State != Closed || got.ConsecutiveFailures != 0 {
		t.Errorf("Info after Reset = %+v, want a zero closed circuit", got)
	}
}

// One bad webhook target must not gate the others - state is per-id
func TestCircuitsAreIndependentPerTarget(t *testing.T) {
	b, _ := newTestBreaker(2, time.Minute)
	dead, healthy := uuid.New(), uuid.New()
	b.Record(dead, false)
	b.Record(dead, false)
	if b.Allow(dead) {
		t.Fatal("the failing target did not open")
	}
	if !b.Allow(healthy) {
		t.Error("an unrelated target was gated by another target's failures")
	}
	if got := b.Info(healthy); got.State != Closed {
		t.Errorf("healthy target Info = %s, want %s", got.State, Closed)
	}
}

// Threshold 1 is a legal config, and it has to trip on the first failure rather than the second
func TestThresholdOfOneTripsOnTheFirstFailure(t *testing.T) {
	b, _ := newTestBreaker(1, time.Minute)
	id := uuid.New()
	b.Record(id, false)
	if b.Allow(id) {
		t.Error("Allow after one failure at threshold 1 = true, want the circuit open")
	}
}

// Every caller shares one breaker across goroutines - run under -race
func TestConcurrentUseIsSafe(t *testing.T) {
	b, _ := newTestBreaker(3, time.Millisecond)
	ids := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			id := ids[n%len(ids)]
			for range 200 {
				b.Allow(id)
				b.Record(id, n%2 == 0)
				b.Info(id)
			}
		}(i)
	}
	wg.Wait()
}
