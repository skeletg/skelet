package search

import (
	"sync"
	"time"
)

const (
	breakerWindow      = 10 * time.Second
	breakerMinFailures = 5
	breakerCooldown    = 5 * time.Second
)

// Closed counts primary outcomes per window and trips once failures reach the floor and outnumber successes - open skips the primary for the cooldown, then a single probe decides
type breaker struct {
	mu          sync.Mutex
	now         func() time.Time
	windowStart time.Time
	ok, failed  int
	openUntil   time.Time
	probing     bool
}

func newBreaker() *breaker { return &breaker{now: time.Now} }

func (b *breaker) allow() (try, probe bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.openUntil.IsZero() {
		return true, false
	}
	if b.probing || b.now().Before(b.openUntil) {
		return false, false
	}
	b.probing = true
	return true, true
}

func (b *breaker) record(probe, failed bool) (tripped, recovered bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.now()
	if probe {
		b.probing = false
		if failed {
			b.openUntil = now.Add(breakerCooldown)
			return false, false
		}
		b.openUntil = time.Time{}
		b.windowStart, b.ok, b.failed = now, 0, 0
		return false, true
	}
	if !b.openUntil.IsZero() {
		return false, false
	}
	if now.Sub(b.windowStart) > breakerWindow {
		b.windowStart, b.ok, b.failed = now, 0, 0
	}
	if failed {
		b.failed++
	} else {
		b.ok++
	}
	if b.failed >= breakerMinFailures && b.failed >= b.ok {
		b.openUntil = now.Add(breakerCooldown)
		return true, false
	}
	return false, false
}

func (b *breaker) release(probe bool) {
	if !probe {
		return
	}
	b.mu.Lock()
	b.probing = false
	b.mu.Unlock()
}
