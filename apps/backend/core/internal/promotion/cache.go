package promotion

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

// The engine's per-store cached snapshot of active rules plus their conditions and actions - without it every cart read, applyCoupon and placeOrder costs 3 DB queries, which at any reasonable traffic dominates the engine's footprint Invalidation is event-driven, with every rule mutation calling Evict for the affected stores - a per-rule mutation can't tell whether the store scope is changing too, so it wipes the cache entirely, and reorder is the only high-frequency path where the cost is still small Evict only reaches the replica that served the mutation, so the TTL is also the hard staleness bound every OTHER replica pays: an operator who disables a misconfigured rule sees it stop firing fleet-wide within rulesCacheTTL, never later It doubles as the safety net for paths that mutate rules without going through Service, like direct DB editing during testing or a long-running CLI seed
type rulesCache struct {
	mu      sync.RWMutex
	entries map[uuid.UUID]*cachedRules // key: channel_id, uuid.Nil for the "all stores" bucket
	ttl     time.Duration
}

type cachedRules struct {
	rules    []Rule
	expireAt time.Time
}

// The bound with no cross-replica signal wired: Evict only reaches the replica that served the mutation, so this is how long every OTHER replica can keep pricing from stale rules
// Deliberately short for that reason, and a rules reload is one indexed query per channel per replica
const rulesCacheTTL = 5 * time.Second

// The bound once a CacheBus is wired - the broadcast handles the normal case, so the TTL only has to cover a dropped message or a Valkey outage, and can trade staleness for far fewer reloads
const rulesCacheTTLBroadcast = 60 * time.Second

func newRulesCache(ttl time.Duration) *rulesCache {
	return &rulesCache{
		entries: make(map[uuid.UUID]*cachedRules),
		ttl:     ttl,
	}
}

// Normalises the nil channel pointer to a sentinel uuid, so the map lookup works for both the global "all stores" slot and the per-store ones
func cacheKey(channelID *uuid.UUID) uuid.UUID {
	if channelID == nil {
		return uuid.Nil
	}
	return *channelID
}

// (rules, true) on a hit, (nil, false) on a miss or expired entry
func (c *rulesCache) load(channelID *uuid.UUID) ([]Rule, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.RLock()
	entry, ok := c.entries[cacheKey(channelID)]
	c.mu.RUnlock()
	if !ok || time.Now().After(entry.expireAt) {
		return nil, false
	}
	// Return a defensive copy, so the engine's per-Evaluate mutation of Conditions and Actions can't poison the cache for the next call
	out := make([]Rule, len(entry.rules))
	for i, r := range entry.rules {
		r.Conditions = nil
		r.Actions = nil
		out[i] = r
	}
	return out, true
}

func (c *rulesCache) store(channelID *uuid.UUID, rules []Rule) {
	if c == nil {
		return
	}
	// Stash a copy stripped of mutable per-eval state - the engine hydrates Conditions and Actions on each Evaluate, so they don't belong in the cache
	clean := make([]Rule, len(rules))
	for i, r := range rules {
		r.Conditions = nil
		r.Actions = nil
		clean[i] = r
	}
	c.mu.Lock()
	c.entries[cacheKey(channelID)] = &cachedRules{
		rules:    clean,
		expireAt: time.Now().Add(c.ttl),
	}
	c.mu.Unlock()
}

// Drops the cache entry for one store - the service calls it after any rule mutation, so the next Evaluate hits the DB instead of returning stale state
func (c *rulesCache) Evict(channelID *uuid.UUID) {
	if c == nil {
		return
	}
	c.mu.Lock()
	delete(c.entries, cacheKey(channelID))
	c.mu.Unlock()
}

// Wipes every entry, for a mutation that might affect rules across multiple store buckets like a priority reorder - cheaper than tracking precise scope changes
func (c *rulesCache) EvictAll() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.entries = make(map[uuid.UUID]*cachedRules)
	c.mu.Unlock()
}

// The cache-aware wrapper engine.Evaluate uses in place of repo.ListActiveRulesForChannel - on a miss it loads fresh, stores and returns; on a hit it skips the DB entirely
func loadCachedActiveRules(
	ctx context.Context,
	cache *rulesCache,
	repo Repository,
	channelID *uuid.UUID,
) ([]Rule, error) {
	if cached, ok := cache.load(channelID); ok {
		return cached, nil
	}
	rules, err := repo.ListActiveRulesForChannel(ctx, channelID)
	if err != nil {
		return nil, err
	}
	cache.store(channelID, rules)
	return rules, nil
}
