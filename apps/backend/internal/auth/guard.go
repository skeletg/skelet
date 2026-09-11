package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/internal/apperr"
	"github.com/skeletg/skelet/apps/backend/internal/channel"
)

var ErrUnauthenticated = errors.New("unauthenticated") // classAuth so the front presenter shows these verbatim, not scrubbed to "internal error"

var ErrForbidden = errors.New("forbidden")

// ErrForbidden plus what was denied - it unwraps to ErrForbidden so callers and the registry are unaffected, and the fields feed the dev "why forbidden" presenter
type ScopedDenial struct {
	Slug      string
	ChannelID *uuid.UUID
	Locale    *string
}

func (e *ScopedDenial) Error() string { return ErrForbidden.Error() }
func (e *ScopedDenial) Unwrap() error { return ErrForbidden }

func (e *ScopedDenial) DenialDetails() (slug, channelID, locale string) { // denialDetails feeds the graphqlx presenter via errors.As, keeping it decoupled from auth
	if e.ChannelID != nil {
		channelID = e.ChannelID.String()
	}
	if e.Locale != nil {
		locale = *e.Locale
	}
	return e.Slug, channelID, locale
}

var ErrInvalidRefreshToken = errors.New("invalid refresh token") // every refusal (unknown/expired/disabled/wrong-store) collapses here, so it can't leak which precondition failed

func init() {
	apperr.RegisterSentinel(ErrUnauthenticated, "UNAUTHENTICATED", apperr.ClassAuth)
	apperr.RegisterSentinel(ErrForbidden, "FORBIDDEN", apperr.ClassForbidden)
	apperr.RegisterSentinel(ErrInvalidRefreshToken, "INVALID_REFRESH_TOKEN", apperr.ClassAuth)
	apperr.RegisterSentinel(ErrTokenNotFound, "TOKEN_NOT_FOUND", apperr.ClassNotFound)
}

type PermissionChecker interface {
	HasPermission(ctx context.Context, adminID uuid.UUID, slug string) (bool, error)
}

// Both halves at once, for guards that pick between them by target
type ScopeAwareChecker interface {
	PermissionChecker
	ScopedPermissionChecker
}

type ScopedPermissionChecker interface {
	HasGlobalPermission(ctx context.Context, adminID uuid.UUID, slug string) (bool, error)
	HasChannelPermission(ctx context.Context, adminID uuid.UUID, slug string, channelID uuid.UUID, locale *string) (bool, error)
}

func RequireAuth(ctx context.Context) (*Claims, error) {
	claims, ok := AdminFromContext(ctx)
	if !ok {
		return nil, ErrUnauthenticated
	}
	return claims, nil
}

// Claims only when their channel_id matches the resolved store - a store-A cookie reads as signed-out on store-B, since a mismatch is unauthenticated
func RequireCustomerForCurrentStore(ctx context.Context) (*CustomerClaims, error) {
	claims, ok := CustomerFromContext(ctx)
	if !ok {
		return nil, ErrUnauthenticated
	}
	sv, ok := channel.FromContext(ctx)
	if !ok || sv == nil {
		return nil, ErrUnauthenticated
	}
	if claims.ChannelID != sv.ID {
		return nil, ErrUnauthenticated
	}
	return claims, nil
}

// For endpoints allowing guest or customer - nil when unauthenticated or authed on a different store, so wrong-store degrades to guest
func OptionalCustomerID(ctx context.Context) *uuid.UUID {
	claims, ok := CustomerFromContext(ctx)
	if !ok {
		return nil
	}
	sv, ok := channel.FromContext(ctx)
	if !ok || sv == nil {
		return nil
	}
	if claims.ChannelID != sv.ID {
		return nil
	}
	id := claims.CustomerID
	return &id
}

// The integration-token paths shared by every guard below - consulted only when no admin is signed
// in, so a request carrying both credentials authorizes purely as the admin and a denial is never
// rescued by the token
// An UNPINNED token's grant is global and satisfies unscoped, global and store-scoped checks alike.
// A channel-pinned token satisfies the unscoped check (parity with an admin whose only grants are
// channel-scoped) and the matching channel check, but never a global one
func requireTokenPermission(claims *ApiTokenClaims, slug string) error {
	if _, has := claims.Permissions[slug]; has {
		return nil
	}
	return &ScopedDenial{Slug: slug}
}

