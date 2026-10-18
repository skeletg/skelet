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
