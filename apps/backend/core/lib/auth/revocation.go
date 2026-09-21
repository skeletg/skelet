package auth

import (
	"context"

	"github.com/google/uuid"
)

// A service authorizing from a token cannot see admin_sessions, so core publishes the killed id and the service checks that list per request

// Revocations is the shared list of sessions killed since their tokens were minted, and entries only have to outlive a token so an implementation is expected to expire them
type Revocations interface {
	Revoke(ctx context.Context, sessionID uuid.UUID)
	Revoked(ctx context.Context, sessionID uuid.UUID) bool
}
