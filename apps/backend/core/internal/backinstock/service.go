// Package backinstock powers "notify me when it's back" - shoppers register interest in an out-of-stock SKU and are told when it's restocked.
//
// Notification takes one of two paths per requester, so nobody who asked is missed: a resolved customer with an active back_in_stock flow is enrolled into it for rich operator-controlled content, and everyone else gets a direct built-in email.
//
// It's a solicited one-time notification the shopper explicitly requested, so it sends without marketing consent - but it still honours the suppression list, never mailing a hard-bounced or complained address.
package backinstock

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/mail"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/inventory"
	"github.com/skeletg/skelet/apps/backend/core/internal/mailsend"
	"github.com/skeletg/skelet/apps/backend/core/internal/ordermail"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/email"
)

type salableReader interface { // narrow dependency seams - accepting interfaces keeps the package testable and free of import cycles
	Salable(ctx context.Context, productID, channelID uuid.UUID) (float64, error)
}
type customerFinder interface {
	CustomerIDByEmail(ctx context.Context, channelID uuid.UUID, email string) (uuid.UUID, bool)
}
type flowEnroller interface {
	OnBackInStock(ctx context.Context, channelID, customerID uuid.UUID, sku, name, image, url string) int
}
type channelReader interface {
	GetByID(ctx context.Context, id uuid.UUID) (channel.Channel, error)
}
type suppressionChecker interface {
	IsSuppressed(ctx context.Context, email string) bool
	SuppressedSet(ctx context.Context, emails []string) map[string]bool // one query for a whole recipient list, keyed by the caller's original email strings
}

type Service struct {
	q        *db.Queries
	inv      salableReader
	cust     customerFinder
	flows    flowEnroller
	chans    channelReader
	config   *config.Service
	sender   email.Sender
	direct   *mailsend.Sender // records and tracks the notices sent here rather than through a flow
	suppress suppressionChecker
	bus      *events.Bus
	media    mediaURLer
}

type mediaURLer interface {
	PublicURL(key string) string
}

func (s *Service) SetMedia(m mediaURLer) { s.media = m }

// How many notices one address may wait on per store, so the endpoint can't be used to queue mail at scale
const maxPendingPerEmail = 20

var (
	ErrNotSoldHere      = errors.New("product is not sold on this store")
	ErrTooManyRequested = errors.New("too many back-in-stock requests for this address")
	ErrInvalidEmail     = errors.New("not a valid email address") // a bad address only bounces, and bounces cost the store's sending reputation
)

// Names this notice in the send log - back-in-stock has no editable slot, so it is not an emailtemplate key
const MessageKey = "back_in_stock"

func New(q *db.Queries, inv salableReader, cust customerFinder, flows flowEnroller, chans channelReader, cfg *config.Service, sender email.Sender, suppress suppressionChecker) *Service {
	return &Service{q: q, inv: inv, cust: cust, flows: flows, chans: chans, config: cfg, sender: sender, direct: mailsend.New(sender, cfg), suppress: suppress}
}

// Optional - without it the notice still goes out, just unrecorded
func (s *Service) SetDirectSender(a *mailsend.Sender) { s.direct = a }

// Falls back to an untracked sender so a Service assembled without New still delivers rather than panicking
func (s *Service) directSender() *mailsend.Sender {
	if s.direct != nil {
		return s.direct
	}
	return mailsend.New(s.sender, s.config)
}

// Only who asked, for what, on which store - a notice reads everything else from the catalog when it goes out
type Request struct {
	ChannelID  uuid.UUID
	ProductID  uuid.UUID
	Email      string
	CustomerID *uuid.UUID // set when the shopper is logged in
	AnonID     string
}

