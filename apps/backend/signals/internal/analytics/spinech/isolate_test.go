package spinech

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
)

// A ClickHouse that refuses any block holding a poisoned event, and can be taken down whole
type refusingCH struct {
	down   bool
	writes int
	stored []string
}

func (c *refusingCH) write(_ context.Context, events []bufEvent) error {
	c.writes++
	if c.down {
		return errors.New("connection refused")
	}
	for _, e := range events {
		if strings.HasPrefix(e.e.DedupeKey, "poison") {
			return errors.New("too many partitions for single INSERT block")
		}
	}
	for _, e := range events {
		c.stored = append(c.stored, e.e.DedupeKey)
	}
	return nil
}

func bufferedStore(ch *refusingCH, keys ...string) *Store {
	s := &Store{seen: map[string]struct{}{}, seenRing: make([]string, 64), write: ch.write}
	for _, k := range keys {
		s.markSeen(k)
		s.buf = append(s.buf, bufEvent{e: analytics.SpineEvent{DedupeKey: k}, props: "{}"})
	}
	return s
}

func TestARefusedEventIsDroppedWithoutWedgingTheBatch(t *testing.T) {
	ch := &refusingCH{}
	keys := []string{"a", "b", "c", "poison-1", "d", "e", "f", "g"}
	s := bufferedStore(ch, keys...)
	err := s.flushEvents(context.Background())
	if err == nil || !strings.Contains(err.Error(), "refused 1 of 8") {
		t.Fatalf("flush err = %v, want the one refused event reported", err)
	}
	if len(ch.stored) != 7 || len(s.buf) != 0 {
		t.Fatalf("stored %v, %d left buffered - want the seven good events written and nothing re-queued", ch.stored, len(s.buf))
	}
	if _, ok := s.seen["poison-1"]; ok {
		t.Error("the dropped event's dedupe key is still remembered, so a fixed retry would be swallowed")
	}
}

func TestAnOutageReBuffersTheWholeBatch(t *testing.T) {
	ch := &refusingCH{down: true}
	s := bufferedStore(ch, "a", "b", "c", "d")
	if err := s.flushEvents(context.Background()); err == nil {
		t.Fatal("an outage flushed without error")
	}
	if len(s.buf) != 4 || len(ch.stored) != 0 {
		t.Fatalf("buffered %d after an outage, want all 4 kept for the next tick", len(s.buf))
	}
	if ch.writes > chFlushRetry+2 {
		t.Errorf("an outage cost %d writes - splitting should stop once both halves fail", ch.writes)
	}
}
