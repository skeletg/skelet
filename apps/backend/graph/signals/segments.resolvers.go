package signals

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/graph/signals/model"
	"github.com/skeletg/skelet/apps/backend/internal/auth"
	"github.com/skeletg/skelet/apps/backend/internal/ptr"
	"github.com/skeletg/skelet/apps/backend/internal/segment"
)

func (r *customerSegmentResolver) Members(ctx context.Context, obj *model.CustomerSegment, limit *int32, offset *int32) ([]*model.CustomerSegmentMember, error) {
	if r.SegmentSvc == nil {
		return []*model.CustomerSegmentMember{}, nil
	}
	checker := auth.GrantCheckerFrom(ctx)
	if err := auth.RequirePermission(ctx, checker, "promotions.view"); err != nil {
		return nil, err
	}
	// Members are customers, so reading them needs the customers permission too
	if err := auth.RequirePermission(ctx, checker, "customers.view"); err != nil {
		return nil, err
	}
	perm, err := auth.LoadPermittedChannels(ctx, checker, "customers.view")
	if err != nil {
		return nil, err
	}
	var channels []uuid.UUID
	if !perm.Bypass() {
		channels = perm.IDs()
		if len(channels) == 0 {
			return []*model.CustomerSegmentMember{}, nil
		}
	}
	sid, err := uuid.Parse(obj.ID)
	if err != nil {
		return nil, errInvalidID("segment id", err)
	}
	lim := int32(50)
	if limit != nil && *limit >= 1 && *limit <= 200 {
		lim = *limit
	}
	var off int32
	if offset != nil && *offset > 0 {
		off = *offset
	}
	rows, err := r.SegmentSvc.ListMembers(ctx, sid, lim, off, channels)
	if err != nil {
		return nil, err
	}
	// Ids and join dates only - the dashboard labels them from core
	out := make([]*model.CustomerSegmentMember, len(rows))
	for i, m := range rows {
		out[i] = &model.CustomerSegmentMember{ID: m.ID.String(), JoinedAt: m.JoinedAt}
	}
	return out, nil
}

func (r *mutationResolver) CreateCustomerSegment(ctx context.Context, input model.CreateCustomerSegmentInput) (*model.CustomerSegment, error) {
	if r.SegmentSvc == nil {
		return nil, errNotConfigured("segments")
	}
	if err := auth.RequirePermission(ctx, auth.GrantCheckerFrom(ctx), "promotions.manage"); err != nil {
		return nil, err
	}
	seg, err := r.SegmentSvc.Create(ctx, segment.CreateInput{
		Slug:        input.Slug,
		Name:        input.Name,
		Description: ptr.Deref(input.Description),
		IsActive:    derefBoolDefault(input.IsActive, true),
	})
	if err != nil {
		return nil, err
	}
	return segmentToModel(seg, 0), nil
}

func (r *mutationResolver) UpdateCustomerSegment(ctx context.Context, input model.UpdateCustomerSegmentInput) (*model.CustomerSegment, error) {
	if r.SegmentSvc == nil {
		return nil, errNotConfigured("segments")
	}
	if err := auth.RequirePermission(ctx, auth.GrantCheckerFrom(ctx), "promotions.manage"); err != nil {
		return nil, err
	}
	id, err := uuid.Parse(input.ID)
	if err != nil {
		return nil, errInvalidID("id", err)
	}
	existing, err := r.SegmentSvc.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if input.Slug != nil {
		existing.Slug = *input.Slug
	}
	if input.Name != nil {
		existing.Name = *input.Name
	}
	if input.Description != nil {
		existing.Description = *input.Description
	}
	if input.IsActive != nil {
		existing.IsActive = *input.IsActive
	}
	updated, err := r.SegmentSvc.Update(ctx, segment.UpdateInput{
		ID:          id,
		Slug:        existing.Slug,
		Name:        existing.Name,
		Description: existing.Description,
		RefreshSQL:  existing.RefreshSQL,
		IsActive:    existing.IsActive,
	})
	if err != nil {
		return nil, err
	}
	count, _ := r.SegmentSvc.MemberCount(ctx, id)
	return segmentToModel(updated, count), nil
}

