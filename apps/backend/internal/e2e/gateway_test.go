package e2e

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/order"
	"github.com/skeletg/skelet/apps/backend/internal/payment"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

// A deterministic gateway registered through the real SDK - authorize succeeds with a stable
// reference, capture's reference is configurable per test, webhooks decode a plain JSON body
type fakeGateway struct {
	mu         sync.Mutex
	captureRef string
	// what the last Authorize actually received - the typed token slots once had NO producer and every
	// gateway silently authorized with nothing, so e2e now watches the seam itself
	lastAuthorize sdk.AuthorizeRequest
}

func (g *fakeGateway) lastAuth() sdk.AuthorizeRequest {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.lastAuthorize
}

func (g *fakeGateway) setCaptureRef(ref string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.captureRef = ref
}

func (g *fakeGateway) Authorize(_ context.Context, req sdk.AuthorizeRequest) (sdk.AuthorizeResult, error) {
	g.mu.Lock()
	g.lastAuthorize = req
	g.mu.Unlock()
	return sdk.AuthorizeResult{ProviderReference: "auth-" + req.OrderID.String()[:8]}, nil
}

func (g *fakeGateway) Capture(_ context.Context, _ sdk.CaptureRequest) (sdk.CaptureResult, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return sdk.CaptureResult{ProviderReference: g.captureRef}, nil
}

func (g *fakeGateway) Refund(_ context.Context, _ sdk.RefundRequest) (sdk.RefundResult, error) {
	return sdk.RefundResult{ProviderReference: "refund-ok"}, nil
}

func (g *fakeGateway) Void(_ context.Context, _ sdk.VoidRequest) (sdk.VoidResult, error) {
	return sdk.VoidResult{}, nil
}

func (g *fakeGateway) VaultPaymentMethod(_ context.Context, req sdk.VaultRequest) (sdk.VaultedMethod, error) {
	return sdk.VaultedMethod{ProviderToken: "vault-" + req.Token, Brand: "e2e", Last4: "4242", ExpMonth: 12, ExpYear: 2039}, nil
}

func (g *fakeGateway) DeleteVaultedPaymentMethod(context.Context, sdk.VaultDeleteRequest) error {
	return nil
}

func (g *fakeGateway) HandleWebhook(_ context.Context, req sdk.WebhookRequest) (sdk.WebhookResult, error) {
	var body struct {
		OrderID string `json:"order_id"`
		Event   string `json:"event"`
		Ref     string `json:"ref"`
	}
	if err := json.NewDecoder(req.HTTPRequest.Body).Decode(&body); err != nil {
		return sdk.WebhookResult{}, err
	}
	id, err := uuid.Parse(body.OrderID)
	if err != nil {
		return sdk.WebhookResult{}, err
	}
	return sdk.WebhookResult{OrderID: id, EventName: body.Event, ProviderReference: body.Ref}, nil
}

const gwSlug = "e2e-gw"

var (
	gatewayOnce sync.Once
	fakeGW      = &fakeGateway{}
)

// Registration is process-global (duplicate slugs and config groups panic by design), the
// per-channel rows and enablement are per fixture
func (h *harness) gatewayStore(t *testing.T, stock float64, priceCents int64) *store {
	t.Helper()
	gatewayOnce.Do(func() {
		sdk.NewPluginForTest(gwSlug).RegisterPaymentProvider(gwSlug, fakeGW)
	})
	f := h.NewStore(t, stock, priceCents)
	if err := h.D.Payment.Sync(h.Ctx); err != nil {
		t.Fatalf("payment sync for gateway: %v", err)
	}
	if _, err := h.D.Payment.SetEnabled(h.Ctx, f.Channel.ID, gwSlug, true); err != nil {
		t.Fatalf("enable gateway: %v", err)
	}
	return f
}

func (h *harness) placeWithGateway(t *testing.T, f *store) order.Order {
	t.Helper()
	gw := gwSlug
	o, err := h.TryPlace(t, f.Channel, h.NewGuestCart(t, f, 1), func(in *order.PlaceOrderInput) {
		in.PaymentProviderSlug = &gw
	})
	if err != nil {
		t.Fatalf("place with gateway: %v", err)
	}
	return o
}

