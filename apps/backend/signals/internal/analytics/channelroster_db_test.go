package analytics

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	signalsdb "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

// Gated on a disposable marker - the roster is what the beacon surface resolves every request
// against, so what a push leaves behind decides which channels are served and which go dark

func newRoster(t *testing.T) *ChannelRoster {
	t.Helper()
	conn := testdb.RequireSignals(t)
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM channel_roster`) })
	if _, err := conn.Exec(`DELETE FROM channel_roster`); err != nil {
		t.Fatalf("clear roster: %v", err)
	}
	return NewChannelRoster(conn)
}

func TestAChannelIsServedOnlyOnceThePushCarriesIt(t *testing.T) {
	r := newRoster(t)
	ctx := context.Background()
	ch := uuid.New()

	if r.Active(ctx, ch) {
		t.Error("a channel no push has carried is being served")
	}
	if _, _, err := r.SyncChannels(ctx, []ChannelEntry{{ID: ch, IsActive: true}}); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if !r.Active(ctx, ch) {
		t.Error("a pushed channel is not being served")
	}
}

// A channel an operator retires has to stop being served, and the push says so by leaving it out -
// without the sweep its beacons would keep landing on the spine forever
func TestAChannelDroppedFromThePushStopsBeingServed(t *testing.T) {
	r := newRoster(t)
	ctx := context.Background()
	kept, retired := uuid.New(), uuid.New()

	if _, _, err := r.SyncChannels(ctx, []ChannelEntry{{ID: kept, IsActive: true}, {ID: retired, IsActive: true}}); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	synced, pruned, err := r.SyncChannels(ctx, []ChannelEntry{{ID: kept, IsActive: true}})
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if synced != 1 || pruned != 1 {
		t.Errorf("synced %d pruned %d, want 1 and 1", synced, pruned)
	}
	if r.Active(ctx, retired) {
		t.Error("a channel the push left out is still being served")
	}
	if !r.Active(ctx, kept) {
		t.Error("the sweep took a channel the push did carry")
	}
}

// A beacon with no channel of its own lands on the default store, so which channel carries the flag
// is what decides where that traffic is recorded
func TestTheDefaultIsTheChannelThePushFlagged(t *testing.T) {
	r := newRoster(t)
	ctx := context.Background()
	plain, def := uuid.New(), uuid.New()

	if _, _, err := r.SyncChannels(ctx, []ChannelEntry{{ID: plain, IsActive: true}, {ID: def, IsActive: true, IsDefault: true}}); err != nil {
		t.Fatalf("sync: %v", err)
	}
	got, ok := r.Default(ctx)
	if !ok || got != def {
		t.Errorf("Default = %s (ok=%t), want %s", got, ok, def)
	}
}

func TestAnEmptyRosterHasNoDefault(t *testing.T) {
	r := newRoster(t)

	if got, ok := r.Default(context.Background()); ok {
		t.Errorf("Default = %s on an empty roster, want none", got)
	}
}

// The set is cached because a query per beacon is the thing this exists to avoid, so a push has to
// drop that cache - otherwise a channel switched on stays dark for the TTL and one retired keeps serving
func TestAPushIsVisibleWithoutWaitingOutTheCache(t *testing.T) {
	r := newRoster(t)
	ctx := context.Background()
	first, second := uuid.New(), uuid.New()

	if _, _, err := r.SyncChannels(ctx, []ChannelEntry{{ID: first, IsActive: true}}); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	if !r.Active(ctx, first) { // loads and caches the set
		t.Fatal("the first channel is not being served")
	}
	if _, _, err := r.SyncChannels(ctx, []ChannelEntry{{ID: first, IsActive: true}, {ID: second, IsActive: true}}); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if !r.Active(ctx, second) {
		t.Error("a channel added by a push is not served until the cache times out")
	}
}

// A message sent from the suite has to look like the shop sent it, and the suite cannot read core's
// config to find out - so whatever the push carried is the whole of what it has to go on
func TestThePushCarriesTheIdentityASendGoesOutUnder(t *testing.T) {
	r := newRoster(t)
	ctx := context.Background()
	ch := uuid.New()

	if _, _, err := r.SyncChannels(ctx, []ChannelEntry{{
		ID: ch, IsDefault: true, Name: "Vetrina", PublicURL: "https://vetrina.test",
		FromAddress: "hello@vetrina.test", FromName: "Vetrina Store", LogoURL: "https://cdn.test/logo.png",
	}}); err != nil {
		t.Fatalf("sync: %v", err)
	}

	got, ok := r.Entry(ctx, ch)
	if !ok {
		t.Fatal("the channel the push carried is not on the roster")
	}
	for _, c := range []struct{ field, got, want string }{
		{"name", got.Name, "Vetrina"},
		{"public url", got.PublicURL, "https://vetrina.test"},
		{"from address", got.FromAddress, "hello@vetrina.test"},
		{"from name", got.FromName, "Vetrina Store"},
		{"logo url", got.LogoURL, "https://cdn.test/logo.png"},
	} {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.field, c.got, c.want)
		}
	}
}

// An operator changing the from-address must not wait out the cache, for the same reason switching
// a channel on does not - and a channel the roster never heard of has no identity to offer
func TestAChangedIdentityIsVisibleImmediately(t *testing.T) {
	r := newRoster(t)
	ctx := context.Background()
	ch := uuid.New()

	if _, ok := r.Entry(ctx, uuid.New()); ok {
		t.Error("a channel never pushed came back with an identity")
	}
	if _, _, err := r.SyncChannels(ctx, []ChannelEntry{{ID: ch, IsActive: true, FromAddress: "old@shop.test"}}); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if got, _ := r.Entry(ctx, ch); got.FromAddress != "old@shop.test" {
		t.Fatalf("from address = %q before the change, want old@shop.test", got.FromAddress)
	}

	if _, _, err := r.SyncChannels(ctx, []ChannelEntry{{ID: ch, IsActive: true, FromAddress: "new@shop.test"}}); err != nil {
		t.Fatalf("resync: %v", err)
	}
	if got, _ := r.Entry(ctx, ch); got.FromAddress != "new@shop.test" {
		t.Errorf("from address = %q, want the pushed change rather than the cached answer", got.FromAddress)
	}
}

func TestTraitsAreComputedStalestChannelFirst(t *testing.T) {
	conn := testdb.RequireSignals(t)
	ctx := context.Background()
	qs := signalsdb.New(conn)
	fresh, stale, never := uuid.New(), uuid.New(), uuid.New()
	if _, _, err := NewChannelRoster(conn).SyncChannels(ctx, []ChannelEntry{{ID: fresh, IsActive: true}, {ID: stale, IsActive: true}, {ID: never, IsActive: true}}); err != nil {
		t.Fatal(err)
	}
	r := NewRoster(qs)
	now := time.Now()
	if err := r.MarkTraitsComputed(ctx, fresh, now); err != nil {
		t.Fatal(err)
	}
	if err := r.MarkTraitsComputed(ctx, stale, now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	got, err := r.TraitComputeChannels(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0] != never || got[1] != stale || got[2] != fresh {
		t.Fatalf("order = %v, want never-computed, then stale, then fresh", got)
	}
}

func TestAChannelThatNeverFinishesStopsLeadingTheTraitRun(t *testing.T) {
	conn := testdb.RequireSignals(t)
	ctx := context.Background()
	slow, other := uuid.New(), uuid.New()
	if _, _, err := NewChannelRoster(conn).SyncChannels(ctx, []ChannelEntry{{ID: slow, IsActive: true}, {ID: other, IsActive: true}}); err != nil {
		t.Fatal(err)
	}
	r := NewRoster(signalsdb.New(conn))
	now := time.Now()
	if err := r.MarkTraitsComputed(ctx, other, now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := r.MarkTraitsAttempted(ctx, slow, now); err != nil {
		t.Fatal(err)
	}
	got, err := r.TraitComputeChannels(ctx)
	if err != nil {
		t.Fatal(err)
	}
	at := map[uuid.UUID]int{}
	for i, id := range got {
		at[id] = i
	}
	if at[slow] < at[other] {
		t.Fatalf("order = %v, want the channel just attempted behind the one computed an hour ago", got)
	}
}
