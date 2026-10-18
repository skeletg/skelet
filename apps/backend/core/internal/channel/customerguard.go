package channel

import (
	"context"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

// The customer guards that need the resolved store - whether a session belongs to this store is a channel question

// Claims only when their channel_id matches the resolved store - a store-A cookie reads as signed-out on store-B, since a mismatch is unauthenticated
func RequireCustomerForCurrentStore(ctx context.Context) (*auth.CustomerClaims, error) {
	claims, ok := auth.CustomerFromContext(ctx)
	if !ok {
		return nil, auth.ErrUnauthenticated
	}
	sv, ok := FromContext(ctx)
	if !ok || sv == nil {
		return nil, auth.ErrUnauthenticated
	}
	if claims.ChannelID != sv.ID {
		return nil, auth.ErrUnauthenticated
	}
	return claims, nil
}

// For endpoints allowing guest or customer - nil when unauthenticated or authed on a different store, so wrong-store degrades to guest
func OptionalCustomerID(ctx context.Context) *uuid.UUID {
	claims, ok := auth.CustomerFromContext(ctx)
	if !ok {
		return nil
	}
	sv, ok := FromContext(ctx)
	if !ok || sv == nil {
		return nil
	}
	if claims.ChannelID != sv.ID {
		return nil
	}
	id := claims.CustomerID
	return &id
}
