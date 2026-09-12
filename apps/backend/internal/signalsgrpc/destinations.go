package signalsgrpc

import (
	"context"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/destination"
	signalsv1 "github.com/skeletg/skelet/apps/backend/internal/signalspb/skelet/signals/v1"
)

type destinationServer struct {
	signalsv1.UnimplementedDestinationServiceServer
	svc *destination.Service
}

func (s *destinationServer) ListDestinations(ctx context.Context, req *signalsv1.ListDestinationsRequest) (*signalsv1.ListDestinationsResponse, error) {
	if err := requirePermission(ctx, "settings.manage"); err != nil {
		return nil, err
	}
	cid, err := parseID("channel_id", req.GetChannelId())
	if err != nil {
		return nil, err
	}
	rows, err := s.svc.ListByChannel(ctx, cid)
	if err != nil {
		return nil, err
	}
	out := make([]*signalsv1.EventDestination, 0, len(rows))
	for _, d := range rows {
		out = append(out, destinationProto(d))
	}
	return &signalsv1.ListDestinationsResponse{Destinations: out}, nil
}

func (s *destinationServer) CreateDestination(ctx context.Context, req *signalsv1.CreateDestinationRequest) (*signalsv1.CreateDestinationResponse, error) {
	if err := requirePermission(ctx, "settings.manage"); err != nil {
		return nil, err
	}
	in, err := destinationInput(nil, req.GetDestination())
	if err != nil {
		return nil, err
	}
	d, err := s.svc.Create(ctx, in)
	if err != nil {
		return nil, err
	}
	return &signalsv1.CreateDestinationResponse{Destination: destinationProto(d)}, nil
}

func (s *destinationServer) UpdateDestination(ctx context.Context, req *signalsv1.UpdateDestinationRequest) (*signalsv1.UpdateDestinationResponse, error) {
	if err := requirePermission(ctx, "settings.manage"); err != nil {
		return nil, err
	}
	id, err := parseID("id", req.GetId())
	if err != nil {
		return nil, err
	}
	in, err := destinationInput(&id, req.GetDestination())
	if err != nil {
		return nil, err
	}
	d, err := s.svc.Update(ctx, in)
	if err != nil {
		return nil, err
	}
	return &signalsv1.UpdateDestinationResponse{Destination: destinationProto(d)}, nil
}

func (s *destinationServer) DeleteDestination(ctx context.Context, req *signalsv1.DeleteDestinationRequest) (*signalsv1.DeleteDestinationResponse, error) {
	if err := requirePermission(ctx, "settings.manage"); err != nil {
		return nil, err
	}
	id, err := parseID("id", req.GetId())
	if err != nil {
		return nil, err
	}
	if err := s.svc.Delete(ctx, id); err != nil {
		return nil, err
	}
	return &signalsv1.DeleteDestinationResponse{Deleted: true}, nil
}

func (s *destinationServer) SyncAudience(ctx context.Context, req *signalsv1.SyncAudienceRequest) (*signalsv1.SyncAudienceResponse, error) {
	if err := requirePermission(ctx, "settings.manage"); err != nil {
		return nil, err
	}
	segID, err := parseID("segment_id", req.GetSegmentId())
	if err != nil {
		return nil, err
	}
	destID, err := parseID("destination_id", req.GetDestinationId())
	if err != nil {
		return nil, err
	}
	n, err := s.svc.SyncAudience(ctx, segID, destID)
	if err != nil {
		return nil, err
	}
	return &signalsv1.SyncAudienceResponse{IdentifiersSent: int32(n)}, nil
}

// The secret is write-only - it goes in on an upsert and never comes back out
func destinationProto(d destination.Destination) *signalsv1.EventDestination {
	out := &signalsv1.EventDestination{
		Id: d.ID.String(), ChannelId: d.ChannelID.String(), Name: d.Name, Kind: d.Kind,
		TargetUrl: d.TargetURL, EventSources: d.EventSources, EventNames: d.EventNames,
		IsActive: d.IsActive, LastError: d.LastError,
		CreatedAt: ts(d.CreatedAt), UpdatedAt: ts(d.UpdatedAt),
	}
	if d.LastForwardedAt != nil {
		out.LastForwardedAt = ts(*d.LastForwardedAt)
	}
	return out
}

func destinationInput(id *uuid.UUID, in *signalsv1.DestinationInput) (destination.UpsertInput, error) {
	if in == nil {
		return destination.UpsertInput{}, invalidArgument("destination is required")
	}
	cid, err := parseID("channel_id", in.GetChannelId())
	if err != nil {
		return destination.UpsertInput{}, err
	}
	return destination.UpsertInput{
		ID: id, ChannelID: cid, Name: in.GetName(), Kind: in.GetKind(), TargetURL: in.GetTargetUrl(),
		Secret: in.GetSecret(), EventSources: in.GetEventSources(), EventNames: in.GetEventNames(),
		IsActive: in.GetIsActive(),
	}, nil
}
