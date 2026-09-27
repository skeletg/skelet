package db

import (
	"database/sql"
	"os"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

func runFile(t *testing.T, tx *sql.Tx, path string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(string(raw)); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}

func seedChannel(t *testing.T, tx *sql.Tx, prefix string) (uuid.UUID, string) {
	t.Helper()
	id, slug := uuid.New(), prefix+"-"+uuid.NewString()[:8]
	if _, err := tx.Exec(`INSERT INTO channels (id, slug, name, currency_code, is_active) VALUES ($1,$2,'L','USD',true)`, id, slug); err != nil {
		t.Fatal(err)
	}
	return id, slug
}

func TestMigration9KeepsGiftCardsAndRestoresReviewSummaries(t *testing.T) {
	conn := testdb.Require(t)
	tx, err := conn.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	runFile(t, tx, "migrations/000009_channel_liabilities.down.sql")

	ch, _ := seedChannel(t, tx, "liab")
	var orderID, cardID, typeID, productID uuid.UUID
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(tx.QueryRow(`INSERT INTO orders (channel_id, currency_code) VALUES ($1, 'USD') RETURNING id`, ch).Scan(&orderID))
	must(tx.QueryRow(`INSERT INTO gift_cards (code, currency_code, initial_balance, balance, issued_order_id) VALUES ($1, 'USD', 500, 500, $2) RETURNING id`, "GC-"+uuid.NewString()[:8], orderID).Scan(&cardID))
	must(tx.QueryRow(`INSERT INTO catalog_product_types (slug, name) VALUES ($1, 'T') RETURNING id`, "liab-"+uuid.NewString()[:8]).Scan(&typeID))
	sku := "liab-" + uuid.NewString()[:8]
	must(tx.QueryRow(`INSERT INTO catalog_products (product_type_id, sku, url, kind) VALUES ($1, $2, $2, 'simple') RETURNING id`, typeID, sku).Scan(&productID))
	_, err = tx.Exec(`INSERT INTO reviews (product_id, guest_name, channel_id, locale, title, status) VALUES ($1, 'G', $2, 'en', 'ok', 'approved')`, productID, ch)
	must(err)

	runFile(t, tx, "migrations/000009_channel_liabilities.up.sql")

	var cardChannel uuid.NullUUID
	must(tx.QueryRow(`SELECT channel_id FROM gift_cards WHERE id = $1`, cardID).Scan(&cardChannel))
	if cardChannel.UUID != ch {
		t.Fatalf("order-issued card channel = %v, want its order's %v", cardChannel, ch)
	}
	var stale bool
	if err := tx.QueryRow(`SELECT is_stale FROM product_review_summaries WHERE product_id = $1 AND channel_id = $2`, productID, ch).Scan(&stale); err != nil || !stale {
		t.Fatalf("summary for an approved product = %v, %v, want a stale row for regeneration", stale, err)
	}
	lone, _ := seedChannel(t, tx, "liab")
	_, err = tx.Exec(`INSERT INTO gift_cards (code, currency_code, initial_balance, balance, channel_id) VALUES ($1, 'USD', 500, 500, $2)`, "GC-"+uuid.NewString()[:8], lone)
	must(err)
	if _, err := tx.Exec(`DELETE FROM channels WHERE id = $1`, lone); !IsForeignKeyViolation(err) {
		t.Fatalf("deleting a channel holding a gift card = %v, want a foreign key refusal", err)
	}
}

func TestMigration6DownSurvivesPerChannelSlugs(t *testing.T) {
	conn := testdb.Require(t)
	tx, err := conn.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	a, aSlug := seedChannel(t, tx, "dup")
	b, _ := seedChannel(t, tx, "dup")
	slug := "sale-" + uuid.NewString()[:8]
	for _, ch := range []any{nil, a, b} {
		if _, err := tx.Exec(`INSERT INTO promotion_rules (slug, name, channel_id) VALUES ($1, 'P', $2)`, slug, ch); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(`INSERT INTO remote_providers (kind, slug, name, target_url, signing_secret, channel_id) VALUES ('pricing', $1, 'R', 'https://p.example.test', 's', $2)`, slug, ch); err != nil {
			t.Fatal(err)
		}
	}

	runFile(t, tx, "migrations/000006_channel_integrity.down.sql")

	for _, table := range []string{"promotion_rules", "remote_providers"} {
		var global string
		if err := tx.QueryRow(`SELECT slug FROM `+table+` WHERE slug LIKE $1 AND channel_id IS NULL`, slug+"%").Scan(&global); err != nil || global != slug {
			t.Fatalf("%s global row slug = %q, %v, want it kept as %q", table, global, err, slug)
		}
		var renamed string
		if err := tx.QueryRow(`SELECT slug FROM `+table+` WHERE slug LIKE $1 AND channel_id = $2`, slug+"%", a).Scan(&renamed); err != nil || renamed != slug+"-"+aSlug {
			t.Fatalf("%s channel row slug = %q, %v, want the channel's slug appended", table, renamed, err)
		}
	}
}
