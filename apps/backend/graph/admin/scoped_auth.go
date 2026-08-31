package admin

import (
	"context"
	"fmt"

	"github.com/skeletg/skelet/apps/backend/internal/auth"
	"github.com/skeletg/skelet/apps/backend/internal/config"
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

func hasGlobalPermission(ctx context.Context, checker auth.ScopedPermissionChecker, slug string) (bool, error) {
	claims, err := auth.RequireAuth(ctx)
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
