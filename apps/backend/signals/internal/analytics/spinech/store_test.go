package spinech

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
	"github.com/skeletg/skelet/apps/backend/signals/internal/chtest"
)

func TestStoreIngest(t *testing.T) {
	ctx := context.Background()
	st, err := New(ctx, chtest.DSN(t))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer st.Close()

	ch, cust := uuid.New(), uuid.New()
	anon := "anon-" + uuid.NewString()

	ok, err := st.AppendEvent(ctx, analytics.SpineEvent{
		EventName: "page_view", OccurredAt: time.Now(), ChannelID: &ch,
		AnonID: anon, Source: "web", Props: map[string]any{"title": "Home"},
		DedupeKey: "test:" + uuid.NewString(),
	})
	if err != nil || !ok {
		t.Fatalf("append event: ok=%v err=%v", ok, err)
	}

	if err := st.UpsertIdentity(ctx, anon, cust, ch); err != nil {
		t.Fatalf("upsert identity: %v", err)
	}
	ids, err := st.AnonIDsForCustomer(ctx, cust)
	if err != nil {
		t.Fatalf("anon ids: %v", err)
	}
	found := false
	for _, a := range ids {
		if a == anon {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected anon %q in %v", anon, ids)
	}
	t.Logf("ok: appended event + stitched identity; anon ids=%v", ids)
}

// The drop path forgets a batch's keys so a later retry reads as new instead of being swallowed, and the ring must survive slots whose key was forgotten early
func TestSeenCacheForgetsDroppedKeys(t *testing.T) {
	s := &Store{seen: make(map[string]struct{}, 4), seenRing: make([]string, 4)}
	if !s.markSeen("a") {
		t.Fatal("first sighting must be new")
	}
	if s.markSeen("a") {
		t.Fatal("repeat must not be new")
	}
	delete(s.seen, "a") // what flush does when the batch is dropped
	if !s.markSeen("a") {
		t.Fatal("retry after a dropped batch must read as new again")
	}
	for _, k := range []string{"b", "c", "d", "e", "f"} {
		if !s.markSeen(k) {
			t.Fatalf("%s should be new", k)
		}
	}
	if s.markSeen("f") {
		t.Fatal("f is inside the remembered window")
	}
}

// Opens a gated store, or skips
func testStore(t *testing.T) *Store {
	t.Helper()
	st, err := New(context.Background(), chtest.DSN(t))
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// A replay carries a fresh occurred_at (activity.Tracker stamps time.Now() per delivery), so with occurred_at in the sort key the copy became a permanent second row
// Two stores stand in for two replicas, which is the case the in-process key cache can't catch - the collapse has to come from storage
func TestAppendEventReplayCollapses(t *testing.T) {
	ctx := context.Background()
	ch, anon := uuid.New(), "anon-"+uuid.NewString()
	key := "replay:" + uuid.NewString()
	// Midday, so the pair can't straddle a day boundary - dedup is within a partition
	at := time.Now().UTC().Truncate(24 * time.Hour).Add(12 * time.Hour)

	ev := func(when time.Time) analytics.SpineEvent {
		return analytics.SpineEvent{
			EventName: "order_placed", OccurredAt: when, ChannelID: &ch,
			AnonID: anon, Source: "commerce", Props: map[string]any{}, DedupeKey: key,
		}
	}
	first := testStore(t)
	if _, err := first.AppendEvent(ctx, ev(at)); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	replica := testStore(t)
	if _, err := replica.AppendEvent(ctx, ev(at.Add(4*time.Second))); err != nil {
		t.Fatal(err)
	}
	if err := replica.Close(); err != nil {
		t.Fatal(err)
	}

	reader := testStore(t)
	defer reader.Close()
	p, err := reader.VisitorProfile(ctx, ch, anon, at.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if p.TotalEvents != 1 {
		t.Fatalf("TotalEvents = %d, want 1 (the replay must collapse onto the original)", p.TotalEvents)
	}
}

// The bool drives commerce writes (RecordOpen gates the first_open_at stamp), so a repeat inside one process must not read as new
func TestAppendEventRepeatIsNotNew(t *testing.T) {
	ctx := context.Background()
	st := testStore(t)
	defer st.Close()
	ch := uuid.New()
	e := analytics.SpineEvent{
		EventName: "email_opened", OccurredAt: time.Now().UTC(), ChannelID: &ch,
		AnonID: "anon-" + uuid.NewString(), Source: "campaign",
		Props: map[string]any{}, DedupeKey: "open:" + uuid.NewString(),
	}
	isNew, err := st.AppendEvent(ctx, e)
	if err != nil || !isNew {
		t.Fatalf("first append: isNew=%v err=%v", isNew, err)
	}
	if isNew, err = st.AppendEvent(ctx, e); err != nil || isNew {
		t.Fatalf("repeat append: isNew=%v err=%v, want false/nil", isNew, err)
	}
}

// A lone page view has no measurable dwell, and avgIf over the empty set is nan - toInt64(nan) throws, and a swallowed error there blanks the profile's pages
func TestProfilePagesSurviveSinglePageView(t *testing.T) {
	ctx := context.Background()
	st := testStore(t)
	ch, anon := uuid.New(), "anon-"+uuid.NewString()
	at := time.Now().UTC()
	if _, err := st.AppendEvent(ctx, analytics.SpineEvent{
		EventName: "page_view", OccurredAt: at, ChannelID: &ch, AnonID: anon, Source: "web",
		Props:     map[string]any{"title": "Home", "page_key": "/"},
		DedupeKey: "pv:" + uuid.NewString(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	reader := testStore(t)
	defer reader.Close()
	p, err := reader.VisitorProfile(ctx, ch, anon, at.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.TopPages) != 1 || p.TopPages[0].Title != "Home" {
		t.Fatalf("TopPages = %+v, want the one page view", p.TopPages)
	}
	if p.TopPages[0].AvgDwellSeconds != 0 {
		t.Fatalf("AvgDwellSeconds = %d, want 0 for an unmeasurable dwell", p.TopPages[0].AvgDwellSeconds)
	}
}

// Postgres updates in place, so first_seen survives and a null channel COALESCEs - a bare insert here reset both
func TestUpsertIdentityCarriesForward(t *testing.T) {
	ctx := context.Background()
	st := testStore(t)
	defer st.Close()
	anon, cust, ch := "anon-"+uuid.NewString(), uuid.New(), uuid.New()

	if err := st.UpsertIdentity(ctx, anon, cust, ch); err != nil {
		t.Fatal(err)
	}
	var first0, first1 time.Time
	var channel1 uuid.UUID
	if err := st.conn.QueryRow(ctx, "SELECT first_seen FROM analytics_identities FINAL WHERE anon_id = ?", anon).Scan(&first0); err != nil {
		t.Fatal(err)
	}
	// re-identify with no channel, the shape that must not clobber either column
	if err := st.UpsertIdentity(ctx, anon, cust, uuid.Nil); err != nil {
		t.Fatal(err)
	}
	if err := st.conn.QueryRow(ctx, "SELECT first_seen, channel_id FROM analytics_identities FINAL WHERE anon_id = ?", anon).Scan(&first1, &channel1); err != nil {
		t.Fatal(err)
	}
	if !first1.Equal(first0) {
		t.Fatalf("first_seen moved %v -> %v, want it preserved", first0, first1)
	}
	if channel1 != ch {
		t.Fatalf("channel_id = %v, want the original %v preserved", channel1, ch)
	}
}

// The Lens retention window must hold on this engine too - web events past the cutoff go, other sources and fresher web events stay
func TestPruneEventsBySourceScopesToSource(t *testing.T) {
	ctx := context.Background()
	st := testStore(t)
	defer st.Close()
	ch := uuid.New()
	old := time.Now().UTC().Add(-48 * time.Hour)
	ev := func(source string, at time.Time) analytics.SpineEvent {
		return analytics.SpineEvent{
			EventName: "page_view", OccurredAt: at, ChannelID: &ch,
			AnonID: "anon-" + uuid.NewString(), Source: source,
			Props: map[string]any{}, DedupeKey: "prunesrc:" + uuid.NewString(),
		}
	}
	for _, e := range []analytics.SpineEvent{ev("web", old), ev("commerce", old), ev("web", time.Now().UTC())} {
		if _, err := st.AppendEvent(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.flush(ctx); err != nil {
		t.Fatal(err)
	}

	n, err := st.PruneEventsBySource(ctx, "web", time.Now().UTC().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if n < 1 {
		t.Fatalf("pruned %d rows, want at least the old web event", n)
	}
	var webOld, commerceOld uint64
	if err := st.conn.QueryRow(ctx,
		"SELECT countIf(source = 'web' AND occurred_at < ?), countIf(source = 'commerce' AND channel_id = ?) FROM analytics_events FINAL WHERE channel_id = ?",
		time.Now().UTC().Add(-time.Hour), ch, ch).Scan(&webOld, &commerceOld); err != nil {
		t.Fatal(err)
	}
	if webOld != 0 {
		t.Fatalf("old web events survived the source prune: %d", webOld)
	}
	if commerceOld != 1 {
		t.Fatalf("commerce events must survive a web prune, found %d", commerceOld)
	}
}

// GDPR erasure must drop every mapping for the customer in this engine's own identities - a bulk resolve afterwards must come back empty too
func TestDeleteCustomerIdentitiesErases(t *testing.T) {
	ctx := context.Background()
	st := testStore(t)
	defer st.Close()
	cust, ch := uuid.New(), uuid.New()
	anonA, anonB := "anon-"+uuid.NewString(), "anon-"+uuid.NewString()

	for _, a := range []string{anonA, anonB} {
		if err := st.UpsertIdentity(ctx, a, cust, ch); err != nil {
			t.Fatal(err)
		}
	}
	resolved, err := st.CustomersForAnonIDs(ctx, []string{anonA, anonB, "anon-unknown"})
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved) != 2 || resolved[anonA] != cust || resolved[anonB] != cust {
		t.Fatalf("CustomersForAnonIDs = %v, want both anon ids -> %v", resolved, cust)
	}

	if err := st.DeleteCustomerIdentities(ctx, []uuid.UUID{cust}); err != nil {
		t.Fatal(err)
	}
	st.RefreshIdentities(ctx) // what the sweep does once per batch - keeps dictGet reads honest for later tests
	if ids, err := st.AnonIDsForCustomer(ctx, cust); err != nil || len(ids) != 0 {
		t.Fatalf("AnonIDsForCustomer after erasure = %v err=%v, want none", ids, err)
	}
	if gone, err := st.CustomersForAnonIDs(ctx, []string{anonA, anonB}); err != nil || len(gone) != 0 {
		t.Fatalf("CustomersForAnonIDs after erasure = %v err=%v, want none", gone, err)
	}
}

func TestDeleteCustomerIdentitiesDetachesEvents(t *testing.T) {
	ctx := context.Background()
	st := testStore(t)
	defer st.Close()
	cust, ch := uuid.New(), uuid.New()
	at := time.Now().UTC()
	plain, keyed := "order:"+uuid.NewString(), "act:account_updated:"+cust.String()+":1"
	for _, key := range []string{plain, keyed} {
		if _, err := st.AppendEvent(ctx, analytics.SpineEvent{
			EventName: "order_placed", OccurredAt: at, ChannelID: &ch, CustomerID: &cust,
			Source: "commerce", Props: map[string]any{}, DedupeKey: key,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteCustomerIdentities(ctx, []uuid.UUID{cust}); err != nil {
		t.Fatal(err)
	}
	var attached, kept uint64
	if err := st.conn.QueryRow(ctx, "SELECT countIf(customer_id = ?), countIf(dedupe_key = ?) FROM analytics_events FINAL WHERE channel_id = ?",
		cust, plain, ch).Scan(&attached, &kept); err != nil {
		t.Fatal(err)
	}
	if attached != 0 {
		t.Errorf("%d events still name the erased customer", attached)
	}
	if kept != 1 {
		t.Errorf("the plain event survived %d times, want once - detached, not deleted", kept)
	}
	var keyedLeft uint64
	if err := st.conn.QueryRow(ctx, "SELECT count() FROM analytics_events FINAL WHERE dedupe_key = ?", keyed).Scan(&keyedLeft); err != nil {
		t.Fatal(err)
	}
	if keyedLeft != 0 {
		t.Error("an event whose dedupe key embeds the customer id survived, so the id is still on the spine")
	}
}

// The retention sweep must only take mappings whose last confirmation predates the cutoff
func TestPruneIdentitiesAgesOut(t *testing.T) {
	ctx := context.Background()
	st := testStore(t)
	defer st.Close()
	cust, ch := uuid.New(), uuid.New()
	stale, fresh := "anon-stale-"+uuid.NewString(), "anon-fresh-"+uuid.NewString()
	if err := st.UpsertIdentity(ctx, fresh, cust, ch); err != nil {
		t.Fatal(err)
	}
	// last_seen is the ReplacingMergeTree version column, so it can't be aged by mutation - insert the stale mapping directly
	old := time.Now().UTC().AddDate(0, 0, -400)
	if err := st.conn.Exec(ctx,
		"INSERT INTO analytics_identities (anon_id, customer_id, channel_id, first_seen, last_seen) VALUES (?, ?, ?, ?, ?)",
		stale, cust, ch, old, old); err != nil {
		t.Fatal(err)
	}
	n, err := st.PruneIdentities(ctx, time.Now().UTC().AddDate(0, 0, -365))
	if err != nil {
		t.Fatal(err)
	}
	if n < 1 {
		t.Fatalf("PruneIdentities = %d, want at least the aged mapping", n)
	}
	resolved, err := st.CustomersForAnonIDs(ctx, []string{stale, fresh})
	if err != nil {
		t.Fatal(err)
	}
	if _, gone := resolved[stale]; gone {
		t.Fatalf("stale mapping survived the prune: %v", resolved)
	}
	if resolved[fresh] != cust {
		t.Fatalf("fresh mapping did not survive: %v", resolved)
	}
}

// Grid increments live as raw events under this engine - the reads must aggregate them back to the exact grid the Postgres upserts would hold
func TestLensGridFromEvents(t *testing.T) {
	ctx := context.Background()
	st := testStore(t)
	defer st.Close()
	ch := uuid.New()
	cell := func(kind string, x, y, n int) analytics.SpineEvent {
		return analytics.SpineEvent{
			EventName: "lens_cell", OccurredAt: time.Now().UTC(), ChannelID: &ch, Source: "lens",
			Props:     map[string]any{"page_key": "/p", "viewport": "desktop", "state": "default", "kind": kind, "x": x, "y": y, "n": n},
			DedupeKey: "lenstest:" + uuid.NewString(),
		}
	}
	elem := analytics.SpineEvent{
		EventName: "lens_element", OccurredAt: time.Now().UTC(), ChannelID: &ch, Source: "lens",
		Props:     map[string]any{"page_key": "/p", "viewport": "desktop", "selector": "#buy", "text": "Buy", "n": 4},
		DedupeKey: "lenstest:" + uuid.NewString(),
	}
	for _, e := range []analytics.SpineEvent{cell("click", 2, 3, 5), cell("click", 2, 3, 2), cell("scroll", 0, 80, 10), elem} {
		if _, err := st.AppendEvent(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.flush(ctx); err != nil {
		t.Fatal(err)
	}

	cells, err := st.LensCellsFromEvents(ctx, ch, "/p", "desktop", "default", "click")
	if err != nil {
		t.Fatal(err)
	}
	if len(cells) != 1 || cells[0].X != 2 || cells[0].Y != 3 || cells[0].Count != 7 {
		t.Fatalf("cells = %+v, want one (2,3)=7", cells)
	}
	els, err := st.LensElementsFromEvents(ctx, ch, "/p", "desktop", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(els) != 1 || els[0].Selector != "#buy" || els[0].Text != "Buy" || els[0].Count != 4 {
		t.Fatalf("elements = %+v, want #buy Buy 4", els)
	}
	grid, err := st.LensPageGridFromEvents(ctx, ch, "/p")
	if err != nil {
		t.Fatal(err)
	}
	if grid.Clicks != 7 || grid.ScrollTotal != 10 || grid.ScrollWeight != 800 {
		t.Fatalf("grid = %+v, want clicks 7, scroll 10 weighted 800", grid)
	}
}

// The member set rides an external temp table (not parameter binding), so prove the round trip against a live server - a non-member's events must not count
func TestSegmentAffinitiesExternalTable(t *testing.T) {
	ctx := context.Background()
	st := testStore(t)
	defer st.Close()
	ch := uuid.New()
	inA, inB, outsider := uuid.New(), uuid.New(), uuid.New()
	ev := func(cust uuid.UUID, val string) analytics.SpineEvent {
		return analytics.SpineEvent{
			EventName: "product_view", OccurredAt: time.Now().UTC(), ChannelID: &ch, CustomerID: &cust,
			Source: "web", Props: map[string]any{"attr:color": val}, DedupeKey: "afftest:" + uuid.NewString(),
		}
	}
	for _, e := range []analytics.SpineEvent{ev(inA, "red"), ev(inB, "red"), ev(outsider, "blue")} {
		if _, err := st.AppendEvent(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.flush(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := st.SegmentAffinitiesForMembers(ctx, ch, []uuid.UUID{inA, inB}, time.Now().Add(-time.Hour), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Attr != "color" || got[0].Value != "red" || got[0].Members != 2 {
		t.Fatalf("affinities = %+v, want one color=red with 2 members (blue belongs to a non-member)", got)
	}
}

// Anonymous events must resolve to the customer once the visitor is stitched, which is what the eleven aggregate reads use the identity dictionary for - the reload stands in for the LIFETIME refresh a live deployment waits out
func TestIdentityDictResolvesAnonToCustomer(t *testing.T) {
	ctx := context.Background()
	st := testStore(t)
	defer st.Close()
	ch, cust := uuid.New(), uuid.New()
	anon := "anon-" + uuid.NewString()
	at := time.Now().UTC()

	// no customer_id on the event, so only the stitch can attribute it
	if _, err := st.AppendEvent(ctx, analytics.SpineEvent{
		EventName: "order_placed", OccurredAt: at, ChannelID: &ch, AnonID: anon,
		Source: "commerce", Props: map[string]any{}, DedupeKey: "dict:" + uuid.NewString(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.flush(ctx); err != nil {
		t.Fatal(err)
	}

	before, err := st.EventActors(ctx, ch, "order_placed", 1, 2, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 0 {
		t.Fatalf("unstitched anon resolved to %v, want no actors", before)
	}

	if err := st.UpsertIdentity(ctx, anon, cust, ch); err != nil {
		t.Fatal(err)
	}
	if err := st.conn.Exec(ctx, "SYSTEM RELOAD DICTIONARY "+identityDictName); err != nil {
		t.Fatal(err)
	}
	after, err := st.EventActors(ctx, ch, "order_placed", 1, 2, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 1 || after[0] != cust {
		t.Fatalf("EventActors = %v, want [%v] resolved through the dictionary", after, cust)
	}
}

// Retention drops whole day partitions and reports the rows removed, where an ALTER DELETE always answers 0 - a partition only goes once it's fully past the cutoff
func TestPruneEventsDropsWholeDays(t *testing.T) {
	ctx := context.Background()
	st := testStore(t)
	defer st.Close()
	ch, anon := uuid.New(), "anon-"+uuid.NewString()
	old := time.Now().UTC().Truncate(24 * time.Hour).Add(-30 * 24 * time.Hour).Add(9 * time.Hour)
	if _, err := st.AppendEvent(ctx, analytics.SpineEvent{
		EventName: "page_view", OccurredAt: old, ChannelID: &ch, AnonID: anon, Source: "web",
		Props:     map[string]any{},
		DedupeKey: "prune:" + uuid.NewString(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.flush(ctx); err != nil {
		t.Fatal(err)
	}

	before, err := st.VisitorProfile(ctx, ch, anon, old.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if before.TotalEvents != 1 {
		t.Fatalf("seeded TotalEvents = %d, want 1", before.TotalEvents)
	}
	n, err := st.PruneEvents(ctx, old.Add(48*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if n < 1 {
		t.Fatalf("PruneEvents reported %d rows, want at least the 1 seeded", n)
	}
	after, err := st.VisitorProfile(ctx, ch, anon, old.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if after.TotalEvents != 0 {
		t.Fatalf("TotalEvents after prune = %d, want 0", after.TotalEvents)
	}
}

// Seeds a fresh channel with `visitors` people who each did page_view, of whom `returners` came back a day later for cart_add - the fixture Retention and Flow assert against, so neither depends on whatever the dev volume happens to hold
func seedJourneys(t *testing.T, visitors, returners int) (uuid.UUID, time.Time) {
	t.Helper()
	ctx := context.Background()
	st := testStore(t)
	ch := uuid.New()
	day0 := time.Now().UTC().Truncate(24 * time.Hour).Add(-7 * 24 * time.Hour).Add(9 * time.Hour)

	for i := range visitors {
		anon := "anon-" + uuid.NewString()
		add := func(name string, when time.Time) {
			if _, err := st.AppendEvent(ctx, analytics.SpineEvent{
				EventName: name, OccurredAt: when, ChannelID: &ch, AnonID: anon, Source: "web",
				Props:     map[string]any{"title": "Home", "page_key": "/"},
				DedupeKey: name + ":" + uuid.NewString(),
			}); err != nil {
				t.Fatal(err)
			}
		}
		add("page_view", day0)
		if i < returners {
			add("cart_add", day0.Add(26*time.Hour))
		}
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	return ch, day0.Add(-time.Hour)
}

// The shared retention walker over a ClickHouse fetch - cohort is the distinct people who did the start event, and bucket 1 the returners a day later
func TestRetention(t *testing.T) {
	ch, since := seedJourneys(t, 5, 2)
	st := testStore(t)
	defer st.Close()
	r, err := st.Retention(context.Background(), ch, "page_view", "cart_add", since, time.Now(), 8, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if r.Cohort != 5 {
		t.Fatalf("cohort = %d, want 5 (the seeded page_view visitors)", r.Cohort)
	}
	if r.Buckets[1].Count != 2 {
		t.Fatalf("buckets[1].Count = %d, want 2 (the seeded returners)", r.Buckets[1].Count)
	}
}

func TestFlow(t *testing.T) {
	ch, since := seedJourneys(t, 5, 2)
	st := testStore(t)
	defer st.Close()
	f, err := st.Flow(context.Background(), ch, "page_view", since, 3, 3)
	if err != nil {
		t.Fatal(err)
	}
	if f.Cohort != 5 {
		t.Fatalf("flow cohort = %d, want 5 (the seeded page_view visitors)", f.Cohort)
	}
	if f.Continued != 2 {
		t.Fatalf("flow continued = %d, want 2 (only the returners did anything after)", f.Continued)
	}
}

// Pins the keyset cursor - a batch insert shares one created_at, so the uuid tiebreaker decides the boundary and a page must not re-serve rows the previous one returned
func TestEventsForForwardingPagesInIngestOrder(t *testing.T) {
	ctx := context.Background()
	st := testStore(t)
	ch := uuid.New()
	at := time.Now().UTC()
	for i := range 3 {
		if _, err := st.AppendEvent(ctx, analytics.SpineEvent{
			EventName: "page_view", OccurredAt: at, ChannelID: &ch, AnonID: "fwd-" + uuid.NewString(),
			Source: "web", Props: map[string]any{"n": i}, DedupeKey: "fwd:" + uuid.NewString(),
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.flush(ctx); err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	// the forwarding read hides rows younger than its 5s visibility watermark, so age these out of it
	if err := st.conn.Exec(ctx,
		"ALTER TABLE analytics_events UPDATE created_at = created_at - INTERVAL 10 SECOND WHERE channel_id = ? SETTINGS mutations_sync = 1", ch); err != nil {
		t.Fatal(err)
	}

	page, err := st.EventsForForwarding(ctx, ch, time.Unix(0, 0).UTC(), uuid.Nil, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != 2 {
		t.Fatalf("first page = %d rows, want the limit of 2", len(page))
	}
	last := page[len(page)-1]
	rest, err := st.EventsForForwarding(ctx, ch, last.CreatedAt, last.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rest) != 1 {
		t.Fatalf("second page = %d rows, want the remaining 1", len(rest))
	}
	if len(rest[0].Properties) == 0 {
		t.Fatal("properties should carry through for the destination shapers")
	}
}

// The Postgres LATERAL matched a goal to an exposure on `anon OR customer` plus a time bound - this pins the actor-key rewrite that replaces it, including that a goal firing BEFORE the exposure doesn't count and doesn't drop the exposure
func TestExperimentResultsMatchesGoalsAcrossIdentity(t *testing.T) {
	ctx := context.Background()
	st := testStore(t)
	ch, cust := uuid.New(), uuid.New()
	anon := "anon-" + uuid.NewString()
	key := "exp-" + uuid.NewString()
	t0 := time.Now().UTC().Truncate(24 * time.Hour).Add(10 * time.Hour)

	add := func(name, source string, when time.Time, cid *uuid.UUID, props map[string]any) {
		if _, err := st.AppendEvent(ctx, analytics.SpineEvent{
			EventName: name, OccurredAt: when, ChannelID: &ch, CustomerID: cid, AnonID: anon,
			Source: source, Props: props, DedupeKey: "exp:" + uuid.NewString(),
		}); err != nil {
			t.Fatal(err)
		}
	}
	// a goal BEFORE the exposure must not convert it, but must not lose it either
	add("order_placed", "commerce", t0.Add(-time.Hour), &cust, map[string]any{"revenue_cents": 999})
	add("custom", "web", t0, nil, map[string]any{"name": "$experiment", "experiment": key, "variation": "a"})
	// the converting goal arrives customer-keyed, so only identity resolution bridges it
	add("order_placed", "commerce", t0.Add(time.Hour), &cust, map[string]any{"revenue_cents": 2500})
	if err := st.flush(ctx); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertIdentity(ctx, anon, cust, ch); err != nil {
		t.Fatal(err)
	}
	if err := st.conn.Exec(ctx, "SYSTEM RELOAD DICTIONARY "+identityDictName); err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	res, err := st.ExperimentResults(ctx, ch, key, "order_placed", t0.Add(-24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res[0].Variation != "a" {
		t.Fatalf("results = %+v, want one row for variation a", res)
	}
	got := res[0]
	if got.Exposed != 1 || got.Converted != 1 {
		t.Fatalf("exposed/converted = %d/%d, want 1/1", got.Exposed, got.Converted)
	}
	if got.Revenue != 2500 {
		t.Fatalf("revenue = %d, want only the post-exposure 2500", got.Revenue)
	}
}

// Smoke-runs every ported Lens read against a real ClickHouse - these translate percentile_cont, regex guards and COUNT(DISTINCT) by hand, so the point is that they parse and bind at all, plus the two whose values are worth asserting
func TestLensReadsRunAndAggregate(t *testing.T) {
	ctx := context.Background()
	st := testStore(t)
	ch := uuid.New()
	page := "/p-" + uuid.NewString()
	at := time.Now().UTC().Truncate(24 * time.Hour).Add(11 * time.Hour)

	add := func(name string, props map[string]any) {
		if _, err := st.AppendEvent(ctx, analytics.SpineEvent{
			EventName: name, OccurredAt: at, ChannelID: &ch, AnonID: "lens-" + uuid.NewString(),
			Source: "web", Props: props, DedupeKey: "lens:" + uuid.NewString(),
		}); err != nil {
			t.Fatal(err)
		}
	}
	for range 3 {
		add("page_view", map[string]any{"page_key": page, "referrer": "https://ref.example", "client": "Chrome", "device": "desktop"})
	}
	add("custom", map[string]any{"name": "$rage_click", "selector": ".buy", "text": "Buy"})
	add("custom", map[string]any{"name": "$js_error", "message": "boom", "page_key": page})
	for _, v := range []string{"10", "20", "30", "40"} {
		add("custom", map[string]any{"name": "$web_vitals", "metric": "LCP", "value": v, "page_key": page})
	}
	if err := st.flush(ctx); err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	since := at.Add(-24 * time.Hour)
	views, visitors, err := st.LensPageTotals(ctx, ch, page)
	if err != nil || views != 3 || visitors != 3 {
		t.Fatalf("PageTotals = %d/%d err=%v, want 3/3", views, visitors, err)
	}
	for name, run := range map[string]func() error{
		"Referrers":   func() error { _, e := st.LensPageReferrers(ctx, ch, page); return e },
		"Clients":     func() error { _, e := st.LensPageClients(ctx, ch, page); return e },
		"DeviceSplit": func() error { _, e := st.LensPageDeviceSplit(ctx, ch, page); return e },
		"Timeline":    func() error { _, e := st.LensPageTimeline(ctx, ch, page, since); return e },
		"ListPages":   func() error { _, e := st.LensListPages(ctx, ch); return e },
		"Rage":        func() error { _, e := st.LensTopRageElements(ctx, ch, since, 10); return e },
		"Errors":      func() error { _, e := st.LensTopErrors(ctx, ch, since, 10); return e },
		"VitalsPage":  func() error { _, e := st.LensVitalsByPage(ctx, ch, since); return e },
		"Signals":     func() error { _, e := st.LensSessionSignals(ctx, ch, "sid", since, at.Add(time.Hour)); return e },
	} {
		if err := run(); err != nil {
			t.Fatalf("Lens%s: %v", name, err)
		}
	}
	// p75 of 10/20/30/40 interpolates to 32.5 under percentile_cont, which quantileExactInclusive matches and the non-interpolating quantileExact would not
	overall, err := st.LensVitalsOverall(ctx, ch, since)
	if err != nil || len(overall) != 1 {
		t.Fatalf("VitalsOverall = %+v err=%v, want one metric", overall, err)
	}
	if overall[0].Metric != "LCP" || overall[0].Samples != 4 || overall[0].P75 != 32.5 {
		t.Fatalf("vital = %+v, want LCP p75 32.5 over 4 samples", overall[0])
	}
}

// A segment rule's "ordered in the last N days" and trait compute's lifetime RFM inputs are one read with and
// without a window - the window has to cut on placement alone, and a cancellation has to void its order either way
func TestChannelOrderAggregatesWindow(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	ch, cust := uuid.New(), uuid.New()
	now := time.Now().UTC()
	add := func(name, orderID string, at time.Time, cents int64) {
		t.Helper()
		if _, err := st.AppendEvent(ctx, analytics.SpineEvent{
			EventName: name, OccurredAt: at, ChannelID: &ch, CustomerID: &cust, Source: "commerce",
			Props:     map[string]any{"order_id": orderID, "total_cents": cents}, // a number, as core sends it
			DedupeKey: "agg:" + name + ":" + orderID,
		}); err != nil {
			t.Fatal(err)
		}
	}
	add("order_placed", "old", now.Add(-200*24*time.Hour), 70000)
	add("order_placed", "recent", now.Add(-5*24*time.Hour), 30000)
	add("order_placed", "voided", now.Add(-3*24*time.Hour), 500000)
	add("order_cancelled", "voided", now.Add(-time.Hour), 500000)
	if err := st.flush(ctx); err != nil {
		t.Fatal(err)
	}

	find := func(rows []analytics.CustomerOrders) (analytics.CustomerOrders, bool) {
		for _, r := range rows {
			if r.CustomerID == cust {
				return r, true
			}
		}
		return analytics.CustomerOrders{}, false
	}
	lifetime, err := st.ChannelOrderAggregates(ctx, ch, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := find(lifetime); !ok || got.OrderCount != 2 || got.MonetaryCents != 100000 {
		t.Errorf("lifetime = %+v (found %v), want 2 orders for 100000 with the cancelled one voided", got, ok)
	}
	windowed, err := st.ChannelOrderAggregates(ctx, ch, now.Add(-30*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := find(windowed); !ok || got.OrderCount != 1 || got.MonetaryCents != 30000 {
		t.Errorf("last 30 days = %+v (found %v), want the one recent order for 30000", got, ok)
	}
	nothing, err := st.ChannelOrderAggregates(ctx, ch, now.Add(-24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := find(nothing); ok {
		t.Error("a window after every placement still returned the customer")
	}
}

func TestAVisitorProfileShowsOnlyTheAskedChannelsActivity(t *testing.T) {
	ctx := context.Background()
	st := testStore(t)
	defer st.Close()
	mine, theirs, anon := uuid.New(), uuid.New(), "anon-"+uuid.NewString()
	at := time.Now().UTC()
	for _, ch := range []uuid.UUID{mine, theirs, theirs} {
		if _, err := st.AppendEvent(ctx, analytics.SpineEvent{
			EventName: "page_view", OccurredAt: at, ChannelID: &ch, AnonID: anon, Source: "web",
			Props: map[string]any{"title": "Page", "page_key": "/", "ip": "203.0.113.9"}, DedupeKey: "pv:" + uuid.NewString(),
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.flush(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := st.VisitorProfile(ctx, mine, anon, at.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if p.TotalEvents != 1 || len(p.Timeline) != 1 {
		t.Fatalf("profile on one channel counted %d events, %d timeline rows, want only that channel's 1 - another shop's visits leaked", p.TotalEvents, len(p.Timeline))
	}
	if len(p.IPs) != 1 || p.IPs[0].Count != 1 {
		t.Errorf("IPs = %+v, want the one seen on this channel", p.IPs)
	}
}
