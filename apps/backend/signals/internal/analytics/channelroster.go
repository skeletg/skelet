package analytics

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	signalsdb "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

// The channel roster, core's channels kept in the signals database because every beacon resolves one before doing anything else
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
	db        *sql.DB
	qs        *signalsdb.Queries
	blobs     BlobDeleter
	set       atomic.Pointer[channelSet]
	loading   atomic.Bool
	firstLoad sync.Mutex
}

func NewChannelRoster(db *sql.DB) *ChannelRoster {
	if db == nil {
		return &ChannelRoster{}
	}
	return &ChannelRoster{db: db, qs: signalsdb.New(db)}
}

// BlobDeleter removes a retired channel's Lens recordings from object storage before their rows go
type BlobDeleter interface {
	Delete(ctx context.Context, key string) error
}

func (c *ChannelRoster) SetBlobStore(b BlobDeleter) { c.blobs = b }

// ChannelEntry is one of core's channels as the push carries it - the identity fields are
// what a message sent for this channel goes out under, resolved by core from its config registry
type ChannelEntry struct {
	ID          uuid.UUID
	IsDefault   bool
	IsActive    bool
	Name        string
	PublicURL   string
	FromAddress string
	FromName    string
	LogoURL     string
}

// SyncChannels replaces the whole set - a channel the push leaves out is one core deleted, and only then does its Pulse content go
func (c *ChannelRoster) SyncChannels(ctx context.Context, channels []ChannelEntry) (synced, pruned int, err error) {
	return c.sync(ctx, channels, true)
}

// UpsertChannels applies a push without pruning, for a push that can't say which channels are deactivated
func (c *ChannelRoster) UpsertChannels(ctx context.Context, channels []ChannelEntry) (synced int, err error) {
	synced, _, err = c.sync(ctx, channels, false)
	return synced, err
}

func (c *ChannelRoster) sync(ctx context.Context, channels []ChannelEntry, prune bool) (synced, pruned int, err error) {
	if c.qs == nil {
		return 0, 0, nil
	}
	p := signalsdb.UpsertChannelRosterParams{SyncedAt: time.Now()}
	for _, ch := range channels {
		p.ChannelIds = append(p.ChannelIds, ch.ID)
		p.IsDefaults = append(p.IsDefaults, ch.IsDefault)
		p.IsActives = append(p.IsActives, ch.IsActive)
		p.Names = append(p.Names, ch.Name)
		p.PublicUrls = append(p.PublicUrls, ch.PublicURL)
		p.FromAddresses = append(p.FromAddresses, ch.FromAddress)
		p.FromNames = append(p.FromNames, ch.FromName)
		p.LogoUrls = append(p.LogoUrls, ch.LogoURL)
	}
	var gone int64
	err = c.inTx(ctx, func(q *signalsdb.Queries) error {
		// Serialised, so an older push can never land between a newer one's upsert and its prune
		if err := q.LockChannelRoster(ctx); err != nil {
			return fmt.Errorf("lock channel roster: %w", err)
		}
		if len(p.ChannelIds) > 0 {
			if err := q.UpsertChannelRoster(ctx, p); err != nil {
				return fmt.Errorf("upsert channel roster: %w", err)
			}
		}
		if !prune {
			return nil
		}
		// Never nil: a nil array reaches SQL as NULL and would keep every channel an empty push retires
		n, err := q.PruneChannelRoster(ctx, append([]uuid.UUID{}, p.ChannelIds...))
		if err != nil {
			return fmt.Errorf("prune channel roster: %w", err)
		}
		gone = n
		return nil
	})
	if err != nil {
		return 0, 0, err
	}
	c.set.Store(nil) // the next read picks the push up rather than waiting out the TTL
	return len(channels), int(gone), c.sweepRetired(ctx)
}

