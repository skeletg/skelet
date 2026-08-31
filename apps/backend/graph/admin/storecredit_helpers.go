package admin

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/internal/auth"
	"github.com/skeletg/skelet/apps/backend/internal/customer"
)

// storeCreditContext loads the customer, enforces the channel-scoped permission, and resolves the channel currency
// store credit is denominated in
func (r *Resolver) storeCreditContext(ctx context.Context, customerID, perm string) (customer.Customer, string, error) {
	cid, err := parseID(customerID, "customer")
	if err != nil {
		return customer.Customer{}, "", err
	}
	c, err := r.CustomerSvc.GetByID(ctx, cid)
	if err != nil {
		return customer.Customer{}, "", errNotFound("customer")
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, perm, c.ChannelID, nil); err != nil {
		return customer.Customer{}, "", err
	}
	ch, err := r.ChannelSvc.GetByID(ctx, c.ChannelID)
	if err != nil {
		return customer.Customer{}, "", fmt.Errorf("load channel: %w", err)
	}
	return c, ch.CurrencyCode, nil
}

func (r *Resolver) storeCreditSummary(ctx context.Context, customerID uuid.UUID, currency string) (*model.StoreCreditSummary, error) {
	bal, err := r.StoreCreditSvc.Balance(ctx, customerID, currency)
	if err != nil {
		return nil, err
	}
	entries, err := r.StoreCreditSvc.Entries(ctx, customerID, 50)
	if err != nil {
		return nil, err
	}
	out := &model.StoreCreditSummary{
		BalanceCents: int32(bal),
		CurrencyCode: currency,
		Entries:      make([]*model.StoreCreditEntry, 0, len(entries)),
	}
	for _, e := range entries {
		m := &model.StoreCreditEntry{
			ID:           e.ID.String(),
			AmountCents:  int32(e.AmountCents),
			CurrencyCode: e.CurrencyCode,
			Reason:       e.Reason,
			CreatedAt:    e.CreatedAt,
		}
		if e.OrderID != nil {
			id := e.OrderID.String()
			m.OrderID = &id
		}
		out.Entries = append(out.Entries, m)
	}
	return out, nil
}

// loyaltySummary assembles the points balance + recent ledger projection.
func (r *Resolver) loyaltySummary(ctx context.Context, customerID uuid.UUID) (*model.LoyaltySummary, error) {
	bal, err := r.LoyaltySvc.Balance(ctx, customerID)
	if err != nil {
		return nil, err
	}
	entries, err := r.LoyaltySvc.Entries(ctx, customerID, 50)
	if err != nil {
		return nil, err
	}
	out := &model.LoyaltySummary{
		BalancePoints: int32(bal),
		Entries:       make([]*model.LoyaltyEntry, 0, len(entries)),
	}
	for _, e := range entries {
		m := &model.LoyaltyEntry{
			ID:        e.ID.String(),
			Points:    int32(e.Points),
			Reason:    e.Reason,
			CreatedAt: e.CreatedAt,
		}
		if e.OrderID != nil {
			id := e.OrderID.String()
			m.OrderID = &id
		}
		out.Entries = append(out.Entries, m)
	}
	return out, nil
}
