package inventory

import (
	"context"
	"log"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/internal/remoteprovider"
)

// A merchant HTTP endpoint (remoteprovider kind=inventory_availability) answering salable stock from an external WMS or ERP - the escape hatch for ERP-owned inventory without a Go plugin
// Read on every card, PDP, cart and search hit, making it the hottest provider - SET A CACHE TTL - Channel-scoped, and always fails open to native
// Wire contract: request {productIds, channelId}, response {salable: {"<productId>": qty}} where -1 is unlimited

type RemoteAvailabilitySource interface {
	// Salable qty per product id - ok=false falls back to native, and omitted products fall back per-product
	Salable(ctx context.Context, productIDs []uuid.UUID, channelID uuid.UUID) (map[uuid.UUID]float64, bool)
}

func (s *Service) SetRemoteAvailability(src RemoteAvailabilitySource) { s.remoteAvail = src }

type remoteAvailRequest struct {
	ProductIDs []string `json:"productIds"`
	ChannelID  string   `json:"channelId"`
}

type remoteAvailResponse struct {
	Salable map[string]float64 `json:"salable"`
}

type remoteAvailabilitySource struct {
	svc     *remoteprovider.Service
	invoker *remoteprovider.Invoker
}

func NewRemoteAvailabilitySource(svc *remoteprovider.Service, invoker *remoteprovider.Invoker) RemoteAvailabilitySource {
	return &remoteAvailabilitySource{svc: svc, invoker: invoker}
}

func (r *remoteAvailabilitySource) Salable(ctx context.Context, productIDs []uuid.UUID, channelID uuid.UUID) (map[uuid.UUID]float64, bool) {
	ch := channelID
	providers, err := r.svc.ListActiveByKind(ctx, remoteprovider.KindInventoryAvail, &ch)
	if err != nil || len(providers) == 0 {
		return nil, false
	}
	p := providers[0]

	ids := make([]string, len(productIDs))
	for i, id := range productIDs {
		ids[i] = id.String()
	}
	var resp remoteAvailResponse
	if err := r.invoker.Invoke(ctx, p, remoteAvailRequest{ProductIDs: ids, ChannelID: channelID.String()}, &resp); err != nil {
		log.Printf("[inventory] remote availability %q failed (native fallback): %v", p.Slug, err)
		return nil, false
	}
	if len(resp.Salable) == 0 {
		return nil, false
	}
	out := make(map[uuid.UUID]float64, len(resp.Salable))
	for idStr, qty := range resp.Salable {
		if id, perr := uuid.Parse(idStr); perr == nil {
			out[id] = qty
		}
	}
	return out, true
}
