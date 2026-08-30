// Package config is a runtime-configurable key-value store - scoped paths, typed values, a code-defined schema registry, and a public subset exposed to unauthenticated callers
package config

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrUnknownPath = errors.New("unknown config path")

	ErrInvalidValue = errors.New("invalid config value")

	ErrInvalidScope = errors.New("invalid config scope")

	// Returned when a stored value is encrypted but no key is configured to decrypt it - a misconfiguration, e.g - CONFIG_ENCRYPTION_KEY missing or rotated away
	ErrSecretCipherUnavailable = errors.New("config: encrypted secret but no encryption key configured")
)

const (
	ScopeDefault = "default"
	ScopeChannel = "channel"
)

type Scope struct {
	Kind      string
	ChannelID *uuid.UUID
	Locale    *string
}

var DefaultScope = Scope{Kind: ScopeDefault}

// The resolver- and UI-facing view of a config key - the Registry Definition plus either the stored Value or the Registry default, with IsDefault saying which
type Entry struct {
	Definition Definition
	Value      string
	IsDefault  bool
	Scope      Scope     // the scope where Value came from; DefaultScope when IsDefault
	UpdatedAt  time.Time // zero when IsDefault
}

type StoredValue struct {
	Scope     Scope
	Path      string
	Value     string
	UpdatedAt time.Time
}

type Repository interface {
	Upsert(ctx context.Context, scope Scope, path, value string) (StoredValue, error)
	Delete(ctx context.Context, scope Scope, path string) error
	// Every row that could satisfy this scope - the exact scope, the channel-wide row when a locale was asked for, and the
	// default row - The service picks among them, so the ladder can't drift between the SQL and a test double
	GetCandidates(ctx context.Context, scope Scope, path string) ([]StoredValue, error)
	ListInScopes(ctx context.Context, scope Scope) ([]StoredValue, error)
	ListAll(ctx context.Context) ([]StoredValue, error)
}
