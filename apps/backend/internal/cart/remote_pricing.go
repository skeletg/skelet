package cart

import (
	"context"
	"fmt"
	"log"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/internal/remoteprovider"
)

// A merchant HTTP endpoint (remoteprovider kind=pricing) resolving a SKU's unit price per customer and channel - the escape hatch for B2B contract or dynamic pricing without a Go plugin, CART and checkout only so catalog display keeps the hot read paths off the network; it runs on every cart op, so SET A CACHE TTL on the provider
// Wire contract: request {channelId, sku, customerId, basePriceCents}, response {unitPriceCents} where omitted or negative means no override - fail_open (the default) keeps the catalog price on an error or no price, fail_closed aborts the cart op for catalogs that must never sell at list price

type RemotePricingSource interface {
	Price(ctx context.Context, channelID uuid.UUID, sku string, customerID *uuid.UUID, basePriceCents int64) (cents int64, ok bool, err error) // ok=false keeps the catalog price, whether from no provider, no override, or a fail_open error - a non-nil error means a fail_closed provider failed, so abort the cart op
}

func (s *Service) SetRemotePricing(src RemotePricingSource) { s.remotePricing = src }

func (s *Service) resolvePricing(ctx context.Context, channelID uuid.UUID, sku string, customerID *uuid.UUID) (SimplePricing, error) { // the single funnel for cart pricing, catalog price then the remote override when wired, so every call site applies it uniformly
	p, err := s.products.ResolveSimplePricing(ctx, channelID, sku, customerID)
	if err != nil {
		return p, err
	}
	if s.remotePricing != nil {
		cents, ok, rerr := s.remotePricing.Price(ctx, channelID, sku, customerID, p.UnitPrice)
		if rerr != nil {
			return SimplePricing{}, rerr
		}
		if ok {
			p.UnitPrice = cents
		}
	}
	return p, nil
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

type remotePricingSource struct {
	svc     *remoteprovider.Service
	invoker *remoteprovider.Invoker
}

func NewRemotePricingSource(svc *remoteprovider.Service, invoker *remoteprovider.Invoker) RemotePricingSource {
	return &remotePricingSource{svc: svc, invoker: invoker}
}

func (r *remotePricingSource) Price(ctx context.Context, channelID uuid.UUID, sku string, customerID *uuid.UUID, basePriceCents int64) (int64, bool, error) {
	ch := channelID
	providers, err := r.svc.ListActiveByKind(ctx, remoteprovider.KindPricing, &ch)
	if err != nil {
		return 0, false, fmt.Errorf("pricing provider lookup: %w", err) // not the same as "no provider" - we can't see whether one exists or its failure policy, and falling through would snapshot the line at catalog price: list price for a contract customer, or below cost where catalog is a placeholder
	}
	if len(providers) == 0 {
		return 0, false, nil // no provider - catalog price
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
		log.Printf("[cart] remote pricing %q failed for %s (keeping catalog price): %v", p.Slug, sku, err)
		return 0, false, nil
	}
	if resp.UnitPriceCents == nil || *resp.UnitPriceCents < 0 {
		return 0, false, nil // no override for this SKU
	}
	return *resp.UnitPriceCents, true, nil
}
