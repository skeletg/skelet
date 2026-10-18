// Package spinech is the ClickHouse implementation of analytics.SpineStore, the columnar driver for high-volume deployments, computing results with native ClickHouse functions (windowFunnel, retention, uniq) rather than streaming rows and walking them in Go
// A SEPARATE package so analytics and everything importing it never depends on clickhouse-go - only cmd/server imports spinech, to inject it
package spinech

import (
	"context"
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

	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
)

const ( // ClickHouse wants few, large INSERTs since each makes a part, so AppendEvent buffers and a background loop flushes on size or tick
	chFlushBatch   = 2000                   // flush once the buffer reaches this many events
	chFlushEvery   = 1 * time.Second        // ...or at least this often
	chMaxBuffer    = 100_000                // hard cap; beyond it we shed (CH down too long)
	chFlushRetry   = 3                      // in-loop retries before re-buffering a failed batch
	chRecentKeys   = 200_000                // dedupe keys remembered for the was-new answer
	chFlushBackoff = 250 * time.Millisecond // multiplied by attempt number
	identityMaxAge = 2 * time.Minute        // a failing identify is dropped after this, well inside analytics.erasureGrace
)

type bufEvent struct {
	e     analytics.SpineEvent
	props string // marshalled at ingest - a poison Props map is rejected where it entered, not as a whole failed batch at flush
}

type pendingIdentity struct {
	customerID, channelID uuid.UUID
	lastSeen              time.Time
}

type Store struct { // ingest is batched by a background flusher, and Close drains it
	conn  driver.Conn
	write func(ctx context.Context, events []bufEvent) error // writeBatch, swapped only by tests

	mu     sync.Mutex
	buf    []bufEvent
	notify chan struct{} // nudges the flusher when the buffer hits a batch
	closed chan struct{}
	wg     sync.WaitGroup

	flushMu sync.Mutex // serializes flush end-to-end - an explicit Flush (the consumer's ack gate) must never see an empty buffer while a ticker flush of those events is failing elsewhere

	idMu sync.Mutex
	ids  map[string]pendingIdentity // identify upserts waiting for the flusher, newest per anon id

	seen     map[string]struct{} // recently-seen dedupe keys, so AppendEvent answers "was new" without a round-trip - see its doc for what that does and doesn't guarantee
	seenRing []string            // insertion order, for O(1) eviction at chRecentKeys
	seenNext int
}

var _ analytics.SpineStore = (*Store)(nil)

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
	deadline := time.Now().Add(30 * time.Second) // patient ping for container start ordering - a single attempt crash-loops the app whenever ClickHouse warms up slower
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
		ids:      map[string]pendingIdentity{},
		seen:     make(map[string]struct{}, chRecentKeys),
		seenRing: make([]string, chRecentKeys),
	}
	s.write = s.writeBatch
	s.wg.Add(1)
	go s.flushLoop()
	return s, nil
}

const (
	identityDictName     = "identity_map"            // the in-memory anon_id to customer_id map the reads resolve identity through
	identityDictLifetime = "LIFETIME(MIN 30 MAX 60)" // how long a newly stitched visitor takes to resolve, so aggregate reads can lag a live identify by up to a minute - fine for dashboards and the scheduled segment jobs, and AnonIDsForCustomer still reads the table directly
)

// CREATE OR REPLACE, so a changed query or lifetime takes effect on the next boot - it lives here rather than schema/001_spine.sql because a ClickHouse-sourced dictionary authenticates in its own right and its credentials would otherwise have to be committed, where these come off the DSN the driver already holds update_field makes the LIFETIME refresh INCREMENTAL
// only mappings stitched since the last refresh get re-fetched and merged, not a full GROUP BY + hash rebuild every minute forever - deletes never ride the delta, so the erasure sweep's RefreshIdentities is the one full rebuild still paid (update_lag absorbs writer clock skew) Two shapes are load-bearing, proven on a live 25.3 server: the wrapping subquery, because ClickHouse appends `WHERE stitched_at >= ...` verbatim on refresh and that only parses on a plain outer SELECT - and the alias must not be named last_seen, which would shadow the column inside argMax into an aggregate-in-aggregate error
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

// Buffers and returns immediately, so the event lives in memory until the flusher writes it (on chFlushBatch or every chFlushEvery) and a crash loses the unflushed tail - the standard throughput-for-durability trade of a batching analytics client The outbox only re-delivers commerce events on a crash or lease expiry BEFORE the row is marked delivered, never after a successful dispatch, so spine appends are best-effort past that point - Close drains on graceful shutdown, and a sustained outage caps the buffer at chMaxBuffer and sheds with a loud log rather than growing unbounded The bool means "genuinely new" and callers turn it into commerce writes - RecordOpen gates the send's first_open_at stamp on it, which COALESCE makes idempotent, so an optimistic answer just repeats harmless work It is the last chRecentKeys keys THIS process saw - enough for a repeat tracking pixel in its minute bucket or an outbox replay seconds later, but not for a duplicate landing on another replica or after a restart, which still collapses in storage
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

