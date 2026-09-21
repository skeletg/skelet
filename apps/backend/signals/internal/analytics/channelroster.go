package analytics

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	signalsdb "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

// The channel roster, core's active channels kept in the signals database because every beacon resolves one before doing anything else
// At the volume the spine is built for, that answer cannot be a call into core

// How long a loaded set is served before the next read refreshes it - a channel an operator just
// switched on starts being served within this, and a retired one stops
const channelRosterTTL = 30 * time.Second

type channelSet struct {
	active    map[uuid.UUID]bool
	entries   map[uuid.UUID]ChannelEntry
	defaultID uuid.UUID
	loadedAt  time.Time
}

// ChannelRoster answers the beacon surface's "is this a channel core runs" from an in-memory copy
// of the table, since a query per beacon would be a database round trip per tracked page view
type ChannelRoster struct {
	qs        *signalsdb.Queries
	set       atomic.Pointer[channelSet]
	loading   atomic.Bool
	firstLoad sync.Mutex
}

func NewChannelRoster(qs *signalsdb.Queries) *ChannelRoster { return &ChannelRoster{qs: qs} }

// ChannelEntry is one of core's active channels as the push carries it - the identity fields are
// what a message sent for this channel goes out under, resolved by core from its config registry
type ChannelEntry struct {
	ID          uuid.UUID
	IsDefault   bool
	Name        string
	PublicURL   string
	FromAddress string
	FromName    string
	LogoURL     string
}

// SyncChannels replaces the whole set - the push carries every channel core runs, so anything it
// did not carry is one core stopped running and its beacons stop being served
func (c *ChannelRoster) SyncChannels(ctx context.Context, channels []ChannelEntry) (synced, pruned int, err error) {
	if c.qs == nil {
		return 0, 0, nil
	}
	startedAt := time.Now()
	p := signalsdb.UpsertChannelRosterParams{SyncedAt: startedAt}
	for _, ch := range channels {
		p.ChannelIds = append(p.ChannelIds, ch.ID)
		p.IsDefaults = append(p.IsDefaults, ch.IsDefault)
		p.Names = append(p.Names, ch.Name)
		p.PublicUrls = append(p.PublicUrls, ch.PublicURL)
		p.FromAddresses = append(p.FromAddresses, ch.FromAddress)
		p.FromNames = append(p.FromNames, ch.FromName)
		p.LogoUrls = append(p.LogoUrls, ch.LogoURL)
	}
	if len(p.ChannelIds) > 0 {
		if err := c.qs.UpsertChannelRoster(ctx, p); err != nil {
			return 0, 0, fmt.Errorf("upsert channel roster: %w", err)
		}
	}
	gone, err := c.qs.PruneChannelRoster(ctx, startedAt)
	if err != nil {
		return len(channels), 0, fmt.Errorf("prune channel roster: %w", err)
	}
	if gone > 0 {
		// Everything that hung off those channels goes with them - the cascades could not cross databases
		for _, p := range []struct {
			what  string
			prune func(context.Context) (int64, error)
		}{
			{"flows", c.qs.PruneOrphanedFlows},
			{"campaigns", c.qs.PruneOrphanedCampaigns},
			{"saved emails", c.qs.PruneOrphanedEmailLibrary},
			{"block types", c.qs.PruneOrphanedEmailBlocks},
			{"template overrides", c.qs.PruneOrphanedEmailTemplates},
		} {
			if _, err := p.prune(ctx); err != nil {
				return len(channels), int(gone), fmt.Errorf("prune orphaned %s: %w", p.what, err)
			}
		}
	}
	c.set.Store(nil) // the next read picks the push up rather than waiting out the TTL
	return len(channels), int(gone), nil
}

// Active reports whether the id names a channel core runs. An unreadable roster answers false:
// a beacon is best-effort, and serving an unresolvable channel writes spine rows nothing can scope
func (c *ChannelRoster) Active(ctx context.Context, id uuid.UUID) bool {
	set := c.current(ctx)
	return set != nil && set.active[id]
}

// Entry is the channel as core last pushed it - a send needs more than "does this exist", and false
// means the roster has never heard of it rather than that it has no identity
func (c *ChannelRoster) Entry(ctx context.Context, id uuid.UUID) (ChannelEntry, bool) {
	set := c.current(ctx)
	if set == nil {
		return ChannelEntry{}, false
	}
	e, ok := set.entries[id]
	return e, ok
}

// Default is the channel a beacon carrying no channel of its own belongs to
func (c *ChannelRoster) Default(ctx context.Context) (uuid.UUID, bool) {
	set := c.current(ctx)
	if set == nil || set.defaultID == uuid.Nil {
		return uuid.Nil, false
	}
	return set.defaultID, true
}

// current serves the loaded set, refreshing behind it once the TTL is out - the stale answer is
// served meanwhile, since blocking every beacon on one query is the thing this cache exists to avoid
func (c *ChannelRoster) current(ctx context.Context) *channelSet {
	set := c.set.Load()
	if set != nil && time.Since(set.loadedAt) < channelRosterTTL {
		return set
	}
	if set == nil {
		// Nothing to serve, so this one waits - and only one of them queries
		c.firstLoad.Lock()
		defer c.firstLoad.Unlock()
		if loaded := c.set.Load(); loaded != nil {
			return loaded
		}
		return c.load(ctx)
	}
	if c.loading.CompareAndSwap(false, true) {
		go func() {
			defer c.loading.Store(false)
			c.load(context.WithoutCancel(ctx))
		}()
	}
	return set
}

func (c *ChannelRoster) load(ctx context.Context) *channelSet {
	if c.qs == nil {
		return nil
	}
	rows, err := c.qs.ChannelRoster(ctx)
	if err != nil {
		return nil
	}
	set := &channelSet{
		active:   make(map[uuid.UUID]bool, len(rows)),
		entries:  make(map[uuid.UUID]ChannelEntry, len(rows)),
		loadedAt: time.Now(),
	}
	for _, r := range rows {
		set.active[r.ChannelID] = true
		set.entries[r.ChannelID] = ChannelEntry{
			ID: r.ChannelID, IsDefault: r.IsDefault, Name: r.Name, PublicURL: r.PublicUrl,
			FromAddress: r.FromAddress, FromName: r.FromName, LogoURL: r.LogoUrl,
		}
		if r.IsDefault {
			set.defaultID = r.ChannelID
		}
	}
	c.set.Store(set)
	return set
}
