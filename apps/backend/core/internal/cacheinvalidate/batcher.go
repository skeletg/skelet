package cacheinvalidate

import (
	"context"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"
)

// How long purges gather before one send per surface - a bulk import or restock fires thousands of events, and without this each is its own log row and HTTP round-trip to every storefront, most of them the same catalog:browse tag
var batchWindow = time.Second

const maxReasons = 5 // a merged purge names a few of its causes, not all of them

// Gathers purges per surface over a short window and sends each surface's merged tags and paths once
type Batcher struct {
	purger Purger
	window time.Duration

	mu      sync.Mutex
	pending map[string]*pendingPurge
	timer   *time.Timer
}

type pendingPurge struct {
	tags, paths map[string]struct{}
	reasons     []string
}

func NewBatcher(purger Purger, window time.Duration) *Batcher {
	return &Batcher{purger: purger, window: window, pending: map[string]*pendingPurge{}}
}

func (b *Batcher) Purge(surface string, tags, paths []string, reason string) {
	if len(tags) == 0 && len(paths) == 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	p := b.pending[surface]
	if p == nil {
		p = &pendingPurge{tags: map[string]struct{}{}, paths: map[string]struct{}{}}
		b.pending[surface] = p
	}
	for _, t := range tags {
		p.tags[t] = struct{}{}
	}
	for _, path := range paths {
		p.paths[path] = struct{}{}
	}
	if len(p.reasons) < maxReasons && !slices.Contains(p.reasons, reason) {
		p.reasons = append(p.reasons, reason)
	}
	if b.timer == nil {
		b.timer = time.AfterFunc(b.window, func() { b.Flush(context.Background()) }) // detached: the events that queued these are long done
	}
}

// Sends everything pending now - the timer calls it, and shutdown does too so a purge queued in the last second isn't lost
func (b *Batcher) Flush(ctx context.Context) {
	b.mu.Lock()
	pending := b.pending
	b.pending = map[string]*pendingPurge{}
	if b.timer != nil {
		b.timer.Stop()
		b.timer = nil
	}
	b.mu.Unlock()

	for _, surface := range slices.Sorted(maps.Keys(pending)) {
		p := pending[surface]
		b.purger.Purge(ctx, surface, slices.Sorted(maps.Keys(p.tags)), slices.Sorted(maps.Keys(p.paths)), strings.Join(p.reasons, ", "), nil)
	}
}
