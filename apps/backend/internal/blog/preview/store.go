// Package preview mints short-lived signed tokens authorising admin-side draft previews of blog posts - read access to one post's current draft regardless of published status
// Each token is a Redis key "preview:<ns>:<sha256(token)>" holding JSON claims, with no DB writes - previews are short-lived enough that Redis-only is fine
package preview

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const DefaultTTL = 15 * time.Minute

var ErrInvalid = errors.New("preview: invalid or expired token")

type Claims struct { // PostID is the only mandatory link to a post - the rest is observational metadata for logging and auditing
	PostID    uuid.UUID `json:"post_id"`
	AdminID   uuid.UUID `json:"admin_id"`
	Locale    string    `json:"locale,omitempty"`
	IssuedAt  time.Time `json:"issued_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

type Store struct { // one instance per server process, safe for concurrent use
	client *redis.Client
	ttl    time.Duration
	ns     string // namespaces the Redis key so different preview kinds don't share a keyspace and a token minted for one can't be validated by another - defaults to "blog" so existing callers keep their layout
}

func NewStore(client *redis.Client, ttl time.Duration) *Store { // pinned to the "blog" namespace, so other preview kinds should call NewStoreNS - ttl == 0 falls back to DefaultTTL
	return NewStoreNS(client, ttl, "blog")
}

var ErrUnavailable = errors.New("preview: unavailable (requires VALKEY_URL)") // previews genuinely need a keyed-blob store with a TTL, since the payload is JSON claims rather than an id and doesn't fit the auth_tokens fallback login tokens use - a Valkey-less deployment reports previews unavailable and everything else keeps working

func NewStoreNS(client *redis.Client, ttl time.Duration, ns string) *Store {
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	if ns == "" {
		ns = "blog"
	}
	return &Store{client: client, ttl: ttl, ns: ns}
}

type MintInput struct { // split out so the API stays stable as fields get added, e.g. a channelID for multi-store previews
	PostID  uuid.UUID
	AdminID uuid.UUID
	Locale  string
}

type MintResult struct {
	Token     string
	ExpiresAt time.Time
}

func (s *Store) Mint(ctx context.Context, in MintInput) (MintResult, error) { // the raw token is shown to the admin once and embedded in the iframe URL - only its SHA-256 hash is persisted, so a Redis dump doesn't leak usable tokens
	if s == nil || s.client == nil {
		return MintResult{}, ErrUnavailable
	}
	if in.PostID == uuid.Nil {
		return MintResult{}, fmt.Errorf("preview: post_id is required")
	}
	if in.AdminID == uuid.Nil {
		return MintResult{}, fmt.Errorf("preview: admin_id is required")
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return MintResult{}, fmt.Errorf("preview: generate token: %w", err)
	}
	token := hex.EncodeToString(raw)

	now := time.Now().UTC()
	claims := Claims{
		PostID:    in.PostID,
		AdminID:   in.AdminID,
		Locale:    in.Locale,
		IssuedAt:  now,
		ExpiresAt: now.Add(s.ttl),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return MintResult{}, fmt.Errorf("preview: marshal claims: %w", err)
	}
	if err := s.client.Set(ctx, s.key(token), payload, s.ttl).Err(); err != nil {
		return MintResult{}, fmt.Errorf("preview: store token: %w", err)
	}
	return MintResult{Token: token, ExpiresAt: claims.ExpiresAt}, nil
}

func (s *Store) Validate(ctx context.Context, token string) (Claims, error) { // ErrInvalid covers unknown, expired and malformed alike, so the caller surfaces a 401-equivalent without leaking which condition matched
	if s == nil || s.client == nil {
		return Claims{}, ErrUnavailable
	}
	if token == "" {
		return Claims{}, ErrInvalid
	}
	val, err := s.client.Get(ctx, s.key(token)).Result()
	if errors.Is(err, redis.Nil) {
		return Claims{}, ErrInvalid
	}
	if err != nil {
		return Claims{}, fmt.Errorf("preview: lookup: %w", err)
	}
	var claims Claims
	if err := json.Unmarshal([]byte(val), &claims); err != nil {
		return Claims{}, ErrInvalid
	}
	if time.Now().UTC().After(claims.ExpiresAt) {
		return Claims{}, ErrInvalid // the Redis TTL should have evicted this by now - belt and braces against clock skew between Redis and the app
	}
	return claims, nil
}

func (s *Store) Revoke(ctx context.Context, token string) error { // best-effort - deleting an already-evicted or unknown token returns nil
	if s == nil || s.client == nil {
		return ErrUnavailable
	}
	if err := s.client.Del(ctx, s.key(token)).Err(); err != nil && !errors.Is(err, redis.Nil) {
		return fmt.Errorf("preview: revoke: %w", err)
	}
	return nil
}

func (s *Store) key(token string) string {
	sum := sha256.Sum256([]byte(token))
	ns := s.ns
	if ns == "" {
		ns = "blog"
	}
	return "preview:" + ns + ":" + hex.EncodeToString(sum[:])
}
