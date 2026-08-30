// Package spinech is the ClickHouse implementation of analytics.SpineStore, the columnar driver for high-volume deployments, computing results with native ClickHouse functions (windowFunnel, retention, uniq) rather than streaming rows and walking them in Go
// A SEPARATE package so analytics and everything importing it never depends on clickhouse-go - only cmd/server imports spinech, to inject it
package spinech

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/google/uuid"

	"github.com/skeletg/skelet/internal/analytics"
)

const ( // ClickHouse wants few, large INSERTs since each makes a part, so AppendEvent buffers and a background loop flushes on size or tick
	chFlushBatch   = 2000                   // flush once the buffer reaches this many events
	chFlushEvery   = 1 * time.Second        // ...or at least this often
	chMaxBuffer    = 100_000                // hard cap; beyond it we shed (CH down too long)
	chFlushRetry   = 3                      // in-loop retries before re-buffering a failed batch
	chRecentKeys   = 200_000                // dedupe keys remembered for the was-new answer
	chFlushBackoff = 250 * time.Millisecond // multiplied by attempt number
)

type bufEvent struct {
	e     analytics.SpineEvent
	props string // marshalled at ingest - a poison Props map is rejected where it entered, not as a whole failed batch at flush
}

type Store struct { // ingest is batched by a background flusher, and Close drains it
	conn driver.Conn

	mu     sync.Mutex
	buf    []bufEvent
	notify chan struct{} // nudges the flusher when the buffer hits a batch
	closed chan struct{}
	wg     sync.WaitGroup

	flushMu sync.Mutex // serializes flush end-to-end - an explicit Flush (the consumer's ack gate) must never see an empty buffer while a ticker flush of those events is failing elsewhere

	seen     map[string]struct{} // recently-seen dedupe keys, so AppendEvent answers "was new" without a round-trip - see its doc for what that does and doesn't guarantee
	seenRing []string            // insertion order, for O(1) eviction at chRecentKeys
	seenNext int
}

var _ analytics.SpineStore = (*Store)(nil) // compile-time proof this driver is interchangeable with the Postgres one

var errBufferFull = errors.New("spinech: ingest buffer full, event dropped") // shedding is data loss, not deduplication - callers get an error so it can't read as "already recorded"

func New(ctx context.Context, dsn string) (*Store, error) {
	opts, err := clickhouse.ParseDSN(dsn)
	if err != nil {
		if uerr, ok := errors.AsType[*url.Error](err); ok {
			return nil, fmt.Errorf("parse SIGNALS_CH_URL (value redacted): %w", uerr.Err)
		}
		return nil, errors.New("parse SIGNALS_CH_URL: invalid DSN (value redacted)")
	}
	if opts.Settings == nil {
		opts.Settings = clickhouse.Settings{}
	}
	opts.Settings["do_not_merge_across_partitions_select_final"] = 1 // every read runs FINAL to collapse not-yet-merged duplicates - this keeps that affordable, and it's sound because ReplacingMergeTree only dedups within a partition
	conn, err := clickhouse.Open(opts)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(30 * time.Second) // patient ping mirroring the Postgres pool, for container start ordering - a single attempt crash-loops the app whenever ClickHouse warms up slower
	for {
		pctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		err = conn.Ping(pctx)
		cancel()
		if err == nil {
			break
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			return nil, fmt.Errorf("ping clickhouse: %w", err)
		}
		time.Sleep(300*time.Millisecond + time.Duration(rand.Int63n(int64(400*time.Millisecond))))
	}
	if err := ensureIdentityDict(ctx, conn, opts); err != nil {
		return nil, err
	}
	s := &Store{
		conn:     conn,
		notify:   make(chan struct{}, 1),
		closed:   make(chan struct{}),
		seen:     make(map[string]struct{}, chRecentKeys),
		seenRing: make([]string, chRecentKeys),
	}
	s.wg.Add(1)
	go s.flushLoop()
	return s, nil
}

const (
	identityDictName     = "identity_map"            // the in-memory anon_id to customer_id map the reads resolve identity through
	identityDictLifetime = "LIFETIME(MIN 30 MAX 60)" // how long a newly stitched visitor takes to resolve, so aggregate reads can lag a live identify by up to a minute - fine for dashboards and the scheduled segment jobs, and AnonIDsForCustomer still reads the table directly
)

