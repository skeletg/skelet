package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// Email verification and password reset are the same primitive - a namespaced token hash mapping to a subject id, expiring after a TTL and redeemed once - so the two stores share one implementation per storage engine instead of four
// Valkey is the default, since TTLs are free and the tokens are throwaway state - Postgres exists so a deployment can run without Valkey at all, these being the only auth state with nowhere else to live and losing them means nobody can verify an email or reset a password
type TokenBackend interface {
	Put(ctx context.Context, key string, subjectID uuid.UUID, ttl time.Duration) error
	Get(ctx context.Context, key string) (uuid.UUID, error) // the subject id, else ErrTokenNotFound - both backends treat expiry as not-found
	Delete(ctx context.Context, key string) error
}

var ErrTokenNotFound = errors.New("auth: token not found or expired") // lets callers tell "bad link" from "storage is broken" and only show the friendly message for the former

type redisTokenBackend struct{ client *redis.Client }

func NewRedisTokenBackend(client *redis.Client) TokenBackend {
	return redisTokenBackend{client: client}
}

func (b redisTokenBackend) Put(ctx context.Context, key string, id uuid.UUID, ttl time.Duration) error {
	return b.client.Set(ctx, key, id.String(), ttl).Err()
}

func (b redisTokenBackend) Get(ctx context.Context, key string) (uuid.UUID, error) {
	val, err := b.client.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return uuid.Nil, ErrTokenNotFound
	}
	if err != nil {
		return uuid.Nil, err
	}
	id, err := uuid.Parse(val)
	if err != nil {
		return uuid.Nil, fmt.Errorf("parse id: %w", err)
	}
	return id, nil
}

func (b redisTokenBackend) Delete(ctx context.Context, key string) error {
	return b.client.Del(ctx, key).Err()
}
