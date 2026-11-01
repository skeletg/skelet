package catalogio

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/objstore"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

func TestProductSnapshotsRespectListingsPricesStockAndKeepTheLastGoodFile(t *testing.T) {
	r := newFeedRig(t)
	ctx := context.Background()
	ch, other := r.channel(), r.id(`INSERT INTO channels (slug, name, currency_code) VALUES ($1, 'other', 'USD') RETURNING id`, "other-"+r.sfx)
	good, out, hidden, noindex, elsewhere := r.product("GOOD"), r.product("OUT"), r.product("HIDDEN"), r.product("NOINDEX"), r.product("ELSEWHERE")
	r.list(ch, good, out, hidden, noindex)
	r.list(other, elsewhere)
	r.exec(`UPDATE catalog_product_channel_listings SET is_published = FALSE WHERE product_id = $1`, hidden)
	r.exec(`UPDATE catalog_products SET seo_noindex = TRUE WHERE id = $1`, noindex)
	r.exec(`INSERT INTO inventory_items (product_id, tracked) VALUES ($1, TRUE)`, out)
	r.exec(`UPDATE catalog_product_channel_listings SET special_price = 300, url = 'mug' WHERE product_id = $1`, good)
	r.exec(`UPDATE catalog_products SET attributes = '{"brand":"Northline"}' WHERE id = $1`, good)
	r.exec(`INSERT INTO catalog_product_content (product_id, locale, name, meta_description) VALUES ($1, 'en', 'Mug', 'Blue ceramic mug')`, good)
	r.exec(`INSERT INTO catalog_product_media (id, product_id, s3_key, mime_type, status) VALUES ($1, $2, 'mug.jpg', 'image/jpeg', 'ready')`, uuid.New(), good)
	storage, err := objstore.New(ctx, objstore.Config{Driver: objstore.DriverFilesystem, Dir: t.TempDir(), PublicURLBase: "https://media.example"})
	if err != nil {
		t.Fatal(err)
	}
	exporter := NewProductFeedExporter(db.New(r.conn), storage, fakeSalable{good: -1, out: 0}, nil)
	exporter.source.pageSize = 1 // a page containing only a skipped item must not end the snapshot
	encode := func(w io.Writer, p sdk.ProductFeedItem) (bool, error) {
		if p.Availability == "out_of_stock" {
			return false, nil
		}
		return true, json.NewEncoder(w).Encode(p)
	}
	feed := sdk.ProductFeed{Kind: "test", Extension: "jsonl.gz", ContentType: "application/gzip", NewWriter: func(w io.Writer, _ sdk.DestinationConfig) (sdk.ProductFeedWriter, error) {
		return &testFeedWriter{Writer: gzip.NewWriter(w), encode: encode}, nil
	}}
	dest := sdk.DestinationConfig{ID: uuid.New(), ChannelID: ch}
	opts := MerchantFeedOptions{ChannelID: ch, BaseURL: "https://shop.example", CurrencyCode: "KWD", Locale: "en", Title: "Northline Home"}
	result, err := exporter.Export(ctx, feed, opts, dest)
	if err != nil || result.Exported != 1 || result.Skipped != 1 {
		t.Fatalf("export: %+v, %v", result, err)
	}
	raw, err := storage.GetBytes(ctx, productFeedKey("test", ch, dest.ID, feed.Extension))
	if err != nil {
		t.Fatal(err)
	}
	gz, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	dec := json.NewDecoder(gz)
	var p sdk.ProductFeedItem
	if err := dec.Decode(&p); err != nil {
		t.Fatal(err)
	}
	if p.SKU != "GOOD-"+r.sfx || p.Price != "0.300 KWD" || p.URL != "https://shop.example/mug" || p.ImageURL != "https://media.example/mug.jpg" || p.Brand != "Northline" || p.SellerName != opts.Title {
		t.Fatalf("wrong listing, currency or content: %+v", p)
	}
	if err := dec.Decode(&p); !errors.Is(err, io.EOF) {
		t.Fatalf("unavailable or other-channel item leaked: %v", err)
	}
	if _, found, err := exporter.Open(ctx, feed, other, dest.ID); err != nil || found {
		t.Fatalf("channel shared a snapshot: %v, %v", found, err)
	}
	encode = func(io.Writer, sdk.ProductFeedItem) (bool, error) { return false, io.ErrClosedPipe }
	if _, err := exporter.Export(ctx, feed, opts, dest); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("failed export reported success: %v", err)
	}
	if after, err := storage.GetBytes(ctx, productFeedKey("test", ch, dest.ID, feed.Extension)); err != nil || !bytes.Equal(raw, after) {
		t.Fatal("a failed render replaced the last good snapshot")
	}
	encode = func(w io.Writer, p sdk.ProductFeedItem) (bool, error) {
		if p.Availability == "out_of_stock" {
			return false, nil
		}
		return true, json.NewEncoder(w).Encode(p)
	}
	r.exec(`UPDATE catalog_product_channel_listings SET is_published = FALSE WHERE product_id = $1`, good)
	if result, err := exporter.Export(ctx, feed, opts, dest); err != nil || result.Exported != 0 {
		t.Fatalf("an empty current catalog must replace the previous snapshot: %+v, %v", result, err)
	}
	after, _ := storage.GetBytes(ctx, productFeedKey("test", ch, dest.ID, feed.Extension))
	gzEmpty, err := gzip.NewReader(bytes.NewReader(after))
	if err != nil {
		t.Fatal(err)
	}
	defer gzEmpty.Close()
	if b, err := io.ReadAll(gzEmpty); err != nil || len(b) != 0 {
		t.Fatalf("unpublished items survived a successful refresh: %s, %v", b, err)
	}
}