// CREATE OR REPLACE, so a changed query or lifetime takes effect on the next boot - it lives here rather than schema/001_spine.sql because a ClickHouse-sourced dictionary authenticates in its own right and its credentials would otherwise have to be committed, where these come off the DSN the driver already holds
// update_field makes the LIFETIME refresh INCREMENTAL - only mappings stitched since the last refresh get re-fetched and merged, not a full GROUP BY + hash rebuild every minute forever - deletes never ride the delta, so the erasure sweep's RefreshIdentities is the one full rebuild still paid (update_lag absorbs writer clock skew)
// Two shapes are load-bearing, proven on a live 25.3 server: the wrapping subquery, because ClickHouse appends `WHERE stitched_at >= ...` verbatim on refresh and that only parses on a plain outer SELECT - and the alias must not be named last_seen, which would shadow the column inside argMax into an aggregate-in-aggregate error
func ensureIdentityDict(ctx context.Context, conn driver.Conn, opts *clickhouse.Options) error {
	db := opts.Auth.Database
	if db == "" {
		db = "default"
	}
	ddl := fmt.Sprintf(`CREATE OR REPLACE DICTIONARY %s (anon_id String, customer_id UUID)
		PRIMARY KEY anon_id
		SOURCE(CLICKHOUSE(USER '%s' PASSWORD '%s' QUERY $q$
			SELECT anon_id, customer_id FROM (
				SELECT anon_id, argMax(customer_id, last_seen) AS customer_id, max(last_seen) AS stitched_at
				FROM %s.analytics_identities GROUP BY anon_id
			)
		$q$ update_field 'stitched_at' update_lag 300))
		%s
		LAYOUT(COMPLEX_KEY_HASHED())`,
		identityDictName, chEscape(opts.Auth.Username), chEscape(opts.Auth.Password), db, identityDictLifetime)
	if err := conn.Exec(ctx, ddl); err != nil {
		return fmt.Errorf("spinech: create %s dictionary (needs CREATE DICTIONARY on the app user): %w", identityDictName, err)
	}
	return nil
}

func chEscape(v string) string {
	return strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(v)
}

func (s *Store) markSeen(key string) bool { // reports whether it's the first sighting - caller holds s.mu
	if _, dup := s.seen[key]; dup {
		return false
	}
	if old := s.seenRing[s.seenNext]; old != "" {
		delete(s.seen, old)
	}
	s.seenRing[s.seenNext] = key
	s.seenNext = (s.seenNext + 1) % len(s.seenRing)
	s.seen[key] = struct{}{}
	return true
}

func (s *Store) Ping(ctx context.Context) error { return s.conn.Ping(ctx) } // health endpoints - the buffer can hide an unreachable server

func (s *Store) Flush(ctx context.Context) error { return s.flush(ctx) } // the queue consumer calls this before acking a batch, moving the durability boundary back to the stream

func (s *Store) Close() error { // drains the ingest buffer, then releases the connection pool
	close(s.closed)
	s.wg.Wait()
	return s.conn.Close()
}

func nz(p *uuid.UUID) uuid.UUID { // zero UUID is the "none" sentinel in the CH schema - cheaper than Nullable(UUID), and the reads treat zero as null
	if p == nil {
		return uuid.Nil
	}
	return *p
}

// Buffers and returns immediately, so the event lives in memory until the flusher writes it (on chFlushBatch or every chFlushEvery) and a crash loses the unflushed tail - the standard throughput-for-durability trade of a batching analytics client
// The outbox only re-delivers commerce events on a crash or lease expiry BEFORE the row is marked delivered, never after a successful dispatch, so spine appends are best-effort past that point - Close drains on graceful shutdown, and a sustained outage caps the buffer at chMaxBuffer and sheds with a loud log rather than growing unbounded
// The bool means "genuinely new" and callers turn it into commerce writes - RecordOpen gates the send's first_open_at stamp on it, which COALESCE makes idempotent, so an optimistic answer just repeats harmless work
// Postgres answers that exactly from a unique index, where this is the last chRecentKeys keys THIS process saw - enough for a repeat tracking pixel in its minute bucket or an outbox replay seconds later, but not for a duplicate landing on another replica or after a restart, which still collapses in storage
func (s *Store) AppendEvent(_ context.Context, e analytics.SpineEvent) (bool, error) {
	raw, err := json.Marshal(e.Props)
	if err != nil {
		return false, err
	}
	s.mu.Lock()
	if len(s.buf) >= chMaxBuffer {
		s.mu.Unlock()
		log.Printf("[spinech] ingest buffer full (%d), shedding event %q", chMaxBuffer, e.EventName)
		return false, errBufferFull
	}
	if e.DedupeKey != "" && !s.markSeen(e.DedupeKey) {
		s.mu.Unlock()
		return false, nil
	}
	s.buf = append(s.buf, bufEvent{e: e, props: string(raw)})
	full := len(s.buf) >= chFlushBatch
	s.mu.Unlock()
	if full {
		select {
		case s.notify <- struct{}{}:
		default:
		}
	}
	return true, nil
}

