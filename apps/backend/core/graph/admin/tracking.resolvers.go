package admin

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/tracking"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

func (r *mutationResolver) CreateTrackingDestination(ctx context.Context, channelID string, kind string, input model.TrackingDestinationInput) (*model.TrackingDestination, error) {
	cid, err := r.requireTrackingChannel(ctx, trackingManage, channelID)
	if err != nil {
		return nil, err
	}
	k, ok := tracking.KindFor(kind)
	if !ok {
		return nil, fmt.Errorf("%w: %q", tracking.ErrUnknownKind, kind)
	}
	if k.Available != nil && !k.Available(ctx, cid) {
		return nil, apperr.New("TRACKING_PLUGIN_OFF", apperr.ClassValidation, "the "+k.Label+" plugin is switched off on this channel")
	}
	d := destinationFromInput(k, input)
	d.ChannelID = cid
	created, err := r.TrackingSvc.Create(ctx, d)
	if err != nil {
		return nil, err
	}
	return toTrackingDestination(created, tracking.Health{}), nil
}

func (r *mutationResolver) UpdateTrackingDestination(ctx context.Context, id string, input model.TrackingDestinationInput) (*model.TrackingDestination, error) {
	current, err := r.requireTrackingDestination(ctx, trackingManage, id)
	if err != nil {
		return nil, err
	}
	k, ok := tracking.KindFor(current.Kind)
	if !ok {
		return nil, fmt.Errorf("%w: %q", tracking.ErrUnknownKind, current.Kind)
	}
	d := destinationFromInput(k, input)
	d.ID = current.ID
	if input.Enabled == nil {
		d.Enabled = current.Enabled
	}
	updated, err := r.TrackingSvc.Update(ctx, d)
	if err != nil {
		return nil, err
	}
	return r.trackingDestinationWithHealth(ctx, updated)
}

func (r *mutationResolver) SetTrackingDestinationEnabled(ctx context.Context, id string, enabled bool) (*model.TrackingDestination, error) {
	current, err := r.requireTrackingDestination(ctx, trackingManage, id)
	if err != nil {
		return nil, err
	}
	current.Enabled = enabled
	current.Secret = "" // empty keeps the stored secret
	updated, err := r.TrackingSvc.Update(ctx, current)
	if err != nil {
		return nil, err
	}
	return r.trackingDestinationWithHealth(ctx, updated)
}

func (r *mutationResolver) DeleteTrackingDestination(ctx context.Context, id string) (bool, error) {
	d, err := r.requireTrackingDestination(ctx, trackingManage, id)
	if err != nil {
		return false, err
	}
	return true, r.TrackingSvc.Delete(ctx, d.ID)
}

func (r *mutationResolver) SendTrackingTest(ctx context.Context, id string) (*model.TrackingTestResult, error) {
	d, err := r.requireTrackingDestination(ctx, trackingManage, id)
	if err != nil {
		return nil, err
	}
	if _, err := r.TrackingSvc.SendTest(ctx, r.TrackingClient, d.ID); err != nil {
		return &model.TrackingTestResult{Ok: false, Message: err.Error()}, nil // already redacted by the service
	}
	return &model.TrackingTestResult{Ok: true, Message: ""}, nil
}

func (r *mutationResolver) RedeliverTrackingDelivery(ctx context.Context, id string) (*model.TrackingDelivery, error) {
	d, err := r.requireTrackingDelivery(ctx, trackingManage, id)
	if err != nil {
		return nil, err
	}
	if err := r.TrackingSvc.Redeliver(ctx, d.ID); err != nil {
		return nil, err
	}
	// Not read back: a worker may send it, and so delete it, before a second read
	d.Status, d.Attempts, d.LastError, d.NextAttemptAt = "pending", 0, "", time.Now()
	return toTrackingDelivery(d), nil
}

