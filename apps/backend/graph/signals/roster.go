package signals

import (
	"context"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/internal/auth"
)

// This service has no customers table, so a customer's channel comes off the roster the trait sync keeps, and one absent from it reads as not found - which is also what a deleted customer should look like here
func (r *queryResolver) customerChannel(ctx context.Context, customerID uuid.UUID) (uuid.UUID, error) {
	if r.Roster == nil {
		return uuid.Nil, errNotConfigured("customer roster")
	}
	channelID, err := r.Roster.ChannelForCustomer(ctx, customerID)
	if err != nil {
		return uuid.Nil, errNotFound("customer")
	}
	return channelID, nil
}

// customers.view on the customer's own channel, for the two per-customer queries that take an id rather than a channel
func (r *queryResolver) scopedCustomer(ctx context.Context, customerID string) (uuid.UUID, error) {
	if err := auth.RequirePermission(ctx, auth.GrantCheckerFrom(ctx), "customers.view"); err != nil {
		return uuid.Nil, err
	}
	cid, err := uuid.Parse(customerID)
	if err != nil {
		return uuid.Nil, errInvalidID("customerId", err)
	}
	channelID, err := r.customerChannel(ctx, cid)
	if err != nil {
		return uuid.Nil, err
	}
	if err := auth.RequireChannelPermission(ctx, auth.GrantCheckerFrom(ctx), "customers.view", channelID, nil); err != nil {
		return uuid.Nil, err
	}
	return cid, nil
}
