package auth

import (
	"context"

	"github.com/google/uuid"
)

type contextKey string

const adminKey contextKey = "admin"

type Claims struct {
	AdminID     uuid.UUID
	SessionID   uuid.UUID // admin_sessions row ID; zero when the token was minted without a session row
	Email       string
	IsSuperuser bool
}

func WithAdmin(ctx context.Context, claims *Claims) context.Context {
	return context.WithValue(ctx, adminKey, claims)
}

func AdminFromContext(ctx context.Context) (*Claims, bool) { // returns nil, false when no authenticated admin is present - a nil *Claims, which AdminSessionGuard uses to revoke a request whose session was killed, counts as absent, so a downstream RequireAuth can never hand a guard a nil to deref
	claims, ok := ctx.Value(adminKey).(*Claims)
	if !ok || claims == nil {
		return nil, false
	}
	return claims, true
}

const customerKey contextKey = "customer"

type CustomerClaims struct { // channelID is the store the customer registered on - resolvers cross-check it against the request-time store to reject cross-store login attempts
	CustomerID uuid.UUID
	ChannelID  uuid.UUID
	Email      string
}

func WithCustomer(ctx context.Context, claims *CustomerClaims) context.Context {
	return context.WithValue(ctx, customerKey, claims)
}

func CustomerFromContext(ctx context.Context) (*CustomerClaims, bool) {
	claims, ok := ctx.Value(customerKey).(*CustomerClaims)
	return claims, ok
}

const apiTokenKey contextKey = "api_token"

// An integration token's identity and permission set, injected by integration.Middleware on the admin API - deliberately distinct from admin Claims so a machine caller never reads as a person
type ApiTokenClaims struct {
	TokenID     uuid.UUID
	Name        string              // the integration's display name - audit rows carry it since there's no admin email to attribute
	Permissions map[string]struct{} // slug → present; loaded once at validation so guard checks stay in-memory
	// nil = the token's grants hold globally; set = they only satisfy checks scoped to this channel
	ChannelID *uuid.UUID
}

func WithApiToken(ctx context.Context, claims *ApiTokenClaims) context.Context {
	return context.WithValue(ctx, apiTokenKey, claims)
}

func ApiTokenFromContext(ctx context.Context) (*ApiTokenClaims, bool) { // a stored nil counts as absent, same invariant as AdminFromContext, so guards never deref a revoked stamp
	claims, ok := ctx.Value(apiTokenKey).(*ApiTokenClaims)
	if !ok || claims == nil {
		return nil, false
	}
	return claims, true
}
