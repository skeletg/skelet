package example

import (
	"context"
	"fmt"
	"sync"

	"github.com/skeletg/skelet/apps/backend/sdk"
)

// Records what the host handed it so the test can assert the translation layer, and reads config
// through the injected scoped reader so a broken prefix shows up as a failed authorize
type provider struct {
	mu   sync.Mutex
	last sdk.AuthorizeRequest
	mode string
}

func (p *provider) lastAuthorize() (sdk.AuthorizeRequest, string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.last, p.mode
}

func (p *provider) Authorize(ctx context.Context, req sdk.AuthorizeRequest) (sdk.AuthorizeResult, error) {
	// short key, not the full path - proves the reader prefixes with payment/<provider>/
	mode, err := req.Config.Get(ctx, "mode", req.ChannelID)
	if err != nil {
		return sdk.AuthorizeResult{}, fmt.Errorf("read mode: %w", err)
	}
	p.mu.Lock()
	p.last, p.mode = req, mode
	p.mu.Unlock()
	return sdk.AuthorizeResult{
		ProviderReference: "auth_" + req.OrderID.String(),
		RawResponse:       []byte(`{"status":"authorized"}`),
	}, nil
}

func (p *provider) Capture(_ context.Context, req sdk.CaptureRequest) (sdk.CaptureResult, error) {
	return sdk.CaptureResult{ProviderReference: req.ProviderReference}, nil
}

func (p *provider) Refund(_ context.Context, req sdk.RefundRequest) (sdk.RefundResult, error) {
	return sdk.RefundResult{ProviderReference: req.ProviderReference}, nil
}

func (p *provider) Void(context.Context, sdk.VoidRequest) (sdk.VoidResult, error) {
	return sdk.VoidResult{}, nil
}

func (p *provider) HandleWebhook(context.Context, sdk.WebhookRequest) (sdk.WebhookResult, error) {
	return sdk.WebhookResult{EventName: EventSettled}, nil
}
