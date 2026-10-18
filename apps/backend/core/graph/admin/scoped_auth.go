package admin

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

func requireConfigScopePermission(ctx context.Context, checker auth.ScopedPermissionChecker, slug string, scope config.Scope) error {
	if scope.Kind != config.ScopeChannel {
		return auth.RequireGlobalPermission(ctx, checker, slug)
	}
	if scope.ChannelID == nil {
		return fmt.Errorf("channel scope requires a channelID")
	}
	return auth.RequireChannelPermission(ctx, checker, slug, *scope.ChannelID, scope.Locale)
}

// tokenChannels is the channel-picker fallback for a token - its grants hold on every channel unless it is pinned, and a token holding nothing sees none
func tokenChannels(tok *auth.ApiTokenClaims) (all bool, ids []uuid.UUID) {
	if len(tok.Permissions) == 0 {
		return false, nil
	}
	if tok.ChannelID != nil {
		return false, []uuid.UUID{*tok.ChannelID}
	}
	return true, nil
}

func isAdminCaller(ctx context.Context) bool {
	_, ok := auth.AdminFromContext(ctx)
	return ok
}

func hasGlobalPermission(ctx context.Context, checker auth.ScopedPermissionChecker, slug string) (bool, error) {
	claims, err := auth.RequireAdmin(ctx)
	if err != nil {
		return false, err
	}
	if claims.IsSuperuser {
		return true, nil
	}
	ok, err := checker.HasGlobalPermission(ctx, claims.AdminID, slug)
	if err != nil {
		return false, fmt.Errorf("check permission: %w", err)
	}
	return ok, nil
}