func (s *Store) AppendEvents(ctx context.Context, events []analytics.SpineEvent) error { // just the seam - the batching already lives in the flusher
	for _, e := range events {
		if _, err := s.AppendEvent(ctx, e); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) flushLoop() { // writes buffered events in batches until Close
	defer s.wg.Done()
	t := time.NewTicker(chFlushEvery)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			s.logFlush()
		case <-s.notify:
			s.logFlush()
		case <-s.closed:
			s.logFlush() // final drain
			return
		}
	}
}

func (s *Store) logFlush() {
	if err := s.flush(context.Background()); err != nil { // detached - buffered events outlive their request
		log.Printf("[spinech] %v", err)
	}
}

// Writes the buffered events as one INSERT - on failure the batch is re-buffered, bounded by chMaxBuffer, so a transient blip retries with backoff
func (s *Store) flush(ctx context.Context) error {
	s.flushMu.Lock()
	defer s.flushMu.Unlock()
	s.mu.Lock()
	batch := s.buf
	s.buf = nil
	s.mu.Unlock()
	if len(batch) == 0 {
		return nil
	}
	var err error
	for attempt := range chFlushRetry {
		if attempt > 0 {
			select {
			case <-time.After(time.Duration(attempt) * chFlushBackoff):
			case <-ctx.Done():
			}
		}
		if err = s.writeBatch(ctx, batch); err == nil {
			return nil
		}
	}
	s.mu.Lock()
	dropped := len(s.buf)+len(batch) > chMaxBuffer
	if !dropped {
		s.buf = append(batch, s.buf...) // prepend; retry on the next tick
	} else {
		// forget the dropped keys, or a legitimate retry of these events would read as "already recorded" and be swallowed
		for _, e := range batch {
			if e.e.DedupeKey != "" {
				delete(s.seen, e.e.DedupeKey)
			}
		}
	}
	s.mu.Unlock()
	if dropped {
		return fmt.Errorf("flush %d events failed and buffer is over cap, DROPPED: %w", len(batch), err)
	}
	return fmt.Errorf("flush %d events failed, re-buffering: %w", len(batch), err)
}

func (s *Store) writeBatch(ctx context.Context, events []bufEvent) error {
	b, err := s.conn.PrepareBatch(ctx,
		"INSERT INTO analytics_events (event_name, occurred_at, channel_id, customer_id, anon_id, source, send_id, properties, dedupe_key)")
	if err != nil {
		return err
	}
	for _, ev := range events {
		e := ev.e
		if err := b.Append(e.EventName, e.OccurredAt, nz(e.ChannelID), nz(e.CustomerID), e.AnonID, e.Source, nz(e.SendID), ev.props, e.DedupeKey); err != nil {
			_ = b.Abort() // an abandoned batch never returns its pooled connection
			return err
		}
	}
	return b.Send()
}

// Drops whole day partitions older than the cutoff, returning the rows removed - near-instant metadata work where the equivalent ALTER DELETE rewrites every part, at the cost of only releasing a partition once all of it is past the cutoff
// Postgres prunes on received_at and this on occurred_at, the partition key - they differ only for late arrivals, and occurred_at is the more meaningful retention axis anyway
func (s *Store) PruneEvents(ctx context.Context, before time.Time) (int64, error) {
	var rows uint64 // physical pre-collapse count (unmerged duplicates inflate it) - only feeds a log line
	if err := s.conn.QueryRow(ctx, `SELECT sum(rows) FROM system.parts
		WHERE database = currentDatabase() AND table = 'analytics_events'
		AND active AND toDate(partition) < toDate(?)`, before).Scan(&rows); err != nil {
		return 0, err
	}
	names, err := s.stalePartitions(ctx, before)
	if err != nil {
		return 0, err
	}
	for _, p := range names {
		if err := s.conn.Exec(ctx, "ALTER TABLE analytics_events DROP PARTITION ?", p); err != nil {
			return 0, err
		}
	}
	return int64(rows), nil
}

func (s *Store) PruneEventsBySource(ctx context.Context, source string, before time.Time) (int64, error) { // source-scoped retention (the shorter Lens window on web events) - a lightweight DELETE on occurred_at, since a source subset can't be a partition drop
	var rows uint64
	if err := s.conn.QueryRow(ctx,
		"SELECT count() FROM analytics_events WHERE source = ? AND occurred_at < ?", source, before,
	).Scan(&rows); err != nil {
		return 0, err
	}
	if rows == 0 {
		return 0, nil
	}
	if err := s.conn.Exec(ctx, "DELETE FROM analytics_events WHERE source = ? AND occurred_at < ?", source, before); err != nil {
		return 0, err
	}
	return int64(rows), nil
}

