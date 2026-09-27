package wiring

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/signalsclient"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

type perChannelInts map[uuid.UUID]map[string]int64

func (p perChannelInts) GetInt(_ context.Context, path string, scope config.Scope) (int64, error) {
	if scope.ChannelID == nil {
		return 0, nil
	}
	return p[*scope.ChannelID][path], nil
}

type channelCarts struct {
	byChannel map[uuid.UUID][]cart.AbandonedCart
	asked     map[uuid.UUID]time.Time
	marked    []uuid.UUID
}

func (c *channelCarts) ListAbandoned(_ context.Context, ch uuid.UUID, threshold, _ time.Time, limit int32) ([]cart.AbandonedCart, error) {
	c.asked[ch] = threshold
	list := c.byChannel[ch]
	if len(list) > int(limit) {
		list = list[:limit]
	}
	return list, nil
}

func (c *channelCarts) MarkAbandonedNotified(_ context.Context, ids []uuid.UUID) error {
	c.marked = append(c.marked, ids...)
	return nil
}

type enrolled struct{ perChannel map[uuid.UUID]int }

func (e *enrolled) OnCartAbandoned(_ context.Context, ch, _ uuid.UUID, _ string, _ uuid.UUID, _ time.Time) error {
	e.perChannel[ch]++
	return nil
}

func (e *enrolled) OnBrowseAbandoned(_ context.Context, ch, _ uuid.UUID, _, _, _, _ string, _ time.Time) error {
	e.perChannel[ch]++
	return nil
}

func TestABusyChannelCannotStarveAnotherOfCartRecovery(t *testing.T) {
	busy, quiet := uuid.New(), uuid.New()
	carts := &channelCarts{byChannel: map[uuid.UUID][]cart.AbandonedCart{}, asked: map[uuid.UUID]time.Time{}}
	for range abandonmentPerChannel + 50 {
		carts.byChannel[busy] = append(carts.byChannel[busy], cart.AbandonedCart{ID: uuid.New(), ChannelID: busy, Email: "a@b.test"})
	}
	carts.byChannel[quiet] = []cart.AbandonedCart{{ID: uuid.New(), ChannelID: quiet, Email: "q@b.test"}}
	cfg := perChannelInts{quiet: {"outreach/triggers/abandoned_cart_minutes": 10}}
	got := &enrolled{perChannel: map[uuid.UUID]int{}}
	now := time.Now()

	if err := runCartAbandonment(context.Background(), []uuid.UUID{busy, quiet}, cfg, carts, got, now); err != nil {
		t.Fatal(err)
	}
	if got.perChannel[quiet] != 1 || got.perChannel[busy] != abandonmentPerChannel {
		t.Fatalf("enrolled %v, want the quiet channel's cart and the busy one capped at %d", got.perChannel, abandonmentPerChannel)
	}
	if want := now.Add(-10 * time.Minute); !carts.asked[quiet].Equal(want) {
		t.Errorf("quiet channel asked with threshold %v, want its own 10 minute setting (%v)", carts.asked[quiet], want)
	}
	if want := now.Add(-60 * time.Minute); !carts.asked[busy].Equal(want) {
		t.Errorf("busy channel asked with threshold %v, want the 60 minute default", carts.asked[busy])
	}
}

type browseFeed struct {
	cands  []signalsclient.BrowseAbandoner
	marked int
	asked  map[uuid.UUID]int32
}

func (b *browseFeed) BrowseAbandoners(_ context.Context, ch uuid.UUID, _, _ time.Time, limit int32) ([]signalsclient.BrowseAbandoner, error) {
	if b.asked == nil {
		b.asked = map[uuid.UUID]int32{}
	}
	b.asked[ch] = limit
	var out []signalsclient.BrowseAbandoner
	for _, c := range b.cands {
		if c.ChannelID == ch && int32(len(out)) < limit {
			out = append(out, c)
		}
	}
	return out, nil
}

func (b *browseFeed) MarkBrowseNotified(context.Context, uuid.UUID, uuid.UUID, time.Time) error {
	b.marked++
	return nil
}

type noExclusions struct{}

func (noExclusions) BrowseAbandonersExcluded(context.Context, db.BrowseAbandonersExcludedParams) ([]uuid.UUID, error) {
	return nil, nil
}

