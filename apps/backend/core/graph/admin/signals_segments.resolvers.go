package admin

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
)

// Members are customers, so reading them needs the customers grant on top of the promotions one
func (r *customerSegmentResolver) Members(ctx context.Context, obj *model.CustomerSegment, limit *int32, offset *int32) ([]*model.CustomerSegmentMember, error) {
	checker := auth.GrantCheckerFrom(ctx)
	if err := auth.RequirePermission(ctx, checker, "promotions.view"); err != nil {
		return nil, err
	}
	if err := auth.RequirePermission(ctx, checker, "customers.view"); err != nil {
		return nil, err
	}
	authed, err := signalsCtx(ctx, r.Resolver)
	if err != nil {
		return nil, err
	}
	if _, err := uuid.Parse(obj.ID); err != nil {
		return nil, errInvalidID("segment id", err)
	}
	lim := int32(50)
	if limit != nil && *limit >= 1 && *limit <= 200 {
		lim = *limit
	}
	token := ""
	if offset != nil && *offset > 0 {
		token = fmt.Sprint(*offset)
	}
	res, err := r.SignalsAPI.Segments.ListMembers(authed, &signalsv1.ListMembersRequest{
		SegmentId: obj.ID, PageSize: lim, PageToken: token,
	})
	if err != nil {
		return nil, signalsErr(err)
	}
	out := make([]*model.CustomerSegmentMember, 0, len(res.GetMembers()))
	for _, m := range res.GetMembers() {
		out = append(out, &model.CustomerSegmentMember{ID: m.GetCustomerId(), JoinedAt: m.GetJoinedAt().AsTime()})
	}
	return out, nil
}

func (r *mutationResolver) CreateCustomerSegment(ctx context.Context, input model.CreateCustomerSegmentInput) (*model.CustomerSegment, error) {
	authed, err := r.signalsScoped(ctx, "promotions.manage")
	if err != nil {
		return nil, err
	}
	req := &signalsv1.CreateSegmentRequest{Slug: input.Slug, Name: input.Name}
	if input.Description != nil {
		req.Description = *input.Description
	}
	req.IsActive = input.IsActive
	res, err := r.SignalsAPI.Segments.CreateSegment(authed, req)
	if err != nil {
		return nil, signalsErr(err)
	}
	return customerSegmentModel(res.GetSegment()), nil
}

func (r *mutationResolver) UpdateCustomerSegment(ctx context.Context, input model.UpdateCustomerSegmentInput) (*model.CustomerSegment, error) {
	authed, err := r.signalsScoped(ctx, "promotions.manage")
	if err != nil {
		return nil, err
	}
	if _, err := uuid.Parse(input.ID); err != nil {
		return nil, errInvalidID("id", err)
	}
	// An absent field is left as it stands, which the contract spells with optional
	res, err := r.SignalsAPI.Segments.UpdateSegment(authed, &signalsv1.UpdateSegmentRequest{
		Id: input.ID, Slug: input.Slug, Name: input.Name, Description: input.Description, IsActive: input.IsActive,
	})
	if err != nil {
		return nil, signalsErr(err)
	}
	return customerSegmentModel(res.GetSegment()), nil
}

func (r *mutationResolver) DeleteCustomerSegment(ctx context.Context, id string) (bool, error) {
	authed, err := r.signalsScoped(ctx, "promotions.manage")
	if err != nil {
		return false, err
	}
	if _, err := uuid.Parse(id); err != nil {
		return false, errInvalidID("id", err)
	}
	if _, err := r.SignalsAPI.Segments.DeleteSegment(authed, &signalsv1.DeleteSegmentRequest{Id: id}); err != nil {
		return false, signalsErr(err)
	}
	return true, nil
}

func (r *mutationResolver) AssignCustomersToSegment(ctx context.Context, segmentID string, customerIds []string) (bool, error) {
	authed, err := r.signalsScoped(ctx, "promotions.manage")
	if err != nil {
		return false, err
	}
	if _, err := uuid.Parse(segmentID); err != nil {
		return false, errInvalidID("segmentId", err)
	}
	for _, raw := range customerIds {
		if _, err := uuid.Parse(raw); err != nil {
			return false, fmt.Errorf("invalid customerId %q: %w", raw, err)
		}
	}
	if _, err := r.SignalsAPI.Segments.AssignCustomers(authed, &signalsv1.AssignCustomersRequest{
		SegmentId: segmentID, CustomerIds: customerIds,
	}); err != nil {
		return false, signalsErr(err)
	}
	return true, nil
}

