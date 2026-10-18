package catalogio

// Google Merchant Center product feed (RSS 2.0 with the g: namespace) - Mirrors internal/blog/feed.go: a renderer the HTTP layer calls with channel-resolved options - Standalone from Service on purpose - the feed reads a purpose-built flat query (no renderProduct hydration) so a 50k catalog renders in one pass without the attribute pipeline

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/xml"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/singleflight"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/pricing"
)

const (
	merchantFeedPageSize            = 5000 // bounds one DB round-trip - the renderer loops pages until exhaustion
	merchantFeedDescriptionMaxRunes = 5000 // Merchant Center's description limit, counted in characters
	merchantFeedCacheTTL            = 5 * time.Minute
	merchantFeedContentType         = "application/xml; charset=utf-8"
	merchantFeedNamespace           = "http://base.google.com/ns/1.0"
	merchantFeedBrandAttribute      = "brand" // the attribute slug whose value fills g:brand
	conditionNew                    = "new"
	availabilityInStock             = "in_stock"
	availabilityBackorder           = "backorder"
	availabilityOutOfStock          = "out_of_stock"
)

// MediaURLer resolves a stored media key to its public URL - Satisfied by objstore.Storage
type MediaURLer interface {
	PublicURL(key string) string
}

type SalableReader interface {
	SalableByProductIDs(ctx context.Context, productIDs []uuid.UUID, channelID uuid.UUID) (map[uuid.UUID]float64, error)
}

// DefaultLocaler names the locale a translation-less product falls back to - satisfied by *catalog.Service
type DefaultLocaler interface {
	DefaultLocale(ctx context.Context) string
}

// Rendered bytes are kept per (channel, locale) for a few minutes and concurrent misses share one render, so a burst of crawler pulls costs one pass over the catalog
type MerchantFeed struct {
	queries *db.Queries
	media   MediaURLer
	salable SalableReader
	locales DefaultLocaler

	now      func() time.Time
	ttl      time.Duration
	pageSize int32
	mu       sync.Mutex
	cache    map[string]cachedFeed
	renders  singleflight.Group
}

type cachedFeed struct {
	body    []byte
	expires time.Time
}

func NewMerchantFeed(queries *db.Queries, media MediaURLer, salable SalableReader, locales DefaultLocaler) *MerchantFeed {
	return &MerchantFeed{queries: queries, media: media, salable: salable, locales: locales, now: time.Now, ttl: merchantFeedCacheTTL, pageSize: merchantFeedPageSize, cache: map[string]cachedFeed{}}
}

// MerchantFeedOptions carries the channel-resolved rendering inputs - Locale must already be one the channel serves, since it keys the cache
type MerchantFeedOptions struct {
	ChannelID    uuid.UUID
	BaseURL      string // absolute storefront origin, no trailing slash
	CurrencyCode string // ISO 4217 for g:price
	Locale       string // content locale for title/description
	Title        string // channel name
}

type merchantItem struct {
	XMLName      xml.Name `xml:"item"`
	ID           string   `xml:"g:id"`
	Title        string   `xml:"title"`
	Description  string   `xml:"description,omitempty"`
	Link         string   `xml:"link"`
	ImageLink    string   `xml:"g:image_link,omitempty"`
	ItemGroupID  string   `xml:"g:item_group_id,omitempty"`
	Price        string   `xml:"g:price"`
	Availability string   `xml:"g:availability"`
	Condition    string   `xml:"g:condition"`
	Brand        string   `xml:"g:brand,omitempty"`
	MPN          string   `xml:"g:mpn,omitempty"`
}

