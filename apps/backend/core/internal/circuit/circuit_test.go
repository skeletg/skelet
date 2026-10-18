package circuit

import (
	"context"
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

var bg = context.Background()

// One admitted call that reports its outcome - the closed-circuit shorthand most tests need
func call(t *testing.T, b *Breaker, id uuid.UUID, success bool) {
	t.Helper()
	tk, ok := b.Allow(id)
	if !ok {
		t.Fatal("call refused, want it admitted")
	}
	tk.Done(bg, success)
}

func allows(b *Breaker, id uuid.UUID) bool {
	_, ok := b.Allow(id)
	return ok
}

func trip(t *testing.T, b *Breaker, id uuid.UUID, n int) {
	t.Helper()
	for range n {
		call(t, b, id, false)
	}
}

// An unknown target is closed - a fresh process must not gate anything it has never called
func TestUnknownTargetIsClosed(t *testing.T) {
	b, _ := newTestBreaker(3, time.Minute)
	id := uuid.New()
	if !allows(b, id) {
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
		call(t, b, id, false)
		if got := b.Info(id); got.State != Closed || got.ConsecutiveFailures != i {
			t.Errorf("after %d failures Info = %+v, want closed with %d failures", i, got, i)
		}
	}
}

// A success is the only thing that clears the count, so an intermittent endpoint never creeps to the threshold
func TestSuccessResetsTheFailureCount(t *testing.T) {
	b, _ := newTestBreaker(3, time.Minute)
	id := uuid.New()
	trip(t, b, id, 2)
	call(t, b, id, true)
	if got := b.Info(id); got.State != Closed || got.ConsecutiveFailures != 0 {
		t.Fatalf("Info after a success = %+v, want a fully cleared circuit", got)
	}
	trip(t, b, id, 2)
	if !allows(b, id) {
		t.Error("two failures after a reset already tripped the circuit, want the count to have restarted")
	}
}

func TestTripsAtThresholdAndBlocksForTheCooldown(t *testing.T) {
	b, clock := newTestBreaker(3, time.Minute)
	id := uuid.New()
	trip(t, b, id, 3)
	if allows(b, id) {
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
	if allows(b, id) {
		t.Error("Allow one second before the cooldown elapses = true, want false")
	}
}

// Exactly one probe crosses the gate - letting the whole backlog through at once is what makes a breaker useless against an endpoint that is merely slow
func TestCooldownReleasesASingleProbe(t *testing.T) {
	b, clock := newTestBreaker(2, time.Minute)
	id := uuid.New()
	trip(t, b, id, 2)
	clock.advance(time.Minute)
	if !allows(b, id) {
		t.Fatal("Allow after the cooldown = false, want the probe let through")
	}
	if allows(b, id) {
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
	trip(t, b, id, 2)
	clock.advance(time.Minute)
	probe, ok := b.Allow(id)
	if !ok {
		t.Fatal("no probe was released")
	}
	if !probe.Done(bg, true) {
		t.Error("a successful probe reported no state change")
	}
	if got := b.Info(id); got.State != Closed || got.ConsecutiveFailures != 0 || got.OpenedAt != nil {
		t.Fatalf("Info after a successful probe = %+v, want a fully closed circuit", got)
	}
	for i := range 5 {
		if !allows(b, id) {
			t.Fatalf("call %d after recovery was blocked, want full traffic restored", i)
		}
	}
}

// A failed probe must re-arm the full cooldown from now, not from the original trip, or a dead endpoint gets probed on every call once its first window has passed
func TestFailedProbeReArmsTheCooldown(t *testing.T) {
	b, clock := newTestBreaker(2, time.Minute)
	id := uuid.New()
	trip(t, b, id, 2)
	trippedAt := clock.now()
	clock.advance(time.Minute)
	probe, ok := b.Allow(id)
	if !ok {
		t.Fatal("no probe was released")
	}
	probe.Done(bg, false)

	got := b.Info(id)
	if got.State != Open {
		t.Errorf("Info after a failed probe = %s, want %s", got.State, Open)
	}
	if got.OpenedAt == nil || !got.OpenedAt.After(trippedAt) {
		t.Errorf("OpenedAt = %v, want it moved past the original trip at %v", got.OpenedAt, trippedAt)
	}
	if allows(b, id) {
		t.Error("Allow right after a failed probe = true, want the new cooldown enforced")
	}
	clock.advance(59 * time.Second)
	if allows(b, id) {
		t.Error("Allow inside the re-armed cooldown = true, want false")
	}
	clock.advance(time.Second)
	if !allows(b, id) {
		t.Error("Allow after the re-armed cooldown = false, want the next probe released")
	}
}

// The bug this API exists for - a probe whose caller walked away used to hold the half-open slot forever, blocking the target until a restart
func TestACancelledProbeHandsItsSlotBack(t *testing.T) {
	for _, tc := range []struct {
		name   string
		finish func(*Ticket)
	}{
		{"cancel", func(tk *Ticket) { tk.Cancel() }},
		{"done with a cancelled ctx", func(tk *Ticket) {
			ctx, cancel := context.WithCancel(bg)
			cancel()
			tk.Done(ctx, false)
		}},
		{"done with an expired ctx", func(tk *Ticket) {
			ctx, cancel := context.WithDeadline(bg, time.Now().Add(-time.Second))
			defer cancel()
			tk.Done(ctx, true)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, clock := newTestBreaker(1, time.Minute)
			id := uuid.New()
			trip(t, b, id, 1)
			clock.advance(time.Minute)
			probe, ok := b.Allow(id)
			if !ok {
				t.Fatal("no probe was released")
			}
			tc.finish(probe)
			if got := b.Info(id); got.State != HalfOpen || got.ConsecutiveFailures != 1 {
				t.Errorf("Info after an abandoned probe = %+v, want still half-open with the count untouched", got)
			}
			if !allows(b, id) {
				t.Error("the next caller was refused, want it to take the released probe slot")
			}
		})
	}
}

// A cancelled caller says nothing about the target, so a run of abandoned calls must never trip a healthy circuit
func TestCancelledCallsNeverCount(t *testing.T) {
	b, _ := newTestBreaker(2, time.Minute)
	id := uuid.New()
	ctx, cancel := context.WithCancel(bg)
	cancel()
	for range 5 {
		tk, _ := b.Allow(id)
		tk.Done(ctx, false)
	}
	if got := b.Info(id); got.State != Closed || got.ConsecutiveFailures != 0 {
		t.Errorf("Info after cancelled calls = %+v, want a zero closed circuit", got)
	}
}

// Only the probe speaks for an open circuit - a call admitted before the trip, landing late, must neither close it mid-cooldown nor re-arm it
func TestALateOutcomeDoesNotMoveAnOpenCircuit(t *testing.T) {
	b, clock := newTestBreaker(2, time.Minute)
	id := uuid.New()
	early, _ := b.Allow(id)
	early2, _ := b.Allow(id)
	trip(t, b, id, 2)
	opened := *b.Info(id).OpenedAt

	early.Done(bg, true)
	if got := b.Info(id); got.State != Open {
		t.Fatalf("a late success moved the circuit to %s, want it to stay open", got.State)
	}
	clock.advance(30 * time.Second)
	early2.Done(bg, false)
	if got := b.Info(id); got.ConsecutiveFailures != 2 || !got.OpenedAt.Equal(opened) {
		t.Errorf("a late failure changed the circuit to %+v, want the trip untouched", got)
	}
}

// A ticket reports once - a second Done or a Cancel after it is a no-op
func TestATicketReportsOnce(t *testing.T) {
	b, clock := newTestBreaker(1, time.Minute)
	id := uuid.New()
	trip(t, b, id, 1)
	clock.advance(time.Minute)
	probe, _ := b.Allow(id)
	probe.Done(bg, false)
	clock.advance(time.Minute)
	next, ok := b.Allow(id)
	if !ok {
		t.Fatal("no second probe was released")
	}
	probe.Done(bg, true)
	probe.Cancel()
	if got := b.Info(id); got.State != HalfOpen {
		t.Errorf("a spent ticket moved the circuit to %s, want the second probe still holding it", got.State)
	}
	next.Done(bg, true)
	if got := b.Info(id); got.State != Closed {
		t.Errorf("the live probe's success left the circuit %s, want closed", got.State)
	}
}

// An ordinary ticket reported twice still counts once
func TestAnOrdinaryTicketCountsOnce(t *testing.T) {
	b, _ := newTestBreaker(3, time.Minute)
	id := uuid.New()
	tk, _ := b.Allow(id)
	tk.Done(bg, false)
	tk.Done(bg, false)
	if got := b.Info(id); got.ConsecutiveFailures != 1 {
		t.Errorf("failures = %d, want the one report", got.ConsecutiveFailures)
	}
}

// A reset while a probe is in flight wins - the stale probe can't re-open the freshly closed circuit
func TestAProbeOutlivedByAResetIsIgnored(t *testing.T) {
	b, clock := newTestBreaker(3, time.Minute)
	id := uuid.New()
	trip(t, b, id, 3)
	clock.advance(time.Minute)
	probe, _ := b.Allow(id)
	b.Reset(id)
	call(t, b, id, false)
	probe.Done(bg, false)
	if got := b.Info(id); got.State != Closed || got.ConsecutiveFailures != 1 {
		t.Errorf("a stale probe counted against the fresh circuit: %+v", got)
	}
}

// Info is what the admin UI renders, so a circuit whose cooldown has quietly elapsed must read half-open rather than open, even though no probe has been claimed yet
func TestInfoReportsHalfOpenOnceTheCooldownElapses(t *testing.T) {
	b, clock := newTestBreaker(1, time.Minute)
	id := uuid.New()
	trip(t, b, id, 1)
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
	trip(t, b, id, 2)
	if allows(b, id) {
		t.Fatal("circuit did not open")
	}
	b.Reset(id)
	if !allows(b, id) {
		t.Error("Allow after Reset = false, want the circuit closed")
	}
	if got := b.Info(id); got.State != Closed || got.ConsecutiveFailures != 0 {
		t.Errorf("Info after Reset = %+v, want a zero closed circuit", got)
	}
}

// One bad target must not gate the others - state is per-id
func TestCircuitsAreIndependentPerTarget(t *testing.T) {
	b, _ := newTestBreaker(2, time.Minute)
	dead, healthy := uuid.New(), uuid.New()
	trip(t, b, dead, 2)
	if allows(b, dead) {
		t.Fatal("the failing target did not open")
	}
	if !allows(b, healthy) {
		t.Error("an unrelated target was gated by another target's failures")
	}
}

// Threshold 1 trips on the first failure, and a threshold below one behaves the same rather than tripping on nothing
func TestLowThresholdsTripOnTheFirstFailure(t *testing.T) {
	for _, threshold := range []int{1, 0, -3} {
		b, _ := newTestBreaker(threshold, time.Minute)
		id := uuid.New()
		if !allows(b, id) {
			t.Errorf("threshold %d: a fresh target was refused", threshold)
		}
		call(t, b, id, false)
		if allows(b, id) {
			t.Errorf("threshold %d: Allow after one failure = true, want the circuit open", threshold)
		}
	}
}

// Every caller shares one breaker across goroutines - run under -race
func TestConcurrentUseIsSafe(t *testing.T) {
	b, _ := newTestBreaker(3, time.Millisecond)
	ids := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			id := ids[i%len(ids)]
			for range 200 {
				if tk, ok := b.Allow(id); ok {
					tk.Done(bg, i%2 == 0)
				}
				b.Info(id)
			}
		})
	}
	wg.Wait()
}
