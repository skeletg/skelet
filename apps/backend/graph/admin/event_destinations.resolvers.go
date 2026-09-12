package admin

import (
	"context"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
	signalsv1 "github.com/skeletg/skelet/apps/backend/internal/signalspb/skelet/signals/v1"
)

func (r *mutationResolver) CreateEventDestination(ctx context.Context, input model.EventDestinationInput) (*model.EventDestination, error) {
	authed, err := r.signalsWrite(ctx)
	if err != nil {
		return nil, err
	}
	res, err := r.SignalsAPI.Destination.CreateDestination(authed, &signalsv1.CreateDestinationRequest{
		Destination: destinationInputProto(input),
	})
	if err != nil {
		return nil, signalsErr(err)
	}
	return eventDestinationModel(res.GetDestination()), nil
}

func (r *mutationResolver) UpdateEventDestination(ctx context.Context, id string, input model.EventDestinationInput) (*model.EventDestination, error) {
	authed, err := r.signalsWrite(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := uuid.Parse(id); err != nil {
		return nil, errInvalidID("id", err)
	}
	res, err := r.SignalsAPI.Destination.UpdateDestination(authed, &signalsv1.UpdateDestinationRequest{
		Id: id, Destination: destinationInputProto(input),
	})
	if err != nil {
		return nil, signalsErr(err)
	}
	return eventDestinationModel(res.GetDestination()), nil
}

func (r *mutationResolver) DeleteEventDestination(ctx context.Context, id string) (bool, error) {
	authed, err := r.signalsWrite(ctx)
	if err != nil {
		return false, err
	}
	if _, err := uuid.Parse(id); err != nil {
		return false, errInvalidID("id", err)
	}
	if _, err := r.SignalsAPI.Destination.DeleteDestination(authed, &signalsv1.DeleteDestinationRequest{Id: id}); err != nil {
		return false, signalsErr(err)
	}
	return true, nil
}

func (r *mutationResolver) SyncAudience(ctx context.Context, segmentID string, destinationID string) (int32, error) {
	authed, err := r.signalsWrite(ctx)
	if err != nil {
		return 0, err
	}
	if _, err := uuid.Parse(segmentID); err != nil {
		return 0, errInvalidID("segmentId", err)
	}
	if _, err := uuid.Parse(destinationID); err != nil {
		return 0, errInvalidID("destinationId", err)
	}
	res, err := r.SignalsAPI.Destination.SyncAudience(authed, &signalsv1.SyncAudienceRequest{
		SegmentId: segmentID, DestinationId: destinationID,
	})
	if err != nil {
		return 0, signalsErr(err)
	}
	return res.GetIdentifiersSent(), nil
}

func (r *queryResolver) EventDestinations(ctx context.Context, channelID string) ([]*model.EventDestination, error) {
	authed, err := r.signalsWrite(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := uuid.Parse(channelID); err != nil {
		return nil, errInvalidID("channelId", err)
	}
	res, err := r.SignalsAPI.Destination.ListDestinations(authed, &signalsv1.ListDestinationsRequest{ChannelId: channelID})
	if err != nil {
		return nil, signalsErr(err)
	}
	out := make([]*model.EventDestination, 0, len(res.GetDestinations()))
	for _, d := range res.GetDestinations() {
		out = append(out, eventDestinationModel(d))
	}
	return out, nil
}
