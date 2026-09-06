package sdk

import (
	"context"

	"github.com/google/uuid"
)

// The channel-scoped half of the admin permission model, for a plugin whose records belong to one
// channel - which is most of them
//
// RequirePermission asks only whether the caller HOLDS a permission. That is the right check for a
// server-wide surface and the wrong one for a per-channel record: an admin scoped to one store would
// pass it and then act on another store's data. Where a plugin's object has a channel, gate on this
//
// Declare the permission with RegisterPermission first, or you are gating on one no role can hold. A
// superuser passes everything; an unauthenticated caller is always refused, so this is safe as the
// first line of a resolver
//
//	co, err := loadCompany(ctx, id)   // load first - its channel IS the scope
//	if err != nil { return err }
//	if err := sdk.RequireChannelPermission(ctx, PermManage, co.ChannelID); err != nil { return err }
func RequireChannelPermission(ctx context.Context, slug string, channelID uuid.UUID) error {
	h := hostNow()
	if h == nil {
		// no host means no auth wired, which is a test or an early-boot caller - refuse rather than admit
		return ErrPermissionUnavailable
	}
	return h.RequireAdminChannelPermission(ctx, slug, channelID)
}

// Which channels the caller may see for one permission, for LIST queries that have no single object
// to scope on
//
// bypass means every channel - a superuser, or a role holding the permission globally. It is NOT the
// same as an empty id list, which means the caller may see NOTHING; conflating the two turns a
// scoped-out admin into an unrestricted one, so branch on bypass explicitly
//
// Check the permission is held BEFORE calling this: a caller with no admin claims at all comes back
// as bypass, which a listing would read as see-everything
//
//	ids, bypass, err := sdk.PermittedChannels(ctx, PermView)
func PermittedChannels(ctx context.Context, slug string) (ids []uuid.UUID, bypass bool, err error) {
	h := hostNow()
	if h == nil {
		return nil, false, ErrPermissionUnavailable
	}
	return h.PermittedChannels(ctx, slug)
}

// The signed-in STOREFRONT customer, nil for a guest. The storefront half of AdminID: a front
// resolver acts for a customer whose identity rides in the request context rather than in the admin
// permission table, so this is how a plugin scopes its answer to "whoever is asking"
//
// A nil here is not an error. Plenty of storefront surfaces answer sensibly for a guest, and the
// ones that cannot should say so with a UserError rather than assume an identity
func CustomerID(ctx context.Context) *uuid.UUID {
	h := hostNow()
	if h == nil {
		return nil
	}
	return h.RequestCustomerID(ctx)
}

// The channel this request resolved to, uuid.Nil off a storefront request - for scoping a per-channel
// read, and for the channel argument to IsEnabled
func RequestChannelID(ctx context.Context) uuid.UUID {
	h := hostNow()
	if h == nil {
		return uuid.Nil
	}
	return h.RequestChannelID(ctx)
}

// The acting admin, for ATTRIBUTION - stamping who moved money on an audit row, naming an author.
// nil for an integration token or an unauthenticated caller
//
// Never an authorization check. "There is an admin here" is not "this admin may do that": gate on
// RequirePermission or RequireChannelPermission, and use this only to record who it was
func AdminID(ctx context.Context) *uuid.UUID {
	h := hostNow()
	if h == nil {
		return nil
	}
	return h.ActingAdminID(ctx)
}
