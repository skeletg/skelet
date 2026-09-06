package shipping

import (
	"context"
	"log"
	"time"

	"github.com/skeletg/skelet/apps/backend/internal/remoteprovider"
)

// A merchant HTTP endpoint (remoteprovider kind=shipping_tracking) resolving a shipment's status for ANY carrier - the escape hatch for a universal tracker like AfterShip, EasyPost or Narvar without a Go plugin
// Service.Track consults it first and falls back to the carrier's own Tracker
// Wire contract: request {carrierSlug, trackingNumber}, response {status, url, events: [{time, status, location}]} - fail_open, or no provider, falls back to native carrier tracking

type RemoteTrackingSource interface {
	// Resolves a shipment - ok=false falls back to the carrier's own Tracker
	Track(ctx context.Context, carrierSlug, trackingNumber string) (TrackResult, bool)
}

// Optional - nil leaves tracking to the carrier's own Tracker
func (s *Service) SetRemoteTracking(src RemoteTrackingSource) { s.remoteTracking = src }

// Consults the remote provider before the carrier's Tracker, ok=false to fall through
func (s *Service) trackRemoteFirst(ctx context.Context, carrierSlug, trackingNumber string) (TrackResult, bool) {
	if s.remoteTracking == nil {
		return TrackResult{}, false
	}
	res, ok := s.remoteTracking.Track(ctx, carrierSlug, trackingNumber)
	if !ok {
		return TrackResult{}, false
	}
	log.Printf("[shipping] remote tracking resolved %s/%s", carrierSlug, trackingNumber)
	return res, true
}

type remoteTrackRequest struct {
	CarrierSlug    string `json:"carrierSlug"`
	TrackingNumber string `json:"trackingNumber"`
}

type remoteTrackEvent struct {
	Time     string `json:"time"`
	Status   string `json:"status"`
	Location string `json:"location"`
}

type remoteTrackResponse struct {
	Status string             `json:"status"`
	URL    string             `json:"url"`
	Events []remoteTrackEvent `json:"events"`
}

type remoteTrackingSource struct {
	svc     *remoteprovider.Service
	invoker *remoteprovider.Invoker
}

func NewRemoteTrackingSource(svc *remoteprovider.Service, invoker *remoteprovider.Invoker) RemoteTrackingSource {
	return &remoteTrackingSource{svc: svc, invoker: invoker}
}

func (r *remoteTrackingSource) Track(ctx context.Context, carrierSlug, trackingNumber string) (TrackResult, bool) {
	providers, err := r.svc.ListActiveByKind(ctx, remoteprovider.KindShippingTracking, nil) // account-level tracker
	if err != nil || len(providers) == 0 {
		return TrackResult{}, false
	}
	p := providers[0]

	var resp remoteTrackResponse
	if err := r.invoker.Invoke(ctx, p, remoteTrackRequest{CarrierSlug: carrierSlug, TrackingNumber: trackingNumber}, &resp); err != nil {
		log.Printf("[shipping] remote tracking %q failed (carrier fallback): %v", p.Slug, err)
		return TrackResult{}, false
	}
	if resp.Status == "" && resp.URL == "" && len(resp.Events) == 0 {
		return TrackResult{}, false // empty -> carrier fallback
	}
	out := TrackResult{Status: resp.Status, URL: resp.URL}
	for _, e := range resp.Events {
		ev := TrackEvent{Status: e.Status, Location: e.Location}
		if t, perr := time.Parse(time.RFC3339, e.Time); perr == nil {
			ev.Time = t
		}
		out.Events = append(out.Events, ev)
	}
	return out, true
}
