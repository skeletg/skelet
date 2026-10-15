package delivery

import (
	"errors"
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

func find(reqs []Requirement, scope, typ string) (Requirement, bool) {
	for _, r := range reqs {
		if r.Scope == scope && r.Type == typ {
			return r, true
		}
	}
	return Requirement{}, false
}

func kindPtr(slug string) *Kind {
	k, _ := KindBySlug(slug)
	return &k
}

func TestRequirementsDigitalOnlyAsksBillingAndPayment(t *testing.T) {
	reqs := Requirements(Snapshot{
		Plan:             Plan{Need: NeedNone, Digital: []Line{line("ebook")}},
		ChannelPhoneMode: ModeHidden, Email: true,
	})
	if _, ok := find(reqs, ScopeOrder, BlockShippingAddress); ok {
		t.Fatal("a digital order has no shipping address to ask for")
	}
	b, ok := find(reqs, ScopeOrder, BlockBillingAddress)
	if !ok || b.Mode != ModeRequired || b.Status != StatusMissing {
		t.Fatalf("billing = %+v, want required and missing", b)
	}
	if err := Blocking(reqs); err == nil || apperr.CodeOf(err) != codeBilling {
		t.Fatalf("blocking = %v, want %s", err, codeBilling)
	}
}

func TestRequirementsShipToAddressOwesShippingBillingPayment(t *testing.T) {
	opt := &Option{Name: "Courier", Kind: KindShipToAddress}
	reqs := Requirements(Snapshot{
		Plan:             Plan{Need: NeedPhysical},
		ChannelPhoneMode: ModeOptional, Email: true,
		Groups: []GroupSnapshot{{Group: Group{Key: CombinedGroupKey}, Option: opt, Kind: kindPtr(KindShipToAddress), MethodChosen: true}},
	})
	for _, typ := range []string{BlockShippingAddress, BlockBillingAddress, BlockPayment} {
		r, ok := find(reqs, ScopeOrder, typ)
		if !ok || r.Status != StatusMissing {
			t.Fatalf("%s = %+v, want missing", typ, r)
		}
	}
}

func TestRequirementsLockerAsksPointNotShipping(t *testing.T) {
	opt := &Option{Name: "Paketomat", Kind: KindParcelLocker}
	reqs := Requirements(Snapshot{
		Plan: Plan{Need: NeedPhysical}, Email: true, Billing: true, Payment: true,
		Groups: []GroupSnapshot{{Group: Group{Key: CombinedGroupKey}, Option: opt, Kind: kindPtr(KindParcelLocker), MethodChosen: true}},
	})
	if _, ok := find(reqs, ScopeOrder, BlockShippingAddress); ok {
		t.Fatal("a locker supplies the ship-to address")
	}
	p, ok := find(reqs, CombinedGroupKey, BlockPoint)
	if !ok || p.Status != StatusMissing || p.Label != "Pickup point" {
		t.Fatalf("point = %+v, want a missing pickup point", p)
	}
	ph, _ := find(reqs, ScopeOrder, BlockPhone)
	if ph.Mode != ModeRequired {
		t.Fatalf("a locker locks the phone, got %s", ph.Mode)
	}
}

func TestRequirementsCarrierPointAsksAddressNotPoint(t *testing.T) {
	opt := &Option{Name: "Nearest parcel shop", Kind: KindCarrierPoint}
	reqs := Requirements(Snapshot{
		Plan: Plan{Need: NeedPhysical}, Email: true, Billing: true, Payment: true,
		Groups: []GroupSnapshot{{Group: Group{Key: CombinedGroupKey}, Option: opt, Kind: kindPtr(KindCarrierPoint), MethodChosen: true}},
	})
	if _, ok := find(reqs, CombinedGroupKey, BlockPoint); ok {
		t.Fatal("the carrier picks the point - the shopper is never asked")
	}
	if _, ok := find(reqs, ScopeOrder, BlockShippingAddress); !ok {
		if _, ok := find(reqs, CombinedGroupKey, BlockShippingAddress); !ok {
			t.Fatal("the carrier routes by the shopper's address, so it must be asked")
		}
	}
	ph, _ := find(reqs, ScopeOrder, BlockPhone)
	if ph.Mode != ModeRequired {
		t.Fatalf("the pickup notice needs a phone, got %s", ph.Mode)
	}
}

func TestRequirementsPerGroupForSplitDelivery(t *testing.T) {
	courier := &Option{Name: "Courier", Kind: KindShipToAddress}
	pickup := &Option{Name: "Store pickup", Kind: KindStorePickup}
	reqs := Requirements(Snapshot{
		Plan: Plan{Need: NeedMixed, Digital: []Line{line("ebook")}}, Email: true, ShippingAddress: true, Billing: true, Payment: true,
		Groups: []GroupSnapshot{
			{Group: Group{Key: "k:ship_to_address"}, Option: courier, Kind: kindPtr(KindShipToAddress), MethodChosen: true},
			{Group: Group{Key: "k:store_pickup"}, Option: pickup, Kind: kindPtr(KindStorePickup), MethodChosen: true},
		},
	})
	if p, _ := find(reqs, "k:store_pickup", BlockPoint); p.Status != StatusMissing {
		t.Fatalf("store = %+v, want missing", p)
	}
	if c, _ := find(reqs, "k:store_pickup", BlockCollector); c.Status != StatusMissing {
		t.Fatalf("collector = %+v, want missing", c)
	}
	if _, ok := find(reqs, "k:ship_to_address", BlockPoint); ok {
		t.Fatal("the courier group asks no point")
	}
	if s, _ := find(reqs, ScopeOrder, BlockShippingAddress); s.Status != StatusSatisfied {
		t.Fatalf("the courier group still owes the shopper's address: %+v", s)
	}
	if err := Blocking(reqs); !errors.Is(err, err) || apperr.CodeOf(err) != codePoint {
		t.Fatalf("blocking = %v, want the store first", err)
	}
}

func TestRequirementsPickupBillingFollowsTheOption(t *testing.T) {
	snap := func(modes map[string]Mode, floor Mode) Snapshot {
		return Snapshot{
			Plan: Plan{Need: NeedPhysical}, Email: true, Payment: true, BillingFloor: floor,
			Groups: []GroupSnapshot{{Group: Group{Key: CombinedGroupKey}, Option: &Option{Name: "Pickup", Kind: KindStorePickup, BlockModes: modes},
				Kind: kindPtr(KindStorePickup), MethodChosen: true, Point: true, Collector: true}},
		}
	}
	if b, _ := find(Requirements(snap(nil, ModeHidden)), ScopeOrder, BlockBillingAddress); b.Mode != ModeOptional || b.Blocking() {
		t.Fatalf("billing = %+v, want optional by default for pickup", b)
	}
	if err := Blocking(Requirements(snap(nil, ModeHidden))); err != nil {
		t.Fatalf("a pickup without billing should place: %v", err)
	}
	if b, _ := find(Requirements(snap(map[string]Mode{BlockBillingAddress: ModeRequired}, ModeHidden)), ScopeOrder, BlockBillingAddress); b.Status != StatusMissing {
		t.Fatalf("billing = %+v, want required when the option asks it", b)
	}
	if b, _ := find(Requirements(snap(nil, ModeRequired)), ScopeOrder, BlockBillingAddress); b.Status != StatusMissing {
		t.Fatalf("billing = %+v, want required when the store's floor asks it", b)
	}
}

func TestRequirementsNothingQuotedBlocksPlacement(t *testing.T) {
	reqs := Requirements(Snapshot{
		Plan: Plan{Need: NeedPhysical}, Email: true, Billing: true, Payment: true,
		Groups: []GroupSnapshot{{Group: Group{Key: CombinedGroupKey}, Choices: 0}},
	})
	d, _ := find(reqs, CombinedGroupKey, BlockDeliveryOption)
	if d.Status != StatusMissing || d.Code != codeNoDelivery {
		t.Fatalf("delivery = %+v, want it missing when nothing quotes - it would ship unpriced", d)
	}
	if s, ok := find(reqs, ScopeOrder, BlockShippingAddress); !ok || s.Status != StatusMissing {
		t.Fatalf("shipping address = %+v, want the shopper's address still owed", s)
	}

	// Beside a group that is chosen, the unquoted one still blocks - otherwise its lines leave without a package or a price
	opt := Option{Kind: KindShipToAddress}
	mixed := Snapshot{
		Plan: Plan{Need: NeedPhysical}, Email: true, ShippingAddress: true, Billing: true, Payment: true,
		Groups: []GroupSnapshot{
			{Group: Group{Key: "a"}, Option: &opt, Kind: kindPtr(KindShipToAddress), MethodChosen: true, Choices: 1},
			{Group: Group{Key: "b"}, Choices: 0},
		},
	}
	if err := Blocking(Requirements(mixed)); apperr.CodeOf(err) != codeNoDelivery {
		t.Fatalf("Blocking = %v, want %s", err, codeNoDelivery)
	}
}