func (r *mutationResolver) DeleteCustomerSegment(ctx context.Context, id string) (bool, error) {
	if r.SegmentSvc == nil {
		return false, errNotConfigured("segments")
	}
	if err := auth.RequirePermission(ctx, auth.GrantCheckerFrom(ctx), "promotions.manage"); err != nil {
		return false, err
	}
	sid, err := uuid.Parse(id)
	if err != nil {
		return false, errInvalidID("id", err)
	}
	if err := r.SegmentSvc.Delete(ctx, sid); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) AssignCustomersToSegment(ctx context.Context, segmentID string, customerIds []string) (bool, error) {
	if r.SegmentSvc == nil {
		return false, errNotConfigured("segments")
	}
	if err := auth.RequirePermission(ctx, auth.GrantCheckerFrom(ctx), "promotions.manage"); err != nil {
		return false, err
	}
	sid, err := uuid.Parse(segmentID)
	if err != nil {
		return false, errInvalidID("segmentId", err)
	}
	ids := make([]uuid.UUID, 0, len(customerIds))
	for _, raw := range customerIds {
		id, err := uuid.Parse(raw)
		if err != nil {
			return false, fmt.Errorf("invalid customerId %q: %w", raw, err)
		}
		ids = append(ids, id)
	}
	if err := r.SegmentSvc.AssignCustomers(ctx, sid, ids); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) RemoveCustomerFromSegment(ctx context.Context, segmentID string, customerID string) (bool, error) {
	if r.SegmentSvc == nil {
		return false, errNotConfigured("segments")
	}
	if err := auth.RequirePermission(ctx, auth.GrantCheckerFrom(ctx), "promotions.manage"); err != nil {
		return false, err
	}
	sid, err := uuid.Parse(segmentID)
	if err != nil {
		return false, errInvalidID("segmentId", err)
	}
	cid, err := uuid.Parse(customerID)
	if err != nil {
		return false, errInvalidID("customerId", err)
	}
	if err := r.SegmentSvc.RemoveCustomer(ctx, cid, sid); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) ClearSegmentMembership(ctx context.Context, segmentID string) (bool, error) {
	if r.SegmentSvc == nil {
		return false, errNotConfigured("segments")
	}
	if err := auth.RequirePermission(ctx, auth.GrantCheckerFrom(ctx), "promotions.manage"); err != nil {
		return false, err
	}
	sid, err := uuid.Parse(segmentID)
	if err != nil {
		return false, errInvalidID("segmentId", err)
	}
	if err := r.SegmentSvc.ClearMembership(ctx, sid); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) SetSegmentRules(ctx context.Context, segmentID string, rules interface{}) (*model.CustomerSegment, error) {
	if r.SegmentSvc == nil {
		return nil, errNotConfigured("segments")
	}
	if err := auth.RequirePermission(ctx, auth.GrantCheckerFrom(ctx), "promotions.manage"); err != nil {
		return nil, err
	}
	sid, err := uuid.Parse(segmentID)
	if err != nil {
		return nil, errInvalidID("segmentId", err)
	}
	var raw json.RawMessage
	if rules != nil {
		b, merr := json.Marshal(rules)
		if merr != nil {
			return nil, errInvalidInput("rules", merr)
		}
		raw = b
	}
	seg, err := r.SegmentSvc.SetRules(ctx, sid, raw)
	if err != nil {
		return nil, err
	}
	count, _ := r.SegmentSvc.MemberCount(ctx, sid)
	return segmentToModel(seg, count), nil
}

func (r *mutationResolver) RefreshSegment(ctx context.Context, segmentID string) (int32, error) {
	if r.SegmentSvc == nil {
		return 0, errNotConfigured("segments")
	}
	if err := auth.RequirePermission(ctx, auth.GrantCheckerFrom(ctx), "promotions.manage"); err != nil {
		return 0, err
	}
	sid, err := uuid.Parse(segmentID)
	if err != nil {
		return 0, errInvalidID("segmentId", err)
	}
	n, err := r.SegmentSvc.RefreshDynamic(ctx, sid)
	if err != nil {
		return 0, err
	}
	return int32(n), nil
}

func (r *queryResolver) CustomerSegments(ctx context.Context, limit *int32, offset *int32) (*model.CustomerSegmentList, error) {
	if r.SegmentSvc == nil {
		return &model.CustomerSegmentList{Items: []*model.CustomerSegment{}, TotalCount: 0}, nil
	}
	if err := auth.RequirePermission(ctx, auth.GrantCheckerFrom(ctx), "promotions.view"); err != nil {
		return nil, err
	}
	segs, total, err := r.SegmentSvc.List(ctx, derefInt32Default(limit, 25), derefInt32Default(offset, 0))
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, len(segs))
	for i, s := range segs {
		ids[i] = s.ID
	}
	counts, err := r.SegmentSvc.MemberCounts(ctx, ids)
	if err != nil {
		return nil, err
	}
	items := make([]*model.CustomerSegment, len(segs))
	for i, s := range segs {
		items[i] = segmentToModel(s, counts[s.ID])
	}
	return &model.CustomerSegmentList{Items: items, TotalCount: total}, nil
}

func (r *queryResolver) CustomerSegment(ctx context.Context, id string) (*model.CustomerSegment, error) {
	if r.SegmentSvc == nil {
		return nil, nil
	}
	if err := auth.RequirePermission(ctx, auth.GrantCheckerFrom(ctx), "promotions.view"); err != nil {
		return nil, err
	}
	sid, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("id", err)
	}
	s, err := r.SegmentSvc.Get(ctx, sid)
	if err != nil {
		return nil, err
	}
	count, _ := r.SegmentSvc.MemberCount(ctx, sid)
	return segmentToModel(s, count), nil
}

func (r *queryResolver) ExperimentSegments(ctx context.Context) ([]*model.SegmentOption, error) {
	if err := auth.RequirePermission(ctx, auth.GrantCheckerFrom(ctx), "experiments.view"); err != nil {
		return nil, err
	}
	if r.SegmentSvc == nil {
		return []*model.SegmentOption{}, nil
	}
	segs, _, err := r.SegmentSvc.List(ctx, 200, 0)
	if err != nil {
		return nil, err
	}
	out := make([]*model.SegmentOption, 0, len(segs))
	for _, s := range segs {
		if s.IsActive {
			out = append(out, &model.SegmentOption{Slug: s.Slug, Name: s.Name})
		}
	}
	return out, nil
}

func (r *Resolver) CustomerSegment() CustomerSegmentResolver { return &customerSegmentResolver{r} }

type customerSegmentResolver struct{ *Resolver }
