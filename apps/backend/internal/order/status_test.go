package order

import (
	"testing"

	shipping "github.com/skeletg/skelet/internal/shipping"
)

func TestValidTransition(t *testing.T) {
	tests := []struct {
		from, to string
		want     bool
	}{
		{OrderStatusDraft, OrderStatusPending, true},    // finalize (placeDraftOrder)
		{OrderStatusDraft, OrderStatusCancelled, true},  // discard a draft
		{OrderStatusDraft, OrderStatusConfirmed, false}, // must be placed first
		{OrderStatusDraft, OrderStatusFulfilled, false}, // must be placed first
		{OrderStatusPending, OrderStatusDraft, false},   // no going back to draft
		{OrderStatusPending, OrderStatusConfirmed, true},
		{OrderStatusPending, OrderStatusCancelled, true},
		{OrderStatusPending, OrderStatusProcessing, false}, // must go via confirmed
		{OrderStatusPending, OrderStatusFulfilled, false},
		{OrderStatusConfirmed, OrderStatusProcessing, true},
		{OrderStatusConfirmed, OrderStatusCancelled, true},
		{OrderStatusConfirmed, OrderStatusFulfilled, false}, // must go via processing
		{OrderStatusProcessing, OrderStatusFulfilled, true},
		{OrderStatusProcessing, OrderStatusPartiallyFulfilled, true}, // first partial shipment
		{OrderStatusProcessing, OrderStatusCancelled, true},
		{OrderStatusProcessing, OrderStatusPending, false},                   // no going back
		{OrderStatusPartiallyFulfilled, OrderStatusPartiallyFulfilled, true}, // another partial shipment
		{OrderStatusPartiallyFulfilled, OrderStatusFulfilled, true},          // final shipment
		{OrderStatusPartiallyFulfilled, OrderStatusCancelled, true},          // cancel mid-fulfillment
		{OrderStatusPartiallyFulfilled, OrderStatusPending, false},           // no going back
		{OrderStatusFulfilled, OrderStatusPartiallyFulfilled, false},         // terminal
		{OrderStatusFulfilled, OrderStatusCancelled, false},                  // terminal
		{OrderStatusCancelled, OrderStatusPending, false},                    // terminal
		{OrderStatusPending, OrderStatusPending, false},                      // no self-transition
		{"bogus", OrderStatusConfirmed, false},                               // unknown from-status
	}
	for _, tt := range tests {
		t.Run(tt.from+"->"+tt.to, func(t *testing.T) {
			if got := validTransition(tt.from, tt.to); got != tt.want {
				t.Errorf("validTransition(%q, %q) = %v, want %v", tt.from, tt.to, got, tt.want)
			}
		})
	}
}

func TestOrderStatusConstants(t *testing.T) {
	// Stored string values - changing one is a data migration, so pin them
	for _, tt := range []struct{ got, want string }{
		{OrderStatusDraft, "draft"},
		{OrderStatusPending, "pending"},
		{OrderStatusConfirmed, "confirmed"},
		{OrderStatusProcessing, "processing"},
		{OrderStatusPartiallyFulfilled, "partially_fulfilled"},
		{OrderStatusFulfilled, "fulfilled"},
		{OrderStatusCancelled, "cancelled"},
		{OrderStatusOnHold, "on_hold"},
	} {
		if tt.got != tt.want {
			t.Errorf("status constant = %q, want %q", tt.got, tt.want)
		}
	}
	// The transition map must only reference known statuses
	known := map[string]bool{
		OrderStatusDraft: true, OrderStatusPending: true, OrderStatusConfirmed: true, OrderStatusProcessing: true,
		OrderStatusPartiallyFulfilled: true, OrderStatusFulfilled: true, OrderStatusCancelled: true,
		OrderStatusOnHold: true,
	}
	for from, tos := range orderStatusTransitions {
		if !known[from] {
			t.Errorf("orderStatusTransitions has unknown from-status %q", from)
		}
		for _, to := range tos {
			if !known[to] {
				t.Errorf("transition %q -> unknown status %q", from, to)
			}
		}
	}
}

func TestCheapestRatedQuote(t *testing.T) {
	rq := func(slug, code string, cents int64) shipping.RatedQuote {
		return shipping.RatedQuote{CarrierSlug: slug, Method: shipping.RatedMethod{Code: code, PriceCents: cents}}
	}
	tests := []struct {
		name     string
		quotes   []shipping.RatedQuote
		wantOK   bool
		wantCode string
		wantCent int64
	}{
		{"empty → not ok", nil, false, "", 0},
		{"single", []shipping.RatedQuote{rq("builtin", "std", 500)}, true, "std", 500},
		{"cheapest of many", []shipping.RatedQuote{rq("a", "x", 900), rq("b", "y", 350), rq("a", "z", 700)}, true, "y", 350},
		{"cheapest is first", []shipping.RatedQuote{rq("a", "x", 100), rq("b", "y", 800)}, true, "x", 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := cheapestRatedQuote(tt.quotes)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if got.Method.Code != tt.wantCode || got.Method.PriceCents != tt.wantCent {
				t.Errorf("got {%s,%d}, want {%s,%d}", got.Method.Code, got.Method.PriceCents, tt.wantCode, tt.wantCent)
			}
		})
	}
}
