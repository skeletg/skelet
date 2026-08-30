package admin

import (
	"context"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/graph/admin/model"
	"github.com/skeletg/skelet/internal/auth"
)

func (r *mutationResolver) CreateEventDestination(ctx context.Context, input model.EventDestinationInput) (*model.EventDestination, error) {
	if err := r.requireSignals(); err != nil {
		return nil, err
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "settings.manage"); err != nil {
		return nil, err
	}
	in, err := destinationInputFromModel(input, nil)
	if err != nil {
		return nil, err
	}
	d, err := r.DestinationSvc.Create(ctx, in)
	if err != nil {
		return nil, err
	}
	return toEventDestinationModel(d), nil
}

func (r *mutationResolver) UpdateEventDestination(ctx context.Context, id string, input model.EventDestinationInput) (*model.EventDestination, error) {
	if err := r.requireSignals(); err != nil {
		return nil, err
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "settings.manage"); err != nil {
		return nil, err
	}
	destID, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("id", err)
	}
	in, err := destinationInputFromModel(input, &destID)
	if err != nil {
		return nil, err
	}
	d, err := r.DestinationSvc.Update(ctx, in)
	if err != nil {
		return nil, err
	}
	return toEventDestinationModel(d), nil
}

func (r *mutationResolver) DeleteEventDestination(ctx context.Context, id string) (bool, error) {
	if err := r.requireSignals(); err != nil {
		return false, err
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "settings.manage"); err != nil {
		return false, err
	}
	destID, err := uuid.Parse(id)
	if err != nil {
		return false, errInvalidID("id", err)
	}
	if err := r.DestinationSvc.Delete(ctx, destID); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) SyncAudience(ctx context.Context, segmentID string, destinationID string) (int32, error) {
	if err := r.requireSignals(); err != nil {
		return 0, err
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "settings.manage"); err != nil {
		return 0, err
	}
	segID, err := uuid.Parse(segmentID)
	if err != nil {
		return 0, errInvalidID("segmentId", err)
	}
	destID, err := uuid.Parse(destinationID)
	if err != nil {
		return 0, errInvalidID("destinationId", err)
	}
	n, err := r.DestinationSvc.SyncAudience(ctx, segID, destID)
	if err != nil {
		return 0, err
	}
	return int32(n), nil
}

func (r *queryResolver) EventDestinations(ctx context.Context, channelID string) ([]*model.EventDestination, error) {
	if err := r.requireSignals(); err != nil {
		return nil, err
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "settings.manage"); err != nil {
		return nil, err
	}
	chID, err := uuid.Parse(channelID)
	if err != nil {
		return nil, errInvalidID("channelId", err)
	}
	list, err := r.DestinationSvc.ListByChannel(ctx, chID)
	if err != nil {
		return nil, err
	}
	out := make([]*model.EventDestination, len(list))
	for i, d := range list {
		out[i] = toEventDestinationModel(d)
	}
	return out, nil
}
