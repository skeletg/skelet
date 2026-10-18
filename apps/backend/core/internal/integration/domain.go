// Package integration manages API integrations - machine credentials that authenticate external applications against the admin GraphQL API via the X-Api-Token header, each with a name, an enable toggle, optional expiry and a permission set drawn from the same catalog admin roles use
// A token is optionally pinned to ONE channel - unpinned grants hold globally, pinned ones only satisfy checks scoped to that channel, so a feed token for one storefront can't read the others
package integration

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
)

type Integration struct {
	ID          uuid.UUID
	Name        string
	Description string
	IsActive    bool
	CreatedAt   time.Time
	ExpiresAt   sql.NullTime
	// nil = global token; set = every grant only counts on this channel
	ChannelID   *uuid.UUID
	Permissions []Permission
}

type Permission struct {
	ID   uuid.UUID
	Slug string
	Name string
}

type CreateIntegrationInput struct {
	Name        string
	Description string
	Permissions []string     // permission slugs
	ExpiresAt   sql.NullTime // optional expiry - zero-value Valid=false means no expiry
	ChannelID   *uuid.UUID   // optional channel pin - nil grants globally
}

// Pointer fields mean no change when nil, set otherwise, and the Permission slice follows the same convention with nil for no change and an empty slice to clear all
type UpdateIntegrationInput struct {
	ID              uuid.UUID
	Name            *string
	Description     *string
	PermissionSlugs []string
	ExpiresAt       *time.Time
	ClearExpiresAt  bool
	// SetChannel distinguishes "leave the pin alone" from "set it to ChannelID" (nil ChannelID with SetChannel = unpin to global)
	ChannelID  *uuid.UUID
	SetChannel bool
}

// The newly created integration plus the raw token string - RawToken is shown once and never stored, so callers must return it immediately
type CreateIntegrationResult struct {
	RawToken    string // e.g. "sk_a3f2c1b4..." - show once
	Integration Integration
}

type Repository interface {
	Create(ctx context.Context, input CreateIntegrationInput, tokenHash string) (Integration, error)
	GetByID(ctx context.Context, id uuid.UUID) (Integration, error)
	GetByHash(ctx context.Context, tokenHash string) (Integration, error)
	List(ctx context.Context) ([]Integration, error)
	Update(ctx context.Context, input UpdateIntegrationInput) (Integration, error)
	SetActive(ctx context.Context, id uuid.UUID, active bool) (Integration, error)
	// Rotates the token hash to a new value - the caller generates the raw and hash pair
	RegenerateTokenHash(ctx context.Context, id uuid.UUID, newHash string) (Integration, error)
	Delete(ctx context.Context, id uuid.UUID) error
	GetPermissions(ctx context.Context, integrationID uuid.UUID) ([]Permission, error)
}
