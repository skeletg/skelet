package authstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

type pgRefreshBackend struct { // auth_refresh_tokens - rotation atomicity comes from a conditional UPDATE rather than a lock, since only one caller can match rotated_at IS NULL
	q     *db.Queries
	scope string
}

func NewPostgresRefreshBackend(q *db.Queries, scope string) auth.RefreshBackend {
	return pgRefreshBackend{q: q, scope: scope}
}

func (b pgRefreshBackend) Put(ctx context.Context, hash string, owner uuid.UUID, ttl time.Duration) error {
	return b.q.PutRefreshToken(ctx, db.PutRefreshTokenParams{
		TokenHash: hash,
		Scope:     b.scope,
		OwnerID:   owner,
		ExpiresAt: time.Now().Add(ttl),
	})
}

func (b pgRefreshBackend) Get(ctx context.Context, hash string) (uuid.UUID, error) {
	owner, err := b.q.GetLiveRefreshToken(ctx, db.GetLiveRefreshTokenParams{TokenHash: hash, Scope: b.scope})
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, auth.ErrTokenNotFound
	}
	return owner, err
}

func (b pgRefreshBackend) Consume(ctx context.Context, hash string) (uuid.UUID, error) {
	owner, err := b.q.ConsumeRefreshToken(ctx, db.ConsumeRefreshTokenParams{TokenHash: hash, Scope: b.scope})
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, auth.ErrTokenNotFound
	}
	return owner, err
}

func (b pgRefreshBackend) Rotate(ctx context.Context, oldHash, newHash, newRaw string, ttl, grace time.Duration) (auth.RotateResult, error) {
	graceUntil := time.Now().Add(grace)
	owner, err := b.q.ClaimRefreshTokenForRotate(ctx, db.ClaimRefreshTokenForRotateParams{
		TokenHash:      oldHash,
		Scope:          b.scope,
		SuccessorToken: sql.NullString{String: newRaw, Valid: true},
		GraceExpiresAt: sql.NullTime{Time: graceUntil, Valid: true},
	})
	if err == nil {
		if perr := b.Put(ctx, newHash, owner, ttl); perr != nil { // we won the claim, so mint the successor
			return auth.RotateResult{}, perr
		}
		return auth.RotateResult{Outcome: auth.RotateWon, Owner: owner}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return auth.RotateResult{}, fmt.Errorf("rotate refresh token: %w", err)
	}

	row, err := b.q.GetRotatedRefreshToken(ctx, db.GetRotatedRefreshTokenParams{TokenHash: oldHash, Scope: b.scope}) // lost the claim - either someone rotated it moments ago and it's in grace, or it's a replay of a long-consumed token, or it never existed
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return auth.RotateResult{Outcome: auth.RotateInvalid}, nil
		}
		return auth.RotateResult{}, fmt.Errorf("read rotated refresh token: %w", err)
	}
	if row.SuccessorToken.Valid && row.GraceExpiresAt.Valid && row.GraceExpiresAt.Time.After(time.Now()) {
		return auth.RotateResult{Outcome: auth.RotateGrace, Owner: row.OwnerID, Successor: row.SuccessorToken.String}, nil
	}
	return auth.RotateResult{Outcome: auth.RotateInvalid}, nil
}

func (b pgRefreshBackend) MarkRotated(context.Context, string, uuid.UUID, time.Duration) error { // ClaimRefreshTokenForRotate already stamped rotated_at, which IS the marker - this is on the interface only because the Valkey layout needs a separate key
	return nil
}

func (b pgRefreshBackend) RotatedOwner(ctx context.Context, hash string) (uuid.UUID, bool) {
	row, err := b.q.GetRotatedRefreshToken(ctx, db.GetRotatedRefreshTokenParams{TokenHash: hash, Scope: b.scope})
	if err != nil || !row.RotatedAt.Valid {
		return uuid.Nil, false
	}
	return row.OwnerID, true // the prune job drops rows once the reuse window lapses, so a surviving rotated row is inside it by definition
}

func (b pgRefreshBackend) Delete(ctx context.Context, hash string) error {
	return b.q.DeleteRefreshToken(ctx, db.DeleteRefreshTokenParams{TokenHash: hash, Scope: b.scope})
}

func (b pgRefreshBackend) DeleteOwner(ctx context.Context, owner uuid.UUID) error {
	return b.q.DeleteRefreshTokensByOwner(ctx, db.DeleteRefreshTokensByOwnerParams{Scope: b.scope, OwnerID: owner})
}

func (b pgRefreshBackend) DeleteOwnerExcept(ctx context.Context, owner uuid.UUID, keepHash string) error {
	return b.q.DeleteRefreshTokensByOwnerExcept(ctx, db.DeleteRefreshTokensByOwnerExceptParams{
		Scope:     b.scope,
		OwnerID:   owner,
		TokenHash: keepHash,
	})
}

func NewRefreshBackend(client *redis.Client, q *db.Queries, prefix string) auth.RefreshBackend { // Valkey when a client is wired, Postgres otherwise
	if client != nil {
		return auth.NewRedisRefreshBackend(client, prefix)
	}
	return NewPostgresRefreshBackend(q, prefix)
}