func (r *mutationResolver) ImportSignalsDestinations(ctx context.Context, channelID string, legacyIds []string) ([]*model.TrackingDestination, error) {
	cid, err := r.requireTrackingChannel(ctx, trackingManage, channelID)
	if err != nil {
		return nil, err
	}
	// The rows are read again here rather than taken from the client, so an import is only ever what Signals holds
	legacy, err := r.signalsDestinations(ctx, channelID)
	if err != nil {
		return nil, err
	}
	var items []tracking.Imported
	for _, l := range legacy {
		lid, err := uuid.Parse(l.GetId())
		if err != nil || !slices.Contains(legacyIds, l.GetId()) {
			continue
		}
		kind, cfg, err := legacyTarget(l)
		if err != nil {
			return nil, err // before anything is saved
		}
		items = append(items, tracking.Imported{ChannelID: cid, Kind: kind, Name: l.GetName(), Config: cfg, LegacyID: lid})
	}
	imported, err := r.TrackingSvc.Import(ctx, items)
	if err != nil {
		return nil, err
	}
	out := make([]*model.TrackingDestination, 0, len(imported))
	for _, d := range imported {
		out = append(out, toTrackingDestination(d, tracking.Health{}))
	}
	return out, nil
}

func (r *queryResolver) TrackingPlatforms(ctx context.Context, channelID string) ([]*model.TrackingPlatform, error) {
	cid, err := r.requireTrackingChannel(ctx, trackingView, channelID)
	if err != nil {
		return nil, err
	}
	return r.trackingPlatforms(ctx, cid), nil
}

func (r *queryResolver) TrackingDestinations(ctx context.Context, channelID string) ([]*model.TrackingDestination, error) {
	cid, err := r.requireTrackingChannel(ctx, trackingView, channelID)
	if err != nil {
		return nil, err
	}
	ds, err := r.TrackingSvc.List(ctx, cid)
	if err != nil {
		return nil, err
	}
	health, err := r.TrackingSvc.Health(ctx, cid)
	if err != nil {
		return nil, err
	}
	out := make([]*model.TrackingDestination, 0, len(ds))
	for _, d := range ds {
		out = append(out, toTrackingDestination(d, health[d.ID]))
	}
	return out, nil
}

func (r *queryResolver) TrackingDestination(ctx context.Context, id string) (*model.TrackingDestination, error) {
	d, err := r.requireTrackingDestination(ctx, trackingView, id)
	if err != nil {
		return nil, err
	}
	return r.trackingDestinationWithHealth(ctx, d)
}

func (r *queryResolver) TrackingDeliveries(ctx context.Context, destinationID string, status *model.TrackingDeliveryStatus, limit *int32) ([]*model.TrackingDelivery, error) {
	d, err := r.requireTrackingDestination(ctx, trackingView, destinationID)
	if err != nil {
		return nil, err
	}
	want := "" // every status
	if status != nil {
		want = strings.ToLower(string(*status))
	}
	n := int32(50)
	if limit != nil && *limit > 0 {
		n = min(*limit, 200)
	}
	rows, err := r.TrackingSvc.Deliveries(ctx, d.ID, want, n)
	if err != nil {
		return nil, err
	}
	out := make([]*model.TrackingDelivery, 0, len(rows))
	for _, row := range rows {
		out = append(out, toTrackingDelivery(row))
	}
	return out, nil
}

func (r *queryResolver) SignalsImportCandidates(ctx context.Context, channelID string) ([]*model.SignalsImportCandidate, error) {
	cid, err := r.requireTrackingChannel(ctx, trackingView, channelID)
	if err != nil {
		return nil, err
	}
	legacy, err := r.signalsDestinations(ctx, channelID)
	if err != nil {
		return nil, err
	}
	mine, err := r.TrackingSvc.List(ctx, cid)
	if err != nil {
		return nil, err
	}
	existing := importedFrom(mine)
	out := make([]*model.SignalsImportCandidate, 0, len(legacy))
	for _, l := range legacy {
		out = append(out, importCandidate(l, existing))
	}
	return out, nil
}