func (s *Store) flush(ctx context.Context) error {
	s.flushMu.Lock()
	defer s.flushMu.Unlock()
	// Only the events gate the result - the queue acks on it, and identifies are not on the stream
	if err := s.flushIdentities(ctx); err != nil {
		log.Printf("[spinech] %v", err)
	}
	return s.flushEvents(ctx)
}

// Writes the buffered events as one INSERT - on failure the batch is re-buffered, bounded by chMaxBuffer, so a transient blip retries with backoff
func (s *Store) flushEvents(ctx context.Context) error {
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
		if err = s.write(ctx, batch); err == nil {
			return nil
		}
	}
	if dropped, isolated := s.isolate(ctx, batch); isolated {
		s.mu.Lock()
		s.forget(dropped)
		s.mu.Unlock()
		if len(dropped) > 0 {
			return fmt.Errorf("ClickHouse refused %d of %d events, dropped: %w", len(dropped), len(batch), err)
		}
		return nil
	}
	s.mu.Lock()
	dropped := len(s.buf)+len(batch) > chMaxBuffer
	if !dropped {
		s.buf = append(batch, s.buf...) // prepend; retry on the next tick
	} else {
		s.forget(batch)
	}
	s.mu.Unlock()
	if dropped {
		return fmt.Errorf("flush %d events failed and buffer is over cap, DROPPED: %w", len(batch), err)
	}
	return fmt.Errorf("flush %d events failed, re-buffering: %w", len(batch), err)
}

// A batch that keeps failing is split in half - when one half writes, ClickHouse is up and refusing what the other carries, so the
// failing side is split again until the refused events stand alone and are dropped, rather than wedging every event queued behind them
// Both halves failing reads as an outage, and the caller re-buffers the whole batch as before
func (s *Store) isolate(ctx context.Context, batch []bufEvent) (dropped []bufEvent, isolated bool) {
	if len(batch) < 2 {
		return nil, false
	}
	a, b := batch[:len(batch)/2], batch[len(batch)/2:]
	errA, errB := s.write(ctx, a), s.write(ctx, b)
	if errA != nil && errB != nil {
		return nil, false
	}
	for _, half := range []struct {
		events []bufEvent
		err    error
	}{{a, errA}, {b, errB}} {
		if half.err != nil {
			dropped = append(dropped, s.narrow(ctx, half.events)...)
		}
	}
	return dropped, true
}

func (s *Store) narrow(ctx context.Context, failed []bufEvent) []bufEvent {
	if len(failed) == 1 {
		return failed
	}
	var dropped []bufEvent
	for _, half := range [][]bufEvent{failed[:len(failed)/2], failed[len(failed)/2:]} {
		if s.write(ctx, half) != nil {
			dropped = append(dropped, s.narrow(ctx, half)...)
		}
	}
	return dropped
}

// Forgets dropped events' dedupe keys, or a legitimate retry of them would read as "already recorded" and be swallowed - caller holds mu
func (s *Store) forget(events []bufEvent) {
	for _, e := range events {
		if e.e.DedupeKey != "" {
			delete(s.seen, e.e.DedupeKey)
		}
	}
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
// Prunes on occurred_at, the partition key
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
	return s.deleteSourceBefore(ctx, source, before)
}

