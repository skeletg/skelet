package wiring

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/remoteprovider"
)

// Adapts a remoteprovider kind=pricing endpoint to cart.RemotePricingSource - it runs on every cart op, so SET A CACHE TTL on the provider
// Wire contract: request {channelId, sku, customerId, basePriceCents}, response {unitPriceCents} where omitted or negative means no override - fail_open (the default) keeps the catalog price on an error, fail_closed aborts the cart op for catalogs that must never sell at list price
type cartRemotePricing struct {
	svc     *remoteprovider.Service
	invoker *remoteprovider.Invoker
}

func NewCartRemotePricing(svc *remoteprovider.Service, invoker *remoteprovider.Invoker) cart.RemotePricingSource {
	return &cartRemotePricing{svc: svc, invoker: invoker}
}

type remotePricingRequest struct {
	ChannelID      string `json:"channelId"`
	SKU            string `json:"sku"`
	CustomerID     string `json:"customerId,omitempty"`
	BasePriceCents int64  `json:"basePriceCents"`
}

type remotePricingResponse struct {
	UnitPriceCents *int64 `json:"unitPriceCents"`
}

func (r *cartRemotePricing) Price(ctx context.Context, channelID uuid.UUID, sku string, customerID *uuid.UUID, basePriceCents int64) (int64, bool, error) {
	ch := channelID
	providers, err := r.svc.ListActiveByKind(ctx, remoteprovider.KindPricing, &ch)
	if err != nil {
		return 0, false, fmt.Errorf("pricing provider lookup: %w", err) // not the same as "no provider" - we can't see whether one exists or its failure policy, and falling through would snapshot the line at catalog price: list price for a contract customer, or below cost where catalog is a placeholder
	}
	if len(providers) == 0 {
		return 0, false, nil
	}
	p := providers[0]

	req := remotePricingRequest{ChannelID: channelID.String(), SKU: sku, BasePriceCents: basePriceCents}
	if customerID != nil {
		req.CustomerID = customerID.String()
	}
	var resp remotePricingResponse
	if err := r.invoker.Invoke(ctx, p, req, &resp); err != nil {
		if p.FailurePolicy == remoteprovider.FailClosed {
			return 0, false, err
		}
		slog.WarnContext(ctx, "cart: remote pricing failed, keeping the catalog price", "provider", p.Slug, "sku", sku, "err", err)
		return 0, false, nil
	}
	if resp.UnitPriceCents == nil || *resp.UnitPriceCents < 0 {
		return 0, false, nil
	}
	return *resp.UnitPriceCents, true, nil
}