// Scenario: a gateway placement authorizes post-commit - the ledger holds the authorize row
// immediately, and the async payment.authorized event settles the status
func TestGatewayAuthorizeAtPlacement(t *testing.T) {
	h := newHarness(t)
	f := h.gatewayStore(t, 5, 2600)
	o := h.placeWithGateway(t, f)

	sums := h.LedgerByKind(t, o.ID)
	if sums[order.PaymentTxnAuthorize] != 2600 {
		t.Errorf("authorize ledger = %+v, want 2600", sums)
	}
	h.Settle(t)
	row, err := h.D.Queries.GetOrderByID(h.Ctx, o.ID)
	if err != nil {
		t.Fatalf("re-read order: %v", err)
	}
	if row.PaymentStatus != "authorized" {
		t.Errorf("payment_status = %q, want authorized", row.PaymentStatus)
	}
}

// Scenario: the storefront's tokenization handle survives the whole placement path - resolver input
// to order service to wiring adapter to payment service to the gateway - this seam once dropped every
// token silently (the metadata-key indirection was emptied and the typed slot had no producer)
func TestGatewayReceivesThePaymentToken(t *testing.T) {
	h := newHarness(t)
	f := h.gatewayStore(t, 5, 2600)
	gw := gwSlug
	tok := "tok-" + uuid.NewString()[:8]
	if _, err := h.TryPlace(t, f.Channel, h.NewGuestCart(t, f, 1), func(in *order.PlaceOrderInput) {
		in.PaymentProviderSlug = &gw
		in.PaymentToken = &tok
	}); err != nil {
		t.Fatalf("place with token: %v", err)
	}
	got := fakeGW.lastAuth()
	if got.PaymentToken != tok {
		t.Errorf("gateway saw PaymentToken %q, want %q", got.PaymentToken, tok)
	}
	if got.VaultedToken != "" {
		t.Errorf("one-shot placement must not carry a vaulted token, got %q", got.VaultedToken)
	}
}

// Scenario: capture-on-first-ship - fulfilling an authorized order captures through the gateway
// off the durable fulfillment event, and the status derives to paid
func TestGatewayCaptureOnFulfillment(t *testing.T) {
	h := newHarness(t)
	f := h.gatewayStore(t, 5, 2600)
	fakeGW.setCaptureRef("") // empty reference: the event bridge settles status without a second ledger row
	o := h.placeWithGateway(t, f)
	h.Settle(t)

	for _, status := range []string{"confirmed", "processing"} {
		if _, err := h.D.Order.UpdateStatus(h.Ctx, o.ID, status); err != nil {
			t.Fatalf("advance to %s: %v", status, err)
		}
	}
	if _, err := h.D.Order.FulfillRemaining(h.Ctx, o.ID, "", "", 0); err != nil {
		t.Fatalf("fulfill: %v", err)
	}
	if total, _ := h.OutboxRows(t, "order.fulfillment_created", o.ID.String()); total != 1 {
		t.Errorf("fulfillment outbox rows = %d, want 1 - capture rides on it", total)
	}
	h.Settle(t)

	row, err := h.D.Queries.GetOrderByID(h.Ctx, o.ID)
	if err != nil {
		t.Fatalf("re-read order: %v", err)
	}
	if row.PaymentStatus != "paid" {
		t.Errorf("payment_status = %q after capture, want paid", row.PaymentStatus)
	}
	sums := h.LedgerByKind(t, o.ID)
	if sums[order.PaymentTxnCapture] != 2600 {
		t.Errorf("capture ledger = %+v, want exactly 2600", sums)
	}
}

// Scenario: the invariant the ledger must hold regardless of what the gateway echoes back - one fulfillment capture books the amount ONCE.
// The capture event carries the provider reference, and nothing about a reference existing may double the booked money
func TestGatewayCaptureLedgerNotDoubleCounted(t *testing.T) {
	h := newHarness(t)
	f := h.gatewayStore(t, 5, 1800)
	fakeGW.setCaptureRef("cap-" + uuid.NewString()[:8])
	o := h.placeWithGateway(t, f)
	h.Settle(t)

	for _, status := range []string{"confirmed", "processing"} {
		if _, err := h.D.Order.UpdateStatus(h.Ctx, o.ID, status); err != nil {
			t.Fatalf("advance to %s: %v", status, err)
		}
	}
	if _, err := h.D.Order.FulfillRemaining(h.Ctx, o.ID, "", "", 0); err != nil {
		t.Fatalf("fulfill: %v", err)
	}
	h.Settle(t)

	sums := h.LedgerByKind(t, o.ID)
	if sums[order.PaymentTxnCapture] != 1800 {
		t.Errorf("capture ledger = %+v, want 1800 booked exactly once - a reference-bearing capture must not settle twice", sums)
	}
}

