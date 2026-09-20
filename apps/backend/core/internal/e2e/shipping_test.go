package e2e

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
	"github.com/skeletg/skelet/apps/backend/core/internal/shipping"
)

func (h *harness) addFlatShipping(t *testing.T, f *store, flatCents, freeAboveCents int64) {
	t.Helper()
	calc := shipping.CalcFlat
	if freeAboveCents > 0 {
		calc = shipping.CalcFreeAbove
	}
	repo := h.D.Shipping.Repo()
	z, err := repo.CreateZone(h.Ctx, shipping.Zone{
		ChannelID: f.Channel.ID, Code: "e2e-us-" + uuid.NewString()[:8], Name: "US", IsActive: true,
	})
	if err != nil {
		t.Fatalf("create zone: %v", err)
	}
	if err := repo.AddCountryToZone(h.Ctx, z.ID, "US"); err != nil {
		t.Fatalf("zone country: %v", err)
	}
	if _, err := repo.CreateMethod(h.Ctx, shipping.Method{
		ZoneID: z.ID, Code: "standard", Name: "Standard",
		CalcType: calc, FlatCents: flatCents, FreeAboveCents: freeAboveCents,
		IsActive: true, CountryMode: "all",
	}); err != nil {
		t.Fatalf("create method: %v", err)
	}
}

// Scenario: a flat-rate zone puts real shipping money on the order, and the free-shipping
// promotion action removes exactly that component
func TestShippingFlatRateThenFreeShippingPromo(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 2000)
	h.addFlatShipping(t, f, 799, 0)

	before := h.PlaceGuestCOD(t, f, 1)
	if before.ShippingAmount != 799 {
		t.Errorf("shipping = %d, want the flat 799", before.ShippingAmount)
	}
	if before.TotalAmount != 2799 {
		t.Errorf("total = %d, want 2000 + 799", before.TotalAmount)
	}
	if before.ShippingCarrierSlug == nil || *before.ShippingCarrierSlug != "builtin" {
		t.Errorf("carrier = %v, want builtin", before.ShippingCarrierSlug)
	}

	h.newRule(t, promotion.CreateRuleInput{
		Rule: promotion.Rule{
			Slug: "e2e-" + uuid.NewString()[:8], Name: "E2E free ship",
			ChannelID: &f.Channel.ID, IsActive: true,
		},
		Actions: []promotion.Action{{Type: "core.free_shipping", Params: json.RawMessage(`{}`)}},
	})
	after := h.PlaceGuestCOD(t, f, 1)
	if after.ShippingAmount != 0 {
		t.Errorf("shipping after free-ship promo = %d, want 0", after.ShippingAmount)
	}
	if after.TotalAmount != 2000 {
		t.Errorf("total = %d, want the bare 2000", after.TotalAmount)
	}
	h.Settle(t)
}

// Scenario: free_above waives the flat rate only once the post-discount subtotal clears the bar
func TestShippingFreeAboveThreshold(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 900)
	h.addFlatShipping(t, f, 799, 2000)

	small := h.PlaceGuestCOD(t, f, 1) // 900 below the 2000 bar
	if small.ShippingAmount != 799 {
		t.Errorf("below-threshold shipping = %d, want 799", small.ShippingAmount)
	}
	big := h.PlaceGuestCOD(t, f, 3) // 2700 clears it
	if big.ShippingAmount != 0 {
		t.Errorf("above-threshold shipping = %d, want 0", big.ShippingAmount)
	}
	h.Settle(t)
}