func (s *Service) Record(ctx context.Context, r Request) error { // stores or re-arms a request - idempotent per (channel, product, email)
	em := strings.ToLower(strings.TrimSpace(r.Email))
	if em == "" || r.ProductID == uuid.Nil || r.ChannelID == uuid.Nil {
		return fmt.Errorf("channel, product and email are required")
	}
	if a, err := mail.ParseAddress(em); err != nil || a.Address != em {
		return ErrInvalidEmail
	}
	prod, err := s.q.BackInStockProduct(ctx, db.BackInStockProductParams{ProductID: r.ProductID, ChannelID: r.ChannelID})
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotSoldHere
	}
	if err != nil {
		return err
	}
	pending, err := s.q.CountPendingBackInStockForEmail(ctx, db.CountPendingBackInStockForEmailParams{ChannelID: r.ChannelID, Email: em})
	if err != nil {
		return err
	}
	if pending >= maxPendingPerEmail {
		return ErrTooManyRequested
	}
	var cid uuid.NullUUID
	var customer *uuid.UUID
	if r.CustomerID != nil && *r.CustomerID != uuid.Nil && s.ownsAddress(ctx, r.ChannelID, *r.CustomerID, em) {
		cid, customer = uuid.NullUUID{UUID: *r.CustomerID, Valid: true}, r.CustomerID
	}
	if err := s.q.UpsertBackInStockRequest(ctx, db.UpsertBackInStockRequestParams{
		ChannelID: r.ChannelID, ProductID: r.ProductID, Sku: prod.Sku, Email: em,
		CustomerID: cid, AnonID: r.AnonID,
	}); err != nil {
		return err
	}
	if s.bus != nil {
		s.bus.Publish(ctx, &Requested{
			ChannelID: r.ChannelID, ProductID: r.ProductID, SKU: prod.Sku,
			Email: em, CustomerID: customer,
		})
	}
	return nil
}

// A signed-in shopper can type someone else's address - the request is then that address's, since a customer link would send the notice through their account's flow to their own inbox instead
func (s *Service) ownsAddress(ctx context.Context, channelID, customerID uuid.UUID, email string) bool {
	if s.cust == nil {
		return false
	}
	id, found := s.cust.CustomerIDByEmail(ctx, channelID, email)
	return found && id == customerID
}

type display struct{ sku, name, image, url string }

func (s *Service) productDisplay(ctx context.Context, productID uuid.UUID, ch channel.Channel) (display, bool) {
	row, err := s.q.BackInStockProduct(ctx, db.BackInStockProductParams{ProductID: productID, ChannelID: ch.ID, Locale: ch.DefaultLocale})
	if err != nil {
		return display{}, false
	}
	d := display{sku: row.Sku, name: row.Name}
	if base := strings.TrimRight(ch.PublicURL, "/"); base != "" && row.Path != "" {
		d.url = base + "/" + strings.TrimPrefix(row.Path, "/")
	}
	if row.ImageKey != "" && s.media != nil {
		d.image = s.media.PublicURL(row.ImageKey)
	}
	return d, true
}

func (s *Service) PendingCount(ctx context.Context, channelID uuid.UUID) (int64, error) { // the dashboard's per-channel "N shoppers waiting" figure
	return s.q.CountPendingBackInStock(ctx, channelID)
}

func (s *Service) Register(bus *events.Bus) { // also retains the bus, so Record can publish backinstock.requested
	if bus == nil {
		return
	}
	s.bus = bus
	bus.Subscribe(inventory.EventStockChanged, s.onStockChanged)
}

func (s *Service) onStockChanged(ctx context.Context, e events.Event) error { // re-checks per-channel salability against the configured minimum first, so a transient 1-unit blip that instantly re-sells doesn't notify
	ev, ok := e.(*inventory.StockChanged)
	if !ok || ev.Delta <= 0 || ev.ProductID == uuid.Nil {
		return nil // only stock INCREASES can bring something back
	}
	channels, err := s.q.PendingBackInStockChannels(ctx, ev.ProductID)
	if err != nil {
		log.Printf("[backinstock] list pending channels for %s: %v", ev.ProductID, err)
		return nil
	}
	minQty := 1.0
	if n, cerr := s.config.GetInt(ctx, "outreach/triggers/back_in_stock_min_qty", config.DefaultScope); cerr == nil && n > 0 {
		minQty = float64(n)
	}
	for _, chID := range channels {
		salable, serr := s.inv.Salable(ctx, ev.ProductID, chID) // -1 means an always-in-stock untracked product, so treat that as available
		if serr != nil {
			log.Printf("[backinstock] salable %s/%s: %v", ev.ProductID, chID, serr)
			continue
		}
		if salable >= 0 && salable < minQty {
			continue // back below the notify threshold, or still zero
		}
		s.notifyChannel(ctx, ev.ProductID, chID)
	}
	return nil
}

