package cart

import (
	"testing"

	"github.com/google/uuid"
)

func TestLineAddingApplyWebhookResponse(t *testing.T) {
	e := &LineAdding{CartID: uuid.New(), ProductSku: "SKU-1"}

	reject, _, err := e.ApplyWebhookResponse([]byte(`{"unitPriceCents": 4200, "quantity": 3}`))
	if err != nil || reject {
		t.Fatalf("apply: reject=%v err=%v", reject, err)
	}
	if e.UnitPrice == nil || *e.UnitPrice != 4200 {
		t.Fatalf("unit price not applied: %v", e.UnitPrice)
	}
	if e.Quantity == nil || *e.Quantity != 3 {
		t.Fatalf("quantity not applied: %v", e.Quantity)
	}

	if _, _, err := e.ApplyWebhookResponse(nil); err != nil || *e.UnitPrice != 4200 { // empty body = accept, no changes
		t.Fatalf("empty body must be a no-op: %v", err)
	}

	reject, reason, err := e.ApplyWebhookResponse([]byte(`{"reject": true, "reason": "restricted product"}`))
	if err != nil || !reject || reason != "restricted product" {
		t.Fatalf("veto not honored: reject=%v reason=%q err=%v", reject, reason, err)
	}

	if _, _, err := e.ApplyWebhookResponse([]byte(`{"unitPriceCents": -5}`)); err == nil { // invalid values are hard errors
		t.Fatal("negative price should error")
	}
	if _, _, err := e.ApplyWebhookResponse([]byte(`{"quantity": 0}`)); err == nil {
		t.Fatal("zero quantity should error")
	}
}

func TestLineUpdatingApplyWebhookResponse(t *testing.T) {
	e := &LineUpdating{CartID: uuid.New(), LineID: uuid.New()}
	if _, _, err := e.ApplyWebhookResponse([]byte(`{"quantity": 2}`)); err != nil {
		t.Fatal(err)
	}
	if e.Quantity == nil || *e.Quantity != 2 {
		t.Fatalf("quantity not applied: %v", e.Quantity)
	}
}
