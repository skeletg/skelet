// Package circuit is a minimal in-memory circuit breaker keyed by UUID, shared by the outbound-HTTP callers - sync webhooks and remote providers
// A target that keeps failing stops being called for a cooldown window instead of adding its timeout to every operation it takes part in
// State is per-process, so after a restart every circuit starts closed - the safe default, costing one extra probe against a dead endpoint rather than risking a stuck-open gate
package circuit

import (
	"sync"
	"time"

	"github.com/google/uuid"
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
	halfOpen bool // a probe is in flight
	openedAt time.Time
}

type Breaker struct {
	mu        sync.Mutex
	m         map[uuid.UUID]*entry
	threshold int
	cooldown  time.Duration
	now       func() time.Time
}

// Builds a breaker that trips after threshold consecutive failures and blocks calls for cooldown before allowing a probe
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

// Whether a call to this target may proceed - an open circuit blocks until the cooldown elapses, then lets exactly one half-open probe through, whose outcome closes or re-opens it
func (b *Breaker) Allow(id uuid.UUID) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	e := b.m[id]
	if e == nil || !e.open {
		return true
	}
	if e.halfOpen {
		return false
	}
	if b.now().Sub(e.openedAt) < b.cooldown {
		return false
	}
	e.halfOpen = true
	return true
}

// Feeds an attempt outcome back - success fully resets the circuit, while failure counts toward the trip threshold and re-arms the cooldown when the circuit is already open, meaning a failed probe
func (b *Breaker) Record(id uuid.UUID, success bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if success {
		delete(b.m, id)
		return
	}
	e := b.m[id]
	if e == nil {
		e = &entry{}
		b.m[id] = e
	}
	e.fails++
	e.halfOpen = false
	if e.open || e.fails >= b.threshold {
		e.open = true
		e.openedAt = b.now()
	}
}

// Closes the circuit unconditionally - call when an admin edits or re-activates the target, so a fixed endpoint doesn't sit out a cooldown
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
		if e.halfOpen || b.now().Sub(e.openedAt) >= b.cooldown {
			out.State = HalfOpen
		}
	}
	return out
}
