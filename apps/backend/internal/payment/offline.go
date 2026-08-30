package payment

import (
	"context"
	"fmt"
	"net/http"

	"github.com/skeletg/skelet/internal/config"
)

// COD and bank transfer are commerce primitives with no third party behind them, so they ship with the engine rather than as plugins - same call as shipping's builtin and pickup carriers
// Per-channel enable is payment_methods.is_enabled, which admins already own - a plugin would use its manifest kill switch instead

type offlineMethod struct {
	slug         string
	displayName  string
	instructions string // default only - overridable per channel at payment/<slug>/instructions
}

var offlineMethods = []offlineMethod{
	{
		slug:         "cod",
		displayName:  "Cash on Delivery",
		instructions: "Pay with cash when your order is delivered. Please have the exact amount ready for the courier.",
	},
	{
		slug:         "bank-transfer",
		displayName:  "Bank Transfer / Invoice",
		instructions: "We'll email you our bank details. Reference your order number on the transfer; your order ships once payment is received.",
	},
	{
		slug:         "pay-on-pickup",
		displayName:  "Pay on Pickup",
		instructions: "Reserve now and pay in store when you collect your order.",
	},
	{
		slug:         "purchase-order",
		displayName:  "Purchase Order",
		instructions: "Enter your PO number in the order notes. We'll invoice you on your agreed terms.",
	},
}

// init rather than main.go wiring because these are stateless - shipping's builtin carrier needs its constructed service and has to wait
func init() {
	registerOfflineMethods()
	for _, m := range offlineMethods {
		config.RegisterGroup(config.Group{
			Section: "payment",
			Slug:    m.slug,
			Label:   m.displayName,
			// Instructions are shown to the shopper, so each channel gets its own wording
			Scoping: config.ScopingChannel,
		})
		config.Register(config.Definition{
			Path:    "payment/" + m.slug + "/instructions",
			Type:    config.TypeString,
			Default: m.instructions,
			Label:   m.displayName + " - payment instructions shown to the customer",
		})
	}
}

// Separate from the config registration because ResetForTest replays it - config.Register panics on
// a duplicate path, so only the provider half may run twice
func registerOfflineMethods() {
	for _, m := range offlineMethods {
		RegisterProvider(Registration{
			Slug:        m.slug,
			Provider:    &offlineProvider{slug: m.slug},
			DisplayName: m.displayName,
			IsOffline:   true,
		})
	}
}

// Unreachable in the normal flow - IsOffline short-circuits authorize at placement and the service refuses capture/void/refund - so these error rather than return a zero value that would read as a settled payment
// No plugin.Instrument wrapper: that contains third-party code, and this does no I/O
type offlineProvider struct {
	slug string
}

func (p *offlineProvider) Authorize(context.Context, AuthorizeRequest) (AuthorizeResult, error) {
	return AuthorizeResult{}, fmt.Errorf("%s: offline method - no gateway authorization; reconcile with recordOfflinePayment", p.slug)
}

func (p *offlineProvider) Capture(context.Context, CaptureRequest) (CaptureResult, error) {
	return CaptureResult{}, fmt.Errorf("%s: offline method - no gateway capture", p.slug)
}

func (p *offlineProvider) Refund(context.Context, RefundRequest) (RefundResult, error) {
	return RefundResult{}, fmt.Errorf("%s: offline method - no gateway refund", p.slug)
}

func (p *offlineProvider) Void(context.Context, VoidRequest) (VoidResult, error) {
	return VoidResult{}, fmt.Errorf("%s: offline method - no gateway void", p.slug)
}

func (p *offlineProvider) HandleWebhook(context.Context, *http.Request) (WebhookResult, error) {
	return WebhookResult{}, fmt.Errorf("%s: offline method - no webhook", p.slug)
}
