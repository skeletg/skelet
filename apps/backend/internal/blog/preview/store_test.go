package preview

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// deadClient points at a closed port, so any command errors - these tests cover everything that must be decided BEFORE Redis answers
func deadClient() *redis.Client {
	return redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: 50 * time.Millisecond, MaxRetries: -1})
}

func TestNilStoreReportsUnavailable(t *testing.T) { // a Valkey-less deployment wires no store, and every entry point must answer "previews are off" rather than panic
	ctx := context.Background()
	var s *Store

	if _, err := s.Mint(ctx, MintInput{PostID: uuid.New(), AdminID: uuid.New()}); !errors.Is(err, ErrUnavailable) {
		t.Errorf("Mint on a nil store: err = %v, want ErrUnavailable", err)
	}
	if _, err := s.Validate(ctx, "tok"); !errors.Is(err, ErrUnavailable) {
		t.Errorf("Validate on a nil store: err = %v, want ErrUnavailable", err)
	}
	if err := s.Revoke(ctx, "tok"); !errors.Is(err, ErrUnavailable) {
		t.Errorf("Revoke on a nil store: err = %v, want ErrUnavailable", err)
	}

	nilClient := NewStore(nil, 0)
	if _, err := nilClient.Mint(ctx, MintInput{PostID: uuid.New(), AdminID: uuid.New()}); !errors.Is(err, ErrUnavailable) {
		t.Errorf("Mint on a store with no client: err = %v, want ErrUnavailable", err)
	}
}

func TestMintRequiresPostAndAdmin(t *testing.T) { // a claims blob with a nil post id authorises nothing meaningful, and a nil admin id breaks the audit trail
	s := NewStore(deadClient(), 0)
	ctx := context.Background()

	// the client is dead, so a dial error here means validation waved the input through to Redis
	if _, err := s.Mint(ctx, MintInput{AdminID: uuid.New()}); err == nil || !strings.Contains(err.Error(), "post_id") {
		t.Errorf("Mint with no post: err = %v, want the post_id validation error", err)
	}
	if _, err := s.Mint(ctx, MintInput{PostID: uuid.New()}); err == nil || !strings.Contains(err.Error(), "admin_id") {
		t.Errorf("Mint with no admin: err = %v, want the admin_id validation error", err)
	}
}

func TestValidateRefusesAnEmptyTokenBeforeRedis(t *testing.T) { // "" is what a missing query param decodes to, and it must be a 401 - not a Redis round-trip, and never a hit on some empty-hash key
	s := NewStore(deadClient(), 0)
	if _, err := s.Validate(context.Background(), ""); !errors.Is(err, ErrInvalid) {
		t.Errorf("err = %v, want ErrInvalid - a dead client answering proves it never reached Redis", err)
	}
}

func TestInfraFailureIsNotA401(t *testing.T) { // a Redis outage must surface as an error the caller 500s on - mapping it to ErrInvalid would tell admins their fresh links are expired
	s := NewStore(deadClient(), 0)
	ctx := context.Background()

	_, err := s.Validate(ctx, "sometoken")
	if err == nil || errors.Is(err, ErrInvalid) {
		t.Errorf("Validate during an outage: err = %v, want a plain error rather than ErrInvalid", err)
	}
	if _, err := s.Mint(ctx, MintInput{PostID: uuid.New(), AdminID: uuid.New()}); err == nil || errors.Is(err, ErrInvalid) || errors.Is(err, ErrUnavailable) {
		t.Errorf("Mint during an outage: err = %v, want the store error surfaced", err)
	}
	if err := s.Revoke(ctx, "sometoken"); err == nil {
		t.Error("Revoke during an outage: err = nil, want the failure reported so the caller knows the token still lives")
	}
}

func TestKeyHashesTheTokenAndNamespaces(t *testing.T) { // only the hash may be persisted, so a Redis dump doesn't leak usable preview URLs
	s := NewStoreNS(deadClient(), 0, "blog")
	token := "a-raw-token-shown-to-the-admin-once"
	key := s.key(token)

	if strings.Contains(key, token) {
		t.Errorf("key = %q embeds the raw token", key)
	}
	if !strings.HasPrefix(key, "preview:blog:") {
		t.Errorf("key = %q, want the preview:blog: namespace", key)
	}
	if len(key) != len("preview:blog:")+64 {
		t.Errorf("key = %q, want a full sha256 hex digest after the namespace", key)
	}
	if key != s.key(token) {
		t.Error("the same token hashed to two different keys, so a minted token could never validate")
	}
	if key == s.key("another-token") {
		t.Error("two tokens share a key")
	}
}

func TestNamespacesDoNotOverlap(t *testing.T) { // a token minted for one preview kind must not validate under another - the namespace is the only thing separating them
	blog := NewStoreNS(deadClient(), 0, "blog")
	cms := NewStoreNS(deadClient(), 0, "cms")
	if blog.key("tok") == cms.key("tok") {
		t.Error("blog and cms namespaces produced the same key for one token")
	}
}

func TestNamespaceAndTTLDefaults(t *testing.T) {
	c := deadClient()

	s := NewStoreNS(c, 0, "")
	if !strings.HasPrefix(s.key("t"), "preview:blog:") {
		t.Errorf("key = %q, want the blog namespace as the empty-ns fallback", s.key("t"))
	}
	if s.ttl != DefaultTTL {
		t.Errorf("ttl = %s, want the %s default", s.ttl, DefaultTTL)
	}

	if got := NewStoreNS(c, -time.Minute, "blog").ttl; got != DefaultTTL {
		t.Errorf("a negative ttl = %s, want it replaced by the default - Redis would reject the SET outright", got)
	}
	if got := NewStoreNS(c, time.Hour, "blog").ttl; got != time.Hour {
		t.Errorf("ttl = %s, want the caller's hour respected", got)
	}
	if !strings.HasPrefix(NewStore(c, 0).key("t"), "preview:blog:") {
		t.Error("NewStore is not pinned to the blog namespace")
	}
}
