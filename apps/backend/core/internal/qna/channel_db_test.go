package qna

import (
	"context"
	"testing"

	"github.com/google/uuid"
	_ "github.com/lib/pq"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

func TestAStoreReadsOnlyItsOwnPublishedQuestions(t *testing.T) {
	conn := testdb.Require(t)
	ctx := context.Background()
	a, b := uuid.New(), uuid.New()
	for _, ch := range []uuid.UUID{a, b} {
		if _, err := conn.Exec(`INSERT INTO channels (id, slug, name, currency_code, is_active) VALUES ($1,$2,'Q','USD',true)`, ch, "qna-"+ch.String()[:8]); err != nil {
			t.Fatal(err)
		}
	}
	var typeID, productID uuid.UUID
	if err := conn.QueryRow(`INSERT INTO catalog_product_types (slug, name) VALUES ($1, 'T') RETURNING id`, "qna-"+uuid.NewString()[:8]).Scan(&typeID); err != nil {
		t.Fatal(err)
	}
	sku := "qna-" + uuid.NewString()[:8]
	if err := conn.QueryRow(`INSERT INTO catalog_products (product_type_id, sku, url, kind) VALUES ($1, $2, $2, 'simple') RETURNING id`, typeID, sku).Scan(&productID); err != nil {
		t.Fatal(err)
	}
	svc := NewService(db.New(conn))
	for ch, text := range map[uuid.UUID]string{a: "asked on store A?", b: "asked on store B?"} {
		q, err := svc.Submit(ctx, productID, ch, nil, "G", "g@example.test", text)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Answer(ctx, q.ID, uuid.New(), "yes"); err != nil {
			t.Fatal(err)
		}
	}
	got, err := svc.Published(ctx, productID, b, 10)
	if err != nil || len(got) != 1 || got[0].Question != "asked on store B?" || got[0].ChannelID != b {
		t.Fatalf("store B read %+v, %v, want only its own question", got, err)
	}
}
