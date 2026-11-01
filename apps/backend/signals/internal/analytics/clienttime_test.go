package analytics

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

type appendSpine struct {
	SpineStore
	got []SpineEvent
}

func (s *appendSpine) AppendEvent(_ context.Context, e SpineEvent) (bool, error) {
	s.got = append(s.got, e)
	return true, nil
}

func (s *appendSpine) AppendEvents(_ context.Context, es []SpineEvent) error {
	s.got = append(s.got, es...)
	return nil
}

// The spine partitions by event day - a browser clock outside the window must not pick the partition
func TestBrowserTimesAreHeldToTheServerClock(t *testing.T) {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	for name, tc := range map[string]struct{ in, want time.Time }{
		"recent":        {now.Add(-time.Hour), now.Add(-time.Hour)},
		"slightly fast": {now.Add(time.Minute), now.Add(time.Minute)},
		"zero":          {time.Time{}, now},
		"1970":          {time.UnixMilli(1), now},
		"two days old":  {now.Add(-48 * time.Hour), now},
		"far future":    {now.AddDate(10, 0, 0), now},
	} {
		if got := ClientTime(tc.in, now); !got.Equal(tc.want) {
			t.Errorf("%s: ClientTime(%v) = %v, want %v", name, tc.in, got, tc.want)
		}
	}

	spine := &appendSpine{}
	r := NewRecorder(nil, spine)
	ancient := time.Date(1999, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := r.RecordWeb(context.Background(), WebSpec{EventName: EventPageView, ChannelID: uuid.New(), AnonID: "a", EventID: "e", At: ancient}); err != nil {
		t.Fatal(err)
	}
	if err := r.RecordLensGrid(context.Background(), uuid.New(), "k", ancient, []LensCellIncrement{{PageKey: "/", Kind: "click", N: 1}}, nil); err != nil {
		t.Fatal(err)
	}
	for _, e := range spine.got {
		if e.OccurredAt.Year() == 1999 {
			t.Errorf("%s landed on its browser-claimed day %v", e.EventName, e.OccurredAt)
		}
	}
}

// An aggregate event is stored as nobody, carrying only amounts and items - whatever the caller put in it
func TestAnAggregateActivityIsStoredAsNobody(t *testing.T) {
	spine := &appendSpine{}
	r := NewRecorder(nil, spine)
	cust := uuid.New()
	ok, err := r.RecordActivity(context.Background(), ActivitySpec{
		EventName: "order_placed", Source: SourceAggregate, ChannelID: uuid.New(), CustomerID: &cust, AnonID: "visitor-1",
		Props: map[string]any{"total_cents": int64(900), "currency": "EUR", "order_id": "o-1", "email": "a@example.com"},
	})
	if err != nil || !ok || len(spine.got) != 1 {
		t.Fatalf("recorded %v, %v, %d rows", ok, err, len(spine.got))
	}
	e := spine.got[0]
	if e.CustomerID != nil || e.AnonID != "" || e.Source != SourceAggregate {
		t.Errorf("an aggregate event kept an actor: %+v", e)
	}
	if len(e.Props) != 2 || e.Props["total_cents"] != int64(900) || e.Props["currency"] != "EUR" {
		t.Errorf("props %v, want only the amount and currency", e.Props)
	}
	if ok, _ := r.RecordActivity(context.Background(), ActivitySpec{EventName: "cart_add", Source: SourceCommerce, ChannelID: uuid.New()}); ok || len(spine.got) != 1 {
		t.Error("an event with no actor that isn't marked aggregate must still be dropped")
	}
}
