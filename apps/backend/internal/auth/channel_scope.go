package auth

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

type PermittedChannels struct { // a per-request snapshot of the channels an admin can see for one slug - Bypass means see-all for a superuser or global scope, otherwise filter through Allows, IDs or FilterByChannel
	bypass bool
	ids    map[uuid.UUID]struct{}
}

func (p PermittedChannels) Bypass() bool { return p.bypass }

func (p PermittedChannels) Allows(id uuid.UUID) bool {
	if p.bypass {
		return true
	}
	_, ok := p.ids[id]
	return ok
}

func (p PermittedChannels) IDs() []uuid.UUID { // the permitted set as a slice for SQL filters - nil on Bypass meaning every channel, and an empty non-bypass matching nothing - Order unspecified
	if p.bypass {
		return nil
	}
	out := make([]uuid.UUID, 0, len(p.ids))
	for id := range p.ids {
		out = append(out, id)
	}
	return out
}

type ChannelScopeProvider interface { // the admin-service surface LoadPermittedChannels needs - an interface for tests
	HasGlobalPermission(ctx context.Context, adminID uuid.UUID, slug string) (bool, error)
	ListPermittedChannelIDs(ctx context.Context, adminID uuid.UUID, slug string) ([]uuid.UUID, error)
}

// Resolves an admin and slug's permitted channels - Bypass with no admin, a superuser or global scope, else the explicit scoped set, where empty means nothing - Cached per resolver chain
func LoadPermittedChannels(ctx context.Context, svc ChannelScopeProvider, slug string) (PermittedChannels, error) {
	claims, ok := AdminFromContext(ctx)
	if !ok {
		// a channel-pinned integration token sees exactly its channel - unpinned tokens (and the
		// no-credential paths guarded elsewhere) keep the historical bypass
		if tok, tokOk := ApiTokenFromContext(ctx); tokOk && tok.ChannelID != nil {
			return PermittedChannels{ids: map[uuid.UUID]struct{}{*tok.ChannelID: {}}}, nil
		}
		return PermittedChannels{bypass: true}, nil
	}
	if claims.IsSuperuser {
		return PermittedChannels{bypass: true}, nil
	}
	hasGlobal, err := svc.HasGlobalPermission(ctx, claims.AdminID, slug)
	if err != nil {
		return PermittedChannels{}, err
	}
	if hasGlobal {
		return PermittedChannels{bypass: true}, nil
	}
	ids, err := svc.ListPermittedChannelIDs(ctx, claims.AdminID, slug)
	if err != nil {
		return PermittedChannels{}, err
	}
	set := make(map[uuid.UUID]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return PermittedChannels{ids: set}, nil
}

func (p PermittedChannels) AllowsOptional(id *uuid.UUID) bool { // treats a nil id as a global resource, permitted only on Bypass - for nullable channel_id like promotion rules, while non-nil falls through to Allows
	if id == nil {
		return p.Bypass()
	}
	return p.Allows(*id)
}

// Gates a nullable-channel mutation - nil needs global scope, otherwise store scope
func RequireGlobalOrChannelPermission(ctx context.Context, checker ScopedPermissionChecker, slug string, channelID *uuid.UUID) error {
	if channelID == nil {
		return RequireGlobalPermission(ctx, checker, slug)
	}
	return RequireChannelPermission(ctx, checker, slug, *channelID, nil)
}

type AllChannelsListedChecker interface {
	ScopedPermissionChecker
	PermissionChecker
}

// Gates writes to rows SHARED across every channel that lists the entity, which have no channel column of their own
// An (EU, de) grant would otherwise reach a US-listed product through the shared de row, so this demands scope on EVERY listed channel, failing on the first miss - An unlisted entity falls back to a bare slug check, having no leak surface
// Pass the concrete locale for locale content; an empty locale gates a non-localized global row, where per channel a grant of any locale scope counts
func RequireAllListedChannelsPermission(
	ctx context.Context,
	checker AllChannelsListedChecker,
	slug string,
	listedChannelIDs []uuid.UUID,
	locale string,
) error {
	claims, ok := AdminFromContext(ctx)
	if !ok {
		if tok, tokOk := ApiTokenFromContext(ctx); tokOk {
			// an unpinned token's grant is global, exactly the scope this gate demands - a pinned one
			// passes only when its channel is the ONLY one listed
			for _, id := range listedChannelIDs {
				if tok.ChannelID != nil && *tok.ChannelID != id {
					return &ScopedDenial{Slug: slug}
				}
			}
			return requireTokenPermission(tok, slug)
		}
		return ErrUnauthenticated
	}
	if claims.IsSuperuser {
		return nil
	}
	hasGlobal, err := checker.HasGlobalPermission(ctx, claims.AdminID, slug)
	if err != nil {
		return fmt.Errorf("check permission: %w", err)
	}
	if hasGlobal {
		return nil
	}
	if len(listedChannelIDs) == 0 {
		ok, err := checker.HasPermission(ctx, claims.AdminID, slug)
		if err != nil {
			return fmt.Errorf("check permission: %w", err)
		}
		if !ok {
			return ErrForbidden
		}
		return nil
	}
	var localePtr *string
	if locale != "" {
		localePtr = &locale
	}
	for _, channelID := range listedChannelIDs {
		ok, err := checker.HasChannelPermission(ctx, claims.AdminID, slug, channelID, localePtr)
		if err != nil {
			return fmt.Errorf("check permission: %w", err)
		}
		if !ok {
			return ErrForbidden
		}
	}
	return nil
}

func FilterByChannel[T any](items []T, p PermittedChannels, channelOf func(T) uuid.UUID) []T { // drops items whose channel isn't permitted - returns the slice unchanged on Bypass
	if p.bypass {
		return items
	}
	out := make([]T, 0, len(items))
	for _, it := range items {
		if p.Allows(channelOf(it)) {
			out = append(out, it)
		}
	}
	return out
}
