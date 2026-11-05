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
	r.Header.Set(auth.ConsentHeader, consent)
	return auth.WithRequest(context.Background(), r)
}

func TestTrackingRoundTripsThroughMetadata(t *testing.T) {
	in := auth.ShopperFromContext(shopRequest("v2:a1.d1.p0"))
	out := decodeTracking(orderMetadataJSON(in))
	if out == nil || out.Consent != in.Consent || out.AnonID != in.AnonID || out.IP != in.IP || out.UserAgent != in.UserAgent || !maps.Equal(out.ClickIDs, in.ClickIDs) {
		t.Fatalf("round trip: %+v, want %+v", out, in)
	}
	if string(orderMetadataJSON(nil)) != "{}" || decodeTracking([]byte("{}")) != nil || decodeTracking([]byte("garbage")) != nil {
		t.Fatal("an order a shop didn't place must store and read no snapshot")
	}
}
