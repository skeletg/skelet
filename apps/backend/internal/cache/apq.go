package cache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// The per-process LRU gave every replica its own hash registry, so a client registering a hash on one pod got PersistedQueryNotFound on the next - fail-open, so any Valkey error misses and gqlgen falls back to full queries
type APQ struct { // Valkey-backed graphql.Cache[string] for gqlgen's automatic persisted queries
	client *redis.Client
	prefix string
	ttl    time.Duration
}

func NewAPQ(client *redis.Client, prefix string, ttl time.Duration) *APQ {
	return &APQ{client: client, prefix: prefix, ttl: ttl}
}

func (a *APQ) Get(ctx context.Context, key string) (string, bool) {
	if a.client == nil {
		return "", false
	}
	v, err := a.client.Get(ctx, a.prefix+":"+key).Result()
	if err != nil {
		return "", false
	}
	return v, true
}

func (a *APQ) Add(ctx context.Context, key string, value string) {
	if a.client == nil {
		return
	}
	_ = a.client.Set(ctx, a.prefix+":"+key, value, a.ttl).Err()
}
