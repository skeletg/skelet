package lens

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
)

// The spine as Lens sees it - appends and identity stitches captured, and a grid read that can be made to fail
type fakeSpine struct {
	analytics.SpineStore
	mu         sync.Mutex
	events     []analytics.SpineEvent
	identities []uuid.UUID
	gridErr    error
}

func (f *fakeSpine) AppendEvent(_ context.Context, e analytics.SpineEvent) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, e)
	return true, nil
}

func (f *fakeSpine) AppendEvents(ctx context.Context, es []analytics.SpineEvent) error {
	for _, e := range es {
		_, _ = f.AppendEvent(ctx, e)
	}
	return nil
}

func (f *fakeSpine) UpsertIdentity(_ context.Context, _ string, customerID, _ uuid.UUID) error {
	f.identities = append(f.identities, customerID)
	return nil
}

func (f *fakeSpine) LensPagesGridFromEvents(context.Context, uuid.UUID, time.Time) (map[string]analytics.LensGridTotals, error) {
	return nil, f.gridErr
}

func (f *fakeSpine) LensCellsFromEvents(context.Context, uuid.UUID, string, string, string, string, time.Time) ([]analytics.LensCellCount, error) {
	return nil, nil
}

func (f *fakeSpine) LensElementsFromEvents(context.Context, uuid.UUID, string, string, int32, time.Time) ([]analytics.LensElementCount, error) {
	return nil, nil
}

func (f *fakeSpine) named(name string) []analytics.SpineEvent {
	var out []analytics.SpineEvent
	for _, e := range f.events {
		if e.EventName == name {
			out = append(out, e)
		}
	}
	return out
}

func TestIdentifyStitchesOnlyTheVerifiedSession(t *testing.T) {
	spine := &fakeSpine{}
	svc := NewService(nil, analytics.NewRecorder(nil, spine), nil, nil)
	identify := Batch{AnonID: "anon-1", Events: []Event{{Type: analytics.EventIdentify}}}
	svc.Ingest(context.Background(), uuid.New(), "anon-1", analytics.Hit{}, identify, uuid.Nil)
	if len(spine.identities) != 0 {
		t.Fatal("an identify with no verified session stitched the visitor")
	}
	cust := uuid.New()
	svc.Ingest(context.Background(), uuid.New(), "anon-1", analytics.Hit{}, identify, cust)
	if len(spine.identities) != 1 || spine.identities[0] != cust {
		t.Fatalf("stitched %v, want the verified customer", spine.identities)
	}
}

func TestEventsWithoutAPagePathNeverReachTheSpine(t *testing.T) {
	spine := &fakeSpine{}
	svc := NewService(nil, analytics.NewRecorder(nil, spine), nil, nil)
	svc.Ingest(context.Background(), uuid.New(), "anon-1", analytics.Hit{}, Batch{Events: []Event{
		{Type: analytics.EventPageView, PageKey: "/ok?utm=1", Viewport: "mobile"},
		{Type: analytics.EventPageView, PageKey: "https://evil.example/"},
		{Type: analytics.EventPageView, PageKey: "/" + strings.Repeat("a", 600)},
		{Type: "click", PageKey: "/ok", Viewport: "fridge", X: 5, Y: 5},
	}}, uuid.Nil)
	views := spine.named(analytics.EventPageView)
	if len(views) != 1 || views[0].Props["page_key"] != "/ok" {
		t.Fatalf("page views on the spine = %v, want only /ok, query stripped", views)
	}
	if cells := spine.named("lens_cell"); len(cells) != 0 {
		t.Errorf("a click with an unknown viewport became a heatmap cell: %v", cells)
	}
}

func TestCleanBoundsEveryTrackerField(t *testing.T) {
	e := Event{
		Type: "click", PageKey: "/p", Viewport: "desktop",
		ID: strings.Repeat("i", 200), Title: strings.Repeat("t", 1000), Referrer: strings.Repeat("r", 2000),
		Selector: strings.Repeat("s", 1000), Text: "a" + strings.Repeat("ž", 200), Name: strings.Repeat("n", 200),
		State: strings.Repeat("x", 200), LayoutHash: "../../etc", Props: map[string]any{"big": strings.Repeat("p", 5000)},
	}
	if !clean(&e) {
		t.Fatal("a click on a real page was dropped")
	}
	for name, got := range map[string]int{"id": len(e.ID), "title": len(e.Title), "referrer": len(e.Referrer), "selector": len(e.Selector), "text": len(e.Text), "name": len(e.Name), "state": len(e.State)} {
		if got > maxReferrer {
			t.Errorf("%s kept %d bytes", name, got)
		}
	}
	if !utf8.ValidString(e.Text) || len(e.Text) > maxText {
		t.Errorf("text = %d bytes, valid UTF-8 %v", len(e.Text), utf8.ValidString(e.Text))
	}
	if e.LayoutHash != "" || e.Props != nil {
		t.Errorf("layout hash %q and props %v should be dropped", e.LayoutHash, e.Props)
	}
	if !clean(&Event{Type: analytics.EventIdentify}) {
		t.Error("identify carries no page and must still pass")
	}
}

func TestBrowserTimesDontPickTheRollupDay(t *testing.T) {
	spine := &fakeSpine{}
	svc := NewService(nil, analytics.NewRecorder(nil, spine), nil, nil)
	svc.Ingest(context.Background(), uuid.New(), "anon-1", analytics.Hit{}, Batch{Events: []Event{
		{ID: "e1", Type: analytics.EventPageView, PageKey: "/", Viewport: "desktop", TS: time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()},
	}}, uuid.Nil)
	for _, e := range spine.events {
		if e.OccurredAt.Year() == 2001 {
			t.Errorf("%s kept its browser-claimed day", e.EventName)
		}
	}
}

func TestOverviewSurfacesAFailedRead(t *testing.T) {
	conn := lensDB(t)
	spine := &fakeSpine{gridErr: errors.New("clickhouse down")}
	svc := NewService(signalsdbNew(conn), analytics.NewRecorder(nil, spine), nil, nil)
	if _, err := svc.Overview(context.Background(), uuid.New(), time.Now().AddDate(0, 0, -7)); err == nil {
		t.Fatal("the overview answered zeros while the grid read failed")
	}
}
