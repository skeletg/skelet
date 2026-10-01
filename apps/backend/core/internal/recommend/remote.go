package recommend

import (
	"context"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/remoteprovider"
	"github.com/skeletg/skelet/apps/backend/core/lib/applog"
)

const StrategyAlsoBought = "also_bought"

// Merchant HTTP endpoint (remoteprovider kind=recommendation) returning recommended product ids - the escape hatch for Nosto/Dynamic-Yield-class engines without a Go plugin Supplies only the ALGORITHMIC "customers also bought" rail; operator-curated related/upsell/crosssell links stay curated - the storefront request's channel picks its own provider over a global one - Wire contract: request {productId, strategy, limit}, response {productIds: [...]} Effectively always fail-open - fail_closed is treated the same, since a down rec engine must never blank a product page

type RemoteRecommendationSource interface {
	Recommend(ctx context.Context, productID uuid.UUID, strategy string, limit int32) ([]uuid.UUID, bool)
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
	var chID *uuid.UUID
	if ch, ok := channel.FromContext(ctx); ok && ch != nil {
		chID = &ch.ID
	}
	p, found, err := r.svc.ActiveFor(ctx, remoteprovider.KindRecommendation, chID)
	if err != nil {
		applog.From(ctx).Warn("remote recommendation provider lookup failed (native fallback)", "error", err)
		return nil, false
	}
	if !found {
		return nil, false
	}

	var resp remoteRecResponse
	if err := r.invoker.Invoke(ctx, p, remoteRecRequest{ProductID: productID.String(), Strategy: strategy, Limit: limit}, &resp); err != nil {
		applog.From(ctx).Warn("remote recommendation failed (native fallback)", "provider", p.Slug, "error", err)
		return nil, false
	}
	ids := cleanRecommendations(resp.ProductIDs, productID, limit)
	if len(ids) == 0 {
		return nil, false // empty result -> native fallback
	}
	return ids, true
}

// A provider's list is untrusted: unparsable ids, repeats and the page's own product drop out, and it never outgrows limit
func cleanRecommendations(raw []string, self uuid.UUID, limit int32) []uuid.UUID {
	ids := make([]uuid.UUID, 0, min(len(raw), max(int(limit), 0)))
	seen := map[uuid.UUID]struct{}{self: {}}
	for _, s := range raw {
		if limit > 0 && len(ids) >= int(limit) {
			break
		}
		id, err := uuid.Parse(s)
		if err != nil {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}
