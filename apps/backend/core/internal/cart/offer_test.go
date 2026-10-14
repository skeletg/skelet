package cart

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/delivery"
)

func offerCart(customerID *uuid.UUID, mutate func(o *Offer)) Cart {
	o := &Offer{ExpiresAt: time.Now().Add(24 * time.Hour), LinkVersion: 3}
	if mutate != nil {
		mutate(o)
	}
	return Cart{ID: uuid.New(), CustomerID: customerID, Status: "active", Kind: KindOffer, Offer: o, Email: "guest@example.com"}
}

func TestCheckOfferAccess(t *testing.T) {
	owner, other := uuid.New(), uuid.New()
	past := time.Now().Add(-time.Minute)
	granted := func(c Cart, version int32) context.Context {
		return WithOfferGrant(context.Background(), OfferGrant{CartID: c.ID, LinkVersion: version})
	}
	tests := []struct {
		name   string
		cart   Cart
		ctx    func(c Cart) context.Context
		caller *uuid.UUID
		want   error
	}{
		{"guest offer without its link", offerCart(nil, nil), func(Cart) context.Context { return context.Background() }, nil, ErrCartForbidden},
		{"guest offer with its link", offerCart(nil, nil), func(c Cart) context.Context { return granted(c, 3) }, nil, nil},
		{"guest offer with its link, someone signed in", offerCart(nil, nil), func(c Cart) context.Context { return granted(c, 3) }, &other, nil},
		{"guest offer with a replaced link", offerCart(nil, nil), func(c Cart) context.Context { return granted(c, 2) }, nil, ErrCartForbidden},
		{"guest offer with another cart's link", offerCart(nil, nil), func(Cart) context.Context {
			return WithOfferGrant(context.Background(), OfferGrant{CartID: uuid.New(), LinkVersion: 3})
		}, nil, ErrCartForbidden},
		{"customer offer, link but nobody signed in", offerCart(&owner, nil), func(c Cart) context.Context { return granted(c, 3) }, nil, ErrCartForbidden},
		{"customer offer, link and another account", offerCart(&owner, nil), func(c Cart) context.Context { return granted(c, 3) }, &other, ErrCartForbidden},
		{"customer offer, link and its owner", offerCart(&owner, nil), func(c Cart) context.Context { return granted(c, 3) }, &owner, nil},
		{"customer offer, owner without the link", offerCart(&owner, nil), func(Cart) context.Context { return context.Background() }, &owner, ErrCartForbidden},
		{"customer offer shown in account, owner without the link", offerCart(&owner, func(o *Offer) { o.ShowInAccount = true }), func(Cart) context.Context { return context.Background() }, &owner, nil},
		{"expired offer with its link", offerCart(nil, func(o *Offer) { o.ExpiresAt = past }), func(c Cart) context.Context { return granted(c, 3) }, nil, ErrOfferExpired},
		{"expired offer without its link stays forbidden", offerCart(nil, func(o *Offer) { o.ExpiresAt = past }), func(Cart) context.Context { return context.Background() }, nil, ErrCartForbidden},
		{"revoked offer with its link", offerCart(nil, func(o *Offer) { o.RevokedAt = &past }), func(c Cart) context.Context { return granted(c, 3) }, nil, ErrOfferRevoked},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := checkCartAccess(tc.ctx(tc.cart), tc.cart, tc.caller)
			if !errors.Is(err, tc.want) || (tc.want == nil && err != nil) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
	t.Run("a converted offer is no longer active", func(t *testing.T) {
		c := offerCart(nil, nil)
		c.Status = "converted"
		if err := checkCartAccess(granted(c, 3), c, nil); !errors.Is(err, ErrCartNotActive) {
			t.Fatalf("got %v", err)
		}
	})
}

func TestOfferState(t *testing.T) {
	now := time.Now()
	earlier := now.Add(-time.Hour)
	tests := []struct {
		name string
		cart Cart
		want OfferState
	}{
		{"draft", offerCart(nil, nil), OfferStateDraft},
		{"sent", offerCart(nil, func(o *Offer) { o.SentAt = &earlier }), OfferStateSent},
		{"opened", offerCart(nil, func(o *Offer) { o.SentAt, o.OpenedAt = &earlier, &earlier }), OfferStateOpened},
		{"expired", offerCart(nil, func(o *Offer) { o.ExpiresAt = earlier }), OfferStateExpired},
		{"revoked beats expired", offerCart(nil, func(o *Offer) { o.ExpiresAt, o.RevokedAt = earlier, &earlier }), OfferStateRevoked},
	}
	for _, tc := range tests {
		if got := tc.cart.OfferState(now); got != tc.want {
			t.Errorf("%s: got %s, want %s", tc.name, got, tc.want)
		}
	}
	done := offerCart(nil, func(o *Offer) { o.ExpiresAt = earlier })
	done.Status = "converted"
	if got := done.OfferState(now); got != OfferStateCompleted {
		t.Errorf("a placed offer is completed whatever its dates say, got %s", got)
	}
}

func TestCheckOfferLocks(t *testing.T) {
	street := "Main 1"
	addr := &Address{FirstName: "A", LastName: "B", Street1: street, City: "C", PostalCode: "1", CountryCode: "RS"}
	optionID := uuid.New()
	base := func(locks ...string) Cart {
		c := offerCart(nil, func(o *Offer) { o.Locks = locks })
		c.Checkout = Checkout{
			ShippingAddress: addr, BillingSameAsShipping: true, PaymentProviderSlug: "cod",
			Deliveries: map[string]DeliverySelection{"all": {OptionID: optionID, CarrierSlug: "core", MethodCode: "std"}},
		}
		return c
	}
	moves := map[string]func(c *Cart){
		LockBilling: func(c *Cart) {
			c.Checkout.BillingSameAsShipping = false
			c.Checkout.BillingAddress = &Address{FirstName: "X", Street1: "Elsewhere", City: "C", PostalCode: "2", CountryCode: "RS"}
		},
		LockShipping: func(c *Cart) {
			a := *addr
			a.Street1 = "Other 2"
			c.Checkout.ShippingAddress = &a
		},
		LockDelivery: func(c *Cart) {
			c.Checkout.Deliveries = map[string]DeliverySelection{"all": {OptionID: optionID, CarrierSlug: "core", MethodCode: "express"}}
		},
		LockCoupons: func(c *Cart) { c.Checkout.GiftCardCodes = []string{"GIFT"} },
	}
	for lock, move := range moves {
		t.Run(lock+" locked", func(t *testing.T) {
			before := base(lock)
			after := before
			move(&after)
			var le *LockedError
			if err := checkOfferLocks(before, after); !errors.As(err, &le) || le.Lock != lock {
				t.Fatalf("got %v, want a %s lock error", err, lock)
			}
		})
		t.Run(lock+" unlocked", func(t *testing.T) {
			before := base()
			after := before
			move(&after)
			if err := checkOfferLocks(before, after); err != nil {
				t.Fatalf("an unlocked part changed and was refused: %v", err)
			}
		})
	}
	t.Run("an unchanged locked part passes", func(t *testing.T) {
		before := base(OfferLocks...)
		after := before
		same := *addr
		after.Checkout.ShippingAddress = &same // equal by value, a different pointer
		after.Checkout.PONumber = "PO-1"
		if err := checkOfferLocks(before, after); err != nil {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("a shopper cart has no locks", func(t *testing.T) {
		before := base(OfferLocks...)
		before.Kind, before.Offer = KindShopper, nil
		after := before
		after.Checkout.PaymentProviderSlug = "stripe"
		if err := checkOfferLocks(before, after); err != nil {
			t.Fatalf("got %v", err)
		}
	})
}

func TestNormalizeOfferLocks(t *testing.T) {
	got, err := NormalizeOfferLocks([]string{LockCoupons, LockLines, LockLines}, true)
	if err != nil || len(got) != 2 || got[0] != LockLines || got[1] != LockCoupons {
		t.Fatalf("got %v, %v", got, err)
	}
	if _, err := NormalizeOfferLocks([]string{"everything"}, true); !errors.Is(err, ErrOfferLockUnknown) {
		t.Fatalf("an unknown lock passed: %v", err)
	}
	if _, err := NormalizeOfferLocks([]string{LockDelivery}, false); !errors.Is(err, ErrOfferLockNotApplicable) {
		t.Fatalf("a delivery lock on a cart that doesn't ship passed: %v", err)
	}
	if _, err := NormalizeOfferLocks([]string{LockBilling, LockLines}, false); err != nil {
		t.Fatalf("billing stays lockable without shipping: %v", err)
	}
}

func TestOfferLinks(t *testing.T) {
	links := NewOfferLinks([]byte("secret"))
	id := uuid.New()
	tok := links.Sign(id, 4)
	g, err := links.Verify(tok)
	if err != nil || g.CartID != id || g.LinkVersion != 4 {
		t.Fatalf("round trip: %+v, %v", g, err)
	}
	for name, bad := range map[string]string{
		"tampered version": id.String() + ".5." + tok[len(id.String())+3:],
		"other secret":     NewOfferLinks([]byte("other")).Sign(id, 4),
		"garbage":          "nope",
		"empty":            "",
	} {
		if _, err := links.Verify(bad); !errors.Is(err, ErrOfferLinkInvalid) {
			t.Errorf("%s verified: %v", name, err)
		}
	}
}

func TestDeliveryProfilesCustomLinesNeverShip(t *testing.T) {
	lines := []CartLine{
		{ID: uuid.New(), ProductSku: "shirt"},
		{ID: uuid.New(), ProductSku: CustomLineSKUPrefix + "ABC", IsCustom: true},
	}
	profiles := DeliveryProfiles(lines, map[string]delivery.LineProfile{"shirt": {SKU: "shirt", RequiresShipping: true}})
	plan := delivery.PlanGroups(DeliveryLines(lines), profiles, []string{delivery.KindShipToAddress}, false)
	if plan.Need != delivery.NeedMixed || len(plan.Digital) != 1 || plan.Digital[0].SKU != lines[1].ProductSku {
		t.Fatalf("custom line planned as physical: %+v", plan)
	}
	onlyCustom := DeliveryProfiles(lines[1:], nil)
	if p := delivery.PlanGroups(DeliveryLines(lines[1:]), onlyCustom, []string{delivery.KindShipToAddress}, false); p.Need != delivery.NeedNone {
		t.Fatalf("a custom-only cart needs delivery: %+v", p)
	}
}

func TestRepriceKeepsOfferPrices(t *testing.T) {
	overrideID, customID := uuid.New(), uuid.New()
	repo := &repriceRepo{}
	svc := NewService(repo, nil, fakePricer{prices: map[string]SimplePricing{"plain": {UnitPrice: 1200}}})
	c := svc.RepriceCartLines(context.Background(), Cart{ID: uuid.New(), Lines: []CartLine{
		{ID: overrideID, ProductSku: "plain", UnitPrice: 500, Quantity: 1, PriceOverride: true},
		{ID: customID, ProductSku: CustomLineSKUPrefix + "X", UnitPrice: 9900, Quantity: 1, IsCustom: true, PriceOverride: true},
	}})
	if len(repo.writes) != 0 || c.Lines[0].UnitPrice != 500 || c.Lines[1].UnitPrice != 9900 {
		t.Fatalf("an offer price moved: writes=%v lines=%+v", repo.writes, c.Lines)
	}
	if err := svc.ValidateCheckoutLines(context.Background(), c); err != nil {
		t.Fatalf("a custom line was checked against the catalog: %v", err)
	}
}

func TestLineEditsRefusedWhenLinesLocked(t *testing.T) {
	c := offerCart(nil, func(o *Offer) { o.Locks = []string{LockLines} })
	ctx := WithOfferGrant(context.Background(), OfferGrant{CartID: c.ID, LinkVersion: 3})
	repo := fakeRepo{getCart: func(uuid.UUID) (Cart, error) { return c, nil }}
	svc := NewService(repo, nil, fakePricer{})
	if _, err := svc.AddToCart(ctx, c.ID, nil, "plain", 1); !errors.Is(err, ErrOfferLocked) {
		t.Fatalf("add: %v", err)
	}
	if _, err := svc.RemoveFromCart(ctx, c.ID, uuid.New(), nil); !errors.Is(err, ErrOfferLocked) {
		t.Fatalf("remove: %v", err)
	}
	if err := svc.ClearCart(ctx, c.ID, nil); !errors.Is(err, ErrOfferLocked) {
		t.Fatalf("clear: %v", err)
	}
	if err := svc.SetCartEmail(ctx, c.ID, nil, "someone@else.com"); !errors.Is(err, ErrOfferLocked) {
		t.Fatalf("an offer's recipient email changed: %v", err)
	}
	if err := svc.SetCartEmail(ctx, c.ID, nil, "Guest@Example.com"); err != nil {
		t.Fatalf("the recipient's own email was refused: %v", err)
	}
}

func TestMergeRefusesOffer(t *testing.T) {
	customerID := uuid.New()
	c := offerCart(nil, nil)
	repo := &mergeRepo{carts: map[uuid.UUID]Cart{c.ID: c}, linesByCart: map[uuid.UUID][]CartLine{}, activeByCust: map[uuid.UUID]Cart{}}
	svc := NewService(repo, newTestTxManager(t), nil)
	if _, err := svc.MergeGuestCart(context.Background(), customerID, c.ID); !errors.Is(err, ErrCartForbidden) {
		t.Fatalf("an offer merged into a shopper cart: %v", err)
	}
	if len(repo.reassigned) != 0 || len(repo.mergedCarts) != 0 {
		t.Fatalf("the offer was adopted or tombstoned: reassigned=%v merged=%v", repo.reassigned, repo.mergedCarts)
	}
}

func TestOfferPaymentMethods(t *testing.T) {
	before := offerCart(nil, func(o *Offer) { o.PaymentMethods = []string{"cod", "bank"} })
	before.Checkout.PaymentProviderSlug = "cod"
	to := func(slug string) Cart {
		after := before
		after.Checkout.PaymentProviderSlug = slug
		return after
	}
	if err := checkOfferLocks(before, to("bank")); err != nil {
		t.Fatalf("an allowed method was refused: %v", err)
	}
	if err := checkOfferLocks(before, to("stripe")); !errors.Is(err, ErrOfferPaymentNotAllowed) {
		t.Fatalf("a method outside the offer's = %v", err)
	}
	if err := checkOfferLocks(before, to("")); err != nil {
		t.Fatalf("clearing the method was refused: %v", err)
	}
	open := offerCart(nil, nil)
	if !open.Offer.PaymentAllowed("anything") {
		t.Fatal("an offer that narrows nothing refused a method")
	}
}

type offerOpenRepository struct {
	Repository
	calls int
	err   error
}

func (r *offerOpenRepository) MarkOfferOpened(context.Context, uuid.UUID) error {
	r.calls++
	return r.err
}

func TestRecordOfferOpened(t *testing.T) {
	for _, tc := range []struct {
		name       string
		mutate     func(*Offer)
		status     string
		err        error
		wantCalls  int
		wantOpened bool
	}{
		{name: "draft viewed from account", wantCalls: 1, wantOpened: true},
		{name: "sent viewed", mutate: func(o *Offer) { now := time.Now(); o.SentAt = &now }, wantCalls: 1, wantOpened: true},
		{name: "already opened", mutate: func(o *Offer) { now := time.Now(); o.OpenedAt = &now }, wantOpened: true},
		{name: "expired", mutate: func(o *Offer) { o.ExpiresAt = time.Now().Add(-time.Minute) }},
		{name: "withdrawn", mutate: func(o *Offer) { now := time.Now(); o.RevokedAt = &now }},
		{name: "completed", status: "converted"},
		{name: "failed stamp remains new", err: errors.New("database unavailable"), wantCalls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := offerCart(nil, tc.mutate)
			if tc.status != "" {
				c.Status = tc.status
			}
			repo := &offerOpenRepository{err: tc.err}
			s := NewService(repo, nil, nil)
			s.RecordOfferOpened(context.Background(), &c)
			if repo.calls != tc.wantCalls || (c.Offer.OpenedAt != nil) != tc.wantOpened {
				t.Fatalf("calls=%d opened=%v, want calls=%d opened=%v", repo.calls, c.Offer.OpenedAt != nil, tc.wantCalls, tc.wantOpened)
			}
			if tc.wantOpened {
				s.RecordOfferOpened(context.Background(), &c)
				if repo.calls != tc.wantCalls {
					t.Fatal("reopening wrote again")
				}
			}
		})
	}
}
