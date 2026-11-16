package documents

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"image"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/country"
	"github.com/skeletg/skelet/apps/backend/core/internal/currency"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/core/lib/applog"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/money"
)

const (
	logoPath        = "sales/documents/logo"
	sellerPath      = "sales/documents/seller_details"
	footerPathShape = "sales/documents/%s_footer"
	maxLogoBytes    = 5 << 20
	logoCacheTTL    = 10 * time.Minute
	maxCachedLogos  = 64
)

func init() {
	config.RegisterGroup(config.Group{Section: "sales", Slug: "documents", Label: "Invoices and packing slips", SortOrder: 2, Scoping: config.ScopingChannel})
	config.Register(config.Definition{Path: logoPath, Type: config.TypeImage, Label: "Logo",
		Description: "Printed at the top of invoices, packing slips and credit memos. Empty uses the logo under Branding. A PNG, JPEG, GIF or WebP from the media library prints; an SVG, or an image hosted elsewhere, is left off."})
	config.Register(config.Definition{Path: sellerPath, Type: config.TypeTextarea, Label: "Seller details",
		Description: "The lines under the store name on every document, one per line - the company's legal name, address, tax number and registry entry. Most countries require them on an invoice. Each order keeps them, and the footers below, as they read when it was placed."})
	for _, f := range []struct {
		kind               Kind
		label, description string
	}{
		{KindInvoice, "Invoice footer", "Closes every invoice - payment terms, bank details, the registered office. On a business purchase, a note set on the buyer's country prints above it."},
		{KindPackingSlip, "Packing slip footer", "Closes every packing slip, for the warehouse or the customer opening the parcel."},
		{KindCreditMemo, "Credit memo footer", "Closes every credit memo, under its own line about where the money went and, on a business purchase, the buyer's country's note."},
	} {
		config.Register(config.Definition{Path: fmt.Sprintf(footerPathShape, f.kind), Type: config.TypeTextarea, Default: defaultFooters[f.kind],
			Label: f.label, Description: f.description})
	}
}

type Settings interface {
	GetString(ctx context.Context, path string, scope config.Scope) (string, error)
}

type Files interface {
	PublicURL(key string) string
	Head(ctx context.Context, key string) (size int64, contentType string, exists bool, err error)
	GetBytes(ctx context.Context, key string) ([]byte, error)
}

type Lookups struct {
	Channels interface {
		GetByID(context.Context, uuid.UUID) (channel.Channel, error)
	}
	Currencies interface {
		GetByCode(context.Context, string) (currency.Currency, error)
	}
	Countries interface {
		GetByCode(context.Context, string) (country.Country, error)
	}
}

// A plugin's renderer for the input's kind on its channel - ok false leaves the document to core
type PluginRenderer func(ctx context.Context, in Input, core func() ([]byte, error)) (pdf []byte, ok bool, err error)

type Service struct {
	settings Settings
	files    Files
	lookups  Lookups
	plugin   PluginRenderer

	mu    sync.Mutex
	logos map[string]cachedLogo
}

type cachedLogo struct {
	png  []byte
	size image.Point
	at   time.Time
}

func NewService(settings Settings, files Files, lookups Lookups, plugin PluginRenderer) *Service {
	return &Service{settings: settings, files: files, lookups: lookups, plugin: plugin, logos: map[string]cachedLogo{}}
}

func (s *Service) Document(ctx context.Context, kind Kind, o order.Order, memo *order.CreditMemo) ([]byte, error) {
	in, err := s.Input(ctx, kind, o, memo)
	if err != nil {
		return nil, err
	}
	core := func() ([]byte, error) { return Render(in) }
	if s.plugin != nil {
		pdf, ok, err := s.plugin(ctx, in, core)
		switch {
		case err != nil:
			return nil, err
		case !ok:
		case !bytes.HasPrefix(pdf, []byte("%PDF-")):
			return nil, apperr.New("DOCUMENT_NOT_PDF", apperr.ClassInternal, fmt.Sprintf("the plugin making this %s returned something other than a PDF", strings.ReplaceAll(string(kind), "_", " ")))
		default:
			return pdf, nil
		}
	}
	return core()
}

// A document missing its seller's details or terms is worse than none, so a settings failure fails it
func (s *Service) setting(ctx context.Context, path string, channelID uuid.UUID) (string, error) {
	if s.settings == nil {
		return "", nil
	}
	v, err := s.settings.GetString(ctx, path, config.Scope{Kind: config.ScopeChannel, ChannelID: &channelID})
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return strings.TrimSpace(v), nil
}

