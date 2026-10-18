package delivery

import (
	"slices"
	"testing"

	"github.com/google/uuid"
)

func line(sku string) Line { return Line{ID: uuid.New(), SKU: sku, Name: sku, Quantity: 1} }

func TestPlanGroups(t *testing.T) {
	profiles := map[string]LineProfile{
		"ebook":  {SKU: "ebook", RequiresShipping: false},
		"shirt":  {SKU: "shirt", RequiresShipping: true, DeliveryKinds: []string{KindShipToAddress, KindParcelLocker}},
		"fridge": {SKU: "fridge", RequiresShipping: true, DeliveryKinds: []string{KindShipToAddress, KindStorePickup}},
		"mug":    {SKU: "mug", RequiresShipping: true, DeliveryKinds: []string{}},
		"sofa":   {SKU: "sofa", RequiresShipping: true, DeliveryKinds: []string{KindStorePickup}},
	}
	offered := []string{KindShipToAddress, KindParcelLocker}

	cases := []struct {
		name       string
		skus       []string
		split      bool
		need       Need
		groups     int
		combinable bool
		digital    int
		undeliver  int
		firstKey   string
	}{
		{"digital only", []string{"ebook"}, false, NeedNone, 0, false, 1, 0, ""},
		{"one physical line takes every offered kind", []string{"mug"}, false, NeedPhysical, 1, true, 0, 0, CombinedGroupKey},
		{"mixed cart splits digital out", []string{"ebook", "mug"}, false, NeedMixed, 1, true, 1, 0, CombinedGroupKey},
		{"common kind combines by default", []string{"shirt", "fridge"}, false, NeedPhysical, 1, true, 0, 0, CombinedGroupKey},
		{"shopper split keeps a group per kind set", []string{"shirt", "fridge"}, true, NeedPhysical, 2, true, 0, 0, "k:parcel_locker,ship_to_address"},
		{"a single group stays combined even when split", []string{"fridge"}, true, NeedPhysical, 1, true, 0, 0, CombinedGroupKey},
		{"a kind the channel doesn't offer leaves the line undeliverable", []string{"sofa", "mug"}, false, NeedPhysical, 1, true, 0, 1, CombinedGroupKey},
		{"unknown sku is physical and goes anywhere", []string{"ghost"}, false, NeedPhysical, 1, true, 0, 0, CombinedGroupKey},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var lines []Line
			for _, s := range c.skus {
				lines = append(lines, line(s))
			}
			p := PlanGroups(lines, profiles, offered, c.split)
			if p.Need != c.need {
				t.Fatalf("need = %s, want %s", p.Need, c.need)
			}
			if len(p.Groups) != c.groups {
				t.Fatalf("groups = %d, want %d (%+v)", len(p.Groups), c.groups, p.Groups)
			}
			if c.groups > 0 && p.Combinable != c.combinable {
				t.Fatalf("combinable = %v, want %v", p.Combinable, c.combinable)
			}
			if len(p.Digital) != c.digital || len(p.Undeliverable) != c.undeliver {
				t.Fatalf("digital=%d undeliverable=%d, want %d/%d", len(p.Digital), len(p.Undeliverable), c.digital, c.undeliver)
			}
			if c.firstKey != "" && p.Groups[0].Key != c.firstKey {
				t.Fatalf("first key = %q, want %q", p.Groups[0].Key, c.firstKey)
			}
		})
	}
}

// A fridge can't go to a PO box, so a cart with a fridge and a shirt is offered only what both take
func TestPlanGroupsCombinedKindsAreTheIntersection(t *testing.T) {
	profiles := map[string]LineProfile{
		"shirt":  {RequiresShipping: true},
		"fridge": {RequiresShipping: true, DeliveryKinds: []string{KindShipToAddress, KindStorePickup}},
	}
	p := PlanGroups([]Line{line("shirt"), line("fridge")}, profiles, []string{KindShipToAddress, KindStorePickup, KindParcelLocker}, false)
	got := p.Groups[0].Kinds
	if len(got) != 2 || slices.Contains(got, KindParcelLocker) {
		t.Fatalf("combined kinds = %v, want ship_to_address and store_pickup, never a PO box", got)
	}
}