func TestProductFeedReadsLocalizedSelectableBrandsAndVariantValues(t *testing.T) {
	r := newFeedRig(t)
	ch := r.channel()
	parent, child := r.productOf("SHIRT", "configurable"), r.product("BLUE")
	r.list(ch, parent, child)
	r.exec(`INSERT INTO catalog_product_children (parent_id, child_id) VALUES ($1, $2)`, parent, child)
	brandSlug, colorSlug := "feed-brand-"+r.sfx, "feed-color-"+r.sfx
	brand := r.id(`INSERT INTO catalog_attributes (slug, name, input_type) VALUES ($1, 'Brand', 'dropdown') RETURNING id`, brandSlug)
	color := r.id(`INSERT INTO catalog_attributes (slug, name, input_type) VALUES ($1, 'Color', 'dropdown') RETURNING id`, colorSlug)
	brandValue := r.id(`INSERT INTO catalog_attribute_values (attribute_id, slug, name) VALUES ($1, 'northline', 'Northline') RETURNING id`, brand)
	blue := r.id(`INSERT INTO catalog_attribute_values (attribute_id, slug, name) VALUES ($1, 'blue', 'Blue') RETURNING id`, color)
	r.exec(`INSERT INTO catalog_product_attribute_values (product_id, attribute_id, attribute_value_id) VALUES ($1, $2, $3), ($1, $4, $5)`, child, brand, brandValue, color, blue)
	r.exec(`INSERT INTO catalog_product_configurable_options (product_id, attribute_id) VALUES ($1, $2)`, parent, color)
	r.exec(`INSERT INTO catalog_attribute_value_content (value_id, locale, name) VALUES ($1, 'de', 'Nordlinie'), ($2, 'de', 'Blau')`, brandValue, blue)
	rows, err := db.New(r.conn).MerchantFeedProducts(context.Background(), db.MerchantFeedProductsParams{
		ChannelID: ch, BrandAttribute: brandSlug, Locale: "de", DefaultLocale: "en", PageLimit: 100,
	})
	if err != nil || len(rows) != 1 {
		t.Fatalf("variant query: %d rows, %v", len(rows), err)
	}
	var options map[string]string
	if err := json.Unmarshal(rows[0].VariantOptions, &options); err != nil {
		t.Fatal(err)
	}
	if rows[0].Brand != "Nordlinie" || rows[0].ItemGroupID != "SHIRT-"+r.sfx || options[colorSlug] != "Blau" {
		t.Fatalf("localized selection lost: brand %q, group %q, options %v", rows[0].Brand, rows[0].ItemGroupID, options)
	}
}

type testFeedWriter struct {
	*gzip.Writer
	encode func(io.Writer, sdk.ProductFeedItem) (bool, error)
}

func (w *testFeedWriter) WriteItem(p sdk.ProductFeedItem) (bool, error) { return w.encode(w.Writer, p) }
