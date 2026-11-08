package admin

import (
	"context"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/adminsearch"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

// AdminSearch is the resolver for the adminSearch field.
func (r *queryResolver) AdminSearch(ctx context.Context, term string) (*model.AdminSearchResult, error) {
	// Signed in is the floor - each kind then needs its own permission, read in the channels its list page would cover
	if _, err := auth.RequireAdmin(ctx); err != nil {
		return nil, err
	}
	scope, err := r.adminSearchScope(ctx)
	if err != nil {
		return nil, err
	}
	res, err := adminsearch.New(r.Queries).Search(ctx, term, scope, r.activeLocale(ctx))
	if err != nil {
		return nil, err
	}
	return toAdminSearchResult(res), nil
}
