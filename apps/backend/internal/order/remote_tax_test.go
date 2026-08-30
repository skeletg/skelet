package order

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/internal/cart"
	"github.com/skeletg/skelet/internal/channel"
	"github.com/skeletg/skelet/internal/customer"
	"github.com/skeletg/skelet/internal/remoteprovider"
)

func TestRemoteTaxInputFromCheckout(t *testing.T) {
	region := "CA"
	cc := &checkoutContext{
		subtotal: 10000,
		shipping: OrderAddress{CountryCode: "US", State: &region, City: "LA", PostalCode: "90001"},
		store:    &channel.Channel{ID: uuid.New(), CurrencyCode: "USD"},
		customer: customer.Customer{TaxExempt: false},
	}
	cc.cart.Lines = []cart.CartLine{{ProductSku: "SKU-1", Quantity: 2, UnitPrice: 5000}}

	in := remoteTaxInputFromCheckout(cc, -50) // negative discount clamps to 0
	if in.DiscountCents != 0 {
		t.Fatalf("negative discount not clamped: %d", in.DiscountCents)
	}
	if len(in.Lines) != 1 || in.Lines[0].SKU != "SKU-1" || in.Lines[0].UnitPriceCents != 5000 {
		t.Fatalf("lines wrong: %+v", in.Lines)
	}
	if in.Currency != "USD" {
		t.Fatalf("currency wrong: %q", in.Currency)
	}
	req := remoteTaxRequestFrom(in)
	if req.Address.Region != "CA" || req.Address.CountryCode != "US" {
		t.Fatalf("address wire wrong: %+v", req.Address)
	}
}

// fakeTaxSource exercises the RemoteTaxSource contract without HTTP
type fakeTaxSource struct {
	cents int64
	ok    bool
	err   error
}

func (f fakeTaxSource) Tax(context.Context, RemoteTaxInput) (int64, bool, error) {
	return f.cents, f.ok, f.err
}

func TestRemoteTaxSourceContract(t *testing.T) {
	// ok=true -> the cents are authoritative
	c, ok, err := fakeTaxSource{cents: 825, ok: true}.Tax(context.Background(), RemoteTaxInput{ChannelID: uuid.New()})
	if err != nil || !ok || c != 825 {
		t.Fatalf("ok path: %d %v %v", c, ok, err)
	}
	// ok=false, no err -> fall back to core
	_, ok, err = fakeTaxSource{ok: false}.Tax(context.Background(), RemoteTaxInput{})
	if ok || err != nil {
		t.Fatalf("fallback path: %v %v", ok, err)
	}
	// err -> abort placement
	if _, _, err = (fakeTaxSource{err: context.DeadlineExceeded}).Tax(context.Background(), RemoteTaxInput{}); err == nil {
		t.Fatal("expected abort error")
	}
}

// A repository whose lookup fails - everything else panics, which is fine because
// the seam should never get past the failed lookup
type failingProviderRepo struct {
	remoteprovider.Repository
}

func (failingProviderRepo) ListActiveByKind(context.Context, remoteprovider.Kind, *uuid.UUID) ([]remoteprovider.Provider, error) {
	return nil, errors.New("connection reset")
}

// A lookup failure must not read as "no tax provider configured": falling through
// commits the order at zero tax, silently, for a merchant who delegated tax entirely
func TestRemoteTaxLookupFailureDoesNotFallThroughToZeroTax(t *testing.T) {
	svc := remoteprovider.NewService(failingProviderRepo{})
	src := NewRemoteTaxSource(svc, remoteprovider.NewInvoker(svc, nil))

	cents, ok, err := src.Tax(context.Background(), RemoteTaxInput{ChannelID: uuid.New()})
	if err == nil {
		t.Fatalf("lookup failed but Tax returned cents=%d ok=%v with no error - the order would place untaxed", cents, ok)
	}
	if ok {
		t.Error("ok=true on a failed lookup")
	}
}
