package admin

import (
	"context"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/internal/auth"
)

// Loaded at most once per request through the auth request cache - with no cache on the context (background jobs, tests bypassing the middleware) it loads on every call, same result minus the memoization
func (s *Service) grantsFor(ctx context.Context, adminID uuid.UUID) (*auth.GrantSet, error) {
	load := func() (any, error) {
		grants, err := s.repo.ListAllGrants(ctx, adminID)
		if err != nil {
			return nil, err
		}
		return auth.NewGrantSet(grants), nil
	}
	if rc := auth.RequestCacheFrom(ctx); rc != nil {
		v, err := rc.GetOrLoad("admin-grants:"+adminID.String(), load)
		if err != nil {
			return nil, err
		}
		return v.(*auth.GrantSet), nil
	}
	v, err := load()
	if err != nil {
		return nil, err
	}
	return v.(*auth.GrantSet), nil
}