func TestPlanGroupsNothingInCommonSplitsWithoutAsking(t *testing.T) {
	profiles := map[string]LineProfile{
		"a": {RequiresShipping: true, DeliveryKinds: []string{KindParcelLocker}},
		"b": {RequiresShipping: true, DeliveryKinds: []string{KindStorePickup}},
	}
	p := PlanGroups([]Line{line("a"), line("b")}, profiles, []string{KindParcelLocker, KindStorePickup}, false)
	if p.Combinable || len(p.Groups) != 2 {
		t.Fatalf("want two uncombinable groups, got combinable=%v groups=%d", p.Combinable, len(p.Groups))
	}
}

func TestSplitShipmentsByOrigin(t *testing.T) {
	w1, w2 := uuid.New(), uuid.New()
	a, b, c := line("a"), line("b"), line("c")
	origins := map[uuid.UUID]Origin{a.ID: {LocationID: &w1, Name: "Belgrade"}, b.ID: {LocationID: &w2, Name: "Novi Sad"}, c.ID: {LocationID: &w1, Name: "Belgrade"}}
	ships := SplitShipments([]Group{{Key: CombinedGroupKey, Lines: []Line{a, b, c}}}, origins)
	if len(ships) != 2 {
		t.Fatalf("shipments = %d, want 2", len(ships))
	}
	if len(ships[0].Lines) != 2 || ships[0].Origin.Name != "Belgrade" {
		t.Fatalf("first shipment = %+v, want both Belgrade lines", ships[0])
	}
}

func TestKindResolveAndCheckModes(t *testing.T) {
	k, _ := KindBySlug(KindStorePickup)
	if m := k.ModeOf(nil, BlockPoint); m != ModeRequired {
		t.Fatalf("locked point = %s, want required", m)
	}
	if m := k.ModeOf(map[string]Mode{BlockBillingAddress: ModeRequired}, BlockBillingAddress); m != ModeRequired {
		t.Fatalf("configured billing = %s, want required", m)
	}
	if m := k.ModeOf(nil, BlockShippingAddress); m != ModeHidden {
		t.Fatalf("n/a shipping = %s, want hidden", m)
	}
	if err := k.CheckModes(map[string]Mode{BlockPoint: ModeOptional}); err == nil {
		t.Fatal("relaxing a locked block must be refused")
	}
	if err := k.CheckModes(map[string]Mode{"nope": ModeOptional}); err == nil {
		t.Fatal("an unknown block must be refused")
	}
	if err := k.CheckModes(map[string]Mode{BlockPhone: ModeRequired}); err != nil {
		t.Fatalf("a configurable block takes any mode: %v", err)
	}
}

func TestRegisterKindAddsEmailAndPayment(t *testing.T) {
	t.Cleanup(resetForTest)
	RegisterKind(Kind{Slug: "drone", Label: "Drone", ShipTo: ShipToCustomer})
	k, _ := KindBySlug("drone")
	if !k.Needs(BlockEmail) || !k.Needs(BlockPayment) {
		t.Fatalf("every kind owes email and payment: %+v", k.Blocks)
	}
}

func TestUnsetProductTypeTakesEveryOfferedKind(t *testing.T) {
	offered := []string{KindShipToAddress, KindStorePickup, KindParcelLocker}
	for _, sku := range []string{"shirt", "unknown"} {
		p := PlanGroups([]Line{line(sku)}, map[string]LineProfile{"shirt": {RequiresShipping: true}}, offered, false)
		if len(p.Groups) != 1 || len(p.Groups[0].Kinds) != 3 {
			t.Fatalf("%s: %+v, want all three kinds", sku, p)
		}
	}
}
