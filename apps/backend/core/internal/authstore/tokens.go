// Package authstore is core's persistence behind lib/auth - the Postgres token and refresh backends and the admin session log
package authstore

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

type pgTokenBackend struct{ q *db.Queries }

func NewPostgresTokenBackend(q *db.Queries) auth.TokenBackend { // auth_tokens, same key format as Valkey so links already sent by email survive a backend switch
	return pgTokenBackend{q: q}
}

func (b pgTokenBackend) Put(ctx context.Context, key string, id uuid.UUID, ttl time.Duration) error {
	return b.q.PutAuthToken(ctx, db.PutAuthTokenParams{
		TokenKey:  key,
		SubjectID: id,
		ExpiresAt: time.Now().Add(ttl),
	})
}

func (b pgTokenBackend) Get(ctx context.Context, key string) (uuid.UUID, error) {
	return tokenSubject(b.q.GetAuthToken(ctx, key)) // the query filters expires_at > now(), so an expired row reads as no rows
}

func (b pgTokenBackend) Consume(ctx context.Context, key string) (uuid.UUID, error) {
	return tokenSubject(b.q.ConsumeAuthToken(ctx, key))
}

func tokenSubject(id uuid.UUID, err error) (uuid.UUID, error) {
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return uuid.Nil, auth.ErrTokenNotFound
		}
		return uuid.Nil, err
	}
	return id, nil
}

func (b pgTokenBackend) Delete(ctx context.Context, key string) error {
	return b.q.DeleteAuthToken(ctx, key)
}

func NewTokenBackend(client *redis.Client, q *db.Queries) auth.TokenBackend { // Valkey when a client is wired, Postgres otherwise
	if client != nil {
		return auth.NewRedisTokenBackend(client)
	}
	return NewPostgresTokenBackend(q)
}
