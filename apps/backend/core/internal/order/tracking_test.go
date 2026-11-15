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
	r.Header.Set(auth.ClickIDsHeader, "gclid=c1")
	r.Header.Set(auth.ConsentHeader, consent)
	return auth.WithRequest(context.Background(), r)
}

func TestTrackingRoundTripsThroughMetadata(t *testing.T) {
	in := auth.ShopperFromContext(shopRequest("v2:a1.d1.p0"))
	out := decodeShopper(orderMetadataJSON(in, nil))
	if out == nil || out.Consent != in.Consent || out.AnonID != in.AnonID || out.IP != in.IP || out.UserAgent != in.UserAgent || !maps.Equal(out.ClickIDs, in.ClickIDs) {
		t.Fatalf("round trip: %+v, want %+v", out, in)
	}
	if string(orderMetadataJSON(nil, nil)) != "{}" || decodeShopper([]byte("{}")) != nil || decodeShopper([]byte("garbage")) != nil {
		t.Fatal("an order a shop didn't place must store and read no snapshot")
	}
}

func TestPluginMetadataSitsBesideTheTrackingSnapshot(t *testing.T) {
	p := &OrderPlacing{Answers: answers([]CheckoutFieldAnswer{{Key: "gift_message", Label: "Gift message", Value: "Happy birthday"}, {Key: "wrap", Value: ""}})}
	if v, ok := p.CheckoutFieldValue("gift_message"); !ok || v != "Happy birthday" {
		t.Errorf("gift_message = %q, %v", v, ok)
	}
	if _, ok := p.CheckoutFieldValue("wrap"); ok {
		t.Error("an empty answer read as given")
	}
	for _, bad := range []string{"tracking", "Acme.Slot", ""} {
		if p.SetPlacingMetadata(bad, "x") == nil {
			t.Errorf("key %q was taken", bad)
		}
	}
	if err := p.SetPlacingMetadata("acme.slot", "morning"); err != nil {
		t.Fatal(err)
	}
	shopper := auth.ShopperFromContext(shopRequest("v2:a1.d1.p0"))
	raw := orderMetadataJSON(shopper, p.Metadata)
	if got := decodeMetadata(raw); !maps.Equal(got, map[string]string{"acme.slot": "morning"}) || decodeShopper(raw) == nil {
		t.Errorf("metadata %v, tracking kept %v - want both, apart", got, decodeShopper(raw) != nil)
	}
}

// An order's visitor id follows the consent its snapshot holds, so a shop visitor who refused analytics leaves none
func TestAnOrdersVisitorFollowsTheConsentItWasPlacedUnder(t *testing.T) {
	if s, visitor := placedBy(shopRequest("v2:a0.d1.p0")); s == nil || visitor != "" {
		t.Errorf("refused analytics: shopper %v, visitor %q - want the snapshot and no visitor id", s, visitor)
	}
	if s, visitor := placedBy(shopRequest("v2:a1.d0.p0")); s == nil || visitor != "visitor-1" || s.AnonID != visitor {
		t.Errorf("allowed analytics: shopper %v, visitor %q", s, visitor)
	}
	r, _ := http.NewRequest(http.MethodPost, "/", nil)
	r.Header.Set(auth.AnonHeader, "api-visitor")
	if s, visitor := placedBy(auth.WithRequest(context.Background(), r)); s != nil || visitor != "api-visitor" {
		t.Errorf("no banner: shopper %v, visitor %q - want no snapshot and the id sent", s, visitor)
	}
}