func (r *mutationResolver) RemoveCustomerFromSegment(ctx context.Context, segmentID string, customerID string) (bool, error) {
	authed, err := r.signalsScoped(ctx, "promotions.manage")
	if err != nil {
		return false, err
	}
	if _, err := uuid.Parse(segmentID); err != nil {
		return false, errInvalidID("segmentId", err)
	}
	if _, err := uuid.Parse(customerID); err != nil {
		return false, errInvalidID("customerId", err)
	}
	if _, err := r.SignalsAPI.Segments.RemoveCustomer(authed, &signalsv1.RemoveCustomerRequest{
		SegmentId: segmentID, CustomerId: customerID,
	}); err != nil {
		return false, signalsErr(err)
	}
	return true, nil
}

func (r *mutationResolver) ClearSegmentMembership(ctx context.Context, segmentID string) (bool, error) {
	authed, err := r.signalsScoped(ctx, "promotions.manage")
	if err != nil {
		return false, err
	}
	if _, err := uuid.Parse(segmentID); err != nil {
		return false, errInvalidID("segmentId", err)
	}
	if _, err := r.SignalsAPI.Segments.ClearMembership(authed, &signalsv1.ClearMembershipRequest{SegmentId: segmentID}); err != nil {
		return false, signalsErr(err)
	}
	return true, nil
}

func (r *mutationResolver) SetSegmentRules(ctx context.Context, segmentID string, rules interface{}) (*model.CustomerSegment, error) {
	authed, err := r.signalsScoped(ctx, "promotions.manage")
	if err != nil {
		return nil, err
	}
	if _, err := uuid.Parse(segmentID); err != nil {
		return nil, errInvalidID("segmentId", err)
	}
	raw := ""
	if rules != nil {
		b, merr := json.Marshal(rules)
		if merr != nil {
			return nil, errInvalidInput("rules", merr)
		}
		raw = string(b)
	}
	res, err := r.SignalsAPI.Segments.SetRules(authed, &signalsv1.SetRulesRequest{SegmentId: segmentID, RulesJson: raw})
	if err != nil {
		return nil, signalsErr(err)
	}
	return customerSegmentModel(res.GetSegment()), nil
}

func (r *mutationResolver) RefreshSegment(ctx context.Context, segmentID string) (int32, error) {
	authed, err := r.signalsScoped(ctx, "promotions.manage")
	if err != nil {
		return 0, err
	}
	if _, err := uuid.Parse(segmentID); err != nil {
		return 0, errInvalidID("segmentId", err)
	}
	res, err := r.SignalsAPI.Segments.RefreshSegment(authed, &signalsv1.RefreshSegmentRequest{SegmentId: segmentID})
	if err != nil {
		return 0, signalsErr(err)
	}
	return res.GetMemberCount(), nil
}

func (r *queryResolver) CustomerSegments(ctx context.Context, limit *int32, offset *int32) (*model.CustomerSegmentList, error) {
	authed, err := r.signalsScoped(ctx, "promotions.view")
	if err != nil {
		return nil, err
	}
	lim := int32(25)
	if limit != nil && *limit > 0 {
		lim = *limit
	}
	token := ""
	if offset != nil && *offset > 0 {
		token = fmt.Sprint(*offset)
	}
	res, err := r.SignalsAPI.Segments.ListSegments(authed, &signalsv1.ListSegmentsRequest{PageSize: lim, PageToken: token})
	if err != nil {
		return nil, signalsErr(err)
	}
	items := make([]*model.CustomerSegment, 0, len(res.GetSegments()))
	for _, s := range res.GetSegments() {
		items = append(items, customerSegmentModel(s))
	}
	return &model.CustomerSegmentList{Items: items, TotalCount: res.GetTotalCount()}, nil
}

func (r *queryResolver) CustomerSegment(ctx context.Context, id string) (*model.CustomerSegment, error) {
	authed, err := r.signalsScoped(ctx, "promotions.view")
	if err != nil {
		return nil, err
	}
	if _, err := uuid.Parse(id); err != nil {
		return nil, errInvalidID("id", err)
	}
	res, err := r.SignalsAPI.Segments.GetSegment(authed, &signalsv1.GetSegmentRequest{Id: id})
	if err != nil {
		if signalsNotFound(err) {
			return nil, nil
		}
		return nil, signalsErr(err)
	}
	return customerSegmentModel(res.GetSegment()), nil
}

func (r *queryResolver) ExperimentSegments(ctx context.Context) ([]*model.SegmentOption, error) {
	authed, err := r.signalsScoped(ctx, "experiments.view")
	if err != nil {
		return nil, err
	}
	res, err := r.SignalsAPI.Segments.ListSegmentOptions(authed, &signalsv1.ListSegmentOptionsRequest{PageSize: 200})
	if err != nil {
		return nil, signalsErr(err)
	}
	out := make([]*model.SegmentOption, 0, len(res.GetOptions()))
	for _, o := range res.GetOptions() {
		out = append(out, &model.SegmentOption{Slug: o.GetSlug(), Name: o.GetName()})
	}
	return out, nil
}

func (r *Resolver) CustomerSegment() CustomerSegmentResolver { return &customerSegmentResolver{r} }

type customerSegmentResolver struct{ *Resolver }