// Seller is what the channel's settings give today for an order billed to the country - placement keeps it on the
// order, so a later change of address or terms never rewrites an invoice already issued
func (s *Service) Seller(ctx context.Context, ch channel.Channel, billingCountry string, business bool) (order.Seller, error) {
	name, err := s.setting(ctx, channel.ConfigStoreName, ch.ID)
	if err != nil {
		return order.Seller{}, err
	}
	details, err := s.setting(ctx, sellerPath, ch.ID)
	if err != nil {
		return order.Seller{}, err
	}
	sel := order.Seller{Name: cmp.Or(name, ch.Name, "Store"), Footers: map[string]string{}}
	for _, kind := range []Kind{KindInvoice, KindPackingSlip, KindCreditMemo} {
		if sel.Footers[string(kind)], err = s.setting(ctx, fmt.Sprintf(footerPathShape, kind), ch.ID); err != nil {
			return order.Seller{}, err
		}
	}
	if sel.Note, err = s.countryNote(ctx, billingCountry, business); err != nil {
		return order.Seller{}, err
	}
	for l := range strings.SplitSeq(details, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			sel.Lines = append(sel.Lines, l)
		}
	}
	if len(sel.Lines) == 0 && ch.PublicURL != "" {
		sel.Lines = []string{ch.PublicURL}
	}
	return sel, nil
}

// Input is what the document prints, gathered from the order's channel, its currency and the buyer's country
func (s *Service) Input(ctx context.Context, kind Kind, o order.Order, memo *order.CreditMemo) (Input, error) {
	in := Input{Kind: kind, Order: o, Memo: memo, Decimals: money.ISODecimals(o.CurrencyCode), TaxInclusive: o.TaxInclusive}
	if s.lookups.Currencies != nil {
		if cur, err := s.lookups.Currencies.GetByCode(ctx, o.CurrencyCode); err == nil {
			in.Decimals = int(cur.Decimals)
		}
	}
	seller := o.Seller
	if seller == nil {
		var ch channel.Channel
		if s.lookups.Channels != nil {
			var err error
			if ch, err = s.lookups.Channels.GetByID(ctx, o.ChannelID); err != nil {
				return Input{}, fmt.Errorf("read the channel: %w", err)
			}
		}
		ch.ID = o.ChannelID
		now, err := s.Seller(ctx, ch, o.BillingAddress.CountryCode, len(o.BusinessIDs) > 0)
		if err != nil {
			return Input{}, err
		}
		seller = &now
	}
	in.Store.Name, in.Store.Lines, in.Footer = seller.Name, seller.Lines, seller.Footers[string(kind)]
	if kind != KindPackingSlip {
		in.Note = seller.Note
	}
	logo, err := s.setting(ctx, logoPath, o.ChannelID)
	if err != nil {
		return Input{}, err
	}
	if logo == "" {
		if logo, err = s.setting(ctx, channel.ConfigLogoURL, o.ChannelID); err != nil {
			return Input{}, err
		}
	}
	in.Store.Logo, in.Store.LogoSize = s.logo(ctx, logo)
	return in, nil
}

// A reverse-charge or export note is about the buyer's tax standing, which a personal purchase doesn't have
func (s *Service) countryNote(ctx context.Context, code string, business bool) (string, error) {
	if !business || code == "" || s.lookups.Countries == nil {
		return "", nil
	}
	c, err := s.lookups.Countries.GetByCode(ctx, code)
	switch {
	case err == nil:
		return c.InvoiceNote, nil
	case errors.Is(err, country.ErrCountryNotFound):
		return "", nil
	}
	return "", fmt.Errorf("read the billing country: %w", err)
}

// Read from the store by key, never fetched from the URL - a logo hosted elsewhere is left off. Kept a while as the
// small PNG documents print, so a run of invoices doesn't fetch and decode it each time
func (s *Service) logo(ctx context.Context, address string) ([]byte, image.Point) {
	if address == "" || s.files == nil {
		return nil, image.Point{}
	}
	base := s.files.PublicURL("")
	if !strings.HasPrefix(address, base) {
		return nil, image.Point{}
	}
	key, err := url.PathUnescape(strings.SplitN(strings.TrimPrefix(address, base), "?", 2)[0])
	if err != nil {
		return nil, image.Point{}
	}
	s.mu.Lock()
	c, hit := s.logos[key]
	s.mu.Unlock()
	if hit && time.Since(c.at) < logoCacheTTL {
		return c.png, c.size
	}
	c, settled := s.loadLogo(ctx, key)
	if settled {
		c.at = time.Now()
		s.mu.Lock()
		if len(s.logos) >= maxCachedLogos {
			clear(s.logos)
		}
		s.logos[key] = c
		s.mu.Unlock()
	}
	return c.png, c.size
}

// settled is false on a failure worth trying again, which isn't remembered
func (s *Service) loadLogo(ctx context.Context, key string) (logo cachedLogo, settled bool) {
	log := applog.From(ctx)
	size, _, exists, err := s.files.Head(ctx, key)
	switch {
	case err != nil:
		log.Warn("documents: couldn't read the logo", "key", key, "err", err)
		return cachedLogo{}, false
	case !exists:
		return cachedLogo{}, true
	case size > maxLogoBytes:
		log.Warn("documents: the logo is too large to print", "key", key, "bytes", size)
		return cachedLogo{}, true
	}
	raw, err := s.files.GetBytes(ctx, key)
	if err != nil {
		log.Warn("documents: couldn't read the logo", "key", key, "err", err)
		return cachedLogo{}, false
	}
	if len(raw) > maxLogoBytes {
		return cachedLogo{}, true
	}
	logo.png, logo.size, _ = logoPNG(raw)
	return logo, true
}
