// Package seed generates deterministic demo activity for the spine - every board area (shop health, revenue and
// repeat, product usage, email impact) gets data, and the same seed and channel always give the same events
// Every row it writes is marked: dedupe keys start with KeyPrefix and visitor ids with AnonPrefix, which is how purge finds them
package seed

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
)

const (
	KeyPrefix  = "seed:"
	AnonPrefix = "seed-"
)

type Config struct {
	Seed      uint64
	Now       time.Time
	Days      int // how far back the activity reaches
	Customers int // known customers per shop; visitors who never sign in come on top
	Events    int // 0 = the natural amount for Customers, otherwise keep adding visitors until this many events (the load set)
}

// One shape per shop, picked from the channel id, so two shops on one backend don't look the same
type profile struct {
	name     string
	products []product
	currency string
	buyRate  float64 // share of sessions with a cart that go on to order
	repeat   float64 // chance a buyer comes back for another order
}

type product struct {
	sku, name string
	cents     int64
}

var profiles = []profile{
	{name: "apparel", currency: "EUR", buyRate: 0.35, repeat: 0.45, products: []product{
		{"TEE-BLK-M", "Black tee", 2500}, {"TEE-WHT-M", "White tee", 2500}, {"HOOD-GRY-L", "Grey hoodie", 6900},
		{"JEAN-IND-32", "Indigo jeans", 8900}, {"SOCK-3PK", "Socks, 3 pack", 1500}, {"CAP-NVY", "Navy cap", 2900},
	}},
	{name: "electronics", currency: "USD", buyRate: 0.2, repeat: 0.2, products: []product{
		{"HP-NC-200", "Noise-cancelling headphones", 19900}, {"CBL-USBC-2M", "USB-C cable 2 m", 1900},
		{"CHG-65W", "65 W charger", 4900}, {"SPK-MINI", "Mini speaker", 7900}, {"KB-MECH", "Mechanical keyboard", 12900},
	}},
	{name: "grocery", currency: "EUR", buyRate: 0.55, repeat: 0.7, products: []product{
		{"COF-1KG", "Coffee beans 1 kg", 2400}, {"TEA-GRN", "Green tea", 800}, {"OIL-OLV-1L", "Olive oil 1 l", 1300},
		{"HNY-500", "Honey 500 g", 900}, {"PST-500", "Pasta 500 g", 300}, {"CHC-DRK", "Dark chocolate", 450},
	}},
}

// Sink receives the generated rows - the spine store in the command, a recorder in tests
type Sink interface {
	Event(analytics.SpineEvent) error
	Identify(anonID string, customerID, channelID uuid.UUID) error
}

type gen struct {
	cfg     Config
	ch      uuid.UUID
	p       profile
	r       *rand.Rand
	sink    Sink
	n       int // events emitted, also the dedupe counter
	orders  int
	start   time.Time
	cids    []uuid.UUID
	sending []uuid.UUID // one campaign per week, so email impact has campaigns to compare
}

// Generate writes one shop's activity and reports how many events it emitted
func Generate(channelID uuid.UUID, cfg Config, sink Sink) (int, error) {
	if cfg.Days <= 0 {
		cfg.Days = 90
	}
	if cfg.Customers <= 0 {
		cfg.Customers = 200
	}
	h := sha256.Sum256(append(binary.BigEndian.AppendUint64(nil, cfg.Seed), channelID[:]...))
	g := &gen{
		cfg: cfg, ch: channelID, sink: sink,
		r:     rand.New(rand.NewPCG(binary.BigEndian.Uint64(h[:8]), binary.BigEndian.Uint64(h[8:16]))),
		p:     profiles[h[16]%byte(len(profiles))],
		start: cfg.Now.Add(-time.Duration(cfg.Days) * 24 * time.Hour),
	}
	for range cfg.Days/7 + 1 {
		g.sending = append(g.sending, g.uuid())
	}
	err := g.run()
	return g.n, err
}

func (g *gen) run() error {
	for i := range g.cfg.Customers {
		cid := g.uuid()
		g.cids = append(g.cids, cid)
		devices := 1
		if i%10 == 0 { // phone and laptop - two visitor ids stitched to one customer
			devices = 2
		}
		for d := range devices {
			if err := g.customer(cid, g.anon(), d == 0); err != nil {
				return err
			}
		}
	}
	// A shared device: one visitor id signed into by two customers in turn, the case identity stitching has to get right
	if len(g.cids) >= 2 {
		shared := g.anon()
		for _, cid := range g.cids[:2] {
			if err := g.visit(&cid, shared, g.at(), true); err != nil {
				return err
			}
		}
	}
	for v := 0; g.moreVisitors(v); v++ {
		if err := g.visit(nil, g.anon(), g.at(), false); err != nil {
			return err
		}
	}
	return nil
}

