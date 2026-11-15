package documents

import (
	"bytes"
	"cmp"
	"context"
	"fmt"
	"log"
	"net/url"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/country"
	"github.com/skeletg/skelet/apps/backend/core/internal/currency"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/money"
)

const (
	logoPath        = "sales/documents/logo"
	sellerPath      = "sales/documents/seller_details"
	brandingLogo    = "general/branding/logo_url"
	maxLogoBytes    = 5 << 20
	footerPathShape = "sales/documents/%s_footer"
)

func init() {
	config.RegisterGroup(config.Group{Section: "sales", Slug: "documents", Label: "Invoices and packing slips", SortOrder: 2, Scoping: config.ScopingChannel})
	config.Register(config.Definition{Path: logoPath, Type: config.TypeImage, Label: "Logo",
		Description: "Printed at the top of invoices, packing slips and credit memos. Empty uses the logo under Branding. A PNG, JPEG, GIF or WebP from the media library prints; an SVG, or an image hosted elsewhere, is left off."})
	config.Register(config.Definition{Path: sellerPath, Type: config.TypeTextarea, Label: "Seller details",
		Description: "The lines under the store name on every document, one per line - the company's legal name, address, tax number and registry entry. Most countries require them on an invoice."})
	for _, f := range []struct {
		kind               Kind
		label, description string
	}{
		{KindInvoice, "Invoice footer", "Closes every invoice - payment terms, bank details, the registered office. A note set on the buyer's country prints above it."},
		{KindPackingSlip, "Packing slip footer", "Closes every packing slip, for the warehouse or the customer opening the parcel."},
		{KindCreditMemo, "Credit memo footer", "Closes every credit memo, under its own line about where the money went and the buyer's country's note."},
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
}

func NewService(settings Settings, files Files, lookups Lookups, plugin PluginRenderer) *Service {
	return &Service{settings: settings, files: files, lookups: lookups, plugin: plugin}
}

func (s *Service) Document(ctx context.Context, kind Kind, o order.Order, memo *order.CreditMemo) ([]byte, error) {
	in := s.Input(ctx, kind, o, memo)
	core := func() ([]byte, error) { return Render(in) }
	if s.plugin != nil {
		pdf, ok, err := s.plugin(ctx, in, core)
		switch {
		case !ok:
		case err != nil:
			return nil, err
		case !bytes.HasPrefix(pdf, []byte("%PDF-")):
			return nil, apperr.New("DOCUMENT_NOT_PDF", apperr.ClassInternal, fmt.Sprintf("the plugin making this %s returned something other than a PDF", strings.ReplaceAll(string(kind), "_", " ")))
		default:
			return pdf, nil
		}
	}
	return core()
}

func (s *Service) setting(ctx context.Context, path string, channelID uuid.UUID) string {
	if s.settings == nil {
		return ""
	}
	v, err := s.settings.GetString(ctx, path, config.Scope{Kind: config.ScopeChannel, ChannelID: &channelID})
	if err != nil {
		return ""
	}
	return strings.TrimSpace(v)
}

// Input is what the document prints, gathered from the order's channel, its currency and the buyer's country
func (s *Service) Input(ctx context.Context, kind Kind, o order.Order, memo *order.CreditMemo) Input {
	in := Input{Kind: kind, Order: o, Memo: memo, Decimals: money.ISODecimals(o.CurrencyCode), TaxInclusive: o.TaxInclusive,
		Footer: s.setting(ctx, fmt.Sprintf(footerPathShape, kind), o.ChannelID)}
	if s.lookups.Currencies != nil {
		if cur, err := s.lookups.Currencies.GetByCode(ctx, o.CurrencyCode); err == nil {
			in.Decimals = int(cur.Decimals)
		}
	}
	var ch channel.Channel
	if s.lookups.Channels != nil {
		ch, _ = s.lookups.Channels.GetByID(ctx, o.ChannelID)
	}
	in.Store.Name = cmp.Or(s.setting(ctx, channel.ConfigStoreName, o.ChannelID), ch.Name, "Store")
	for l := range strings.SplitSeq(s.setting(ctx, sellerPath, o.ChannelID), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			in.Store.Lines = append(in.Store.Lines, l)
		}
	}
	if len(in.Store.Lines) == 0 && ch.PublicURL != "" {
		in.Store.Lines = []string{ch.PublicURL}
	}
	in.Store.Logo = s.logo(ctx, cmp.Or(s.setting(ctx, logoPath, o.ChannelID), s.setting(ctx, brandingLogo, o.ChannelID)))
	if kind != KindPackingSlip && s.lookups.Countries != nil {
		if c, err := s.lookups.Countries.GetByCode(ctx, o.BillingAddress.CountryCode); err == nil {
			in.Note = c.InvoiceNote
		}
	}
	return in
}

// Read from the store by key, never fetched from the URL - a logo hosted elsewhere is left off
func (s *Service) logo(ctx context.Context, address string) []byte {
	if address == "" || s.files == nil {
		return nil
	}
	base := s.files.PublicURL("")
	if !strings.HasPrefix(address, base) {
		return nil
	}
	key, err := url.PathUnescape(strings.SplitN(strings.TrimPrefix(address, base), "?", 2)[0])
	if err != nil {
		return nil
	}
	raw, err := s.files.GetBytes(ctx, key)
	if err != nil {
		log.Printf("[documents] logo %s: %v", key, err)
		return nil
	}
	if len(raw) > maxLogoBytes {
		return nil
	}
	return raw
}
