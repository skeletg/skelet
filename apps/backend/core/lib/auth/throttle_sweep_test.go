package auth

import (
	"testing"
	"time"
)

// The sweeper is the only thing bounding the fallback map, and it runs on a goroutine nothing else joins - so it has
// to both evict what has aged out and return when its owner is done, or every throttle built leaks one forever

func TestSweepEvictsOnlyEntriesPastBothWindows(t *testing.T) {
	past, future := time.Now().Add(-time.Hour), time.Now().Add(time.Hour)
	for name, tc := range map[string]struct {
		entry     *memThrottleEntry
		wantSwept bool
	}{
		"fails and lock both expired":  {&memThrottleEntry{failReset: past, lockUntil: past}, true},
		"still inside the fail window": {&memThrottleEntry{failReset: future, lockUntil: past}, false},
		"still locked out":             {&memThrottleEntry{failReset: past, lockUntil: future}, false},
		"locked and counting":          {&memThrottleEntry{failReset: future, lockUntil: future}, false},
	} {
		t.Run(name, func(t *testing.T) {
			m := newMemThrottle(3, time.Millisecond, time.Millisecond)
			m.entries["someone@example.invalid"] = tc.entry

			stop := make(chan struct{})
			done := make(chan struct{})
			go func() { defer close(done); m.sweep(stop) }()
			t.Cleanup(func() {
				close(stop)
				<-done
			})

			deadline := time.After(2 * time.Second)
			for {
				m.mu.Lock()
				n := len(m.entries)
				m.mu.Unlock()
				if (n == 0) == tc.wantSwept {
					return
				}
				select {
				case <-deadline:
					t.Fatalf("entries = %d after the sweep window, wantSwept %v", n, tc.wantSwept)
				case <-time.After(2 * time.Millisecond):
				}
			}
		})
	}
}

func TestSweepReturnsWhenStopped(t *testing.T) {
	// A throttle built per request or per test would otherwise pin a ticker goroutine for the life of the process
	m := newMemThrottle(3, time.Hour, time.Hour) // a long tick, so only stop can end this
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() { defer close(done); m.sweep(stop) }()

	close(stop)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("sweep ignored its stop channel; the goroutine outlives its owner")
	}
}

func TestSweepWithoutAStopChannelKeepsRunning(t *testing.T) {
	// nil is what the boot-time throttle passes, and it must not read as "stop immediately"
	m := newMemThrottle(3, time.Hour, time.Hour)
	done := make(chan struct{})
	go func() { defer close(done); m.sweep(nil) }()

	select {
	case <-done:
		t.Fatal("a nil stop ended the sweeper, so the fallback map would grow unbounded")
	case <-time.After(50 * time.Millisecond):
	}
}
