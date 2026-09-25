package review

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

func TestAStorefrontReadsOnlyItsOwnChannelsReviews(t *testing.T) {
	conn := testdb.Require(t)
	ctx := context.Background()
	a, b := uuid.New(), uuid.New()
	for _, ch := range []uuid.UUID{a, b} {
		if _, err := conn.Exec(`INSERT INTO channels (id, slug, name, currency_code, is_active) VALUES ($1,$2,'R','USD',true)`, ch, "rv-"+ch.String()[:8]); err != nil {
			t.Fatal(err)
		}
	}
	var typeID, productID uuid.UUID
	if err := conn.QueryRow(`INSERT INTO catalog_product_types (slug, name) VALUES ($1, 'T') RETURNING id`, "rv-"+uuid.NewString()[:8]).Scan(&typeID); err != nil {
		t.Fatal(err)
	}
	sku := "rv-" + uuid.NewString()[:8]
	if err := conn.QueryRow(`INSERT INTO catalog_products (product_type_id, sku, url, kind) VALUES ($1, $2, $2, 'simple') RETURNING id`, typeID, sku).Scan(&productID); err != nil {
		t.Fatal(err)
	}
	for ch, title := range map[uuid.UUID]string{a: "on A", b: "on B"} {
		if _, err := conn.Exec(`INSERT INTO reviews (product_id, guest_name, channel_id, locale, title, status) VALUES ($1, 'G', $2, 'en', $3, 'approved')`, productID, ch, title); err != nil {
			t.Fatal(err)
		}
	}
	svc := NewService(NewRepository(db.New(conn), txrunner.New(conn)), txrunner.New(conn))
	onB := channel.WithChannel(ctx, &channel.Channel{ID: b})

	rows, total, err := svc.ListReviewsForProduct(onB, productID, 10, 0)
	if err != nil || total != 1 || len(rows) != 1 || rows[0].Title != "on B" {
		t.Fatalf("channel B read %d/%d reviews %+v, %v - want only its own", len(rows), total, rows, err)
	}
	batch, err := svc.ListReviewsForProductIDs(onB, []uuid.UUID{productID}, 10, 0)
	if err != nil || len(batch[productID]) != 1 {
		t.Fatalf("batched read on B = %v, %v", batch, err)
	}
	if rows, _, _ := svc.ListReviewsForProduct(ctx, productID, 10, 0); len(rows) != 2 {
		t.Fatalf("storeless admin read = %d, want every channel's", len(rows))
	}

	if err := svc.repo.UpsertProductSummary(ctx, productID, a, "A's summary", 3); err != nil {
		t.Fatal(err)
	}
	if got, _ := svc.GetProductSummary(onB, productID); got != "" {
		t.Fatalf("channel B served channel A's summary %q", got)
	}
	if got, _ := svc.GetProductSummary(channel.WithChannel(ctx, &channel.Channel{ID: a}), productID); got != "A's summary" {
		t.Fatalf("channel A summary = %q", got)
	}
}
