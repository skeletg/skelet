package wiring

import (
	"context"
	"log"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/remoteprovider"
)

// A merchant HTTP endpoint (remoteprovider kind=customer_segments) answering which segments a customer is in, per an external CDP
// Wire contract: request {customerId}, response {segments: ["vip", "lapsed", ...]} - account-global and it runs during cart evaluation, so set a cache TTL

type remoteSegmentRequest struct {
	CustomerID string `json:"customerId"`
}

type remoteSegmentResponse struct {
	Segments []string `json:"segments"`
}

// RemoteSegments is the CDP half of a customer's audiences - ok=false when there is no provider or it
// failed, which reads as native-only rather than as an error a cart price would surface
type RemoteSegments interface {
	Slugs(ctx context.Context, customerID uuid.UUID) ([]string, bool)
}

type remoteSegmentSource struct {
	svc     *remoteprovider.Service
	invoker *remoteprovider.Invoker
}

func NewRemoteSegmentSource(svc *remoteprovider.Service, invoker *remoteprovider.Invoker) RemoteSegments {
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
