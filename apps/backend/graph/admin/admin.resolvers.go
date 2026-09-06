package admin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/internal/admin"
	"github.com/skeletg/skelet/apps/backend/internal/apperr"
	"github.com/skeletg/skelet/apps/backend/internal/auth"
	"github.com/skeletg/skelet/apps/backend/internal/config"
	"github.com/skeletg/skelet/apps/backend/internal/ptr"
)

func (r *mutationResolver) AdminLogin(ctx context.Context, input model.AdminLoginInput) (*model.AdminAuthPayload, error) {
	// Keyed per-IP (one source spraying many accounts) AND per-email (a botnet rotating sources at one account), on top of a failure-count lockout that survives across rate windows
	email := strings.ToLower(strings.TrimSpace(input.Email))
	if err := r.checkAuthRate(ctx, "admin_login"); err != nil {
		return nil, err
	}
	if err := r.checkAuthRateKey(ctx, "admin_login_email", email); err != nil {
		return nil, err
	}
	if r.LoginThrottle.Locked(ctx, "admin", email) {
		return nil, auth.ErrAccountLocked
	}
	user, err := r.AdminSvc.Authenticate(ctx, input.Email, input.Password, ptr.Deref(input.TotpCode))
	if err != nil {
		if !errors.Is(err, admin.ErrTotpRequired) { // TOTP_REQUIRED means the password WAS right - prompting for the second factor must not burn lockout budget
			r.LoginThrottle.RecordFailure(ctx, "admin", email)
		}
		return nil, err
	}
	r.LoginThrottle.Clear(ctx, "admin", email)

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

	accessToken, err := auth.GenerateToken(user.ID, sessionID, user.Email, user.IsSuperuser, r.JWTSecret, r.JWTTTL)
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

// Always true, whether or not the address maps to an admin - anything else is an enumeration oracle, and the service only sends when it does
func (r *mutationResolver) RequestAdminPasswordReset(ctx context.Context, email string) (bool, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if err := r.checkAuthRate(ctx, "admin_password_reset"); err != nil {
		return false, err
	}
	if err := r.checkAuthRateKey(ctx, "admin_password_reset_email", email); err != nil {
		return false, err
	}
	if err := r.AdminSvc.RequestPasswordReset(ctx, email); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) ResetAdminPassword(ctx context.Context, token string, newPassword string) (bool, error) {
	if err := r.checkAuthRate(ctx, "admin_password_reset_consume"); err != nil {
		return false, err
	}
	if err := r.AdminSvc.ResetPassword(ctx, token, newPassword); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) AdminTotpSetup(ctx context.Context) (*model.AdminTotpSetup, error) {
	claims, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	secret, url, err := r.AdminSvc.TotpSetup(ctx, claims.AdminID)
	if err != nil {
		return nil, err
	}
	return &model.AdminTotpSetup{Secret: secret, OtpauthURL: url}, nil
}

func (r *mutationResolver) AdminTotpEnable(ctx context.Context, code string) ([]string, error) {
	claims, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	return r.AdminSvc.TotpEnable(ctx, claims.AdminID, code)
}

func (r *mutationResolver) AdminTotpDisable(ctx context.Context, code string) (bool, error) {
	claims, err := auth.RequireAuth(ctx)
	if err != nil {
		return false, err
	}
	if err := r.AdminSvc.TotpDisable(ctx, claims.AdminID, code); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) RefreshToken(ctx context.Context, token string) (*model.AdminAuthPayload, error) {
	adminID, err := r.RefreshStore.ValidateAndRevoke(ctx, token) // GETDEL, so a replayed or concurrent refresh of the same token can't mint two successor families
	if err != nil {
		return nil, errInvalidInput("refresh token", nil)
	}

	user, err := r.AdminSvc.GetByID(ctx, adminID)
	if err != nil {
		return nil, errNotFound("admin")
	}

	if !user.IsActive {
		return nil, fmt.Errorf("account is disabled")
	}

	ip, ua := clientMeta(ctx)
	newRefreshToken, sessionID, err := r.RefreshStore.Create(ctx, user.ID, ip, ua)
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}

	accessToken, err := auth.GenerateToken(user.ID, sessionID, user.Email, user.IsSuperuser, r.JWTSecret, r.JWTTTL)
	if err != nil {
		return nil, fmt.Errorf("generate access token: %w", err)
	}

	return &model.AdminAuthPayload{
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
		Admin:        toAdminUserModel(user),
	}, nil
}

