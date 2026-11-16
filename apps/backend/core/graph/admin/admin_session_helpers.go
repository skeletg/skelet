package admin

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/admin"
	"github.com/skeletg/skelet/apps/backend/core/internal/sso"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

func (r *Resolver) adminSession(ctx context.Context, user admin.AdminUser) (*model.AdminAuthPayload, error) {
	if enabled, _ := r.ConfigSvc.GetBool(ctx, "general/security/admin_single_session", config.DefaultScope); enabled {
		if err := r.RefreshStore.RevokeAll(ctx, user.ID); err != nil {
			// Not fatal to the login, but not swallowed either - a failed revoke leaves the admin believing old sessions are dead when they aren't
			slog.Error("admin single-session: revoke existing sessions failed", "admin_id", user.ID, "err", err)
		}
	}

	ip, ua := clientMeta(ctx)
	refreshToken, sessionID, err := r.RefreshStore.Create(ctx, user.ID, ip, ua)
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}

	accessToken, err := auth.GenerateToken(user.ID, sessionID, user.Email, user.IsSuperuser, r.AdminKeys, r.JWTTTL)
	if err != nil {
		return nil, fmt.Errorf("generate access token: %w", err)
	}

	// Stamped after the credentials pass, and read for its PREVIOUS value: nil means nobody has ever signed into this account, which is what
	// the dashboard's first-run introduction keys off Fail-open on purpose - a bookkeeping write must never be what stops someone signing in
	firstLogin := false
	adminModel := toAdminUserModel(user)
	if prev, stamped, err := r.AdminSvc.RecordLogin(ctx, user.ID); err != nil {
		slog.Error("record admin login failed", "admin_id", user.ID, "err", err)
	} else {
		firstLogin = prev == nil
		adminModel.LastLoginAt = &stamped // the user row was read before the stamp, so carry it over
	}

	return &model.AdminAuthPayload{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		Admin:        adminModel,
		FirstLogin:   firstLogin,
	}, nil
}

func toAdminSsoLinkModel(l sso.AdminLink) *model.AdminSsoLink {
	return &model.AdminSsoLink{ID: l.ID.String(), Issuer: l.Issuer, Email: l.Email, CreatedAt: l.CreatedAt, LastUsedAt: l.LastUsedAt}
}
