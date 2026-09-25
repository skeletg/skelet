package admin

import (
	"context"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/audienceio"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
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
	segID, err := uuid.Parse(segmentID)
	if err != nil {
		return 0, errInvalidID("segmentId", err)
	}
	if _, err := uuid.Parse(destinationID); err != nil {
		return 0, errInvalidID("destinationId", err)
	}
	dest, err := r.SignalsAPI.Destination.GetDestination(authed, &signalsv1.GetDestinationRequest{Id: destinationID})
	if err != nil {
		return 0, signalsErr(err)
	}
	channelID, err := uuid.Parse(dest.GetDestination().GetChannelId())
	if err != nil {
		return 0, errInvalidID("channelId", err)
	}
	// The addresses are core's, so core hashes them - the suite forwards identifiers it cannot reverse
	ids, err := audienceio.Hashes(ctx, r.segmentMembers, r.channelCustomerEmails(channelID), uuid.Nil, segID) // a live request, so the caller on the context is the actor
	if err != nil {
		return 0, err
	}
	res, err := r.SignalsAPI.Destination.SyncAudience(authed, &signalsv1.SyncAudienceRequest{
		SegmentId: segmentID, DestinationId: destinationID, Identifiers: ids,
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
