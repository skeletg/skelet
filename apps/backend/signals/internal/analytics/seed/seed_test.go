package seed

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
)

type capture struct {
	events []analytics.SpineEvent
	idents map[string][]uuid.UUID
}

func (c *capture) Event(e analytics.SpineEvent) error { c.events = append(c.events, e); return nil }

func (c *capture) Identify(anon string, cid, _ uuid.UUID) error {
	if c.idents == nil {
		c.idents = map[string][]uuid.UUID{}
	}
	c.idents[anon] = append(c.idents[anon], cid)
	return nil
}

var (
	now = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	ch  = uuid.MustParse("0b6f2c1e-6f3a-4d3e-9a51-2a4a8c1f0e01")
)

func run(t *testing.T, cfg Config) *capture {
	t.Helper()
	c := &capture{}
	n, err := Generate(ch, cfg, c)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(c.events) {
		t.Fatalf("reported %d events, emitted %d", n, len(c.events))
	}
	return c
}

func TestGenerateIsDeterministic(t *testing.T) {
	cfg := Config{Seed: 7, Now: now, Days: 30, Customers: 20}
	a, b := run(t, cfg), run(t, cfg)
	if len(a.events) != len(b.events) {
		t.Fatalf("event counts differ: %d vs %d", len(a.events), len(b.events))
	}
	for i := range a.events {
		if a.events[i].DedupeKey != b.events[i].DedupeKey || a.events[i].EventName != b.events[i].EventName ||
			!a.events[i].OccurredAt.Equal(b.events[i].OccurredAt) || a.events[i].AnonID != b.events[i].AnonID {
			t.Fatalf("event %d differs: %+v vs %+v", i, a.events[i], b.events[i])
		}
	}
	other := run(t, Config{Seed: 8, Now: now, Days: 30, Customers: 20})
	if len(other.events) > 0 && other.events[0].AnonID == a.events[0].AnonID {
		t.Fatal("a different seed produced the same visitors")
	}
}

func TestGenerateCoversEveryBoardArea(t *testing.T) {
	c := run(t, Config{Seed: 1, Now: now, Days: 90, Customers: 100})
	seen := map[string]int{}
	start := now.Add(-90 * 24 * time.Hour)
	for _, e := range c.events {
		seen[e.EventName+"/"+e.Source]++
		if !strings.HasPrefix(e.DedupeKey, KeyPrefix) {
			t.Fatalf("unmarked dedupe key %q", e.DedupeKey)
		}
		if e.AnonID != "" && !strings.HasPrefix(e.AnonID, AnonPrefix) {
			t.Fatalf("unmarked visitor id %q", e.AnonID)
		}
		if e.ChannelID == nil || *e.ChannelID != ch {
			t.Fatal("event without the shop's channel")
		}
		if e.OccurredAt.After(now) || e.OccurredAt.Before(start) {
			t.Fatalf("event outside the window: %s", e.OccurredAt)
		}
		if e.Source == analytics.SourceAggregate && (e.CustomerID != nil || e.AnonID != "") {
			t.Fatal("an aggregate event carries an actor")
		}
	}
	for _, want := range []string{
		"page_view/web", "view_item/web", "cart_add/commerce", "begin_checkout/web", "order_placed/commerce", // shop health, product usage
		"email_sent/campaign", "email_opened/campaign", "email_clicked/campaign", "conversion/campaign", // email impact
		"page_view/aggregate", "identify/web",
	} {
		if seen[want] == 0 {
			t.Errorf("no %s events", want)
		}
	}

	orders := map[uuid.UUID]int{}
	for _, e := range c.events {
		if e.EventName == "order_placed" && e.CustomerID != nil {
			orders[*e.CustomerID]++
		}
	}
	repeat := 0
	for _, n := range orders {
		if n > 1 {
			repeat++
		}
	}
	if repeat == 0 {
		t.Error("no repeat customers") // revenue and repeat
	}

	shared, devices := 0, map[uuid.UUID]int{}
	for _, cids := range c.idents {
		distinct := map[uuid.UUID]bool{}
		for _, id := range cids {
			distinct[id] = true
		}
		if len(distinct) > 1 {
			shared++
		}
		for id := range distinct {
			devices[id]++
		}
	}
	if shared != 1 {
		t.Errorf("want one shared device, got %d", shared)
	}
	multi := 0
	for _, n := range devices {
		if n > 1 {
			multi++
		}
	}
	if multi == 0 {
		t.Error("no customer on more than one device")
	}
}

func TestGenerateReachesTheLoadSet(t *testing.T) {
	c := run(t, Config{Seed: 1, Now: now, Days: 30, Customers: 5, Events: 20000})
	if len(c.events) < 20000 {
		t.Fatalf("want at least 20000 events, got %d", len(c.events))
	}
}