// Scenario: webhooks walk the coarse status machine idempotently and never write ledger rows -
// replays and out-of-order deliveries both no-op
func TestWebhookReplayAndOutOfOrder(t *testing.T) {
	h := newHarness(t)
	f := h.gatewayStore(t, 5, 2100)
	o := h.placeWithGateway(t, f)
	h.Settle(t)

	webhook := func(event, ref string) {
		t.Helper()
		body := `{"order_id":"` + o.ID.String() + `","event":"` + event + `","ref":"` + ref + `"}`
		req := httptest.NewRequest("POST", "/api/v1/payments/webhook/"+gwSlug, strings.NewReader(body))
		if err := h.D.Payment.DispatchWebhook(h.Ctx, gwSlug, req); err != nil {
			t.Fatalf("webhook %s: %v", event, err)
		}
		h.Settle(t)
	}

	paymentStatus := func() string {
		t.Helper()
		row, err := h.D.Queries.GetOrderByID(h.Ctx, o.ID)
		if err != nil {
			t.Fatalf("re-read order: %v", err)
		}
		return row.PaymentStatus
	}

	webhook("payment.captured", "gw-1")
	if got := paymentStatus(); got != "paid" {
		t.Errorf("after captured webhook: %q, want paid", got)
	}
	webhook("payment.captured", "gw-1") // replay
	if got := paymentStatus(); got != "paid" {
		t.Errorf("after replayed webhook: %q, want still paid", got)
	}
	webhook("payment.authorized", "gw-2") // out of order - paid never regresses
	if got := paymentStatus(); got != "paid" {
		t.Errorf("after out-of-order authorize webhook: %q, want still paid", got)
	}

	// The webhook path is status-only: the ledger still holds nothing but the placement authorize
	sums := h.LedgerByKind(t, o.ID)
	if sums[order.PaymentTxnCapture] != 0 || sums[order.PaymentTxnAuthorize] != 2100 {
		t.Errorf("ledger = %+v, want authorize 2100 and no capture rows from webhooks", sums)
	}
}

// Scenario: voiding an authorized, uncaptured order - the admin sequence driven directly
func TestVoidAuthorizedOrder(t *testing.T) {
	h := newHarness(t)
	f := h.gatewayStore(t, 5, 3100)
	o := h.placeWithGateway(t, f)
	h.Settle(t)

	row, err := h.D.Queries.GetOrderByID(h.Ctx, o.ID)
	if err != nil {
		t.Fatalf("re-read order: %v", err)
	}
	ref := row.PaymentReference
	if _, err := h.D.Payment.Void(h.Ctx, gwSlug, payment.VoidRequest{
		OrderID: o.ID, ChannelID: f.Channel.ID, ProviderReference: ref,
	}); err != nil {
		t.Fatalf("gateway void: %v", err)
	}
	if _, err := h.D.Order.RecordVoid(h.Ctx, o.ID, 3100, gwSlug, ref); err != nil {
		t.Fatalf("record void: %v", err)
	}
	if _, err := h.D.Order.SettlePayment(h.Ctx, o.ID, order.PaymentVoided, ref); err != nil {
		t.Fatalf("settle void: %v", err)
	}
	h.Settle(t) // the duplicate settle from payment.voided must no-op

	after, err := h.D.Queries.GetOrderByID(h.Ctx, o.ID)
	if err != nil {
		t.Fatalf("re-read order: %v", err)
	}
	if after.PaymentStatus != "voided" {
		t.Errorf("payment_status = %q, want voided", after.PaymentStatus)
	}
	sums := h.LedgerByKind(t, o.ID)
	if sums[order.PaymentTxnVoid] != 3100 {
		t.Errorf("void ledger = %+v, want 3100", sums)
	}
}
