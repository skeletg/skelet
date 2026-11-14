package medialibrary

import (
	"context"
	"slices"

	authpkg "github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

// The private half's own grant, on top of the library's, for resolvers and HTTP routes alike - only a grant on all channels counts
func RequireHalf(ctx context.Context, grants authpkg.ScopedPermissionChecker, slug string, halves ...Visibility) error {
	if !slices.Contains(halves, Private) {
		return nil
	}
	return authpkg.RequireGlobalPermission(ctx, grants, slug)
}
