package order

import (
	"testing"
)

func TestCheckoutValidatingApplyWebhookResponse(t *testing.T) {
	e := &CheckoutValidating{Subtotal: 5000, EngineDiscountCents: 300}

	reject, _, err := e.ApplyWebhookResponse([]byte(`{"discountCents": 250, "riskScore": 40, "tags": ["screened"]}`))
	if err != nil || reject {
		t.Fatalf("apply: reject=%v err=%v", reject, err)
	}
	if e.DiscountCents != 250 || e.RiskScore != 40 || len(e.Tags) != 1 {
		t.Fatalf("enrichment wrong: %+v", e)
	}
	if e.EngineDiscountCents != 300 {
		t.Fatal("engine discount must stay untouched")
	}

	reject, reason, err := e.ApplyWebhookResponse([]byte(`{"reject": true, "reason": "payment risk"}`))
	if err != nil || !reject || reason != "payment risk" {
		t.Fatalf("veto not honored: %v %q %v", reject, reason, err)
	}
}

func TestOrderPlacingApplyWebhookResponse(t *testing.T) {
	e := &OrderPlacing{Subtotal: 10000}

	// additive enrichment: discount accumulates, risk max, tags append
	reject, _, err := e.ApplyWebhookResponse([]byte(`{"discountCents": 500, "riskScore": 30, "tags": ["b2b"]}`))
	if err != nil || reject {
		t.Fatalf("apply: reject=%v err=%v", reject, err)
	}
	if e.DiscountCents != 500 || e.RiskScore != 30 || len(e.Tags) != 1 {
		t.Fatalf("additive fields wrong: %+v", e)
	}

	// second webhook: discount accumulates, lower risk ignored
	if _, _, err := e.ApplyWebhookResponse([]byte(`{"discountCents": 100, "riskScore": 10}`)); err != nil {
		t.Fatal(err)
	}
	if e.DiscountCents != 600 || e.RiskScore != 30 {
		t.Fatalf("merge semantics wrong: %+v", e)
	}

	// tax/shipping/total are NOT settable via the webhook - those keys are
	// ignored, and the money pointers stay nil (providers own them)
	if _, _, err := e.ApplyWebhookResponse([]byte(`{"taxCents": 825, "shippingCents": 1200, "totalCents": 11525}`)); err != nil {
		t.Fatal(err)
	}
	if e.TaxCents != nil || e.ShippingCents != nil || e.TotalCents != nil {
		t.Fatalf("webhook must not set tax/shipping/total: %+v", e)
	}

	// veto still works
	if reject, reason, _ := e.ApplyWebhookResponse([]byte(`{"reject": true, "reason": "blocked"}`)); !reject || reason != "blocked" {
		t.Fatal("veto not honored")
	}
}
