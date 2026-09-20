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

// The storage primitive under RefreshStore - the security protocol (single-use rotation, grace window, reuse tripwire, events, admin_sessions sync) stays in refresh.go and is identical for every backend, since one copy of the rotation logic is one place for an auth bug to hide rather than two; the owner index is the backend's job (Valkey keeps a SET per owner, Postgres gets it free from an index) and every backend owes two atomicity rules: Consume has at most one winner per hash, and Rotate has at most one winner with everyone else seeing grace
type RefreshBackend interface {
	Put(ctx context.Context, hash string, owner uuid.UUID, ttl time.Duration) error
	Get(ctx context.Context, hash string) (uuid.UUID, error) // a LIVE token only, not expired and not rotated, else ErrTokenNotFound
	Consume(ctx context.Context, hash string) (uuid.UUID, error)
	Rotate(ctx context.Context, oldHash, newHash, newRaw string, ttl, grace time.Duration) (RotateResult, error) // newRaw is the successor grace callers get handed back
	MarkRotated(ctx context.Context, hash string, owner uuid.UUID, window time.Duration) error                   // remembers the rotation for `window`, so a later replay reads as reuse
	RotatedOwner(ctx context.Context, hash string) (uuid.UUID, bool)                                             // the tripwire lookup - the owner of a token rotated within the reuse window
	Delete(ctx context.Context, hash string) error
	DeleteOwner(ctx context.Context, owner uuid.UUID) error
	DeleteOwnerExcept(ctx context.Context, owner uuid.UUID, keepHash string) error
}

type RotateOutcome int

const (
	RotateInvalid RotateOutcome = iota // neither live nor in grace, so check the tripwire before treating it as a bad token
	RotateWon
	RotateGrace
)

type RotateResult struct {
	Outcome   RotateOutcome
	Owner     uuid.UUID
	Successor string // RotateGrace only - the raw successor token to hand back
}

// Runs server-side so check-consume-mint can't interleave between concurrent callers - KEYS are the old token key, its grace key and the new token key; ARGV are the refresh TTL, the new raw token as grace payload, and the grace TTL; it returns {"grace", "<newToken>|<owner>"} when already rotated in the window, {"won", "<owner>"} for the caller that does the rotation, or {"invalid"} when unknown or expired
const rotateScript = `
local grace = redis.call('GET', KEYS[2])
if grace then return {'grace', grace} end
local owner = redis.call('GET', KEYS[1])
if not owner then return {'invalid'} end
redis.call('DEL', KEYS[1])
redis.call('SET', KEYS[3], owner, 'EX', ARGV[1])
redis.call('SET', KEYS[2], ARGV[2] .. '|' .. owner, 'EX', ARGV[3])
return {'won', owner}
`

type redisRefreshBackend struct { // one key per token, a SET per owner as the reverse index, and separate grace and used marker keys
	client *redis.Client
	prefix string
}

func NewRedisRefreshBackend(client *redis.Client, prefix string) RefreshBackend {
	return redisRefreshBackend{client: client, prefix: prefix}
}

func (b redisRefreshBackend) tokenKey(hash string) string {
	return "refresh:" + b.prefix + ":" + hash
}
func (b redisRefreshBackend) graceKey(hash string) string {
	return "refresh:" + b.prefix + ":grace:" + hash
}
func (b redisRefreshBackend) usedKey(hash string) string {
	return "refresh:" + b.prefix + ":used:" + hash
}
func (b redisRefreshBackend) indexKey(owner uuid.UUID) string {
	return "refresh:" + b.prefix + ":idx:" + owner.String()
}

func (b redisRefreshBackend) Put(ctx context.Context, hash string, owner uuid.UUID, ttl time.Duration) error {
	if err := b.client.Set(ctx, b.tokenKey(hash), owner.String(), ttl).Err(); err != nil {
		return fmt.Errorf("store refresh token: %w", err)
	}
	idxKey := b.indexKey(owner)
	if err := b.client.SAdd(ctx, idxKey, hash).Err(); err != nil {
		return fmt.Errorf("index refresh token: %w", err)
	}
	if err := b.client.Expire(ctx, idxKey, ttl).Err(); err != nil {
		return fmt.Errorf("expire refresh index: %w", err)
	}
	return nil
}

func (b redisRefreshBackend) Get(ctx context.Context, hash string) (uuid.UUID, error) {
	val, err := b.client.Get(ctx, b.tokenKey(hash)).Result()
	if errors.Is(err, redis.Nil) {
		return uuid.Nil, ErrTokenNotFound
	}
	if err != nil {
		return uuid.Nil, err
	}
	return uuid.Parse(val)
}

func (b redisRefreshBackend) Consume(ctx context.Context, hash string) (uuid.UUID, error) {
	val, err := b.client.GetDel(ctx, b.tokenKey(hash)).Result()
	if errors.Is(err, redis.Nil) {
		return uuid.Nil, ErrTokenNotFound
	}
	if err != nil {
		return uuid.Nil, err
	}
	owner, err := uuid.Parse(val)
	if err != nil {
		return uuid.Nil, err
	}
	_ = b.client.SRem(ctx, b.indexKey(owner), hash).Err() // best-effort de-index - the token key is already gone, so a stale index member is harmless and a later DEL on a missing key no-ops
	return owner, nil
}