func requireGlobalTokenPermission(claims *ApiTokenClaims, slug string) error {
	if claims.ChannelID != nil {
		return &ScopedDenial{Slug: slug} // pinned - this gate demands scope on every channel
	}
	return requireTokenPermission(claims, slug)
}

func requireChannelTokenPermission(claims *ApiTokenClaims, slug string, channelID uuid.UUID) error {
	if claims.ChannelID != nil && *claims.ChannelID != channelID {
		return &ScopedDenial{Slug: slug}
	}
	return requireTokenPermission(claims, slug)
}

func RequirePermission(ctx context.Context, checker PermissionChecker, slug string) error { // requirePermission checks an unscoped admin permission - Superusers bypass (as do the scoped variants below), integration tokens authorize through their grant set
	claims, ok := AdminFromContext(ctx)
	if !ok {
		if tok, tokOk := ApiTokenFromContext(ctx); tokOk {
			return requireTokenPermission(tok, slug)
		}
		return ErrUnauthenticated
	}
	if claims.IsSuperuser {
		return nil
	}
	ok, err := checker.HasPermission(ctx, claims.AdminID, slug)
	if err != nil {
		return fmt.Errorf("check permission: %w", err)
	}
	if !ok {
		return &ScopedDenial{Slug: slug}
	}
	return nil
}

func RequireGlobalPermission(ctx context.Context, checker ScopedPermissionChecker, slug string) error { // requireGlobalPermission checks the permission through a role scoped to all stores/locales
	claims, ok := AdminFromContext(ctx)
	if !ok {
		if tok, tokOk := ApiTokenFromContext(ctx); tokOk {
			return requireGlobalTokenPermission(tok, slug)
		}
		return ErrUnauthenticated
	}
	if claims.IsSuperuser {
		return nil
	}
	ok, err := checker.HasGlobalPermission(ctx, claims.AdminID, slug)
	if err != nil {
		return fmt.Errorf("check permission: %w", err)
	}
	if !ok {
		return &ScopedDenial{Slug: slug}
	}
	return nil
}

func RequireChannelPermission(ctx context.Context, checker ScopedPermissionChecker, slug string, channelID uuid.UUID, locale *string) error { // requireChannelPermission checks the permission through a role whose channel/locale scope matches the request
	claims, ok := AdminFromContext(ctx)
	if !ok {
		if tok, tokOk := ApiTokenFromContext(ctx); tokOk {
			return requireChannelTokenPermission(tok, slug, channelID)
		}
		return ErrUnauthenticated
	}
	if claims.IsSuperuser {
		return nil
	}
	ok, err := checker.HasChannelPermission(ctx, claims.AdminID, slug, channelID, locale)
	if err != nil {
		return fmt.Errorf("check permission: %w", err)
	}
	if !ok {
		return &ScopedDenial{Slug: slug, ChannelID: &channelID, Locale: locale}
	}
	return nil
}

// HoldsAnywhere reports whether the caller holds a permission at all, at any scope - the check behind a MENU,
// where a channel-scoped grant is enough to show an entry whose run then re-checks the channel it targets
func HoldsAnywhere(ctx context.Context, svc ChannelScopeProvider, slug string) bool {
	if slug == "" {
		return false
	}
	if claims, err := RequireAuth(ctx); err == nil && claims.IsSuperuser {
		return true
	}
	permitted, err := LoadPermittedChannels(ctx, svc, slug)
	if err != nil {
		return false
	}
	return permitted.Bypass() || len(permitted.IDs()) > 0
}

// RequireScopedOrGlobal is the rule for an operation whose blast radius depends on its target: no channel (or an
// explicitly global one) means the whole installation and demands the unscoped grant, while a targeted run only
// needs that channel. One implementation so the GraphQL and HTTP surfaces cannot drift apart
func RequireScopedOrGlobal(ctx context.Context, svc ScopeAwareChecker, slug string, channelID uuid.UUID, global bool) error {
	if slug == "" {
		return ErrForbidden
	}
	if global || channelID == uuid.Nil {
		return RequirePermission(ctx, svc, slug)
	}
	return RequireChannelPermission(ctx, svc, slug, channelID, nil)
}
