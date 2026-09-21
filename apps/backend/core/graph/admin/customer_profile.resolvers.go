package admin

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
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

	// Off the spine, not the send log - core keeps no record of a marketing send once the suite owns them
	authed, err := signalsCtx(ctx, r.Resolver)
	if err != nil {
		return nil, err
	}
	eng, err := r.SignalsAPI.Pulse.GetCustomerEngagement(authed, &signalsv1.GetCustomerEngagementRequest{
		CustomerId: customerID,
	})
	if err != nil {
		return nil, signalsErr(err)
	}
	out.Email = &model.SignalEmail{
		Sent:       eng.GetSent(),
		Opened:     eng.GetOpened(),
		Clicked:    eng.GetClicked(),
		LastSentAt: parseSignalsTime(eng.LastSentAt),
		LastOpenAt: parseSignalsTime(eng.LastOpenAt),
	}

	runs, err := r.SignalsAPI.Flows.ListCustomerFlowRuns(authed, &signalsv1.ListCustomerFlowRunsRequest{CustomerId: customerID})
	if err != nil {
		return nil, signalsErr(err)
	}
	for _, run := range runs.GetRuns() {
		out.Flows = append(out.Flows, &model.SignalFlowRun{
			FlowID:     run.GetFlowId(),
			FlowName:   run.GetFlowName(),
			Trigger:    run.GetTriggerType(),
			Status:     run.GetStatus(),
			EnrolledAt: protoTime(run.GetCreatedAt()),
			UpdatedAt:  protoTime(run.GetUpdatedAt()),
		})
	}

	return out, nil
}
