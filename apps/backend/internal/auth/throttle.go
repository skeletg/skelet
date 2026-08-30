package auth

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/skeletg/skelet/internal/apperr"
)

var ErrAccountLocked = errors.New("too many failed login attempts; please try again later") // classed as rate-limited, so the client shows "try again later" without confirming credentials

func init() {
	apperr.RegisterSentinel(ErrAccountLocked, "ACCOUNT_LOCKED", apperr.ClassRateLimited)
}

// Locks a (scope, email) pair after too many FAILED logins - the rate limiter caps burst rate, while this counts only failures and holds the lock across windows to catch slow distributed guessing; a hard lockout is DoS-able, since an attacker can fail on purpose to lock a victim out, so the threshold is generous, the lock short, and store errors fail open
type LoginThrottle struct {
	client     *redis.Client
	threshold  int
	failWindow time.Duration
	lockFor    time.Duration
	record     *redis.Script // INCR + PEXPIRE + threshold-lock in one atomic step so a failed expire can't leave a TTL-less counter that accumulates across windows
	fallback   *memThrottle  // enforces per-process when Valkey is down rather than going fully open - consulted only on a store error, and empty while Valkey is healthy
}

// Counts a failure and locks on the threshold in one atomic round-trip - same reason RedisLimiter scripts INCR+PEXPIRE together - a non-atomic INCR-then-EXPIRE leaves a persistent key if the second call fails - the count then never rolls over
const recordFailureScript = `
local n = redis.call('INCR', KEYS[1])
if n == 1 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
end
if n >= tonumber(ARGV[3]) then
  redis.call('SET', KEYS[2], '1', 'PX', ARGV[2])
  redis.call('DEL', KEYS[1])
end
return n
`

func NewLoginThrottle(client *redis.Client, threshold int, failWindow, lockFor time.Duration) *LoginThrottle { // a nil client enforces per-process through the fallback, so lockout still works but isn't shared across replicas
	t := &LoginThrottle{client: client, threshold: threshold, failWindow: failWindow, lockFor: lockFor, record: redis.NewScript(recordFailureScript)}
	t.fallback = newMemThrottle(threshold, failWindow, lockFor)
	go t.fallback.sweep(nil)
	return t
}

func (t *LoginThrottle) memKey(scope, email string) string { // same identity as the Valkey keys minus the fail and lock suffix, since one entry holds both
	return scope + ":" + strings.ToLower(strings.TrimSpace(email))
}

func (t *LoginThrottle) key(scope, email, suffix string) string {
	return "loginthrottle:" + scope + ":" + strings.ToLower(strings.TrimSpace(email)) + ":" + suffix
}

func (t *LoginThrottle) Locked(ctx context.Context, scope, email string) bool {
	if t == nil || strings.TrimSpace(email) == "" {
		return false
	}
	if t.client == nil {
		return t.fallback.locked(t.memKey(scope, email))
	}
	n, err := t.client.Exists(ctx, t.key(scope, email, "lock")).Result()
	if err != nil {
		return t.fallback.locked(t.memKey(scope, email)) // Valkey down - per-process state
	}
	return n > 0
}

func (t *LoginThrottle) RecordFailure(ctx context.Context, scope, email string) { // locks once the count hits the threshold
	if t == nil || strings.TrimSpace(email) == "" {
		return
	}
	if t.client == nil {
		t.fallback.recordFailure(t.memKey(scope, email))
		return
	}
	err := t.record.Run(ctx, t.client,
		[]string{t.key(scope, email, "fail"), t.key(scope, email, "lock")},
		t.failWindow.Milliseconds(), t.lockFor.Milliseconds(), t.threshold,
	).Err()
	if err != nil {
		t.fallback.recordFailure(t.memKey(scope, email)) // Valkey down - count per-process
	}
}

func (t *LoginThrottle) Clear(ctx context.Context, scope, email string) {
	if t == nil || strings.TrimSpace(email) == "" {
		return
	}
	if t.client == nil {
		t.fallback.clear(t.memKey(scope, email))
		return
	}
	if err := t.client.Del(ctx, t.key(scope, email, "fail"), t.key(scope, email, "lock")).Err(); err != nil {
		t.fallback.clear(t.memKey(scope, email))
	}
}

type memThrottle struct { // mirrors the Valkey fixed-window-then-lock scheme in a local map, exercised only when the store errors
	mu         sync.Mutex
	entries    map[string]*memThrottleEntry
	threshold  int
	failWindow time.Duration
	lockFor    time.Duration
}

type memThrottleEntry struct {
	failCount int
	failReset time.Time
	lockUntil time.Time
}

func newMemThrottle(threshold int, failWindow, lockFor time.Duration) *memThrottle {
	return &memThrottle{
		entries:    map[string]*memThrottleEntry{},
		threshold:  threshold,
		failWindow: failWindow,
		lockFor:    lockFor,
	}
}

func (m *memThrottle) locked(key string) bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entries[key]
	return ok && time.Now().Before(e.lockUntil)
}

func (m *memThrottle) recordFailure(key string) {
	if m == nil {
		return
	}
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entries[key]
	if !ok || now.After(e.failReset) {
		e = &memThrottleEntry{failReset: now.Add(m.failWindow)}
		m.entries[key] = e
	}
	e.failCount++
	if e.failCount >= m.threshold {
		e.lockUntil = now.Add(m.lockFor)
		e.failCount = 0
	}
}

func (m *memThrottle) clear(key string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.entries, key)
}

// A nil stop runs for the process lifetime, which is what the boot-time throttle wants - the seam exists so a
// caller that builds one per request or per test can shut the sweeper down instead of leaking it
func (m *memThrottle) sweep(stop <-chan struct{}) {
	tk := time.NewTicker(m.failWindow + m.lockFor)
	defer tk.Stop()
	for {
		select {
		case <-stop:
			return
		case now := <-tk.C:
			m.mu.Lock()
			for k, e := range m.entries {
				if now.After(e.failReset) && now.After(e.lockUntil) {
					delete(m.entries, k)
				}
			}
			m.mu.Unlock()
		}
	}
}
