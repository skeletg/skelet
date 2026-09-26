package spinech

import (
	"context"
	"math/rand/v2"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
)

// On the same events the ClickHouse reads must give exactly what the walkers give
func TestSequenceReadsMatchTheWalkers(t *testing.T) {
	ctx := context.Background()
	st := testStore(t)
	defer st.Close()
	ch := uuid.New()
	now := time.Now().UTC().Truncate(time.Millisecond)
	since := now.AddDate(0, 0, -14)
	rng := rand.New(rand.NewPCG(7, 11))
	names := []string{"page_view", "page_view", "add_to_cart", "purchase", "custom:search", "custom:wishlist"}

	var walked []analytics.SpineActorEvent
	for a := range 40 {
		anon := "anon-" + uuid.NewString()
		at := since.Add(time.Duration(rng.IntN(3*24)) * time.Hour)
		for range 3 + rng.IntN(12) {
			at = at.Add(time.Duration(1+rng.IntN(40*60)) * time.Minute).Add(time.Duration(a+1) * time.Millisecond) // strictly increasing per actor, so no ties to order
			if !at.Before(now) {
				break
			}
			name := names[rng.IntN(len(names))]
			e := analytics.SpineEvent{EventName: name, OccurredAt: at, ChannelID: &ch, AnonID: anon, Source: "web", DedupeKey: uuid.NewString()}
			resolved := name
			if len(name) > 7 && name[:7] == "custom:" {
				e.EventName, e.Props, resolved = "custom", map[string]any{"name": name[7:]}, name[7:]
			}
			if _, err := st.AppendEvent(ctx, e); err != nil {
				t.Fatal(err)
			}
			walked = append(walked, analytics.SpineActorEvent{Actor: anon, EventName: resolved, OccurredAt: at})
		}
	}
	if err := st.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	sort.SliceStable(walked, func(i, j int) bool {
		if walked[i].Actor != walked[j].Actor {
			return walked[i].Actor < walked[j].Actor
		}
		return walked[i].OccurredAt.Before(walked[j].OccurredAt)
	})

	for _, tc := range []struct{ start, ret string }{{"page_view", "purchase"}, {"page_view", "page_view"}, {"add_to_cart", "purchase"}} {
		for _, unit := range []time.Duration{24 * time.Hour, 7 * 24 * time.Hour} {
			got, err := st.Retention(ctx, ch, tc.start, tc.ret, since, now, 5, unit)
			if err != nil {
				t.Fatalf("retention %s->%s: %v", tc.start, tc.ret, err)
			}
			var filtered []analytics.SpineActorEvent
			for _, e := range walked {
				if e.EventName == tc.start || e.EventName == tc.ret {
					filtered = append(filtered, e)
				}
			}
			want := analytics.RetentionFromEvents(filtered, tc.start, tc.ret, now, 5, unit)
			normalizeRetention(&got)
			normalizeRetention(&want)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("retention %s->%s per %v:\n got  %+v\n want %+v", tc.start, tc.ret, unit, got, want)
			}
		}
	}

	for _, focus := range []string{"page_view", "add_to_cart", "search"} {
		for _, depth := range []int{1, 3} {
			got, err := st.Flow(ctx, ch, focus, since, depth, 3)
			if err != nil {
				t.Fatalf("flow %s: %v", focus, err)
			}
			want := analytics.FlowFromEvents(walked, focus, depth, 3)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("flow from %s at depth %d:\n got  %+v\n want %+v", focus, depth, got, want)
			}
		}
	}
}

// One side comes from Unix millis, the other from a truncated time.Time
func normalizeRetention(r *analytics.Retention) {
	for i := range r.Cohorts {
		r.Cohorts[i].StartAt = r.Cohorts[i].StartAt.UTC()
	}
	if len(r.Cohorts) == 0 {
		r.Cohorts = nil
	}
}
