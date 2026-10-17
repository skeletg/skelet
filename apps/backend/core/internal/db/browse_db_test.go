package db

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	sqlc "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

func seedChannel(t *testing.T, tx *sql.Tx, prefix string) (uuid.UUID, string) {
	t.Helper()
	id, slug := uuid.New(), prefix+"-"+uuid.NewString()[:8]
	if _, err := tx.Exec(`INSERT INTO channels (id, slug, name, currency_code, is_active) VALUES ($1,$2,'L','USD',true)`, id, slug); err != nil {
		t.Fatal(err)
	}
	return id, slug
}

func TestBrowseRecoveryExcludesShopperCartsButNotOffers(t *testing.T) {
	tx, err := testdb.Require(t).Begin()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback() })
	channelID, _ := seedChannel(t, tx, "browse-offers")
	var groupID uuid.UUID
	if err := tx.QueryRow(`SELECT id FROM customer_groups LIMIT 1`).Scan(&groupID); err != nil {
		t.Fatal(err)
	}
	q := sqlc.New(tx)
	for _, tc := range []struct {
		name, kind                                string
		lines, expired, revoked, shopper, exclude bool
	}{
		{name: "nonempty shopper", kind: "shopper", lines: true, exclude: true},
		{name: "empty shopper", kind: "shopper"},
		{name: "open offer", kind: "offer", lines: true},
		{name: "expired offer", kind: "offer", lines: true, expired: true},
		{name: "withdrawn offer", kind: "offer", lines: true, revoked: true},
		{name: "offer beside shopper cart", kind: "offer", lines: true, shopper: true, exclude: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			customerID, cartID := uuid.New(), uuid.New()
			if _, err := tx.Exec(`INSERT INTO customers (id,email,password_hash,customer_group_id,channel_id) VALUES ($1,$2,'x',$3,$4)`, customerID, customerID.String()+"@example.test", groupID, channelID); err != nil {
				t.Fatal(err)
			}
			expiry := time.Now().Add(time.Hour)
			if tc.expired {
				expiry = time.Now().Add(-time.Hour)
			}
			if _, err := tx.Exec(`INSERT INTO carts (id,customer_id,channel_id,kind,offer_expires_at,offer_link_token,offer_revoked_at) VALUES ($1,$2,$3,$4,$5,translate(rtrim(encode(gen_random_bytes(16),'base64'),'='),'+/','-_'),CASE WHEN $6 THEN NOW() ELSE NULL END)`, cartID, customerID, channelID, tc.kind, expiry, tc.revoked); err != nil {
				t.Fatal(err)
			}
			if tc.lines {
				if _, err := tx.Exec(`INSERT INTO cart_lines (cart_id,product_name,product_sku,unit_price,quantity) VALUES ($1,'Item','SKU',100,1)`, cartID); err != nil {
					t.Fatal(err)
				}
			}
			if tc.shopper {
				shopper, err := q.CreateCart(context.Background(), sqlc.CreateCartParams{CustomerID: uuid.NullUUID{UUID: customerID, Valid: true}, ChannelID: channelID})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := tx.Exec(`INSERT INTO cart_lines (cart_id,product_name,product_sku,unit_price,quantity) VALUES ($1,'Item','SKU',100,1)`, shopper.ID); err != nil {
					t.Fatal(err)
				}
			}
			got, err := q.BrowseAbandonersExcluded(context.Background(), sqlc.BrowseAbandonersExcludedParams{CustomerIds: []uuid.UUID{customerID}, BrowseAts: []time.Time{time.Now().Add(-time.Hour)}})
			if err != nil {
				t.Fatal(err)
			}
			if (len(got) == 1) != tc.exclude {
				t.Fatalf("excluded=%v, want %v", got, tc.exclude)
			}
		})
	}
}