// Stitches an anon id to a customer - ReplacingMergeTree keeps the latest last_seen per anon_id, so last-write-wins
// Postgres updates in place and so leaves first_seen alone and COALESCEs channel_id; a bare insert here would instead reset first_seen to now and let a zero channel clobber a real one, so the current row is read forward first
func (s *Store) UpsertIdentity(ctx context.Context, anonID string, customerID, channelID uuid.UUID) error {
	firstSeen := time.Now().UTC()
	var (
		prevFirst   time.Time
		prevChannel uuid.UUID
	)
	switch err := s.conn.QueryRow(ctx,
		"SELECT first_seen, channel_id FROM analytics_identities FINAL WHERE anon_id = ? LIMIT 1", anonID,
	).Scan(&prevFirst, &prevChannel); {
	case err == nil:
		firstSeen = prevFirst
		if channelID == uuid.Nil {
			channelID = prevChannel
		}
	case errors.Is(err, sql.ErrNoRows):
		// first sighting - keep the defaults above
	default:
		return err
	}

	batch, err := s.conn.PrepareBatch(ctx, "INSERT INTO analytics_identities (anon_id, customer_id, channel_id, first_seen, last_seen)")
	if err != nil {
		return err
	}
	if err := batch.Append(anonID, customerID, channelID, firstSeen, time.Now().UTC()); err != nil {
		return err
	}
	return batch.Send()
}

func (s *Store) DeleteCustomerIdentities(ctx context.Context, customerIDs []uuid.UUID) error { // gDPR erasure - one lightweight DELETE for the whole batch (each DELETE is a mutation, so the Recorder queues requests, sweeps them daily and calls RefreshIdentities once at the end)
	if len(customerIDs) == 0 {
		return nil
	}
	ids := make([]string, len(customerIDs))
	for i, id := range customerIDs {
		ids[i] = id.String()
	}
	return s.conn.Exec(ctx, "DELETE FROM analytics_identities WHERE has(?, toString(customer_id))", ids)
}

// PruneIdentities drops mappings not confirmed since the cutoff - count first, then one lightweight DELETE
// A non-FINAL DELETE on last_seen is collapse-safe: stale versions of a still-fresh anon_id match, and FINAL would have discarded them anyway
func (s *Store) PruneIdentities(ctx context.Context, before time.Time) (int64, error) {
	var n uint64
	if err := s.conn.QueryRow(ctx, "SELECT count() FROM analytics_identities FINAL WHERE last_seen < ?", before).Scan(&n); err != nil {
		return 0, err
	}
	if n == 0 {
		return 0, nil
	}
	if err := s.conn.Exec(ctx, "DELETE FROM analytics_identities WHERE last_seen < ?", before); err != nil {
		return 0, err
	}
	return int64(n), nil
}

func (s *Store) RefreshIdentities(ctx context.Context) { // refreshIdentities fully reloads the identity dictionary so reads stop resolving erased mappings - best-effort and node-local (incremental refreshes never see deletes, so cluster replicas need their own reload, see the prod overlay header)
	if err := s.conn.Exec(ctx, "SYSTEM RELOAD DICTIONARY "+identityDictName); err != nil {
		log.Printf("[spinech] reload %s after identity erasure: %v", identityDictName, err)
	}
}

func (s *Store) CustomersForAnonIDs(ctx context.Context, anonIDs []string) (map[string]uuid.UUID, error) { // fINAL collapses to the latest mapping per anon_id - callers page the id set (the browse scan sends one page at a time), so has() stays bounded
	out := make(map[string]uuid.UUID, len(anonIDs))
	if len(anonIDs) == 0 {
		return out, nil
	}
	rows, err := s.conn.Query(ctx, "SELECT anon_id, customer_id FROM analytics_identities FINAL WHERE has(?, anon_id)", anonIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			a string
			c uuid.UUID
		)
		if err := rows.Scan(&a, &c); err != nil {
			return nil, err
		}
		out[a] = c
	}
	return out, rows.Err()
}

func (s *Store) AnonIDsForCustomer(ctx context.Context, customerID uuid.UUID) ([]string, error) { // the anon ids stitched to a customer - FINAL collapses the ReplacingMergeTree duplicates to the latest row per anon_id
	rows, err := s.conn.Query(ctx, "SELECT anon_id FROM analytics_identities FINAL WHERE customer_id = ?", customerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
