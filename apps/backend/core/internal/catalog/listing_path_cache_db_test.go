package catalog

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/cache"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

func TestAMovedListingURLStopsResolvingAtOnce(t *testing.T) {
	conn := testdb.Require(t)
	dsn := os.Getenv("VALKEY_URL")
	if dsn == "" {
		t.Skip("set VALKEY_URL to exercise the channel path cache")
	}
	opt, err := redis.ParseURL(dsn)
	if err != nil {
		t.Fatal(err)
	}
	client := redis.NewClient(opt)
	t.Cleanup(func() { _ = client.Close() })
	ctx := context.Background()
	suffix := uuid.NewString()[:8]
	var typeID, productID, ch uuid.UUID
	if err := conn.QueryRow(`INSERT INTO catalog_product_types (slug, name) VALUES ($1, 'pc') RETURNING id`, "pc-"+suffix).Scan(&typeID); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(`INSERT INTO catalog_products (product_type_id, sku, url, kind) VALUES ($1, $2, $2, 'simple') RETURNING id`, typeID, "PC-"+suffix).Scan(&productID); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(`INSERT INTO channels (slug, name, currency_code) VALUES ($1, 'pc', 'USD') RETURNING id`, "pc-"+suffix).Scan(&ch); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(`DELETE FROM catalog_products WHERE id = $1`, productID)
		_, _ = conn.Exec(`DELETE FROM catalog_product_types WHERE id = $1`, typeID)
		_, _ = conn.Exec(`DELETE FROM channels WHERE id = $1`, ch)
	})
	svc := &listingService{core: &core{repo: &postgresRepository{queries: db.New(conn)}, channelPathCache: cache.New(client, "test:pathcache:"+suffix)}}
	price := int64(100)
	set := func(url string) {
		t.Helper()
		if _, err := svc.SetProductListing(ctx, ProductChannelListingInput{ProductID: productID, ChannelID: ch, Price: &price, IsPublished: true, URL: &url}); err != nil {
			t.Fatalf("set listing %s: %v", url, err)
		}
	}
	oldPath, newPath := "old-"+suffix, "new-"+suffix
	set(oldPath)
	if id, err := svc.ResolveProductIDForChannelPath(ctx, ch, oldPath); err != nil || id != productID {
		t.Fatalf("old path = %v, %v", id, err)
	}
	if _, err := svc.ResolveProductIDForChannelPath(ctx, ch, newPath); !errors.Is(err, ErrListingNotFound) {
		t.Fatalf("new path before the move = %v, want not found", err)
	}
	set(newPath)
	if _, err := svc.ResolveProductIDForChannelPath(ctx, ch, oldPath); !errors.Is(err, ErrListingNotFound) {
		t.Errorf("the moved-away path still resolved from the cache (%v)", err)
	}
	if id, err := svc.ResolveProductIDForChannelPath(ctx, ch, newPath); err != nil || id != productID {
		t.Errorf("the new path stayed behind a cached miss: %v, %v", id, err)
	}
	if err := svc.ClearProductListing(ctx, productID, ch); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ResolveProductIDForChannelPath(ctx, ch, newPath); !errors.Is(err, ErrListingNotFound) {
		t.Errorf("a cleared listing's path still resolved from the cache (%v)", err)
	}
}