func (b redisRefreshBackend) Rotate(ctx context.Context, oldHash, newHash, newRaw string, ttl, grace time.Duration) (RotateResult, error) {
	res, err := b.client.Eval(ctx, rotateScript,
		[]string{b.tokenKey(oldHash), b.graceKey(oldHash), b.tokenKey(newHash)},
		int(ttl.Seconds()), newRaw, int(grace.Seconds()),
	).Result()
	if err != nil {
		return RotateResult{}, fmt.Errorf("rotate refresh token: %w", err)
	}
	arr, ok := res.([]any)
	if !ok || len(arr) == 0 {
		return RotateResult{Outcome: RotateInvalid}, nil
	}
	tag, _ := arr[0].(string)
	switch tag {
	case "won":
		idStr, _ := arr[1].(string)
		owner, err := uuid.Parse(idStr)
		if err != nil {
			return RotateResult{}, fmt.Errorf("parse id: %w", err)
		}
		idxKey := b.indexKey(owner) // indexing the successor and de-indexing the consumed token is best-effort for the same reason as Consume
		_ = b.client.SAdd(ctx, idxKey, newHash).Err()
		_ = b.client.Expire(ctx, idxKey, ttl).Err()
		_ = b.client.SRem(ctx, idxKey, oldHash).Err()
		return RotateResult{Outcome: RotateWon, Owner: owner}, nil
	case "grace":
		payload, _ := arr[1].(string)
		tok, id, found := strings.Cut(payload, "|")
		if !found {
			return RotateResult{Outcome: RotateInvalid}, nil
		}
		owner, err := uuid.Parse(id)
		if err != nil {
			return RotateResult{}, fmt.Errorf("parse id: %w", err)
		}
		return RotateResult{Outcome: RotateGrace, Owner: owner, Successor: tok}, nil
	default:
		return RotateResult{Outcome: RotateInvalid}, nil
	}
}

func (b redisRefreshBackend) MarkRotated(ctx context.Context, hash string, owner uuid.UUID, window time.Duration) error {
	return b.client.Set(ctx, b.usedKey(hash), owner.String(), window).Err()
}

func (b redisRefreshBackend) RotatedOwner(ctx context.Context, hash string) (uuid.UUID, bool) {
	val, err := b.client.Get(ctx, b.usedKey(hash)).Result()
	if err != nil {
		return uuid.Nil, false
	}
	owner, err := uuid.Parse(val)
	if err != nil {
		return uuid.Nil, false
	}
	return owner, true
}

func (b redisRefreshBackend) Delete(ctx context.Context, hash string) error {
	owner, err := b.Get(ctx, hash) // read the owner first so the index member can go too - a missing key isn't an error, since revoking an already-expired token is a no-op
	if err != nil && !errors.Is(err, ErrTokenNotFound) {
		return fmt.Errorf("lookup refresh token: %w", err)
	}
	if err := b.client.Del(ctx, b.tokenKey(hash)).Err(); err != nil {
		return fmt.Errorf("revoke refresh token: %w", err)
	}
	if owner != uuid.Nil {
		if err := b.client.SRem(ctx, b.indexKey(owner), hash).Err(); err != nil {
			return fmt.Errorf("deindex refresh token: %w", err)
		}
	}
	return nil
}

func (b redisRefreshBackend) DeleteOwner(ctx context.Context, owner uuid.UUID) error {
	idxKey := b.indexKey(owner)
	hashes, err := b.client.SMembers(ctx, idxKey).Result()
	if err != nil {
		return fmt.Errorf("read refresh index: %w", err)
	}
	if len(hashes) > 0 {
		keys := make([]string, len(hashes))
		for i, h := range hashes {
			keys[i] = b.tokenKey(h)
		}
		if err := b.client.Del(ctx, keys...).Err(); err != nil {
			return fmt.Errorf("revoke refresh tokens: %w", err)
		}
	}
	if err := b.client.Del(ctx, idxKey).Err(); err != nil {
		return fmt.Errorf("clear refresh index: %w", err)
	}
	return nil
}

func (b redisRefreshBackend) DeleteOwnerExcept(ctx context.Context, owner uuid.UUID, keepHash string) error {
	idxKey := b.indexKey(owner)
	hashes, err := b.client.SMembers(ctx, idxKey).Result()
	if err != nil {
		return fmt.Errorf("read refresh index: %w", err)
	}
	var keys, drop []string
	for _, h := range hashes {
		if h == keepHash {
			continue
		}
		keys = append(keys, b.tokenKey(h))
		drop = append(drop, h)
	}
	if len(keys) > 0 {
		if err := b.client.Del(ctx, keys...).Err(); err != nil {
			return fmt.Errorf("revoke refresh tokens: %w", err)
		}
		if err := b.client.SRem(ctx, idxKey, toAny(drop)...).Err(); err != nil {
			return fmt.Errorf("deindex refresh tokens: %w", err)
		}
	}
	return nil
}

func toAny(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}