// Render produces the feed XML + content type - Digital and subscription products are included (Merchant Center supports them); availability maps tracked inventory to in_stock / backorder / out_of_stock, and a kit's to its scarcest component
func (f *MerchantFeed) Render(ctx context.Context, opts MerchantFeedOptions) ([]byte, string, error) {
	key := opts.ChannelID.String() + "|" + opts.Locale
	f.mu.Lock()
	hit, ok := f.cache[key]
	f.mu.Unlock()
	if ok && f.now().Before(hit.expires) {
		return hit.body, merchantFeedContentType, nil
	}
	// Detached so one crawler hanging up doesn't fail every request sharing the render
	v, err, _ := f.renders.Do(key, func() (any, error) {
		body, err := f.render(context.WithoutCancel(ctx), opts)
		if err != nil {
			return nil, err
		}
		f.mu.Lock()
		f.cache[key] = cachedFeed{body: body, expires: f.now().Add(f.ttl)}
		f.mu.Unlock()
		return body, nil
	})
	if err != nil {
		return nil, "", err
	}
	return v.([]byte), merchantFeedContentType, nil
}

// Streams items into the buffer page by page - keyset on product id, so no page re-scans the ones before it
func (f *MerchantFeed) render(ctx context.Context, opts MerchantFeedOptions) ([]byte, error) {
	if f.salable == nil {
		return nil, errors.New("merchant feed: no salable reader wired")
	}
	base := strings.TrimRight(opts.BaseURL, "/")
	now := f.now()
	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "  ")
	rss := xml.StartElement{Name: xml.Name{Local: "rss"}, Attr: []xml.Attr{
		{Name: xml.Name{Local: "version"}, Value: "2.0"},
		{Name: xml.Name{Local: "xmlns:g"}, Value: merchantFeedNamespace},
	}}
	channelEl := xml.StartElement{Name: xml.Name{Local: "channel"}}
	if err := enc.EncodeToken(rss); err != nil {
		return nil, fmt.Errorf("merchant feed encode: %w", err)
	}
	if err := enc.EncodeToken(channelEl); err != nil {
		return nil, fmt.Errorf("merchant feed encode: %w", err)
	}
	for _, el := range []struct{ name, value string }{
		{"title", opts.Title},
		{"link", base + "/"},
		{"description", opts.Title + " product feed"},
	} {
		if err := enc.EncodeElement(el.value, xml.StartElement{Name: xml.Name{Local: el.name}}); err != nil {
			return nil, fmt.Errorf("merchant feed encode: %w", err)
		}
	}

	defaultLocale := ""
	if f.locales != nil {
		defaultLocale = f.locales.DefaultLocale(ctx)
	}
	after := uuid.Nil
	for {
		rows, err := f.queries.MerchantFeedProducts(ctx, db.MerchantFeedProductsParams{
			ChannelID:      opts.ChannelID,
			BrandAttribute: merchantFeedBrandAttribute,
			Locale:         opts.Locale,
			DefaultLocale:  defaultLocale,
			AfterID:        after,
			PageLimit:      f.pageSize,
		})
		if err != nil {
			return nil, fmt.Errorf("merchant feed page: %w", err)
		}
		if len(rows) == 0 {
			break
		}
		salable, kits, err := f.pageStock(ctx, rows, opts.ChannelID)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			price := effectiveFeedPrice(r, now)
			item := merchantItem{
				ID:           r.Sku,
				Title:        r.Title,
				Description:  truncateRunes(r.Description, merchantFeedDescriptionMaxRunes),
				Link:         base + "/" + strings.TrimPrefix(r.Path, "/"),
				Price:        fmt.Sprintf("%d.%02d %s", price/100, price%100, opts.CurrencyCode),
				Availability: availabilityFor(r, salable, kits[r.ID]),
				Condition:    conditionNew,
				Brand:        r.Brand,
				MPN:          r.Sku,
				ItemGroupID:  r.ItemGroupID,
			}
			if r.ImageKey != "" && f.media != nil {
				item.ImageLink = f.media.PublicURL(r.ImageKey)
			}
			if err := enc.Encode(item); err != nil {
				return nil, fmt.Errorf("merchant feed encode: %w", err)
			}
		}
		after = rows[len(rows)-1].ID
		if len(rows) < int(f.pageSize) {
			break
		}
	}
	if err := enc.EncodeToken(channelEl.End()); err != nil {
		return nil, fmt.Errorf("merchant feed encode: %w", err)
	}
	if err := enc.EncodeToken(rss.End()); err != nil {
		return nil, fmt.Errorf("merchant feed encode: %w", err)
	}
	if err := enc.Flush(); err != nil {
		return nil, fmt.Errorf("merchant feed encode: %w", err)
	}
	return buf.Bytes(), nil
}