func TestBrowseRecoveryHonoursEachChannelsWindowAndCap(t *testing.T) {
	busy, strict := uuid.New(), uuid.New()
	now := time.Now()
	feed := &browseFeed{}
	for range abandonmentPerChannel + 20 {
		feed.cands = append(feed.cands, signalsclient.BrowseAbandoner{CustomerID: uuid.New(), ChannelID: busy, LastBrowseAt: now.Add(-3 * time.Hour)})
	}
	feed.cands = append(feed.cands,
		signalsclient.BrowseAbandoner{CustomerID: uuid.New(), ChannelID: strict, LastBrowseAt: now.Add(-3 * time.Hour)},
		signalsclient.BrowseAbandoner{CustomerID: uuid.New(), ChannelID: strict, LastBrowseAt: now.Add(-5 * time.Hour)},
	)
	cfg := perChannelInts{strict: {"outreach/triggers/browse_abandoned_max_age_hours": 4}}
	got := &enrolled{perChannel: map[uuid.UUID]int{}}

	if err := runBrowseAbandonment(context.Background(), []uuid.UUID{busy, strict}, cfg, feed, noExclusions{}, got, now); err != nil {
		t.Fatal(err)
	}
	if got.perChannel[busy] != abandonmentPerChannel || got.perChannel[strict] != 1 {
		t.Fatalf("enrolled %v, want busy capped at %d and strict's one browse inside its 4h window", got.perChannel, abandonmentPerChannel)
	}
	if len(feed.asked) != 2 || feed.asked[strict] != abandonmentPerChannel {
		t.Fatalf("asked the suite %v, want each channel for its own page - a shared fetch lets the busy store's backlog crowd out the rest", feed.asked)
	}
}

func TestAbandonmentWindowsCanBeSetPerChannel(t *testing.T) {
	for _, path := range []string{
		"outreach/triggers/abandoned_cart_minutes", "outreach/triggers/abandoned_cart_max_age_hours",
		"outreach/triggers/browse_abandoned_minutes", "outreach/triggers/browse_abandoned_max_age_hours",
	} {
		if !config.ScopingAllows(path, config.ScopeChannel, false) {
			t.Errorf("%s refuses a per-channel value, though the job reads it per channel", path)
		}
	}
}

type failingOn struct {
	enrolled
	bad uuid.UUID
}

func (f *failingOn) OnCartAbandoned(ctx context.Context, ch, cust uuid.UUID, to string, id uuid.UUID, at time.Time) error {
	if ch == f.bad {
		return errors.New("suite down")
	}
	return f.enrolled.OnCartAbandoned(ctx, ch, cust, to, id, at)
}

func (f *failingOn) OnBrowseAbandoned(ctx context.Context, ch, cust uuid.UUID, a, b, c, d string, at time.Time) error {
	if ch == f.bad {
		return errors.New("suite down")
	}
	return f.enrolled.OnBrowseAbandoned(ctx, ch, cust, a, b, c, d, at)
}

func TestOneChannelsFailureDoesNotStarveTheNext(t *testing.T) {
	bad, good := uuid.New(), uuid.New()
	now := time.Now()
	carts := &channelCarts{byChannel: map[uuid.UUID][]cart.AbandonedCart{
		bad:  {{ID: uuid.New(), ChannelID: bad}},
		good: {{ID: uuid.New(), ChannelID: good}},
	}, asked: map[uuid.UUID]time.Time{}}
	flows := &failingOn{perChannel: map[uuid.UUID]int{}, bad: bad}
	err := runCartAbandonment(context.Background(), []uuid.UUID{bad, good}, perChannelInts{}, carts, flows, now)
	if err == nil || !strings.Contains(err.Error(), bad.String()) || flows.perChannel[good] != 1 {
		t.Fatalf("cart: err %v, enrolled %v, want the failure reported and the next channel still served", err, flows.perChannel)
	}

	feed := &browseFeed{cands: []signalsclient.BrowseAbandoner{
		{CustomerID: uuid.New(), ChannelID: bad, LastBrowseAt: now.Add(-3 * time.Hour)},
		{CustomerID: uuid.New(), ChannelID: good, LastBrowseAt: now.Add(-3 * time.Hour)},
	}}
	flows.perChannel = map[uuid.UUID]int{}
	err = runBrowseAbandonment(context.Background(), []uuid.UUID{bad, good}, perChannelInts{}, feed, noExclusions{}, flows, now)
	if err == nil || !strings.Contains(err.Error(), bad.String()) || flows.perChannel[good] != 1 {
		t.Fatalf("browse: err %v, enrolled %v, want the failure reported and the next channel still served", err, flows.perChannel)
	}
}
