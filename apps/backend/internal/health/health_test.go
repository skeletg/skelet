package health

import (
	"context"
	"testing"
	"time"
)

func fixed() time.Time { return time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC) }

func static(r Result) CheckFunc { return func(context.Context) Result { return r } }

func TestRunOrdersAndSummarises(t *testing.T) {
	r := NewRegistry()
	r.RegisterGroup(Group{Key: "second", Label: "Second", Order: 2})
	r.RegisterGroup(Group{Key: "first", Label: "First", Order: 1})
	// Registered out of order on purpose - the panel's reading order is the
	// registration's Order, not whatever sequence wiring happened to run in
	r.Register(Check{Group: "second", Key: "b", Label: "B", Order: 2, Run: static(OK("fine"))})
	r.Register(Check{Group: "first", Key: "z", Label: "Z", Order: 2, Run: static(Off("not configured", ""))})
	r.Register(Check{Group: "first", Key: "a", Label: "A", Order: 1, Run: static(Warn("slow", "look at it"))})

	rep := r.Run(context.Background(), fixed())

	if len(rep.Groups) != 2 || rep.Groups[0].Key != "first" || rep.Groups[1].Key != "second" {
		t.Fatalf("groups out of order: %+v", rep.Groups)
	}
	if got := rep.Groups[0].Checks; len(got) != 2 || got[0].Key != "a" || got[1].Key != "z" {
		t.Fatalf("checks out of order: %+v", got)
	}
	if rep.Status != StatusWarn {
		t.Fatalf("overall = %s, want WARN (worst of OK/OFF/WARN)", rep.Status)
	}
	if !rep.CheckedAt.Equal(fixed()) {
		t.Fatalf("CheckedAt = %v, want the passed-in time", rep.CheckedAt)
	}
}

// OFF is a deployment choice, not a fault - an install running none of the optional
// tiers must not read as broken
func TestOffNeverEscalates(t *testing.T) {
	r := NewRegistry()
	r.Register(Check{Group: "g", Key: "off", Run: static(Off("not configured", ""))})
	if got := r.Run(context.Background(), fixed()).Status; got != StatusOff {
		t.Fatalf("all-off = %s, want OFF", got)
	}

	r.Register(Check{Group: "g", Key: "ok", Run: static(OK("fine"))})
	if got := r.Run(context.Background(), fixed()).Status; got != StatusOK {
		t.Fatalf("off+ok = %s, want OK", got)
	}

	r.Register(Check{Group: "g", Key: "bad", Run: static(Bad("down", "fix it"))})
	if got := r.Run(context.Background(), fixed()).Status; got != StatusBad {
		t.Fatalf("with a bad check = %s, want BAD", got)
	}
}

// An operator opens this page precisely when something is already wrong, so one
// broken subject must not take the panel with it
func TestOneBadProbeCannotBreakThePanel(t *testing.T) {
	r := NewRegistry()
	r.Register(Check{Group: "g", Key: "panics", Run: func(context.Context) Result { panic("boom") }})
	r.Register(Check{Group: "g", Key: "hangs", Run: func(ctx context.Context) Result {
		<-ctx.Done() // outlives its probe budget, as a wedged dependency would
		return OK("never gets here")
	}})
	r.Register(Check{Group: "g", Key: "healthy", Run: static(OK("fine"))})

	done := make(chan Report, 1)
	go func() { done <- r.Run(context.Background(), fixed()) }()
	select {
	case rep := <-done:
		byKey := map[string]Result{}
		for _, c := range rep.Groups[0].Checks {
			byKey[c.Key] = c.Result
		}
		if byKey["panics"].Status != StatusBad {
			t.Fatalf("panicking probe = %+v, want BAD", byKey["panics"])
		}
		if byKey["hangs"].Status != StatusBad {
			t.Fatalf("hanging probe = %+v, want BAD", byKey["hangs"])
		}
		if byKey["healthy"].Status != StatusOK {
			t.Fatalf("healthy probe was collateral damage: %+v", byKey["healthy"])
		}
	case <-time.After(probeTimeout + 5*time.Second):
		t.Fatal("Run never returned - a wedged probe blocked the whole panel")
	}
}

// Every open admin tab polls at 5s - without the memo each tab fires every probe
// (S3 Head, cluster ping, per-webhook queries) on its own schedule
func TestRunMemoizesWithinTheTTL(t *testing.T) {
	r := NewRegistry()
	probe := &countingProbe{}
	r.Register(Check{Group: "g", Key: "c", Run: probe.run})

	base := fixed()
	r.Run(context.Background(), base)
	r.Run(context.Background(), base.Add(time.Second))
	if probe.calls != 1 {
		t.Fatalf("probe ran %d times inside the TTL, want 1", probe.calls)
	}
	r.Run(context.Background(), base.Add(memoTTL+time.Second))
	if probe.calls != 2 {
		t.Fatalf("probe ran %d times after the TTL, want 2", probe.calls)
	}

	// A new registration must invalidate the memo, or the fresh check is invisible
	// for a TTL after wiring adds it
	r.Register(Check{Group: "g", Key: "c2", Run: static(OK("fine"))})
	rep := r.Run(context.Background(), base.Add(memoTTL+2*time.Second))
	if len(rep.Groups[0].Checks) != 2 {
		t.Fatalf("memo served a stale check list after Register: %+v", rep.Groups[0].Checks)
	}
}

// The memo must never cache a report shaped by one caller's dead context
func TestRunDetachesFromTheCallersCancellation(t *testing.T) {
	r := NewRegistry()
	r.Register(Check{Group: "g", Key: "c", Run: static(OK("fine"))})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rep := r.Run(ctx, fixed())
	if rep.Groups[0].Checks[0].Result.Status != StatusOK {
		t.Fatalf("cancelled caller poisoned the sweep: %+v", rep.Groups[0].Checks[0].Result)
	}
}

type countingProbe struct{ calls int }

func (p *countingProbe) run(context.Context) Result {
	p.calls++
	return OK("fine")
}
