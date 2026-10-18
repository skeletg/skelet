package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// Email verification and password reset are the same primitive - a namespaced token hash mapping to a subject id, expiring after a TTL and redeemed once - so the two stores share one implementation per storage engine instead of four
// Valkey is the default, since TTLs are free and the tokens are throwaway state - Postgres exists so a deployment can run without Valkey at all, these being the only auth state with nowhere else to live and losing them means nobody can verify an email or reset a password
// Keys are "<namespace>:<hash>", and DeleteSubject drops every token one subject holds in a namespace
type TokenBackend interface {
	Put(ctx context.Context, key string, t Token, ttl time.Duration) error
	Get(ctx context.Context, key string) (Token, error)     // else ErrTokenNotFound - both backends treat expiry as not-found
	Consume(ctx context.Context, key string) (Token, error) // Get and Delete in one step, so two redemptions of one token can't both succeed
	Delete(ctx context.Context, key string) error
	DeleteSubject(ctx context.Context, namespace string, subjectID uuid.UUID) error
}

// Binding is an opaque digest of what the token was minted for, empty when nothing is bound
type Token struct {
	Subject uuid.UUID
	Binding string
}

var ErrTokenNotFound = errors.New("auth: token not found or expired") // lets callers tell "bad link" from "storage is broken" and only show the friendly message for the former

type redisTokenBackend struct{ client *redis.Client }

func NewRedisTokenBackend(client *redis.Client) TokenBackend {
	return redisTokenBackend{client: client}
}

// The per-subject index is a set of live keys - one TTL per namespace, so refreshing it on every Put always outlives its members
func tokenNamespace(key string) string {
	if i := strings.LastIndexByte(key, ':'); i > 0 {
		return key[:i]
	}
	return key
}

func subjectIndexKey(namespace string, subjectID uuid.UUID) string {
	return namespace + ":subject:" + subjectID.String()
}

func (b redisTokenBackend) Put(ctx context.Context, key string, t Token, ttl time.Duration) error {
	val := t.Subject.String()
	if t.Binding != "" {
		val += "|" + t.Binding
	}
	idx := subjectIndexKey(tokenNamespace(key), t.Subject)
	_, err := b.client.TxPipelined(ctx, func(p redis.Pipeliner) error {
		p.Set(ctx, key, val, ttl)
		p.SAdd(ctx, idx, key)
		p.Expire(ctx, idx, ttl)
		return nil
	})
	return err
}

func (b redisTokenBackend) Get(ctx context.Context, key string) (Token, error) {
	return parseToken(b.client.Get(ctx, key).Result())
}

func (b redisTokenBackend) Consume(ctx context.Context, key string) (Token, error) {
	return parseToken(b.client.GetDel(ctx, key).Result())
}

func parseToken(val string, err error) (Token, error) {
	if errors.Is(err, redis.Nil) {
		return Token{}, ErrTokenNotFound
	}
	if err != nil {
		return Token{}, err
	}
	subject, binding, _ := strings.Cut(val, "|")
	id, err := uuid.Parse(subject)
	if err != nil {
		return Token{}, fmt.Errorf("parse id: %w", err)
	}
	return Token{Subject: id, Binding: binding}, nil
}

func (b redisTokenBackend) Delete(ctx context.Context, key string) error {
	return b.client.Del(ctx, key).Err()
}

func (b redisTokenBackend) DeleteSubject(ctx context.Context, namespace string, subjectID uuid.UUID) error {
	idx := subjectIndexKey(namespace, subjectID)
	keys, err := b.client.SMembers(ctx, idx).Result()
	if err != nil {
		return err
	}
	return b.client.Del(ctx, append(keys, idx)...).Err()
}
