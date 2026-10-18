package cart

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// stubResolver implements just enough of ProductResolver for resolvePricing
type stubResolver struct{ price int64 }

func (s stubResolver) ResolveSimplePricing(context.Context, uuid.UUID, string, *uuid.UUID) (SimplePricing, error) {
	return SimplePricing{ProductName: "Widget", UnitPrice: s.price}, nil
}
func (stubResolver) ResolveBundleBySKU(context.Context, uuid.UUID, string) (BundleSnapshot, error) {
	return BundleSnapshot{}, nil
}
func (stubResolver) ResolveProductOptions(context.Context, uuid.UUID, string, *uuid.UUID) (ProductOptionsResolution, error) {
	return ProductOptionsResolution{}, nil
}

type stubRemote struct {
	cents int64
	ok    bool
	err   error
}

func (r stubRemote) Price(context.Context, uuid.UUID, string, *uuid.UUID, int64) (int64, bool, error) {
	return r.cents, r.ok, r.err
}

func TestResolvePricingAppliesTheRemoteOverride(t *testing.T) {
	s := &Service{products: stubResolver{price: 10000}, remotePricing: stubRemote{cents: 7500, ok: true}}
	p, err := s.resolvePricing(context.Background(), uuid.New(), "SKU-1", nil)
	if err != nil || p.UnitPrice != 7500 {
		t.Fatalf("remote price not applied: price=%d err=%v", p.UnitPrice, err)
	}
}

func TestResolvePricingKeepsTheCatalogWhenTheRemoteDeclines(t *testing.T) {
	s := &Service{products: stubResolver{price: 10000}, remotePricing: stubRemote{}}
	p, err := s.resolvePricing(context.Background(), uuid.New(), "SKU-1", nil)
	if err != nil || p.UnitPrice != 10000 {
		t.Fatalf("catalog price should stand: price=%d err=%v", p.UnitPrice, err)
	}
}

func TestResolvePricingAbortsOnARemoteFailure(t *testing.T) {
	s := &Service{products: stubResolver{price: 10000}, remotePricing: stubRemote{err: errors.New("provider down")}}
	if _, err := s.resolvePricing(context.Background(), uuid.New(), "SKU-1", nil); err == nil {
		t.Fatal("a fail_closed outage must abort the cart operation")
	}
}

func TestResolvePricingWithNoRemoteIsTheCatalog(t *testing.T) {
	s := &Service{products: stubResolver{price: 10000}}
	p, err := s.resolvePricing(context.Background(), uuid.New(), "SKU-1", nil)
	if err != nil || p.UnitPrice != 10000 {
		t.Fatalf("no provider -> catalog price: price=%d err=%v", p.UnitPrice, err)
	}
}
