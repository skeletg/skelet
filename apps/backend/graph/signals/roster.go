package signals

import (
	"context"

	"github.com/google/uuid"
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
