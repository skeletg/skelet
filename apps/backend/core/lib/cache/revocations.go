package cache

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// The shared list of killed admin sessions - one key per session rather than a set, because Valkey expires keys for us and the list must not grow forever

type Revocations struct {
	client *redis.Client
	ttl    time.Duration
}

// ttl only has to outlive an access token - past that the token is refused on its own expiry and the entry is dead weight
func NewRevocations(client *redis.Client, ttl time.Duration) *Revocations {
	return &Revocations{client: client, ttl: ttl}
}

func (r *Revocations) key(sessionID uuid.UUID) string {
	return "revoked:session:" + sessionID.String()
}

func (r *Revocations) Revoke(ctx context.Context, sessionID uuid.UUID) {
	if r == nil || r.client == nil {
		return
	}
	if err := r.client.Set(ctx, r.key(sessionID), "1", r.ttl).Err(); err != nil && !errors.Is(err, context.Canceled) {
		// Logged rather than returned: the session row is already gone, so core has revoked it either way
		slog.Warn("revocation publish failed", "session", sessionID, "err", err)
	}
}

// Revoked fails OPEN on an unreachable Valkey, matching AdminSessionGuard - a blip must not log every operator out
func (r *Revocations) Revoked(ctx context.Context, sessionID uuid.UUID) bool {
	if r == nil || r.client == nil {
		return false
	}
	n, err := r.client.Exists(ctx, r.key(sessionID)).Result()
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			slog.Warn("revocation lookup failed", "session", sessionID, "err", err)
		}
		return false
	}
	return n > 0
}
