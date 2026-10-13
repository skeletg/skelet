package delivery

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/remoteprovider"
)

// Point sources as merchant HTTP endpoints (remoteprovider kind delivery_points) - a locker, parcel-shop or PO-box network plugged in without Go. An option names one as "remote:<provider slug>"
//
// Wire contract, an op naming the operation:
//
//	delivery_points {op: search|get, channelId, kind, near, search, items, limit, id} -> {points: [...]}

const remotePrefix = "remote:"

type remoteSources struct {
	svc     *remoteprovider.Service
	invoker *remoteprovider.Invoker
}

// SetRemoteProviders turns on the remote point sources - without it only registered ones exist
func (s *Service) SetRemoteProviders(svc *remoteprovider.Service, invoker *remoteprovider.Invoker) {
	s.remote = &remoteSources{svc: svc, invoker: invoker}
}

func (r *remoteSources) list(ctx context.Context, kind remoteprovider.Kind, channelID uuid.UUID) []remoteprovider.Provider {
	if r == nil {
		return nil
	}
	ch := channelID
	ps, err := r.svc.ListActiveByKind(ctx, kind, &ch)
	if err != nil {
		log.Printf("[delivery] list remote %s providers: %v", kind, err)
		return nil
	}
	return ps
}

func (r *remoteSources) find(ctx context.Context, kind remoteprovider.Kind, channelID uuid.UUID, slug string) (remoteprovider.Provider, bool) {
	if r == nil || !strings.HasPrefix(slug, remotePrefix) {
		return remoteprovider.Provider{}, false
	}
	want := strings.TrimPrefix(slug, remotePrefix)
	for _, p := range r.list(ctx, kind, channelID) {
		if p.Slug == want {
			return p, true
		}
	}
	return remoteprovider.Provider{}, false
}

func (r *remoteSources) pointSources(ctx context.Context, channelID uuid.UUID) []SourceInfo {
	var out []SourceInfo
	for _, p := range r.list(ctx, remoteprovider.KindDeliveryPoints, channelID) {
		out = append(out, SourceInfo{Slug: remotePrefix + p.Slug, Label: p.Name, Remote: true, Active: p.IsActive, Owner: "remote"})
	}
	return out
}

func (r *remoteSources) pointSource(ctx context.Context, channelID uuid.UUID, slug string) (PointSource, bool) {
	p, ok := r.find(ctx, remoteprovider.KindDeliveryPoints, channelID, slug)
	if !ok {
		return nil, false
	}
	return &remotePointSource{inv: r.invoker, p: p}, true
}

type remotePointRequest struct {
	Op        string `json:"op"` // search | get
	ChannelID string `json:"channelId"`
	Kind      string `json:"kind"`
	Near      Near   `json:"near"`
	Search    string `json:"search,omitempty"`
	Items     []Item `json:"items,omitempty"`
	Limit     int    `json:"limit,omitempty"`
	ID        string `json:"id,omitempty"`
}

type remotePoint struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Address      Address           `json:"address"`
	Lat          *float64          `json:"lat,omitempty"`
	Lng          *float64          `json:"lng,omitempty"`
	DistanceKm   *float64          `json:"distanceKm,omitempty"`
	Hours        string            `json:"hours,omitempty"`
	Instructions string            `json:"instructions,omitempty"`
	Phone        string            `json:"phone,omitempty"`
	Details      map[string]string `json:"details,omitempty"`
}

type remotePointResponse struct {
	Points []remotePoint `json:"points"`
}

type remotePointSource struct {
	inv *remoteprovider.Invoker
	p   remoteprovider.Provider
}

func (s *remotePointSource) call(ctx context.Context, req remotePointRequest) ([]Point, error) {
	var resp remotePointResponse
	if err := s.inv.InvokeUncached(ctx, s.p, req, &resp); err != nil {
		return nil, err
	}
	out := make([]Point, 0, len(resp.Points))
	for _, rp := range resp.Points {
		out = append(out, Point{
			ID: rp.ID, Source: remotePrefix + s.p.Slug, Name: rp.Name, Address: rp.Address, Lat: rp.Lat, Lng: rp.Lng,
			DistanceKm: rp.DistanceKm, Hours: rp.Hours, Instructions: rp.Instructions, Phone: rp.Phone, Details: rp.Details,
		})
	}
	return out, nil
}

func (s *remotePointSource) Search(ctx context.Context, q PointQuery) ([]Point, error) {
	return s.call(ctx, remotePointRequest{Op: "search", ChannelID: q.ChannelID.String(), Kind: q.Kind, Near: q.Near, Search: q.Search, Items: q.Items, Limit: q.Limit})
}

func (s *remotePointSource) Get(ctx context.Context, channelID uuid.UUID, id string) (Point, error) {
	pts, err := s.call(ctx, remotePointRequest{Op: "get", ChannelID: channelID.String(), ID: id})
	if err != nil {
		return Point{}, err
	}
	for _, p := range pts {
		if p.ID == id {
			return p, nil
		}
	}
	return Point{}, fmt.Errorf("point %q not found", id)
}
