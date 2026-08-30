package review

import (
	"context"
	"log"
	"maps"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/internal/remoteprovider"
)

// A merchant HTTP endpoint (remoteprovider kind=reviews) supplying rating aggregates from an external platform - the escape hatch for Yotpo, Bazaarvoice, Okendo or Trustpilot without a Go plugin
// It covers only the headline RATING AGGREGATE - count, average, 5-bucket distribution - and does NOT replace the native workflow of submission, per-question ratings, voting and moderation - PerQuestion stays native and empty here, which is correct since those platforms have no per-question model
// Wire contract: request {productIds}, response {aggregates: {"<productId>": {count, average, distribution}}}
// Account-level, and fail_open - no provider, a missing product or an error all fall back to native, since a down reviews API must never blank a product page

type RemoteAggregateSource interface {
	// Rating aggregates by product id - ok=false falls back to native, and products the provider omits keep their native aggregate
	Aggregates(ctx context.Context, productIDs []uuid.UUID) (map[uuid.UUID]Aggregates, bool)
}

// Optional - nil leaves aggregates on native review data
func (s *Service) SetRemoteAggregates(src RemoteAggregateSource) { s.remoteAggs = src }

// One product's aggregate, remote-first
func (s *Service) GetAggregates(ctx context.Context, productID uuid.UUID) (Aggregates, error) {
	if s.remoteAggs != nil {
		if m, ok := s.remoteAggs.Aggregates(ctx, []uuid.UUID{productID}); ok {
			if agg, present := m[productID]; present {
				return agg, nil
			}
		}
	}
	return s.repo.GetAggregates(ctx, productID)
}

// The batched form - remote overrides native per product, and omitted products fall back to native, so a mixed catalog renders right
func (s *Service) GetAggregatesByProductIDs(ctx context.Context, productIDs []uuid.UUID) (map[uuid.UUID]Aggregates, error) {
	native, err := s.repo.GetAggregatesByProductIDs(ctx, productIDs)
	if err != nil {
		return nil, err
	}
	if s.remoteAggs != nil {
		if remote, ok := s.remoteAggs.Aggregates(ctx, productIDs); ok {
			if native == nil {
				native = make(map[uuid.UUID]Aggregates, len(remote))
			}
			maps.Copy(native, remote)
		}
	}
	return native, nil
}

type remoteReviewRequest struct {
	ProductIDs []string `json:"productIds"`
}

type remoteReviewAggregate struct {
	Count        int      `json:"count"`
	Average      *float64 `json:"average"`
	Distribution [5]int   `json:"distribution"`
}

type remoteReviewResponse struct {
	Aggregates map[string]remoteReviewAggregate `json:"aggregates"`
}

type remoteAggregateSource struct {
	svc     *remoteprovider.Service
	invoker *remoteprovider.Invoker
}

func NewRemoteAggregateSource(svc *remoteprovider.Service, invoker *remoteprovider.Invoker) RemoteAggregateSource {
	return &remoteAggregateSource{svc: svc, invoker: invoker}
}

func (r *remoteAggregateSource) Aggregates(ctx context.Context, productIDs []uuid.UUID) (map[uuid.UUID]Aggregates, bool) {
	providers, err := r.svc.ListActiveByKind(ctx, remoteprovider.KindReviews, nil) // account-level
	if err != nil || len(providers) == 0 {
		return nil, false
	}
	p := providers[0]

	ids := make([]string, len(productIDs))
	for i, id := range productIDs {
		ids[i] = id.String()
	}
	var resp remoteReviewResponse
	if err := r.invoker.Invoke(ctx, p, remoteReviewRequest{ProductIDs: ids}, &resp); err != nil {
		log.Printf("[review] remote reviews %q failed (native fallback): %v", p.Slug, err)
		return nil, false
	}
	out := make(map[uuid.UUID]Aggregates, len(resp.Aggregates))
	for idStr, a := range resp.Aggregates {
		id, perr := uuid.Parse(idStr)
		if perr != nil {
			continue
		}
		out[id] = Aggregates{Count: a.Count, OverallAverage: a.Average, Distribution: a.Distribution}
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}