// DEV ONLY - mints a full session for any admin without a password so the dashboard's user-switcher can hop between permission setups, hard-gated on r.DevMode
// Skipping the single-session revocation is deliberate: switching identities must not kill the session you'll switch back to
func (r *mutationResolver) DevImpersonate(ctx context.Context, adminID string) (*model.AdminAuthPayload, error) {
	if !r.DevMode {
		return nil, fmt.Errorf("devImpersonate is only available in development")
	}
	aid, err := parseID(adminID, "admin")
	if err != nil {
		return nil, err
	}
	user, err := r.AdminSvc.GetByID(ctx, aid)
	if err != nil {
		return nil, errNotFound("admin")
	}
	if !user.IsActive {
		return nil, fmt.Errorf("account is disabled")
	}
	ip, ua := clientMeta(ctx)
	refreshToken, sessionID, err := r.RefreshStore.Create(ctx, user.ID, ip, ua)
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}
	accessToken, err := auth.GenerateToken(user.ID, sessionID, user.Email, user.IsSuperuser, r.JWTSecret, r.JWTTTL)
	if err != nil {
		return nil, fmt.Errorf("generate access token: %w", err)
	}
	return &model.AdminAuthPayload{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		Admin:        toAdminUserModel(user),
	}, nil
}

func (r *mutationResolver) Logout(ctx context.Context, token string) (bool, error) {
	if err := r.RefreshStore.Revoke(ctx, token); err != nil {
		return false, fmt.Errorf("logout: %w", err)
	}
	return true, nil
}

func (r *mutationResolver) LogoutOtherDevices(ctx context.Context, currentRefreshToken string) (bool, error) {
	claims, err := auth.RequireAuth(ctx)
	if err != nil {
		return false, err
	}
	currentHash := auth.HashToken(currentRefreshToken)
	if err := r.RefreshStore.RevokeExcept(ctx, claims.AdminID, currentHash); err != nil {
		return false, fmt.Errorf("logout other devices: %w", err)
	}
	return true, nil
}

func (r *mutationResolver) CreateAdmin(ctx context.Context, input model.CreateAdminInput) (*model.AdminUser, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "admins.manage"); err != nil {
		return nil, err
	}
	user, err := r.AdminSvc.CreateAdmin(ctx, input.Email, input.Password, input.FirstName, input.LastName, input.RoleSlugs)
	if err != nil {
		return nil, err
	}
	return toAdminUserModel(user), nil
}

// input.RoleSlugs nil = leave role assignments alone, [] = remove every role
func (r *mutationResolver) UpdateAdmin(ctx context.Context, id string, input model.UpdateAdminInput) (*model.AdminUser, error) {
	claims, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "admins.manage"); err != nil {
		return nil, err
	}
	adminID, err := parseID(id, "admin")
	if err != nil {
		return nil, err
	}
	// the privilege-bearing fields of your own account need a second admin - self-service role edits
	// are how admins.manage silently becomes whatever the holder wants, and self-deactivation is how
	// the last active admin locks everyone out (name edits on yourself stay allowed)
	if claims.AdminID == adminID && (input.RoleSlugs != nil || input.IsActive != nil) {
		return nil, apperr.New("ADMIN_SELF_EDIT", apperr.ClassValidation, "your own roles and active flag need another admin to change")
	}
	domainInput := admin.UpdateAdminUserInput{
		RoleSlugs: input.RoleSlugs,
	}
	if input.FirstName != nil {
		domainInput.FirstName = *input.FirstName
	}
	if input.LastName != nil {
		domainInput.LastName = *input.LastName
	}
	domainInput.IsActive = input.IsActive
	user, err := r.AdminSvc.Update(ctx, adminID, domainInput)
	if err != nil {
		return nil, err
	}
	return toAdminUserModel(user), nil
}

func (r *mutationResolver) DeleteAdmin(ctx context.Context, id string) (bool, error) {
	claims, err := auth.RequireAuth(ctx)
	if err != nil {
		return false, err
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "admins.manage"); err != nil {
		return false, err
	}
	adminID, err := parseID(id, "admin")
	if err != nil {
		return false, err
	}
	if claims.AdminID == adminID {
		return false, apperr.New("ADMIN_SELF_EDIT", apperr.ClassValidation, "deleting your own account needs another admin")
	}
	if err := r.AdminSvc.Delete(ctx, adminID); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) ChangePassword(ctx context.Context, currentPassword string, newPassword string) (bool, error) {
	claims, err := auth.RequireAuth(ctx)
	if err != nil {
		return false, err
	}
	if err := r.AdminSvc.ChangePassword(ctx, claims.AdminID, currentPassword, newPassword); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) CreateRole(ctx context.Context, input model.CreateRoleInput) (*model.Role, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "admins.manage"); err != nil {
		return nil, err
	}
	description := ""
	if input.Description != nil {
		description = *input.Description
	}
	scopes, err := rolePermissionScopeInputsFromGQL(input.Scopes)
	if err != nil {
		return nil, err
	}
	role, err := r.AdminSvc.CreateRole(ctx, admin.CreateRoleInput{
		Name:        input.Name,
		Slug:        input.Slug,
		Description: description,
		Scopes:      scopes,
	})
	if err != nil {
		return nil, err
	}
	return toRoleModel(role), nil
}