// One partition at a time, only where rows remain - an unscoped DELETE would rewrite every older partition's row mask daily
func (s *Store) deleteSourceBefore(ctx context.Context, source string, before time.Time) (int64, error) {
	rows, err := s.conn.Query(ctx,
		"SELECT _partition_id, count() FROM analytics_events WHERE source = ? AND occurred_at < ? GROUP BY _partition_id", source, before)
	if err != nil {
		return 0, err
	}
	type part struct {
		id string
		n  uint64
	}
	var parts []part
	for rows.Next() {
		var p part
		if err := rows.Scan(&p.id, &p.n); err != nil {
			rows.Close()
			return 0, err
		}
		parts = append(parts, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	var total int64
	for _, p := range parts {
		if err := s.conn.Exec(ctx, "DELETE FROM analytics_events IN PARTITION ID ? WHERE source = ? AND occurred_at < ?", p.id, source, before); err != nil {
			return total, err
		}
		total += int64(p.n)
	}
	return total, nil
}

// Buffered like events - identify fires on every logged-in page view
// Repeats within a flush collapse to the newest, and a zero channel never replaces a real one; the flusher carries first_seen and channel forward from the stored row
func (s *Store) UpsertIdentity(_ context.Context, anonID string, customerID, channelID uuid.UUID) error {
	s.idMu.Lock()
	prev, had := s.ids[anonID]
	if !had && len(s.ids) >= chMaxBuffer {
		s.idMu.Unlock()
		return errBufferFull
	}
	if channelID == uuid.Nil {
		channelID = prev.channelID
	}
	s.ids[anonID] = pendingIdentity{customerID: customerID, channelID: channelID, lastSeen: time.Now().UTC()}
	full := len(s.ids) >= chFlushBatch
	s.idMu.Unlock()
	if full {
		select {
		case s.notify <- struct{}{}:
		default:
		}
	}
	return nil
}

// One keyed read, then one INSERT - a bare insert would reset first_seen and let a zero channel clobber a real one
func (s *Store) flushIdentities(ctx context.Context) error {
	s.idMu.Lock()
	pending := s.ids
	s.ids = map[string]pendingIdentity{}
	s.idMu.Unlock()
	if len(pending) == 0 {
		return nil
	}
	if err := s.writeIdentities(ctx, pending); err != nil {
		stale := time.Now().Add(-identityMaxAge)
		s.idMu.Lock()
		for anon, p := range pending {
			if p.lastSeen.Before(stale) { // the next page view re-sends it, and an erasure waits out this window
				continue
			}
			if cur, newer := s.ids[anon]; newer { // a newer identify wins but keeps a channel it lacks
				if cur.channelID == uuid.Nil {
					cur.channelID = p.channelID
					s.ids[anon] = cur
				}
				continue
			}
			s.ids[anon] = p
		}
		s.idMu.Unlock()
		return fmt.Errorf("flush %d identities failed, re-buffering: %w", len(pending), err)
	}
	return nil
}

func (s *Store) writeIdentities(ctx context.Context, pending map[string]pendingIdentity) error {
	anons := make([]string, 0, len(pending))
	for anon := range pending {
		anons = append(anons, anon)
	}
	type stored struct {
		first   time.Time
		channel uuid.UUID
	}
	prior := make(map[string]stored, len(anons))
	for start := 0; start < len(anons); start += 1000 {
		rows, err := s.conn.Query(ctx, "SELECT anon_id, first_seen, channel_id FROM analytics_identities FINAL WHERE anon_id IN ?", anons[start:min(start+1000, len(anons))])
		if err != nil {
			return err
		}
		for rows.Next() {
			var anon string
			var st stored
			if err := rows.Scan(&anon, &st.first, &st.channel); err != nil {
				rows.Close()
				return err
			}
			prior[anon] = st
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
	}
	now := time.Now().UTC() // write time, so the dictionary's incremental refresh sees rows that waited out an outage
	b, err := s.conn.PrepareBatch(ctx, "INSERT INTO analytics_identities (anon_id, customer_id, channel_id, first_seen, last_seen)")
	if err != nil {
		return err
	}
	for anon, p := range pending {
		first, channel := p.lastSeen, p.channelID
		if st, ok := prior[anon]; ok {
			first = st.first
			if channel == uuid.Nil {
				channel = st.channel
			}
		}
		if err := b.Append(anon, p.customerID, channel, first, now); err != nil {
			_ = b.Abort()
			return err
		}
	}
	return b.Send()
}

// Drops buffered identifies for the erased customers, so a flush after the erasure can't stitch them back
func (s *Store) forgetPendingIdentities(customerIDs []uuid.UUID) {
	gone := make(map[uuid.UUID]bool, len(customerIDs))
	for _, id := range customerIDs {
		gone[id] = true
	}
	s.idMu.Lock()
	for anon, p := range s.ids {
		if gone[p.customerID] {
			delete(s.ids, anon)
		}
	}
	s.idMu.Unlock()
}

// customer_id sits outside the sort key, so events are detached in place - except those whose dedupe key embeds the id, which are deleted
func (s *Store) DeleteCustomerIdentities(ctx context.Context, customerIDs []uuid.UUID) error {
	if len(customerIDs) == 0 {
		return nil
	}
	s.forgetPendingIdentities(customerIDs)
	var running uint64
	if err := s.conn.QueryRow(ctx, "SELECT count() FROM system.mutations WHERE database = currentDatabase() AND table = 'analytics_events' AND NOT is_done AND position(command, 'UPDATE customer_id') > 0").Scan(&running); err != nil {
		return err
	}
	if running > 0 {
		return analytics.ErrErasureInFlight
	}
	sync := clickhouse.Context(ctx, clickhouse.WithSettings(clickhouse.Settings{"mutations_sync": 1}))
	for start := 0; start < len(customerIDs); start += erasureChunk {
		// customer_id stays bare so the idx_customer bloom filter applies
		chunk := customerIDs[start:min(start+erasureChunk, len(customerIDs))]
		if err := s.conn.Exec(ctx, "DELETE FROM analytics_identities WHERE customer_id IN ?", chunk); err != nil {
			return err
		}
		if err := s.conn.Exec(ctx, "DELETE FROM analytics_events WHERE customer_id IN ? AND position(dedupe_key, toString(customer_id)) > 0", chunk); err != nil {
			return err
		}
		if err := s.conn.Exec(sync, "ALTER TABLE analytics_events UPDATE customer_id = toUUID('00000000-0000-0000-0000-000000000000') WHERE customer_id IN ?", chunk); err != nil {
			return err
		}
	}
	return nil
}

// The ids bind into the statement text, so a chunk stays well under ClickHouse's default max_query_size
const erasureChunk = 2000

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