// Three anonymous visitors per customer by default, or as many as it takes to reach the load set's event count
func (g *gen) moreVisitors(v int) bool {
	if g.cfg.Events > 0 {
		return g.n < g.cfg.Events
	}
	return v < g.cfg.Customers*3
}

// A customer's life: sessions spread over the window, email when they're subscribed, a reorder when they repeat
func (g *gen) customer(cid uuid.UUID, anon string, emails bool) error {
	sessions := 1 + g.r.IntN(4)
	for range sessions {
		if err := g.visit(&cid, anon, g.at(), true); err != nil {
			return err
		}
	}
	if !emails {
		return nil
	}
	return g.email(cid, anon)
}

// One session: browse, maybe cart, maybe check out and order. A tenth are bots and a sixth refused analytics (counted as nobody)
func (g *gen) visit(cid *uuid.UUID, anon string, at time.Time, signIn bool) error {
	if g.r.Float64() < 0.1 {
		return g.web("page_view", nil, anon, at, map[string]any{"page_key": "/", "title": "Home", "is_bot": true, "bot_name": "Googlebot", "bot_category": "search"})
	}
	if cid == nil && g.r.Float64() < 0.16 {
		return g.aggregate(at)
	}
	var actor *uuid.UUID
	if signIn {
		actor = cid
		if err := g.sink.Identify(anon, *cid, g.ch); err != nil {
			return err
		}
		if err := g.web(analytics.EventIdentify, cid, anon, at, nil); err != nil {
			return err
		}
	}
	at = at.Add(time.Second)
	if err := g.web("page_view", actor, anon, at, map[string]any{"page_key": "/", "title": "Home", "referrer": g.pick("", "https://www.google.com/", "https://www.instagram.com/"), "device": g.pick("mobile", "desktop", "tablet")}); err != nil {
		return err
	}
	var cart []product
	for range 1 + g.r.IntN(4) {
		pr := g.p.products[g.r.IntN(len(g.p.products))]
		at = at.Add(time.Duration(10+g.r.IntN(120)) * time.Second)
		props := map[string]any{"page_key": "/p/" + pr.sku, "title": pr.name, "entity": "product", "entity_name": pr.name, "entity_price": pr.cents, "sku": pr.sku}
		if err := g.web("page_view", actor, anon, at, props); err != nil {
			return err
		}
		if err := g.web(analytics.EventViewItem, actor, anon, at, map[string]any{"name": analytics.EventViewItem, "sku": pr.sku, "page_key": "/p/" + pr.sku}); err != nil {
			return err
		}
		if g.r.Float64() < 0.3 {
			cart = append(cart, pr)
			if err := g.commerce("cart_add", actor, anon, at.Add(5*time.Second), map[string]any{"sku": pr.sku, "qty": 1, "unit_price": pr.cents}); err != nil {
				return err
			}
		}
	}
	if len(cart) == 0 || g.r.Float64() > g.p.buyRate*2 {
		return nil
	}
	at = at.Add(time.Minute)
	if err := g.web(analytics.EventBeginCheckout, actor, anon, at, map[string]any{"name": analytics.EventBeginCheckout, "page_key": "/checkout"}); err != nil {
		return err
	}
	if g.r.Float64() > 0.5 {
		return nil
	}
	if err := g.order(actor, anon, at.Add(3*time.Minute), cart); err != nil {
		return err
	}
	if actor != nil && g.r.Float64() < g.p.repeat {
		later := at.Add(time.Duration(7+g.r.IntN(30)) * 24 * time.Hour)
		if later.Before(g.cfg.Now) {
			return g.order(actor, anon, later, cart[:1])
		}
	}
	return nil
}

func (g *gen) order(cid *uuid.UUID, anon string, at time.Time, lines []product) error {
	var total int64
	for _, l := range lines {
		total += l.cents
	}
	g.orders++
	props := map[string]any{"order_id": g.uuid().String(), "order_number": fmt.Sprintf("SEED-%05d", g.orders), "total_cents": total, "currency": g.p.currency}
	if err := g.commerce("order_placed", cid, anon, at, props); err != nil {
		return err
	}
	if cid == nil || g.r.Float64() < 0.5 {
		return nil
	}
	return g.emit(analytics.SpineEvent{EventName: analytics.EventConversion, OccurredAt: at, CustomerID: cid, Source: analytics.SourceCampaign,
		SendID: g.sendID(at), Props: map[string]any{"order_id": props["order_id"], "revenue_cents": total, "currency": g.p.currency, "via": analytics.EventEmailClicked, "campaign_id": g.campaign(at).String()}})
}