func (s *Service) notifyChannel(ctx context.Context, productID, channelID uuid.UUID) {
	ch, cerr := s.chans.GetByID(ctx, channelID)
	if cerr != nil {
		return
	}
	prod, sold := s.productDisplay(ctx, productID, ch)
	if !sold {
		return // pulled from the store while shoppers waited - they stay waiting, with nothing to send them to yet
	}
	reqs, err := s.q.ClaimPendingBackInStock(ctx, db.ClaimPendingBackInStockParams{ProductID: productID, ChannelID: channelID})
	if err != nil {
		log.Printf("[backinstock] claim pending %s/%s: %v", productID, channelID, err)
		return
	}
	var suppressed map[string]bool // one query for the whole waiting list rather than a lookup per request - a nil map, meaning no service, is none suppressed
	if s.suppress != nil {
		emails := make([]string, 0, len(reqs))
		for _, r := range reqs {
			emails = append(emails, r.Email)
		}
		suppressed = s.suppress.SuppressedSet(ctx, emails)
	}
	for _, r := range reqs {
		if suppressed[r.Email] {
			continue // deliverability: never mail a suppressed address
		}
		var custID uuid.UUID // the flow path wins for resolved customers with an active flow
		if r.CustomerID.Valid {
			custID = r.CustomerID.UUID
		} else if id, found := s.cust.CustomerIDByEmail(ctx, channelID, r.Email); found {
			custID = id
		}
		enrolled := 0
		if custID != uuid.Nil {
			enrolled = s.flows.OnBackInStock(ctx, channelID, custID, prod.sku, prod.name, prod.image, prod.url)
		}
		if enrolled > 0 {
			continue
		}
		if err := s.sendDirect(ctx, r.Email, prod.name, prod.image, prod.url, ch); err != nil {
			log.Printf("[backinstock] direct notify %s: %v", r.Email, err)
			if rerr := s.q.ReleaseBackInStockClaim(ctx, r.ID); rerr != nil { // back to waiting, so the next restock retries rather than the shopper silently never hearing
				log.Printf("[backinstock] release %s: %v", r.ID, rerr)
			}
		}
	}
}

func (s *Service) sendDirect(ctx context.Context, to, name, image, url string, ch channel.Channel) error { // a minimal self-contained notice
	if name == "" {
		name = "The item you wanted"
	}
	if url == "" {
		url = strings.TrimRight(ch.PublicURL, "/")
	}
	subject := name + " is back in stock"
	img := ""
	if image != "" {
		img = fmt.Sprintf(`<p><img src="%s" alt="%s" style="max-width:280px;height:auto;border-radius:8px"></p>`, htmlEscape(image), htmlEscape(name))
	}
	html := fmt.Sprintf(`<div style="font-family:sans-serif;max-width:520px;margin:0 auto;padding:24px">
<h1 style="font-size:20px">Good news - it's back!</h1>
<p><strong>%s</strong> is available again.</p>
%s
<p><a href="%s" style="display:inline-block;background:#111;color:#fff;padding:12px 20px;border-radius:8px;text-decoration:none">Shop now</a></p>
<p style="color:#888;font-size:12px">You asked to be notified when this item returned.</p>
</div>`, htmlEscape(name), img, htmlEscape(url))
	text := fmt.Sprintf("Good news - %s is back in stock.\nShop now: %s\n\nYou asked to be notified when this item returned.", name, url)
	return s.directSender().Send(ctx, mailsend.Spec{
		ChannelID: ch.ID, FallbackName: ordermail.StoreName(ctx, s.config, ch), To: to, MessageKey: MessageKey,
	}, subject, html, text)
}

func htmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}