// effectiveFeedPrice folds the channel/product special-price trio (listing wholesale-overrides the product's, mirroring EffectivePricingInputs)
func effectiveFeedPrice(r db.MerchantFeedProductsRow, now time.Time) int64 {
	in := pricing.Inputs{BasePrice: r.PriceCents}
	if r.ProductCompareAt.Valid {
		v := r.ProductCompareAt.Int64
		in.CompareAt = &v
	}
	if r.ListingCompareAt.Valid {
		v := r.ListingCompareAt.Int64
		in.CompareAt = &v
	}
	setTrio := func(sp sql.NullInt64, from, to sql.NullTime) {
		if !sp.Valid {
			return
		}
		v := sp.Int64
		in.Special = &v
		in.SpecialFrom, in.SpecialTo = nil, nil
		if from.Valid {
			t := from.Time
			in.SpecialFrom = &t
		}
		if to.Valid {
			t := to.Time
			in.SpecialTo = &t
		}
	}
	setTrio(r.ProductSpecial, r.ProductSpecialFrom, r.ProductSpecialTo)
	setTrio(r.ListingSpecial, r.ListingSpecialFrom, r.ListingSpecialTo)
	return pricing.Resolve(in, now).Price
}

type kitPart struct {
	componentID uuid.UUID
	qty         float64
}

func (f *MerchantFeed) pageStock(ctx context.Context, rows []db.MerchantFeedProductsRow, channelID uuid.UUID) (map[uuid.UUID]float64, map[uuid.UUID][]kitPart, error) {
	ids := make([]uuid.UUID, 0, len(rows))
	var kitIDs []uuid.UUID
	for _, r := range rows {
		ids = append(ids, r.ID)
		if r.Kind == db.CatalogProductKindKit {
			kitIDs = append(kitIDs, r.ID)
		}
	}
	kits := map[uuid.UUID][]kitPart{}
	if len(kitIDs) > 0 {
		comps, err := f.queries.MerchantFeedKitComponents(ctx, kitIDs)
		if err != nil {
			return nil, nil, fmt.Errorf("merchant feed kit components: %w", err)
		}
		for _, c := range comps {
			kits[c.KitID] = append(kits[c.KitID], kitPart{componentID: c.ComponentProductID, qty: c.Quantity})
			ids = append(ids, c.ComponentProductID)
		}
	}
	salable, err := f.salable.SalableByProductIDs(ctx, ids, channelID)
	if err != nil {
		return nil, nil, fmt.Errorf("merchant feed salable: %w", err)
	}
	return salable, kits, nil
}

// A negative salable is the inventory's always-in-stock sentinel - a kit's stock is reserved per component, so its scarcest tracked one decides
func availabilityFor(r db.MerchantFeedProductsRow, salable map[uuid.UUID]float64, parts []kitPart) string {
	if r.Kind == db.CatalogProductKindKit {
		units := math.Inf(1)
		for _, p := range parts {
			if s := salable[p.componentID]; s >= 0 {
				units = math.Min(units, math.Floor(s/p.qty))
			}
		}
		if units > 0 {
			return availabilityInStock
		}
		return availabilityOutOfStock
	}
	s := salable[r.ID]
	if !r.Tracked || s < 0 || s > 0 {
		return availabilityInStock
	}
	if r.Backorderable {
		return availabilityBackorder
	}
	return availabilityOutOfStock
}

// Cuts on a character boundary - a byte cut splits a multibyte rune and the encoder writes U+FFFD in its place
func truncateRunes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	count := 0
	for i := range s {
		if count == n {
			return s[:i]
		}
		count++
	}
	return s
}
