package admin

import (
	"encoding/json"

	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/sqlc-dev/pqtype"

	"github.com/google/uuid"

	pgdb "github.com/skeletg/skelet/apps/backend/core/internal/db"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

type postgresRepository struct {
	queries *db.Queries
	tx      *txrunner.Manager
}

func NewRepository(queries *db.Queries, txMgr *txrunner.Manager) Repository { // txrunner.Manager runs the multi-statement mutations (role assignment, role CRUD) atomically
	return &postgresRepository{queries: queries, tx: txMgr}
}

// --- admin users ---

func (r *postgresRepository) Create(ctx context.Context, input CreateAdminUserInput) (AdminUser, error) {
	var created AdminUser
	err := r.tx.InTransaction(ctx, func(t *sql.Tx) error {
		q := r.queries.WithTx(t)
		row, err := q.CreateAdminUser(ctx, db.CreateAdminUserParams{
			Email:        input.Email,
			PasswordHash: input.PasswordHash,
			FirstName:    input.FirstName,
			LastName:     input.LastName,
			IsSuperuser:  input.IsSuperuser,
		})
		if err != nil {
			if isUniqueViolation(err) {
				return ErrEmailTaken
			}
			return fmt.Errorf("create admin user: %w", err)
		}
		created = toAdminUser(row)
		if len(input.RoleSlugs) > 0 {
			if err := setRolesTx(ctx, q, created.ID, input.RoleSlugs); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return AdminUser{}, err
	}
	// Load roles + effective permissions so the caller sees the final state
	created.Roles, err = r.GetRoles(ctx, created.ID)
	if err != nil {
		return AdminUser{}, err
	}
	created.Permissions, err = r.GetPermissions(ctx, created.ID)
	if err != nil {
		return AdminUser{}, err
	}
	return created, nil
}

func (r *postgresRepository) GetByEmail(ctx context.Context, email string) (AdminUser, error) {
	row, err := r.queries.GetAdminUserByEmail(ctx, email)
	if err != nil {
		return AdminUser{}, fmt.Errorf("get admin user by email: %w", err)
	}
	return toAdminUser(row), nil
}

func (r *postgresRepository) GetByID(ctx context.Context, id uuid.UUID) (AdminUser, error) {
	row, err := r.queries.GetAdminUserByID(ctx, id)
	if err != nil {
		return AdminUser{}, fmt.Errorf("get admin user by id: %w", err)
	}
	return toAdminUser(row), nil
}

func (r *postgresRepository) GetByIDWithRoles(ctx context.Context, id uuid.UUID) (AdminUser, error) {
	row, err := r.queries.GetAdminUserByID(ctx, id)
	if err != nil {
		return AdminUser{}, fmt.Errorf("get admin user by id: %w", err)
	}
	user := toAdminUser(row)
	user.Roles, err = r.GetRoles(ctx, id)
	if err != nil {
		return AdminUser{}, err
	}
	user.Permissions, err = r.GetPermissions(ctx, id)
	if err != nil {
		return AdminUser{}, err
	}
	return user, nil
}

func (r *postgresRepository) List(ctx context.Context) ([]AdminUser, error) {
	rows, err := r.queries.ListAdminUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list admin users: %w", err)
	}
	users := make([]AdminUser, len(rows))
	for i, row := range rows {
		users[i] = toAdminUser(row)
		users[i].Roles, err = r.GetRoles(ctx, row.ID)
		if err != nil {
			return nil, err
		}
		users[i].Permissions, err = r.GetPermissions(ctx, row.ID)
		if err != nil {
			return nil, err
		}
	}
	return users, nil
}

func (r *postgresRepository) Update(ctx context.Context, id uuid.UUID, input UpdateAdminUserInput) (AdminUser, error) {
	var updated AdminUser
	err := r.tx.InTransaction(ctx, func(t *sql.Tx) error {
		q := r.queries.WithTx(t)
		row, err := q.UpdateAdminUser(ctx, db.UpdateAdminUserParams{
			ID:        id,
			FirstName: input.FirstName,
			LastName:  input.LastName,
			IsActive:  *input.IsActive,
		})
		if err != nil {
			return fmt.Errorf("update admin user: %w", err)
		}
		updated = toAdminUser(row)
		if input.RoleSlugs != nil {
			if err := setRolesTx(ctx, q, id, input.RoleSlugs); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return AdminUser{}, err
	}
	updated.Roles, err = r.GetRoles(ctx, id)
	if err != nil {
		return AdminUser{}, err
	}
	updated.Permissions, err = r.GetPermissions(ctx, id)
	if err != nil {
		return AdminUser{}, err
	}
	return updated, nil
}

func (r *postgresRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DeleteAdminUser(ctx, id); err != nil {
		return fmt.Errorf("delete admin user: %w", err)
	}
	return nil
}

func (r *postgresRepository) SetTotpPending(ctx context.Context, id uuid.UUID, encryptedSecret string) error {
	if err := r.queries.SetAdminTotpPending(ctx, db.SetAdminTotpPendingParams{
		ID:                id,
		TotpPendingSecret: sql.NullString{String: encryptedSecret, Valid: true},
	}); err != nil {
		return fmt.Errorf("set totp pending: %w", err)
	}
	return nil
}

func (r *postgresRepository) EnableTotp(ctx context.Context, id uuid.UUID, recoveryHashes []byte) error {
	if err := r.queries.EnableAdminTotp(ctx, db.EnableAdminTotpParams{
		ID:                id,
		TotpRecoveryCodes: pqtype.NullRawMessage{RawMessage: recoveryHashes, Valid: true},
	}); err != nil {
		return fmt.Errorf("enable totp: %w", err)
	}
	return nil
}

func (r *postgresRepository) DisableTotp(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DisableAdminTotp(ctx, id); err != nil {
		return fmt.Errorf("disable totp: %w", err)
	}
	return nil
}

func (r *postgresRepository) SetTotpRecoveryCodes(ctx context.Context, id uuid.UUID, recoveryHashes []byte) error {
	if err := r.queries.SetAdminTotpRecoveryCodes(ctx, db.SetAdminTotpRecoveryCodesParams{
		ID:                id,
		TotpRecoveryCodes: pqtype.NullRawMessage{RawMessage: recoveryHashes, Valid: true},
	}); err != nil {
		return fmt.Errorf("set totp recovery codes: %w", err)
	}
	return nil
}

func (r *postgresRepository) UpdatePassword(ctx context.Context, id uuid.UUID, passwordHash string) error {
	if err := r.queries.UpdateAdminUserPassword(ctx, db.UpdateAdminUserPasswordParams{
		ID:           id,
		PasswordHash: passwordHash,
	}); err != nil {
		return fmt.Errorf("update admin user password: %w", err)
	}
	return nil
}

// --- permissions (effective via roles) ---

func (r *postgresRepository) GetPermissions(ctx context.Context, adminID uuid.UUID) ([]Permission, error) {
	rows, err := r.queries.GetAdminUserPermissions(ctx, adminID)
	if err != nil {
		return nil, fmt.Errorf("get admin user permissions: %w", err)
	}
	perms := make([]Permission, len(rows))
	for i, row := range rows {
		perms[i] = Permission{ID: row.ID, Slug: row.Slug, Name: row.Name}
	}
	return perms, nil
}

// Every scope across the admin's roles in ONE query - the per-request grant snapshot answers all the Has*/ListPermitted* checks off this instead of a query per check
func (r *postgresRepository) ListAllGrants(ctx context.Context, adminID uuid.UUID) ([]PermissionGrant, error) {
	rows, err := r.queries.ListAdminUserGrants(ctx, adminID)
	if err != nil {
		return nil, fmt.Errorf("list admin user grants: %w", err)
	}
	out := make([]PermissionGrant, 0, len(rows))
	for _, row := range rows {
		g := PermissionGrant{Slug: row.Slug}
		if row.ChannelID.Valid {
			id := row.ChannelID.UUID
			g.ChannelID = &id
		}
		if row.Locale.Valid {
			loc := row.Locale.String
			g.Locale = &loc
		}
		out = append(out, g)
	}
	return out, nil
}

func (r *postgresRepository) ListPermissionGroups(ctx context.Context) ([]PermissionGroup, error) {
	groups, err := r.queries.ListPermissionGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("list permission groups: %w", err)
	}
	result := make([]PermissionGroup, len(groups))
	for i, g := range groups {
		perms, err := r.queries.ListPermissionsByGroupID(ctx, g.ID)
		if err != nil {
			return nil, fmt.Errorf("list permissions for group %s: %w", g.Slug, err)
		}
		result[i] = PermissionGroup{
			ID:          g.ID,
			Name:        g.Name,
			Slug:        g.Slug,
			Permissions: make([]Permission, len(perms)),
		}
		for j, p := range perms {
			result[i].Permissions[j] = toPermission(p)
		}
	}
	return result, nil
}

// --- role assignments ---

func (r *postgresRepository) GetRoles(ctx context.Context, adminID uuid.UUID) ([]Role, error) {
	rows, err := r.queries.GetAdminUserRoles(ctx, adminID)
	if err != nil {
		return nil, fmt.Errorf("get admin user roles: %w", err)
	}
	roles := make([]Role, len(rows))
	roleIDs := make([]uuid.UUID, len(rows))
	for i, row := range rows {
		roles[i] = toRole(row)
		roleIDs[i] = row.ID
	}
	scopesByRole, err := r.getRolePermissionScopesByRoleIDs(ctx, roleIDs) // one query grouped by role_id, was one per role
	if err != nil {
		return nil, err
	}
	for i := range roles {
		roles[i].PermissionScopes = scopesByRole[roles[i].ID]
	}
	return roles, nil
}

func (r *postgresRepository) SetRoles(ctx context.Context, adminID uuid.UUID, slugs []string) error {
	return r.tx.InTransaction(ctx, func(t *sql.Tx) error {
		return setRolesTx(ctx, r.queries.WithTx(t), adminID, slugs)
	})
}

func setRolesTx(ctx context.Context, q *db.Queries, adminID uuid.UUID, slugs []string) error { // tx-local, shared by Create/Update/SetRoles
	if err := q.DeleteAdminUserRoles(ctx, adminID); err != nil {
		return fmt.Errorf("clear admin user roles: %w", err)
	}
	for _, slug := range slugs {
		role, err := q.GetAdminRoleBySlug(ctx, slug)
		if err != nil {
			return fmt.Errorf("role not found: %s", slug)
		}
		if err := q.AddAdminUserRole(ctx, db.AddAdminUserRoleParams{
			AdminUserID: adminID,
			RoleID:      role.ID,
		}); err != nil {
			return fmt.Errorf("assign role %s: %w", slug, err)
		}
	}
	return nil
}

// --- role CRUD ---

func (r *postgresRepository) CreateRole(ctx context.Context, input CreateRoleInput) (Role, error) {
	var created Role
	err := r.tx.InTransaction(ctx, func(t *sql.Tx) error {
		q := r.queries.WithTx(t)
		row, err := q.CreateAdminRole(ctx, db.CreateAdminRoleParams{
			Name:        input.Name,
			Slug:        input.Slug,
			Description: input.Description,
		})
		if err != nil {
			if isUniqueViolation(err) {
				return ErrRoleSlugTaken
			}
			return fmt.Errorf("create role: %w", err)
		}
		created = toRole(row)
		return setRolePermissionScopesTx(ctx, q, created.ID, input.Scopes)
	})
	if err != nil {
		return Role{}, err
	}
	created.PermissionScopes, err = r.getRolePermissionScopes(ctx, created.ID)
	if err != nil {
		return Role{}, err
	}
	return created, nil
}

func (r *postgresRepository) UpdateRole(ctx context.Context, id uuid.UUID, input UpdateRoleInput) (Role, error) {
	var updated Role
	err := r.tx.InTransaction(ctx, func(t *sql.Tx) error {
		q := r.queries.WithTx(t)
		current, err := q.GetAdminRoleByID(ctx, id)
		if err != nil {
			return fmt.Errorf("role not found: %w", err)
		}
		name := current.Name
		if input.Name != nil {
			name = *input.Name
		}
		description := current.Description
		if input.Description != nil {
			description = *input.Description
		}
		row, err := q.UpdateAdminRole(ctx, db.UpdateAdminRoleParams{
			ID:          id,
			Name:        name,
			Description: description,
		})
		if err != nil {
			return fmt.Errorf("update role: %w", err)
		}
		updated = toRole(row)
		if input.UpdateScopes {
			if err := setRolePermissionScopesTx(ctx, q, id, input.Scopes); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return Role{}, err
	}
	updated.PermissionScopes, err = r.getRolePermissionScopes(ctx, id)
	if err != nil {
		return Role{}, err
	}
	return updated, nil
}

func (r *postgresRepository) RoleMemberIDs(ctx context.Context, roleID uuid.UUID) ([]uuid.UUID, error) {
	ids, err := r.queries.ListRoleMemberIDs(ctx, roleID)
	if err != nil {
		return nil, fmt.Errorf("list role members: %w", err)
	}
	return ids, nil
}

func (r *postgresRepository) DeleteRole(ctx context.Context, id uuid.UUID) error {
	n, err := r.queries.CountRoleMembers(ctx, id)
	if err != nil {
		return fmt.Errorf("count role members: %w", err)
	}
	if n > 0 {
		return ErrRoleInUse
	}
	if err := r.queries.DeleteAdminRole(ctx, id); err != nil {
		return fmt.Errorf("delete role: %w", err)
	}
	return nil
}

func (r *postgresRepository) GetRoleByID(ctx context.Context, id uuid.UUID) (Role, error) {
	row, err := r.queries.GetAdminRoleByID(ctx, id)
	if err != nil {
		return Role{}, fmt.Errorf("role not found")
	}
	role := toRole(row)
	role.PermissionScopes, err = r.getRolePermissionScopes(ctx, id)
	if err != nil {
		return Role{}, err
	}
	return role, nil
}

func (r *postgresRepository) GetRoleBySlug(ctx context.Context, slug string) (Role, error) {
	row, err := r.queries.GetAdminRoleBySlug(ctx, slug)
	if err != nil {
		return Role{}, fmt.Errorf("role not found")
	}
	role := toRole(row)
	role.PermissionScopes, err = r.getRolePermissionScopes(ctx, role.ID)
	if err != nil {
		return Role{}, err
	}
	return role, nil
}

func (r *postgresRepository) ListRoles(ctx context.Context) ([]Role, error) {
	rows, err := r.queries.ListAdminRolesWithMemberCount(ctx)
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	roles := make([]Role, len(rows))
	roleIDs := make([]uuid.UUID, len(rows))
	for i, row := range rows {
		roles[i] = Role{
			ID:          row.ID,
			Name:        row.Name,
			Slug:        row.Slug,
			Description: row.Description,
			CreatedAt:   row.CreatedAt,
			MemberCount: row.MemberCount,
		}
		roleIDs[i] = row.ID
	}
	// One scopes query for the whole list, grouped by role_id (was one per role)
	scopesByRole, err := r.getRolePermissionScopesByRoleIDs(ctx, roleIDs)
	if err != nil {
		return nil, err
	}
	for i := range roles {
		roles[i].PermissionScopes = scopesByRole[roles[i].ID]
	}
	return roles, nil
}

// Batch form of getRolePermissionScopes so list hydration doesn't fire a scopes query per role - field mapping is identical, so grouped values match the single-role helper exactly
func (r *postgresRepository) getRolePermissionScopesByRoleIDs(ctx context.Context, roleIDs []uuid.UUID) (map[uuid.UUID][]RolePermissionScope, error) {
	out := make(map[uuid.UUID][]RolePermissionScope, len(roleIDs))
	if len(roleIDs) == 0 {
		return out, nil
	}
	rows, err := r.queries.GetRolePermissionScopesByRoleIDs(ctx, roleIDs)
	if err != nil {
		return nil, fmt.Errorf("get role permission scopes: %w", err)
	}
	for _, row := range rows {
		scope := RolePermissionScope{
			Permission: Permission{
				ID:   row.PermissionID,
				Slug: row.PermissionSlug,
				Name: row.PermissionName,
			},
		}
		if row.ChannelID.Valid {
			id := row.ChannelID.UUID
			scope.ChannelID = &id
		}
		if row.Locale.Valid {
			s := row.Locale.String
			scope.Locale = &s
		}
		out[row.RoleID] = append(out[row.RoleID], scope)
	}
	return out, nil
}

func (r *postgresRepository) getRolePermissionScopes(ctx context.Context, roleID uuid.UUID) ([]RolePermissionScope, error) { // Permission comes back hydrated so callers render slug + name without a follow-up query
	rows, err := r.queries.GetRolePermissionScopes(ctx, roleID)
	if err != nil {
		return nil, fmt.Errorf("get role permission scopes: %w", err)
	}
	out := make([]RolePermissionScope, len(rows))
	for i, row := range rows {
		scope := RolePermissionScope{
			Permission: Permission{
				ID:   row.PermissionID,
				Slug: row.PermissionSlug,
				Name: row.PermissionName,
			},
		}
		if row.ChannelID.Valid {
			id := row.ChannelID.UUID
			scope.ChannelID = &id
		}
		if row.Locale.Valid {
			s := row.Locale.String
			scope.Locale = &s
		}
		out[i] = scope
	}
	return out, nil
}

// Tx-local delete-then-insert for CreateRole/UpdateRole - each scope resolves slug → permission UUID and lands as one row, and an unknown slug fails the whole transaction
func setRolePermissionScopesTx(ctx context.Context, q *db.Queries, roleID uuid.UUID, scopes []RolePermissionScopeInput) error {
	if err := q.DeleteRolePermissionScopes(ctx, roleID); err != nil {
		return fmt.Errorf("clear role permission scopes: %w", err)
	}
	permCache := make(map[string]uuid.UUID) // tx-scoped so a role granting the same permission at five scopes resolves it once
	for _, scope := range scopes {
		permID, ok := permCache[scope.PermissionSlug]
		if !ok {
			perm, err := q.GetPermissionBySlug(ctx, scope.PermissionSlug)
			if err != nil {
				return fmt.Errorf("permission not found: %s", scope.PermissionSlug)
			}
			permID = perm.ID
			permCache[scope.PermissionSlug] = permID
		}
		channelID := uuid.NullUUID{}
		if scope.ChannelID != nil {
			channelID = uuid.NullUUID{UUID: *scope.ChannelID, Valid: true}
		}
		if err := q.InsertRolePermissionScope(ctx, db.InsertRolePermissionScopeParams{
			RoleID:       roleID,
			PermissionID: permID,
			ChannelID:    channelID,
			Locale:       nullableString(scope.Locale),
		}); err != nil {
			return fmt.Errorf("add role permission scope: %w", err)
		}
	}
	return nil
}

// --- mappers ---

func (r *postgresRepository) RecordLogin(ctx context.Context, id uuid.UUID) (*time.Time, time.Time, error) {
	row, err := r.queries.RecordAdminLogin(ctx, id)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("record admin login: %w", err)
	}
	var prev *time.Time
	if row.PreviousLoginAt.Valid {
		t := row.PreviousLoginAt.Time
		prev = &t
	}
	stamped := time.Now()
	if row.LastLoginAt.Valid {
		stamped = row.LastLoginAt.Time
	}
	return prev, stamped, nil
}

func toAdminUser(row db.AdminUser) AdminUser {
	u := AdminUser{
		ID:           row.ID,
		Email:        row.Email,
		FirstName:    row.FirstName,
		LastName:     row.LastName,
		IsSuperuser:  row.IsSuperuser,
		IsActive:     row.IsActive,
		CreatedAt:    row.CreatedAt,
		PasswordHash: row.PasswordHash,
		TotpEnabled:  row.TotpEnabled,
	}
	if row.LastLoginAt.Valid {
		t := row.LastLoginAt.Time
		u.LastLoginAt = &t
	}
	if row.TotpSecret.Valid {
		u.TotpSecret = row.TotpSecret.String
	}
	if row.TotpPendingSecret.Valid {
		u.TotpPendingSecret = row.TotpPendingSecret.String
	}
	if len(row.TotpRecoveryCodes.RawMessage) > 0 && row.TotpRecoveryCodes.Valid {
		_ = json.Unmarshal(row.TotpRecoveryCodes.RawMessage, &u.TotpRecoveryCodes)
	}
	return u
}

func toPermission(row db.Permission) Permission {
	return Permission{
		ID:   row.ID,
		Slug: row.Slug,
		Name: row.Name,
	}
}

func toRole(row db.AdminRole) Role {
	return Role{
		ID:          row.ID,
		Name:        row.Name,
		Slug:        row.Slug,
		Description: row.Description,
		CreatedAt:   row.CreatedAt,
	}
}

func nullableString(s *string) sql.NullString {
	if s == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *s, Valid: true}
}

func isUniqueViolation(err error) bool { return pgdb.IsUniqueViolation(err) } // shared SQLSTATE-based check - the per-package copies drifted into fragile message matching
