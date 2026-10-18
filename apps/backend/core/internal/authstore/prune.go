package authstore

import (
	"context"
	"fmt"
	"time"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

type PruneCounts struct {
	AuthTokens, RefreshTokens, Successors, Sessions int64
}

func (c PruneCounts) Total() int64 { return c.AuthTokens + c.RefreshTokens + c.Successors + c.Sessions }

// Postgres has no TTL, so the fallback token tables need sweeping, and admin sessions outlive their refresh tokens on every backend
func Prune(ctx context.Context, q *db.Queries, now time.Time) (PruneCounts, error) {
	var c PruneCounts
	var err error
	if c.AuthTokens, err = q.PruneExpiredAuthTokens(ctx); err != nil {
		return c, fmt.Errorf("prune auth tokens: %w", err)
	}
	if c.RefreshTokens, err = q.PruneExpiredRefreshTokens(ctx, now.Add(-auth.ReuseDetectWindow)); err != nil {
		return c, fmt.Errorf("prune refresh tokens: %w", err)
	}
	if c.Successors, err = q.ClearLapsedRefreshSuccessors(ctx); err != nil {
		return c, fmt.Errorf("clear lapsed successors: %w", err)
	}
	if c.Sessions, err = q.PruneStaleAdminSessions(ctx, now.Add(-auth.RefreshTokenTTL)); err != nil {
		return c, fmt.Errorf("prune admin sessions: %w", err)
	}
	return c, nil
}
