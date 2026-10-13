// Package adminprefs stores per-admin UI preferences - opaque key/value blobs the dashboard owns
// (table layouts, saved list views, nav pins, home layout), always scoped to the acting admin
package adminprefs

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Preference struct {
	Key       string
	Value     json.RawMessage
	UpdatedAt time.Time
}

type Repository interface {
	List(ctx context.Context, adminID uuid.UUID) ([]Preference, error)
	// Get returns a nil value when the key is unset, not an error - an absent preference is normal
	Get(ctx context.Context, adminID uuid.UUID, key string) (json.RawMessage, error)
	Set(ctx context.Context, adminID uuid.UUID, key string, value json.RawMessage) error
	Delete(ctx context.Context, adminID uuid.UUID, key string) error
	CountOthers(ctx context.Context, adminID uuid.UUID, key string) (int64, error) // keys held besides this one
}
