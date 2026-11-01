package order

import (
	"context"
	"maps"
	"net/http"
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func shopRequest(consent string) context.Context {
	r, _ := http.NewRequest(http.MethodPost, "/", nil)
	r.RemoteAddr = "203.0.113.7:4000"
	r.Header.Set("User-Agent", "Mozilla/5.0 test")
	r.Header.Set(auth.AnonHeader, "visitor-1")
	r.Header.Set(auth.ClickIDsHeader, "cjevent=c1")
	if consent != "" {
		r.Header.Set(auth.ConsentHeader, consent)
	}
	return auth.WithRequest(context.Background(), r)
}

func TestTrackingKeepsEachPartOnlyUnderItsConsent(t *testing.T) {
	if got := TrackingFromContext(shopRequest("")); got != nil {
		t.Fatalf("no stated consent: %+v, want nil", got)
	}
	none := TrackingFromContext(shopRequest("v2:a0.d0.p0"))
	if none == nil || none.AnonID != "" || none.IP != "" || none.UserAgent != "" || none.ClickIDs != nil {
		t.Fatalf("nothing granted kept %+v", none)
	}
	analytics := TrackingFromContext(shopRequest("v2:a1.d0.p0"))
	if analytics.AnonID != "visitor-1" || analytics.IP != "" || analytics.ClickIDs != nil {
		t.Fatalf("analytics only: %+v", analytics)
	}
	ads := TrackingFromContext(shopRequest("v2:a0.d1.p0"))
	if ads.AnonID != "" || ads.IP != "203.0.113.7" || ads.UserAgent != "Mozilla/5.0 test" || ads.ClickIDs["cjevent"] != "c1" {
		t.Fatalf("ads only: %+v", ads)
	}
}

func TestTrackingRoundTripsThroughMetadata(t *testing.T) {
	in := TrackingFromContext(shopRequest("v2:a1.d1.p0"))
	out := decodeTracking(orderMetadataJSON(in))
	if out == nil || out.Consent != in.Consent || out.AnonID != in.AnonID || out.IP != in.IP || out.UserAgent != in.UserAgent || !maps.Equal(out.ClickIDs, in.ClickIDs) {
		t.Fatalf("round trip: %+v, want %+v", out, in)
	}
	if string(orderMetadataJSON(nil)) != "{}" || decodeTracking([]byte("{}")) != nil || decodeTracking([]byte("garbage")) != nil {
		t.Fatal("an order a shop didn't place must store and read no snapshot")
	}
}
