package admin

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/internal/auth"
)

func (r *queryResolver) CustomerCommerce(ctx context.Context, customerID string) (*model.CustomerCommerce, error) {
	if err := r.requireSignals(); err != nil {
		return nil, err
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "customers.view"); err != nil {
		return nil, err
	}
	cid, err := uuid.Parse(customerID)
	if err != nil {
		return nil, errInvalidID("customerId", err)
	}
	cust, err := r.CustomerSvc.GetByID(ctx, cid)
	if err != nil {
		return nil, errNotFound("customer")
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "customers.view", cust.ChannelID, nil); err != nil {
		return nil, err
	}
	return buildCustomerCommerce(ctx, r.Resolver, cid, cust)
}

func (r *queryResolver) CustomerSignals(ctx context.Context, customerID string) (*model.CustomerSignals, error) {
	if err := r.requireSignals(); err != nil {
		return nil, err
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "customers.view"); err != nil {
		return nil, err
	}
	cid, err := uuid.Parse(customerID)
	if err != nil {
		return nil, errInvalidID("customerId", err)
	}
	cust, err := r.CustomerSvc.GetByID(ctx, cid)
	if err != nil {
		return nil, errNotFound("customer")
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "customers.view", cust.ChannelID, nil); err != nil {
		return nil, err
	}

	out := &model.CustomerSignals{
		Consents: []*model.SignalConsent{},
		Email:    &model.SignalEmail{},
		Flows:    []*model.SignalFlowRun{},
	}

	cons, err := r.ConsentSvc.ListForCustomer(ctx, cid) // one row per registered consent definition, opted-in or not
	if err != nil {
		return nil, fmt.Errorf("consents: %w", err)
	}
	for _, c := range cons {
		out.Consents = append(out.Consents, &model.SignalConsent{
			Kind:    c.Definition.Slug,
			Name:    c.Definition.Name,
			Channel: string(c.Definition.Channel),
			Purpose: string(c.Definition.Purpose),
			OptedIn: c.OptedIn,
		})
	}

	eng, err := r.SendLog.CustomerEngagement(ctx, cid)
	if err != nil {
		return nil, fmt.Errorf("email engagement: %w", err)
	}
	out.Email = &model.SignalEmail{
		Sent:       int32(eng.Sent),
		Opened:     int32(eng.Opened),
		Clicked:    int32(eng.Clicked),
		LastSentAt: eng.LastSentAt,
		LastOpenAt: eng.LastOpenAt,
	}

	runs, err := r.FlowSvc.RunsForCustomer(ctx, cid)
	if err != nil {
		return nil, fmt.Errorf("flow runs: %w", err)
	}
	for _, run := range runs {
		out.Flows = append(out.Flows, &model.SignalFlowRun{
			FlowID:     run.FlowID.String(),
			FlowName:   run.FlowName,
			Trigger:    run.Trigger,
			Status:     run.Status,
			EnrolledAt: run.CreatedAt,
			UpdatedAt:  run.UpdatedAt,
		})
	}

	return out, nil
}
