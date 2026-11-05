package auth

import (
	"context"
	"net/http"
	"testing"
)

func shopRequest(consent string) context.Context {
	r, _ := http.NewRequest(http.MethodPost, "/", nil)
	r.RemoteAddr = "203.0.113.7:4000"
	r.Header.Set("User-Agent", "Mozilla/5.0 test")
	r.Header.Set(AnonHeader, "visitor-1")
	r.Header.Set(SessionHeader, "1700000000")
	r.Header.Set(ClickIDsHeader, "cjevent=c1")
	if consent != "" {
		r.Header.Set(ConsentHeader, consent)
	}
	return WithRequest(context.Background(), r)
}

func TestAShopperKeepsEachPartOnlyUnderItsConsent(t *testing.T) {
	if got := ShopperFromContext(shopRequest("")); got != nil {
		t.Fatalf("no stated consent: %+v, want nil", got)
	}
	none := ShopperFromContext(shopRequest("v2:a0.d0.p0"))
	r, _ := http.NewRequest(http.MethodPost, "/", nil)
	r.Header.Set(ConsentHeader, "v2:a0.d0.p0")
	r.Header.Set(ClickIDsHeader, "gclid=g1")
	if got := ShopperFromContext(WithRequest(context.Background(), r)); got.ClickIDs != nil {
		t.Fatalf("an ad click id was kept without ads consent: %v", got.ClickIDs)
	}
	// An affiliate id the storefront sent without ads consent is one the merchant made necessary
	if none == nil || none.AnonID != "" || none.IP != "" || none.UserAgent != "" || none.ClickIDs["cjevent"] != "c1" {
		t.Fatalf("nothing granted kept %+v", none)
	}
	analytics := ShopperFromContext(shopRequest("v2:a1.d0.p0"))
	if analytics.AnonID != "visitor-1" || analytics.SessionID != "1700000000" || analytics.IP != "" {
		t.Fatalf("analytics only: %+v", analytics)
	}
	ads := ShopperFromContext(shopRequest("v2:a0.d1.p0"))
	if ads.AnonID != "" || ads.SessionID != "" || ads.IP != "203.0.113.7" || ads.UserAgent != "Mozilla/5.0 test" || ads.ClickIDs["cjevent"] != "c1" {
		t.Fatalf("ads only: %+v", ads)
	}
}

func TestAnalyticsRefused(t *testing.T) {
	if (*Shopper)(nil).AnalyticsRefused() {
		t.Error("a caller with no banner refused nothing")
	}
	if !ShopperFromContext(shopRequest("v2:a0.d1.p1")).AnalyticsRefused() || ShopperFromContext(shopRequest("v2:a1.d0.p0")).AnalyticsRefused() {
		t.Error("AnalyticsRefused must follow the analytics category")
	}
}
