package auth

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// AdminSession is one signed-in device, as the session log keeps it
type AdminSession struct {
	ID           uuid.UUID
	AdminID      uuid.UUID
	TokenHash    string
	IPAddress    string // empty when the client address was not recorded
	UserAgent    string
	CreatedAt    time.Time
	LastActiveAt time.Time
}

// SessionLog keeps one row per signed-in admin device - core's is the admin_sessions table, nil for the customer store
// Every delete answers the ids it removed, so the revocation list can name them
type SessionLog interface {
	Create(ctx context.Context, adminID uuid.UUID, tokenHash, ip, userAgent string) (uuid.UUID, error)
	Delete(ctx context.Context, tokenHash string) (killed []uuid.UUID, err error)
	DeleteAll(ctx context.Context, adminID uuid.UUID) (killed []uuid.UUID, err error)
	DeleteExcept(ctx context.Context, adminID uuid.UUID, keepHash string) (killed []uuid.UUID, err error)
	List(ctx context.Context, adminID uuid.UUID) ([]AdminSession, error)
	Exists(ctx context.Context, adminID, sessionID uuid.UUID) (bool, error) // the per-request guard's lookup
}

// Event is what the refresh store publishes - the lifecycle events in events.go
type Event interface {
	Type() string
}

// Publisher carries those events to whoever listens - optional, without one nothing hears a logout
type Publisher interface {
	Publish(ctx context.Context, e Event)
}
