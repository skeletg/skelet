package cart

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/internal/remoteprovider"
)

// stubResolver implements just enough of ProductResolver for resolvePricing
type stubResolver struct{ price int64 }

func (s stubResolver) ResolveSimplePricing(context.Context, uuid.UUID, string, *uuid.UUID) (SimplePricing, error) {
	return SimplePricing{ProductName: "Widget", UnitPrice: s.price}, nil
}
func (stubResolver) ResolveBundleBySKU(context.Context, string) (BundleSnapshot, error) {
	return BundleSnapshot{}, nil
}
func (stubResolver) ResolveProductOptions(context.Context, uuid.UUID, string, *uuid.UUID) (ProductOptionsResolution, error) {
	return ProductOptionsResolution{}, nil
}

func pricingStack(t *testing.T, url string, policy remoteprovider.FailurePolicy) RemotePricingSource {
	t.Helper()
	svc := remoteprovider.NewService(remoteprovider.NewMemRepo())
	svc.AllowPrivateTargets = true
	if _, err := svc.Create(context.Background(), remoteprovider.CreateInput{
		Kind: remoteprovider.KindPricing, Slug: "contract", Name: "Contract", TargetURL: url, FailurePolicy: policy,
	}); err != nil {
		t.Fatalf("create provider: %v", err)
	}
	return NewRemotePricingSource(svc, remoteprovider.NewInvoker(svc, nil))
}

func TestResolvePricingRemoteOverride(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"unitPriceCents": 7500}`))
	}))
	defer srv.Close()

	s := &Service{products: stubResolver{price: 10000}, remotePricing: pricingStack(t, srv.URL, remoteprovider.FailOpen)}
	p, err := s.resolvePricing(context.Background(), uuid.New(), "SKU-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if p.UnitPrice != 7500 {
		t.Fatalf("remote price not applied: %d", p.UnitPrice)
	}
}

func TestResolvePricingNoOverrideKeepsCatalog(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`)) // no unitPriceCents = decline
	}))
	defer srv.Close()

	s := &Service{products: stubResolver{price: 10000}, remotePricing: pricingStack(t, srv.URL, remoteprovider.FailOpen)}
	p, _ := s.resolvePricing(context.Background(), uuid.New(), "SKU-1", nil)
	if p.UnitPrice != 10000 {
		t.Fatalf("catalog price should stand when provider declines: %d", p.UnitPrice)
	}
}

func TestResolvePricingFailOpenKeepsCatalog(t *testing.T) {
	s := &Service{products: stubResolver{price: 10000}, remotePricing: pricingStack(t, "http://127.0.0.1:9/down", remoteprovider.FailOpen)}
	p, err := s.resolvePricing(context.Background(), uuid.New(), "SKU-1", nil)
	if err != nil || p.UnitPrice != 10000 {
		t.Fatalf("fail_open should keep catalog price: price=%d err=%v", p.UnitPrice, err)
	}
}

func TestResolvePricingFailClosedAborts(t *testing.T) {
	s := &Service{products: stubResolver{price: 10000}, remotePricing: pricingStack(t, "http://127.0.0.1:9/down", remoteprovider.FailClosed)}
	if _, err := s.resolvePricing(context.Background(), uuid.New(), "SKU-1", nil); err == nil {
		t.Fatal("fail_closed should abort the cart operation on a pricing outage")
	}
}

func TestResolvePricingNoProviderIsCatalog(t *testing.T) {
	s := &Service{products: stubResolver{price: 10000}} // remotePricing nil
	p, err := s.resolvePricing(context.Background(), uuid.New(), "SKU-1", nil)
	if err != nil || p.UnitPrice != 10000 {
		t.Fatalf("no provider -> catalog price: price=%d err=%v", p.UnitPrice, err)
	}
}
