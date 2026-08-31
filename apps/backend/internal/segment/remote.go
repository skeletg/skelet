package segment

import (
	"context"
	"log"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/internal/remoteprovider"
)

// A merchant HTTP endpoint (remoteprovider kind=customer_segments) returning the segment slugs a customer belongs to per an external CDP - the escape hatch for CDP-driven audiences without a Go plugin
// Remote slugs are UNIONED with the native ones, since like a risk signal segments are multi-source, so native rule-based segments and CDP audiences both apply - this feeds promotion eligibility and flow segment gating transparently
// Wire contract: request {customerId}, response {segments: ["vip", "lapsed", ...]}
// Account-global, and it runs during cart and flow evaluation, so set a cache TTL - Always fails open to native-only, so a down CDP never drops the native audiences

type RemoteSegmentSource interface {
	// The external segment slugs for a customer - ok=false when there's no provider or it failed, meaning native-only
	Slugs(ctx context.Context, customerID uuid.UUID) ([]string, bool)
}

// Optional - nil leaves segment membership on the native rule engine
func (s *Service) SetRemoteSegments(src RemoteSegmentSource) { s.remoteSegments = src }

// The customer's segment slugs, native UNIONED with any from a remote CDP provider - remote failures fall back to native-only
func (s *Service) SlugsForCustomer(ctx context.Context, customerID uuid.UUID) ([]string, error) {
	native, err := s.repo.SlugsForCustomer(ctx, customerID)
	if err != nil {
		return nil, err
	}
	if s.remoteSegments == nil {
		return native, nil
	}
	remote, ok := s.remoteSegments.Slugs(ctx, customerID)
	if !ok || len(remote) == 0 {
		return native, nil
	}
	seen := make(map[string]struct{}, len(native)+len(remote))
	out := make([]string, 0, len(native)+len(remote))
	for _, s := range native {
		if _, dup := seen[s]; !dup {
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	for _, s := range remote {
		if _, dup := seen[s]; !dup && s != "" {
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	return out, nil
}

type remoteSegmentRequest struct {
	CustomerID string `json:"customerId"`
}

type remoteSegmentResponse struct {
	Segments []string `json:"segments"`
}

type remoteSegmentSource struct {
	svc     *remoteprovider.Service
	invoker *remoteprovider.Invoker
}

func NewRemoteSegmentSource(svc *remoteprovider.Service, invoker *remoteprovider.Invoker) RemoteSegmentSource {
	return &remoteSegmentSource{svc: svc, invoker: invoker}
}

func (r *remoteSegmentSource) Slugs(ctx context.Context, customerID uuid.UUID) ([]string, bool) {
	providers, err := r.svc.ListActiveByKind(ctx, remoteprovider.KindCustomerSegments, nil) // account-level
	if err != nil || len(providers) == 0 {
		return nil, false
	}
	p := providers[0]
	var resp remoteSegmentResponse
	if err := r.invoker.Invoke(ctx, p, remoteSegmentRequest{CustomerID: customerID.String()}, &resp); err != nil {
		log.Printf("[segment] remote segments %q failed (native only): %v", p.Slug, err)
		return nil, false
	}
	return resp.Segments, true
}
