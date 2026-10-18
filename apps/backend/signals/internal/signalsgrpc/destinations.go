package signalsgrpc

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
	"github.com/skeletg/skelet/apps/backend/signals/internal/destination"
)

type destinationServer struct {
	signalsv1.UnimplementedDestinationServiceServer
	svc *destination.Service
}

func (s *destinationServer) ListDestinations(ctx context.Context, req *signalsv1.ListDestinationsRequest) (*signalsv1.ListDestinationsResponse, error) {
	cid, err := channelScope(ctx, "settings.manage", req.GetChannelId())
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
	if err := requireChannel(ctx, "settings.manage", in.ChannelID); err != nil {
		return nil, err
	}
	d, err := s.svc.Create(ctx, in)
	if err != nil {
		return nil, destinationErr(err)
	}
	return &signalsv1.CreateDestinationResponse{Destination: destinationProto(d)}, nil
}

func (s *destinationServer) UpdateDestination(ctx context.Context, req *signalsv1.UpdateDestinationRequest) (*signalsv1.UpdateDestinationResponse, error) {
	existing, err := s.load(ctx, "id", req.GetId())
	if err != nil {
		return nil, err
	}
	in, err := destinationInput(&existing.ID, req.GetDestination())
	if err != nil {
		return nil, err
	}
	in.ChannelID = existing.ChannelID
	d, err := s.svc.Update(ctx, in)
	if err != nil {
		return nil, destinationErr(err)
	}
	return &signalsv1.UpdateDestinationResponse{Destination: destinationProto(d)}, nil
}

func (s *destinationServer) DeleteDestination(ctx context.Context, req *signalsv1.DeleteDestinationRequest) (*signalsv1.DeleteDestinationResponse, error) {
	d, err := s.load(ctx, "id", req.GetId())
	if err != nil {
		return nil, err
	}
	if err := s.svc.Delete(ctx, d.ChannelID, d.ID); err != nil {
		return nil, destinationErr(err)
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
	d, err := s.load(ctx, "destination_id", req.GetDestinationId())
	if err != nil {
		return nil, err
	}
	n, err := s.svc.SyncAudience(ctx, segID, d.ID, req.GetIdentifiers())
	if err != nil {
		return nil, destinationErr(err)
	}
	return &signalsv1.SyncAudienceResponse{IdentifiersSent: int32(n)}, nil
}

func (s *destinationServer) GetDestination(ctx context.Context, req *signalsv1.GetDestinationRequest) (*signalsv1.GetDestinationResponse, error) {
	d, err := s.load(ctx, "id", req.GetId())
	if err != nil {
		return nil, err
	}
	return &signalsv1.GetDestinationResponse{Destination: destinationProto(d)}, nil
}

func (s *destinationServer) load(ctx context.Context, field, rawID string) (destination.Destination, error) {
	if err := requirePermission(ctx, "settings.manage"); err != nil {
		return destination.Destination{}, err
	}
	id, err := parseID(field, rawID)
	if err != nil {
		return destination.Destination{}, err
	}
	d, err := s.svc.Get(ctx, id)
	if err != nil {
		return destination.Destination{}, destinationErr(err)
	}
	if err := requireChannel(ctx, "settings.manage", d.ChannelID); err != nil {
		return destination.Destination{}, err
	}
	return d, nil
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

func destinationErr(err error) error {
	switch {
	case errors.Is(err, destination.ErrNotFound):
		return notFound("destination")
	case errors.Is(err, destination.ErrInvalid):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, destination.ErrAudienceNeedsWebhook):
		return status.Error(codes.FailedPrecondition, err.Error())
	}
	return err
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
