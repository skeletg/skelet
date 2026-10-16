package payment

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestDisabledPluginIsNotOfferedOrEnabled(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)
	channelID := uuid.New()
	method := PaymentMethod{ID: uuid.New(), ChannelID: channelID, ProviderSlug: "test-gateway", IsEnabled: true, CountryMode: "all"}
	RegisterProvider(Registration{Slug: method.ProviderSlug, PluginSlug: "payment-test", Provider: &fakeProvider{}, Available: func(_ context.Context, id uuid.UUID) bool {
		if id != channelID {
			t.Errorf("availability checked for %s, want %s", id, channelID)
		}
		return false
	}})
	svc := NewService(fakePayRepo{methods: []PaymentMethod{method}}, nil, nil)
	methods, err := svc.ListEligiblePaymentMethods(context.Background(), channelID, "RS")
	if err != nil || len(methods) != 0 {
		t.Fatalf("eligible methods = %v, err = %v", methods, err)
	}
	if err := svc.ValidatePaymentForChannel(context.Background(), channelID, method.ProviderSlug, "RS"); !errors.Is(err, ErrProviderDisabled) {
		t.Fatalf("validation error = %v", err)
	}
	if _, err := svc.SetEnabled(context.Background(), channelID, method.ProviderSlug, true); !errors.Is(err, ErrProviderDisabled) {
		t.Fatalf("enable error = %v", err)
	}
	repo := newFakeRepo()
	repo.setMethod(method)
	svc = NewService(repo, nil, nil)
	off, err := svc.SetEnabled(context.Background(), channelID, method.ProviderSlug, false)
	if err != nil || off.IsEnabled {
		t.Fatalf("disable = %v, err = %v", off, err)
	}
}

func TestPresentationPreservesProviderAndChannelAvailability(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)
	channelID := uuid.New()
	other := uuid.New()
	RegisterProvider(Registration{Slug: "cards", Provider: &fakeProvider{}})
	repo := newFakeRepo()
	repo.setMethod(PaymentMethod{ChannelID: channelID, ProviderSlug: "cards", DisplayName: "Gateway", IsEnabled: true, CountryMode: "specific"})
	repo.setMethod(PaymentMethod{ChannelID: other, ProviderSlug: "cards", DisplayName: "Other shop", CountryMode: "all"})
	svc := NewService(repo, nil, nil)
	result, err := svc.UpdatePresentation(context.Background(), channelID, "cards", "  Credit/debit card  ", 5)
	if err != nil {
		t.Fatal(err)
	}
	if result.DisplayName != "Credit/debit card" || result.SortOrder != 5 || !result.IsEnabled || result.CountryMode != "specific" || result.ProviderSlug != "cards" {
		t.Fatalf("updated method = %+v", result)
	}
	if repo.methods[methodKey{other, "cards"}].DisplayName != "Other shop" {
		t.Fatal("another channel was changed")
	}
	if _, err := svc.UpdatePresentation(context.Background(), channelID, "cards", "  ", 5); err == nil {
		t.Fatal("blank label accepted")
	}
	if _, err := svc.UpdatePresentation(context.Background(), channelID, "not-installed", "Card", 5); !errors.Is(err, ErrProviderNotFound) {
		t.Fatalf("missing provider error = %v", err)
	}
}