func (c *ChannelRoster) inTx(ctx context.Context, fn func(*signalsdb.Queries) error) error {
	if c.db == nil {
		return fn(c.qs)
	}
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := fn(c.qs.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit()
}

// sweepRetired clears what hung off pruned channels, once per tombstone - a step that fails leaves the tombstones for the next push to retry
func (c *ChannelRoster) sweepRetired(ctx context.Context) error {
	pending, err := c.qs.ChannelRosterTombstones(ctx)
	if err != nil {
		return fmt.Errorf("list retired channels: %w", err)
	}
	if len(pending) == 0 {
		return nil
	}
	var errs []error
	for _, p := range []struct {
		what  string
		prune func(context.Context) (int64, error)
	}{
		{"flows", c.qs.PruneOrphanedFlows},
		{"campaigns", c.qs.PruneOrphanedCampaigns},
		{"saved emails", c.qs.PruneOrphanedEmailLibrary},
		{"block types", c.qs.PruneOrphanedEmailBlocks},
		{"template overrides", c.qs.PruneOrphanedEmailTemplates},
		{"event destinations", c.qs.PruneOrphanedDestinations},
		{"experiments", c.qs.PruneOrphanedExperiments},
		{"channel settings", c.qs.PruneOrphanedChannelSettings},
		{"lens traffic", c.qs.PruneOrphanedLensTraffic},
		{"lens visitors", c.qs.PruneOrphanedLensVisitors},
		{"traffic rollups", c.qs.PruneOrphanedTrafficRollup},
		{"browse history", c.qs.PruneOrphanedBrowseSeen},
		{"browse notices", c.qs.PruneOrphanedBrowseNotified},
		{"trait runs", c.qs.PruneOrphanedTraitRuns},
	} {
		if _, err := p.prune(ctx); err != nil {
			errs = append(errs, fmt.Errorf("prune orphaned %s: %w", p.what, err))
		}
	}
	if err := c.deleteRetiredLensObjects(ctx); err != nil {
		errs = append(errs, err)
	} else {
		if _, err := c.qs.PruneOrphanedLensSessions(ctx); err != nil {
			errs = append(errs, fmt.Errorf("prune orphaned lens sessions: %w", err))
		}
		if _, err := c.qs.PruneOrphanedLensSnapshots(ctx); err != nil {
			errs = append(errs, fmt.Errorf("prune orphaned lens snapshots: %w", err))
		}
	}
	if len(errs) == 0 {
		if err := c.qs.ClearChannelRosterTombstones(ctx, pending); err != nil {
			errs = append(errs, fmt.Errorf("clear retired channels: %w", err))
		}
	}
	return errors.Join(errs...)
}

func (c *ChannelRoster) deleteRetiredLensObjects(ctx context.Context) error {
	if c.blobs == nil {
		return nil
	}
	var errs []error
	for _, list := range []func(context.Context) ([]string, error){c.qs.OrphanedLensSessionBlobKeys, c.qs.OrphanedLensSnapshotBlobKeys} {
		keys, err := list(ctx)
		if err != nil {
			errs = append(errs, fmt.Errorf("list retired lens objects: %w", err))
			continue
		}
		for _, k := range keys {
			if err := c.blobs.Delete(ctx, k); err != nil {
				errs = append(errs, fmt.Errorf("delete lens object %s: %w", k, err))
			}
		}
	}
	return errors.Join(errs...)
}

// Active reports whether the id names a channel core runs and has switched on. An unreadable roster answers false:
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
		set.active[r.ChannelID] = r.IsActive
		set.entries[r.ChannelID] = ChannelEntry{
			ID: r.ChannelID, IsDefault: r.IsDefault, IsActive: r.IsActive, Name: r.Name, PublicURL: r.PublicUrl,
			FromAddress: r.FromAddress, FromName: r.FromName, LogoURL: r.LogoUrl,
		}
		if r.IsDefault && r.IsActive {
			set.defaultID = r.ChannelID
		}
	}
	c.set.Store(set)
	return set
}