// Sent, mostly opened, sometimes clicked - one per weekly campaign the customer was around for
func (g *gen) email(cid uuid.UUID, anon string) error {
	for w := range g.sending {
		at := g.start.Add(time.Duration(w*7*24)*time.Hour + 10*time.Hour)
		if at.After(g.cfg.Now) || g.r.Float64() < 0.3 {
			continue
		}
		send := g.uuid()
		props := func() map[string]any { return map[string]any{"campaign_id": g.sending[w].String()} }
		if err := g.emit(analytics.SpineEvent{EventName: analytics.EventEmailSent, OccurredAt: at, CustomerID: &cid, Source: analytics.SourceCampaign, SendID: &send, Props: props()}); err != nil {
			return err
		}
		if g.r.Float64() < 0.45 {
			continue
		}
		at = at.Add(time.Duration(1+g.r.IntN(20)) * time.Hour)
		if err := g.emit(analytics.SpineEvent{EventName: analytics.EventEmailOpened, OccurredAt: at, CustomerID: &cid, Source: analytics.SourceCampaign, SendID: &send, Props: props()}); err != nil {
			return err
		}
		if g.r.Float64() < 0.75 {
			continue
		}
		if err := g.emit(analytics.SpineEvent{EventName: analytics.EventEmailClicked, OccurredAt: at.Add(time.Minute), CustomerID: &cid, Source: analytics.SourceCampaign, SendID: &send, Props: props()}); err != nil {
			return err
		}
		if err := g.visit(&cid, anon, at.Add(2*time.Minute), true); err != nil {
			return err
		}
	}
	return nil
}

// A visitor who refused analytics: page view, sometimes a cart add and an order, with no actor at all
func (g *gen) aggregate(at time.Time) error {
	pr := g.p.products[g.r.IntN(len(g.p.products))]
	if err := g.emit(analytics.SpineEvent{EventName: "page_view", OccurredAt: at, Source: analytics.SourceAggregate, Props: map[string]any{"path": "/p/" + pr.sku}}); err != nil {
		return err
	}
	if g.r.Float64() < 0.2 {
		return g.emit(analytics.SpineEvent{EventName: "order_placed", OccurredAt: at.Add(5 * time.Minute), Source: analytics.SourceAggregate, Props: map[string]any{"total_cents": pr.cents, "currency": g.p.currency}})
	}
	return nil
}

func (g *gen) web(name string, cid *uuid.UUID, anon string, at time.Time, props map[string]any) error {
	return g.emit(analytics.SpineEvent{EventName: name, OccurredAt: at, CustomerID: cid, AnonID: anon, Source: analytics.SourceWeb, Props: props})
}

func (g *gen) commerce(name string, cid *uuid.UUID, anon string, at time.Time, props map[string]any) error {
	e := analytics.SpineEvent{EventName: name, OccurredAt: at, CustomerID: cid, Source: analytics.SourceCommerce, Props: props}
	if cid == nil {
		e.AnonID = anon
	}
	return g.emit(e)
}

func (g *gen) emit(e analytics.SpineEvent) error {
	if e.OccurredAt.After(g.cfg.Now) {
		return nil
	}
	g.n++
	ch := g.ch
	e.ChannelID = &ch
	e.DedupeKey = fmt.Sprintf("%s%s:%d", KeyPrefix, g.ch, g.n) // the channel keeps keys apart, since the store dedupes on the key alone
	if e.Props == nil {
		e.Props = map[string]any{}
	}
	return g.sink.Event(e)
}

func (g *gen) at() time.Time {
	return g.start.Add(time.Duration(g.r.Int64N(int64(g.cfg.Now.Sub(g.start)))))
}

func (g *gen) week(at time.Time) int {
	return min(int(at.Sub(g.start)/(7*24*time.Hour)), len(g.sending)-1)
}

func (g *gen) campaign(at time.Time) uuid.UUID { return g.sending[g.week(at)] }

func (g *gen) sendID(at time.Time) *uuid.UUID {
	id := uuid.NewSHA1(g.campaign(at), []byte(at.String()))
	return &id
}

func (g *gen) uuid() uuid.UUID {
	var b [16]byte
	binary.BigEndian.PutUint64(b[:8], g.r.Uint64())
	binary.BigEndian.PutUint64(b[8:], g.r.Uint64())
	b[6], b[8] = b[6]&0x0f|0x40, b[8]&0x3f|0x80 // a valid v4 shape
	return uuid.UUID(b)
}

func (g *gen) anon() string { return AnonPrefix + g.uuid().String() }

func (g *gen) pick(opts ...string) string { return opts[g.r.IntN(len(opts))] }
