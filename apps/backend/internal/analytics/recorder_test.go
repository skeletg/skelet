package analytics

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// Every write on a disabled Recorder must no-op BEFORE it touches the spine or commerce handles - the nil handles here panic if any guard goes missing
func TestRecorderDisabledNoOps(t *testing.T) {
	ctx := context.Background()
	r := NewRecorder(nil, nil)
	r.SetEnabled(false)
	cid := uuid.New()

	if ok, err := r.RecordWeb(ctx, WebSpec{EventName: EventPageView, ChannelID: uuid.New(), AnonID: "a", EventID: "e"}); ok || err != nil {
		t.Fatalf("RecordWeb = %v, %v, want false, nil", ok, err)
	}
	if ok, err := r.RecordActivity(ctx, ActivitySpec{EventName: "order_placed", Source: SourceCommerce, CustomerID: &cid, DedupeKey: "act:x"}); ok || err != nil {
		t.Fatalf("RecordActivity = %v, %v, want false, nil", ok, err)
	}
	if err := r.RecordSent(ctx, SendRef{ID: uuid.New(), ChannelID: uuid.New()}); err != nil {
		t.Fatalf("RecordSent = %v, want nil", err)
	}
	if err := r.Identify(ctx, "anon", cid, uuid.New()); err != nil {
		t.Fatalf("Identify = %v, want nil", err)
	}
	if ok, err := r.RecordOpen(ctx, SendRef{ID: uuid.New()}, Hit{}); ok || err != nil {
		t.Fatalf("RecordOpen = %v, %v, want false, nil", ok, err)
	}
	if ok, err := r.RecordClick(ctx, SendRef{ID: uuid.New()}, "https://x", 0, Hit{}); ok || err != nil {
		t.Fatalf("RecordClick = %v, %v, want false, nil", ok, err)
	}
	if ok, err := r.RecordConversion(ctx, ConversionSpec{CustomerID: cid, OrderID: uuid.New()}); ok || err != nil {
		t.Fatalf("RecordConversion = %v, %v, want false, nil", ok, err)
	}
}
