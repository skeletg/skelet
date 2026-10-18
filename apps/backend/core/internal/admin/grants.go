package admin

import (
	"context"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
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

// RequireWithinOwn refuses handing out any grant wider than the caller holds
func (s *Service) RequireWithinOwn(ctx context.Context, callerID uuid.UUID, wanted []auth.Grant) error {
	own, err := s.grantsFor(ctx, callerID)
	if err != nil {
		return err
	}
	for _, w := range wanted {
		if !own.Covers(w) {
			return &auth.ScopedDenial{Slug: w.Slug, ChannelID: w.ChannelID, Locale: w.Locale}
		}
	}
	return nil
}

func (s *Service) RoleGrants(ctx context.Context, roleSlugs []string) ([]auth.Grant, error) {
	var out []auth.Grant
	for _, slug := range roleSlugs {
		role, err := s.repo.GetRoleBySlug(ctx, slug)
		if err != nil {
			return nil, err
		}
		out = append(out, scopeGrants(role.PermissionScopes)...)
	}
	return out, nil
}

func (s *Service) RoleByID(ctx context.Context, id uuid.UUID) (Role, error) {
	return s.repo.GetRoleByID(ctx, id)
}

func scopeGrants(scopes []RolePermissionScope) []auth.Grant {
	out := make([]auth.Grant, 0, len(scopes))
	for _, sc := range scopes {
		out = append(out, auth.Grant{Slug: sc.Permission.Slug, ChannelID: sc.ChannelID, Locale: sc.Locale})
	}
	return out
}

// RequireOutranks refuses acting on an admin who holds anything the caller doesn't - a superuser only by another
func (s *Service) RequireOutranks(ctx context.Context, callerID, targetID uuid.UUID) error {
	target, err := s.repo.GetByID(ctx, targetID)
	if err != nil {
		return err
	}
	if target.IsSuperuser {
		return auth.ErrForbidden
	}
	grants, err := s.Grants(ctx, targetID)
	if err != nil {
		return err
	}
	return s.RequireWithinOwn(ctx, callerID, grants)
}

// RequireOutranksMembers refuses changing a role's grants under any member the caller does not outrank
func (s *Service) RequireOutranksMembers(ctx context.Context, callerID, roleID uuid.UUID) error {
	members, err := s.repo.RoleMemberIDs(ctx, roleID)
	if err != nil {
		return err
	}
	for _, id := range members {
		if err := s.RequireOutranks(ctx, callerID, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) HoldsRole(ctx context.Context, adminID, roleID uuid.UUID) (bool, error) {
	roles, err := s.repo.GetRoles(ctx, adminID)
	if err != nil {
		return false, err
	}
	for _, r := range roles {
		if r.ID == roleID {
			return true, nil
		}
	}
	return false, nil
}

func (s *Service) Standing(ctx context.Context, adminID uuid.UUID) (superuser, active bool, err error) {
	u, err := s.repo.GetByID(ctx, adminID)
	if err != nil {
		return false, false, err
	}
	return u.IsSuperuser, u.IsActive, nil
}

// Grants returns every scope an admin holds, for minting a service token that carries them
func (s *Service) Grants(ctx context.Context, adminID uuid.UUID) ([]auth.Grant, error) {
	set, err := s.grantsFor(ctx, adminID)
	if err != nil {
		return nil, err
	}
	return set.Grants(), nil
}
