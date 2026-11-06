package catalogio

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/money"
	"github.com/skeletg/skelet/apps/backend/core/lib/objstore"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

// ProductFeedExporter uses the merchant feed's listing, price, locale and stock rules.
// A snapshot is fully rendered on disk before replacing its stable object-store key.
type ProductFeedExporter struct {
	source  *MerchantFeed
	storage objstore.Storage
}

type ProductFeedResult struct {
	Exported int
	Skipped  int
	File     sdk.ProductFeedFile
}

func NewProductFeedExporter(q *db.Queries, storage objstore.Storage, salable SalableReader, locales DefaultLocaler) *ProductFeedExporter {
	return &ProductFeedExporter{source: NewMerchantFeed(q, storage, salable, locales), storage: storage}
}

func productFeedKey(kind string, channelID, destinationID uuid.UUID, extension string) string {
	return "feeds/" + kind + "/" + channelID.String() + "/" + destinationID.String() + "." + extension
}

func (f *ProductFeedExporter) Export(ctx context.Context, feed sdk.ProductFeed, opts MerchantFeedOptions, d sdk.DestinationConfig) (ProductFeedResult, error) {
	var result ProductFeedResult
	if f.storage == nil || f.source.salable == nil {
		return result, fmt.Errorf("product feed: storage and salable reader are required")
	}
	tmp, err := os.CreateTemp("", "skelet-product-feed-*")
	if err != nil {
		return result, err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	writer, err := feed.NewWriter(tmp, d)
	if err != nil {
		return result, err
	}
	result, err = f.write(ctx, writer, opts)
	closeErr := writer.Close()
	if err != nil {
		return result, err
	}
	if closeErr != nil {
		return result, closeErr
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return result, err
	}
	info, err := tmp.Stat()
	if err != nil {
		return result, err
	}
	key := productFeedKey(feed.Kind, opts.ChannelID, d.ID, feed.Extension)
	err = f.storage.UploadStream(ctx, key, feed.ContentType, tmp)
	result.File = sdk.ProductFeedFile{Name: feed.Kind + "." + feed.Extension, ContentType: feed.ContentType, Size: info.Size(),
		Open: func(ctx context.Context) (io.ReadCloser, error) { return f.storage.GetStream(ctx, key) },
	}
	return result, err
}

func (f *ProductFeedExporter) write(ctx context.Context, writer sdk.ProductFeedWriter, opts MerchantFeedOptions) (ProductFeedResult, error) {
	var result ProductFeedResult
	defaultLocale := ""
	if f.source.locales != nil {
		defaultLocale = f.source.locales.DefaultLocale(ctx)
	}
	now := f.source.now()
	after := uuid.Nil
	for {
		rows, err := f.source.queries.MerchantFeedProducts(ctx, db.MerchantFeedProductsParams{
			ChannelID: opts.ChannelID, BrandAttribute: merchantFeedBrandAttribute,
			Locale: opts.Locale, DefaultLocale: defaultLocale, AfterID: after, PageLimit: f.source.pageSize,
		})
		if err != nil {
			return result, fmt.Errorf("product feed page: %w", err)
		}
		if len(rows) == 0 {
			return result, nil
		}
		salable, kits, err := f.source.pageStock(ctx, rows, opts.ChannelID)
		if err != nil {
			return result, err
		}
		for _, r := range rows {
			if err := ctx.Err(); err != nil {
				return result, err
			}
			price := effectiveFeedPrice(r, now)
			availability := availabilityFor(r, salable, kits[r.ID])
			if price <= 0 {
				result.Skipped++
				continue
			}
			p := sdk.ProductFeedItem{
				SKU: r.Sku, Title: r.Title, Description: r.Description, Brand: r.Brand,
				SellerName: opts.Title, URL: strings.TrimRight(opts.BaseURL, "/") + "/" + strings.TrimLeft(r.Path, "/"),
				Price:        money.Format(price, money.ISODecimals(opts.CurrencyCode)) + " " + opts.CurrencyCode,
				Availability: availability, GroupID: r.ItemGroupID,
			}
			if r.ImageKey != "" {
				p.ImageURL = f.storage.PublicURL(r.ImageKey)
			}
			if err := json.Unmarshal(r.VariantOptions, &p.VariantOptions); err != nil {
				return result, fmt.Errorf("product %s variant options: %w", r.Sku, err)
			}
			if p.GroupID != "" && len(p.VariantOptions) == 0 {
				p.VariantOptions = map[string]string{"SKU": p.SKU} // list-mode variants are selected by item rather than attributes
			}
			written, err := writer.WriteItem(p)
			if err != nil {
				return result, fmt.Errorf("encode product %s: %w", r.Sku, err)
			}
			if written {
				result.Exported++
			} else {
				result.Skipped++
			}
		}
		after = rows[len(rows)-1].ID
		if len(rows) < int(f.source.pageSize) {
			return result, nil
		}
	}
}

func (f *ProductFeedExporter) Open(ctx context.Context, feed sdk.ProductFeed, channelID, destinationID uuid.UUID) (io.ReadCloser, bool, error) {
	key := productFeedKey(feed.Kind, channelID, destinationID, feed.Extension)
	_, _, found, err := f.storage.Head(ctx, key)
	if err != nil || !found {
		return nil, found, err
	}
	body, err := f.storage.GetStream(ctx, key)
	return body, true, err
}
