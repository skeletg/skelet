package admin

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

var ErrRoleInUse = errors.New("role has members; reassign them before deleting")

var ErrRoleSlugTaken = errors.New("role slug already exists")

var ErrEmailTaken = errors.New("an admin with this email already exists")

type AdminUser struct {
	ID                uuid.UUID
	Email             string
	FirstName         string
	LastName          string
	IsSuperuser       bool
	IsActive          bool
	CreatedAt         time.Time
	LastLoginAt       *time.Time   // nil until the first successful login - drives the admins list's "last seen" and the first-run introduction
	PasswordHash      string       // internal - never expose via GraphQL
	Roles             []Role       // role assignments
	Permissions       []Permission // effective union across Roles
	TotpEnabled       bool         // two-factor state below is encrypted via internal/secrets and NEVER exposed through GraphQL
	TotpSecret        string       // encrypted-at-rest blob (internal)
	TotpPendingSecret string       // encrypted-at-rest blob (internal)
	TotpRecoveryCodes []string     // bcrypt hashes (internal)
}

type Permission struct {
	ID   uuid.UUID
	Slug string
	Name string
}

// One (slug, channel?, locale?) scope an admin holds through their roles, aliased to the shared model in auth so a token-carried grant set answers the same checks
type PermissionGrant = auth.Grant

type PermissionGroup struct {
	ID          uuid.UUID
	Name        string
	Slug        string
	Permissions []Permission
}

// A named bundle of permission grants, each one a (permission, channel?, locale?) tuple where NULL channel or locale means "any" - an admin's effective access is the union of every role they hold
type Role struct {
	ID               uuid.UUID
	Name             string
	Slug             string
	Description      string
	CreatedAt        time.Time
	PermissionScopes []RolePermissionScope // populated by GetRole* / CreateRole / UpdateRole
	MemberCount      int64                 // populated by ListRoles; zero elsewhere
}

type RolePermissionScope struct { // one grant on a role - nil ChannelID applies to every channel, nil Locale to every locale
	Permission Permission
	ChannelID  *uuid.UUID
	Locale     *string
}

type RolePermissionScopeInput struct { // write-shape of a grant - same semantics as RolePermissionScope, slug instead of a hydrated Permission row
	PermissionSlug string
	ChannelID      *uuid.UUID
	Locale         *string
}

type CreateAdminUserInput struct {
	Email        string
	PasswordHash string
	FirstName    string
	LastName     string
	IsSuperuser  bool
	RoleSlugs    []string // role slugs to assign on creation
}

type UpdateAdminUserInput struct {
	FirstName string
	LastName  string
	IsActive  *bool    // pointer so callers can tell "not provided" from "set to false"
	RoleSlugs []string // nil leaves roles alone, [] removes them all
}

type CreateRoleInput struct {
	Name        string
	Slug        string
	Description string
	Scopes      []RolePermissionScopeInput // the service rejects an empty slice - a role granting nothing is unusable
}

type UpdateRoleInput struct {
	Name         *string // pointers distinguish "not provided" from "set to empty"
	Description  *string
	Scopes       []RolePermissionScopeInput
	UpdateScopes bool // an empty Scopes could mean either, so this separates "leave alone" from "wipe and re-insert"
}

type Repository interface { // implementations must be safe for concurrent use
	Create(ctx context.Context, input CreateAdminUserInput) (AdminUser, error)
	GetByEmail(ctx context.Context, email string) (AdminUser, error)
	GetByID(ctx context.Context, id uuid.UUID) (AdminUser, error)
	GetByIDWithRoles(ctx context.Context, id uuid.UUID) (AdminUser, error)
	List(ctx context.Context) ([]AdminUser, error)
	Update(ctx context.Context, id uuid.UUID, input UpdateAdminUserInput) (AdminUser, error)
	Delete(ctx context.Context, id uuid.UUID) error
	UpdatePassword(ctx context.Context, id uuid.UUID, passwordHash string) error
	// Stamps this login and returns the previous value (nil means this was the account's first) plus
	// the stamp it just wrote
	RecordLogin(ctx context.Context, id uuid.UUID) (previous *time.Time, stamped time.Time, err error)

	SetTotpPending(ctx context.Context, id uuid.UUID, encryptedSecret string) error
	EnableTotp(ctx context.Context, id uuid.UUID, recoveryHashes []byte) error
	DisableTotp(ctx context.Context, id uuid.UUID) error
	SetTotpRecoveryCodes(ctx context.Context, id uuid.UUID, recoveryHashes []byte) error

	GetPermissions(ctx context.Context, adminID uuid.UUID) ([]Permission, error)
	ListAllGrants(ctx context.Context, adminID uuid.UUID) ([]PermissionGrant, error)
	ListPermissionGroups(ctx context.Context) ([]PermissionGroup, error)

	GetRoles(ctx context.Context, adminID uuid.UUID) ([]Role, error)
	SetRoles(ctx context.Context, adminID uuid.UUID, slugs []string) error

	CreateRole(ctx context.Context, input CreateRoleInput) (Role, error)
	UpdateRole(ctx context.Context, id uuid.UUID, input UpdateRoleInput) (Role, error)
	DeleteRole(ctx context.Context, id uuid.UUID) error
	GetRoleByID(ctx context.Context, id uuid.UUID) (Role, error)
	GetRoleBySlug(ctx context.Context, slug string) (Role, error)
	ListRoles(ctx context.Context) ([]Role, error)
}
