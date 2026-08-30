package plugin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/internal/payment"
	"github.com/skeletg/skelet/sdk"
)

// The sdk<->payment translation in providerAdapter is where a dropped field silently breaks a gateway -
// PaymentToken had no producer for weeks and nothing noticed - so every field crossing the boundary is
// pinned here in both directions

type capturingProvider struct {
	got     sdk.AuthorizeRequest
	webhook sdk.WebhookResult
}

func (p *capturingProvider) Authorize(_ context.Context, req sdk.AuthorizeRequest) (sdk.AuthorizeResult, error) {
	p.got = req
	return sdk.AuthorizeResult{ProviderReference: "ref-1"}, nil
}
func (p *capturingProvider) Capture(context.Context, sdk.CaptureRequest) (sdk.CaptureResult, error) {
	return sdk.CaptureResult{}, nil
}
func (p *capturingProvider) Refund(context.Context, sdk.RefundRequest) (sdk.RefundResult, error) {
	return sdk.RefundResult{}, nil
}
func (p *capturingProvider) Void(context.Context, sdk.VoidRequest) (sdk.VoidResult, error) {
	return sdk.VoidResult{}, nil
}
func (p *capturingProvider) HandleWebhook(context.Context, sdk.WebhookRequest) (sdk.WebhookResult, error) {
	return p.webhook, nil
}

func enableAllPlugins(t *testing.T) {
	t.Helper()
	orig := isEnabledFromContext
	t.Cleanup(func() { isEnabledFromContext = orig })
	isEnabledFromContext = func(context.Context, string, uuid.UUID) bool { return true }
}

func TestProviderAdapter_AuthorizeCarriesBothTokens(t *testing.T) {
	enableAllPlugins(t)
	fake := &capturingProvider{}
	a := &providerAdapter{p: fake, slug: "gw", providerID: "gw"}

	cid := uuid.New()
	_, err := a.Authorize(context.Background(), payment.AuthorizeRequest{
		OrderID:      uuid.New(),
		ChannelID:    uuid.New(),
		AmountCents:  999,
		CurrencyCode: "EUR",
		CustomerID:   &cid,
		Email:        "a@b.c",
		PaymentToken: "tok_x",
		VaultedToken: "pm_y",
		Metadata:     map[string]string{"k": "v"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if fake.got.PaymentToken != "tok_x" {
		t.Errorf("PaymentToken = %q, want tok_x", fake.got.PaymentToken)
	}
	if fake.got.VaultedToken != "pm_y" {
		t.Errorf("VaultedToken = %q, want pm_y", fake.got.VaultedToken)
	}
	if fake.got.Metadata["k"] != "v" {
		t.Errorf("Metadata dropped: %v", fake.got.Metadata)
	}
	if fake.got.AmountCents != 999 || fake.got.CurrencyCode != "EUR" || fake.got.Email != "a@b.c" {
		t.Error("a non-token field was dropped crossing the sdk boundary")
	}
}

func TestProviderAdapter_WebhookAmountsSurviveTranslation(t *testing.T) {
	enableAllPlugins(t)
	oid := uuid.New()
	fake := &capturingProvider{webhook: sdk.WebhookResult{
		OrderID:           oid,
		EventName:         "payment.refunded",
		ProviderReference: "re_1",
		AmountCents:       1000,
		CurrencyCode:      "USD",
	}}
	a := &providerAdapter{p: fake, slug: "gw", providerID: "gw"}

	res, err := a.HandleWebhook(context.Background(), httptest.NewRequest(http.MethodPost, "/hook", nil))
	if err != nil {
		t.Fatal(err)
	}
	want := payment.WebhookResult{OrderID: oid, EventName: "payment.refunded", ProviderReference: "re_1", AmountCents: 1000, CurrencyCode: "USD"}
	if res != want {
		t.Errorf("got %+v, want %+v", res, want)
	}
}
