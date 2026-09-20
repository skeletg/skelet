package catalog

import (
	"context"
	"log"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/internal/remoteprovider"
)

// Merchant HTTP endpoint (remoteprovider kind=recommendation) returning recommended product ids - the escape hatch for Nosto/Dynamic-Yield-class engines without a Go plugin Supplies only the ALGORITHMIC "customers also bought" rail; operator-curated related/upsell/crosssell links stay curated Global, no channel scope - Wire contract: request {productId, strategy, limit}, response {productIds: [...]} Effectively always fail-open - fail_closed is treated the same, since a down rec engine must never blank a product page

type RemoteRecommendationSource interface {
	Recommend(ctx context.Context, productID uuid.UUID, strategy string, limit int32) ([]uuid.UUID, bool)
}

// Optional - nil leaves recommendations on the native co-occurrence table
func (s *Service) SetRemoteRecommendations(src RemoteRecommendationSource) { s.remoteRecs = src }

// Shadows the embedded listingService method to consult the remote provider first - the single high-leverage hook, since both the PDP rail and the cart rail ride on it
func (s *Service) ListAlsoBought(ctx context.Context, productID uuid.UUID, limit int32) ([]uuid.UUID, error) {
	if s.remoteRecs != nil {
		if ids, ok := s.remoteRecs.Recommend(ctx, productID, "also_bought", limit); ok {
			return ids, nil
		}
	}
	return s.listingService.ListAlsoBought(ctx, productID, limit)
}

type remoteRecRequest struct {
	ProductID string `json:"productId"`
	Strategy  string `json:"strategy"`
	Limit     int32  `json:"limit"`
}

type remoteRecResponse struct {
	ProductIDs []string `json:"productIds"`
}

type remoteRecommendationSource struct {
	svc     *remoteprovider.Service
	invoker *remoteprovider.Invoker
}

func NewRemoteRecommendationSource(svc *remoteprovider.Service, invoker *remoteprovider.Invoker) RemoteRecommendationSource {
	return &remoteRecommendationSource{svc: svc, invoker: invoker}
}

func (r *remoteRecommendationSource) Recommend(ctx context.Context, productID uuid.UUID, strategy string, limit int32) ([]uuid.UUID, bool) {
	providers, err := r.svc.ListActiveByKind(ctx, remoteprovider.KindRecommendation, nil) // account-level
	if err != nil || len(providers) == 0 {
		return nil, false
	}
	p := providers[0]

	var resp remoteRecResponse
	if err := r.invoker.Invoke(ctx, p, remoteRecRequest{ProductID: productID.String(), Strategy: strategy, Limit: limit}, &resp); err != nil {
		log.Printf("[catalog] remote recommendation %q failed (native fallback): %v", p.Slug, err)
		return nil, false
	}
	ids := make([]uuid.UUID, 0, len(resp.ProductIDs))
	for _, s := range resp.ProductIDs {
		if id, err := uuid.Parse(s); err == nil {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil, false // empty result -> native fallback
	}
	return ids, true
}