// input.Scopes nil = leave grants alone, [] = wipe every grant
func (r *mutationResolver) UpdateRole(ctx context.Context, id string, input model.UpdateRoleInput) (*model.Role, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "admins.manage"); err != nil {
		return nil, err
	}
	roleID, err := parseID(id, "role")
	if err != nil {
		return nil, err
	}
	scopes, err := rolePermissionScopeInputsFromGQL(input.Scopes)
	if err != nil {
		return nil, err
	}
	role, err := r.AdminSvc.UpdateRole(ctx, roleID, admin.UpdateRoleInput{
		Name:         input.Name,
		Description:  input.Description,
		Scopes:       scopes,
		UpdateScopes: input.Scopes != nil,
	})
	if err != nil {
		return nil, err
	}
	return toRoleModel(role), nil
}

// Errors with "role has members" while any admin still holds it - reassign them first
func (r *mutationResolver) DeleteRole(ctx context.Context, id string) (bool, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "admins.manage"); err != nil {
		return false, err
	}
	roleID, err := parseID(id, "role")
	if err != nil {
		return false, err
	}
	if err := r.AdminSvc.DeleteRole(ctx, roleID); err != nil {
		return false, err
	}
	return true, nil
}

func (r *queryResolver) Me(ctx context.Context) (*model.AdminUser, error) {
	claims, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	user, err := r.AdminSvc.GetByIDWithRoles(ctx, claims.AdminID)
	if err != nil {
		return nil, errNotFound("admin")
	}
	m := toAdminUserModel(user)
	sessions, err := r.RefreshStore.ListSessions(ctx, claims.AdminID)
	if err == nil && len(sessions) > 0 {
		m.Sessions = make([]*model.AdminSession, len(sessions))
		for i, s := range sessions {
			m.Sessions[i] = toAdminSessionModel(s, claims.SessionID)
		}
	}
	return m, nil
}

func (r *queryResolver) PermissionGroups(ctx context.Context) ([]*model.PermissionGroup, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "admins.view"); err != nil {
		return nil, err
	}
	groups, err := r.AdminSvc.ListPermissionGroups(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]*model.PermissionGroup, len(groups))
	for i, g := range groups {
		result[i] = toPermissionGroupModel(g)
	}
	return result, nil
}

func (r *queryResolver) Admins(ctx context.Context) ([]*model.AdminUser, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "admins.view"); err != nil {
		return nil, err
	}
	users, err := r.AdminSvc.List(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]*model.AdminUser, len(users))
	for i, u := range users {
		result[i] = toAdminUserModel(u)
	}
	return result, nil
}

func (r *queryResolver) Admin(ctx context.Context, id string) (*model.AdminUser, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "admins.view"); err != nil {
		return nil, err
	}
	adminID, err := parseID(id, "admin")
	if err != nil {
		return nil, err
	}
	user, err := r.AdminSvc.GetByIDWithRoles(ctx, adminID)
	if err != nil {
		return nil, errNotFound("admin")
	}
	return toAdminUserModel(user), nil
}

func (r *queryResolver) Roles(ctx context.Context) ([]*model.Role, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "admins.view"); err != nil {
		return nil, err
	}
	roles, err := r.AdminSvc.ListRoles(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]*model.Role, len(roles))
	for i, role := range roles {
		result[i] = toRoleModel(role)
	}
	return result, nil
}

func (r *queryResolver) Role(ctx context.Context, id string) (*model.Role, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "admins.view"); err != nil {
		return nil, err
	}
	roleID, err := parseID(id, "role")
	if err != nil {
		return nil, err
	}
	role, err := r.AdminSvc.GetRole(ctx, roleID)
	if err != nil {
		return nil, err
	}
	return toRoleModel(role), nil
}

// DEV ONLY and unauthenticated by design, gated on r.DevMode - after switching to a low-permission persona the switcher still has to render so you can switch back
func (r *queryResolver) DevAdmins(ctx context.Context) ([]*model.AdminUser, error) {
	if !r.DevMode {
		return nil, fmt.Errorf("devAdmins is only available in development")
	}
	users, err := r.AdminSvc.List(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]*model.AdminUser, len(users))
	for i, u := range users {
		result[i] = toAdminUserModel(u)
	}
	return result, nil
}
