package catalog

// Google Merchant Center product feed (RSS 2.0 with the g: namespace) - Mirrors internal/blog/feed.go: a renderer the HTTP layer calls with channel-resolved options - Standalone from Service on purpose - the feed reads a purpose-built flat query (no renderProduct hydration) so a 50k catalog renders in one pass without the attribute pipeline

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/xml"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	db "github.com/skeletg/skelet/internal/db/sqlc"
	"github.com/skeletg/skelet/internal/pricing"
)

// merchantFeedPageSize bounds one DB round-trip - the renderer loops pages until exhaustion
const merchantFeedPageSize = 5000

// MediaURLer resolves a stored media key to its public URL - Satisfied by *media.Storage
type MediaURLer interface {
	PublicURL(key string) string
}

type MerchantFeed struct {
	queries *db.Queries
	media   MediaURLer
}

func NewMerchantFeed(queries *db.Queries, media MediaURLer) *MerchantFeed {
	return &MerchantFeed{queries: queries, media: media}
}

// MerchantFeedOptions carries the channel-resolved rendering inputs
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
	Price        string   `xml:"g:price"`
	Availability string   `xml:"g:availability"`
	Condition    string   `xml:"g:condition"`
	Brand        string   `xml:"g:brand,omitempty"`
	MPN          string   `xml:"g:mpn,omitempty"`
}

type merchantChannel struct {
	Title       string         `xml:"title"`
	Link        string         `xml:"link"`
	Description string         `xml:"description"`
	Items       []merchantItem `xml:"item"`
}

type merchantRSS struct {
	XMLName xml.Name        `xml:"rss"`
	Version string          `xml:"version,attr"`
	NSG     string          `xml:"xmlns:g,attr"`
	Channel merchantChannel `xml:"channel"`
}

// Render produces the feed XML + content type - Digital and subscription products are included (Merchant Center supports them); availability maps tracked inventory to in_stock / backorder / out_of_stock
func (f *MerchantFeed) Render(ctx context.Context, opts MerchantFeedOptions) ([]byte, string, error) {
	base := strings.TrimRight(opts.BaseURL, "/")
	now := time.Now()
	items := []merchantItem{}
	for offset := int32(0); ; offset += merchantFeedPageSize {
		rows, err := f.queries.MerchantFeedProducts(ctx, db.MerchantFeedProductsParams{
			ChannelID:  opts.ChannelID,
			Locale:     opts.Locale,
			PageLimit:  merchantFeedPageSize,
			PageOffset: offset,
		})
		if err != nil {
			return nil, "", fmt.Errorf("merchant feed page: %w", err)
		}
		for _, r := range rows {
			price := effectiveFeedPrice(r, now)
			item := merchantItem{
				ID:           r.Sku,
				Title:        r.Title,
				Description:  truncate(r.Description, 5000),
				Link:         base + "/" + strings.TrimPrefix(r.Path, "/"),
				Price:        fmt.Sprintf("%d.%02d %s", price/100, price%100, opts.CurrencyCode),
				Availability: availabilityFor(r),
				Condition:    "new",
				Brand:        r.Brand,
				MPN:          r.Sku,
			}
			if r.ImageKey != "" && f.media != nil {
				item.ImageLink = f.media.PublicURL(r.ImageKey)
			}
			items = append(items, item)
		}
		if len(rows) < merchantFeedPageSize {
			break
		}
	}

	feed := merchantRSS{
		Version: "2.0",
		NSG:     "http://base.google.com/ns/1.0",
		Channel: merchantChannel{
			Title:       opts.Title,
			Link:        base + "/",
			Description: opts.Title + " product feed",
			Items:       items,
		},
	}
	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "  ")
	if err := enc.Encode(feed); err != nil {
		return nil, "", fmt.Errorf("merchant feed encode: %w", err)
	}
	return buf.Bytes(), "application/xml; charset=utf-8", nil
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

func availabilityFor(r db.MerchantFeedProductsRow) string {
	if !r.Tracked {
		return "in_stock"
	}
	if r.Quantity > 0 {
		return "in_stock"
	}
	if r.Backorderable {
		return "backorder"
	}
	return "out_of_stock"
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
