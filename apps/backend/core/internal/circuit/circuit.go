// Package circuit is a minimal in-memory circuit breaker keyed by UUID, shared by the outbound callers - sync webhooks, remote providers and Signals segment reads
// A target that keeps failing stops being called for a cooldown window instead of adding its timeout to every operation it takes part in
// State is per-process, so after a restart every circuit starts closed - the safe default, costing one extra probe against a dead endpoint rather than risking a stuck-open gate
package circuit

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

// The pair sync webhooks and remote providers share - a target is out after five straight failures, for half a minute
const (
	DefaultThreshold = 5
	DefaultCooldown  = 30 * time.Second
)

type State string

const (
	Closed   State = "closed"
	Open     State = "open"
	HalfOpen State = "half_open"
)

type Info struct {
	State               State
	ConsecutiveFailures int
	OpenedAt            *time.Time
}

type entry struct {
	fails    int
	open     bool
	openedAt time.Time
	probe    uint64 // the ticket holding the half-open slot, 0 when none is in flight
}

type Breaker struct {
	mu        sync.Mutex
	m         map[uuid.UUID]*entry
	threshold int
	cooldown  time.Duration
	now       func() time.Time
	seq       uint64
}

// Builds a breaker that trips after threshold consecutive failures and blocks calls for cooldown before allowing a probe - a threshold below one trips on the first failure
func New(threshold int, cooldown time.Duration) *Breaker {
	return &Breaker{
		m:         map[uuid.UUID]*entry{},
		threshold: threshold,
		cooldown:  cooldown,
		now:       time.Now,
	}
}

func (b *Breaker) SetNow(fn func() time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.now = fn
}

// One admitted call - its outcome goes back through Done or Cancel exactly once, and only the half-open probe's outcome can close or re-open an open circuit
type Ticket struct {
	b     *Breaker
	id    uuid.UUID
	probe uint64
	used  bool
}

// Whether a call to this target may proceed - an open circuit refuses until the cooldown elapses, then admits exactly one probe whose outcome closes or re-opens it
func (b *Breaker) Allow(id uuid.UUID) (*Ticket, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	t := &Ticket{b: b, id: id}
	e := b.m[id]
	if e == nil || !e.open {
		return t, true
	}
	if e.probe != 0 || b.now().Sub(e.openedAt) < b.cooldown {
		return nil, false
	}
	b.seq++
	e.probe = b.seq
	t.probe = b.seq
	return t, true
}

// Records the call's outcome, unless ctx says the caller walked away - a cancelled or expired caller says nothing about the target, so the call counts as neither and a probe hands its slot back
// Reports whether the circuit changed between closed and not closed
func (t *Ticket) Done(ctx context.Context, success bool) bool {
	if ctx.Err() != nil {
		t.Cancel()
		return false
	}
	if t == nil || t.used {
		return false
	}
	t.used = true
	b := t.b
	b.mu.Lock()
	defer b.mu.Unlock()
	e := b.m[t.id]
	wasClosed := e == nil || !e.open
	switch {
	case t.probe != 0:
		if e == nil || e.probe != t.probe {
			return false // reset or superseded while in flight
		}
		if success {
			delete(b.m, t.id)
			return true
		}
		e.probe = 0
		e.fails++
		e.openedAt = b.now()
		return false
	case !wasClosed:
		return false // admitted before the circuit tripped - only the probe speaks for an open circuit
	case success:
		delete(b.m, t.id)
		return false
	}
	if e == nil {
		e = &entry{}
		b.m[t.id] = e
	}
	e.fails++
	if e.fails >= b.threshold {
		e.open = true
		e.openedAt = b.now()
		return true
	}
	return false
}

// Releases the ticket without an outcome - a probe frees the half-open slot so the next caller probes instead
func (t *Ticket) Cancel() {
	if t == nil || t.used {
		return
	}
	t.used = true
	if t.probe == 0 {
		return
	}
	b := t.b
	b.mu.Lock()
	defer b.mu.Unlock()
	if e := b.m[t.id]; e != nil && e.probe == t.probe {
		e.probe = 0
	}
}

// Closes the circuit unconditionally - call when an admin edits, re-activates or deletes the target, so a fixed endpoint doesn't sit out a cooldown
func (b *Breaker) Reset(id uuid.UUID) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.m, id)
}

func (b *Breaker) Info(id uuid.UUID) Info {
	b.mu.Lock()
	defer b.mu.Unlock()
	e := b.m[id]
	if e == nil {
		return Info{State: Closed}
	}
	out := Info{State: Closed, ConsecutiveFailures: e.fails}
	if e.open {
		t := e.openedAt
		out.OpenedAt = &t
		out.State = Open
		if e.probe != 0 || b.now().Sub(e.openedAt) >= b.cooldown {
			out.State = HalfOpen
		}
	}
	return out
}
