package front

import (
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/qna"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

// A product taken off a store takes its Q&A with it - the questions name the product and quote its staff
func TestQuestionsFollowTheProductsVisibility(t *testing.T) {
	conn := testdb.Require(t)
	ch, sfx := uuid.New(), uuid.NewString()[:8]
	if _, err := conn.Exec(`INSERT INTO channels (id, slug, name, currency_code) VALUES ($1, $2, 'qna', 'USD')`, ch, "qnavis-"+sfx); err != nil {
		t.Fatal(err)
	}
	var typeID, productID uuid.UUID
	if err := conn.QueryRow(`INSERT INTO catalog_product_types (slug, name) VALUES ($1, 'T') RETURNING id`, "qnavis-"+sfx).Scan(&typeID); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(`INSERT INTO catalog_products (product_type_id, sku, url, kind) VALUES ($1, $2, $2, 'simple') RETURNING id`, typeID, "qnavis-"+sfx).Scan(&productID); err != nil {
		t.Fatal(err)
	}
	q := db.New(conn)
	qs := qna.NewService(q)
	asked, err := qs.Submit(t.Context(), productID, ch, nil, "G", "", "Does it run small?")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := qs.Answer(t.Context(), asked.ID, auth.Actor{Kind: auth.ActorAdmin, ID: uuid.New()}, "True to size."); err != nil {
		t.Fatal(err)
	}
	tx := txrunner.New(conn)
	r := &queryResolver{&Resolver{QnaSvc: qs, CatalogSvc: catalog.NewService(catalog.Deps{Repo: catalog.NewRepository(q, tx), Tx: tx})}}
	ctx := channel.WithChannel(t.Context(), &channel.Channel{ID: ch})

	if got, err := r.ProductQuestions(ctx, productID.String(), nil); err != nil || len(got) != 0 {
		t.Fatalf("unlisted product: %d questions, %v - want none", len(got), err)
	}
	if _, err := conn.Exec(`INSERT INTO catalog_product_channel_listings (product_id, channel_id, is_published) VALUES ($1, $2, TRUE)`, productID, ch); err != nil {
		t.Fatal(err)
	}
	if got, err := r.ProductQuestions(ctx, productID.String(), nil); err != nil || len(got) != 1 {
		t.Errorf("listed product: %d questions, %v - want 1", len(got), err)
	}
}

// The address is what a DSAR export and an erasure match a guest's questions on, so it has to be one
func TestAGuestQuestionNeedsARealEmail(t *testing.T) {
	conn := testdb.Require(t)
	ch, sfx := uuid.New(), uuid.NewString()[:8]
	if _, err := conn.Exec(`INSERT INTO channels (id, slug, name, currency_code) VALUES ($1, $2, 'qna', 'USD')`, ch, "qnamail-"+sfx); err != nil {
		t.Fatal(err)
	}
	var typeID, productID uuid.UUID
	if err := conn.QueryRow(`INSERT INTO catalog_product_types (slug, name) VALUES ($1, 'T') RETURNING id`, "qnamail-"+sfx).Scan(&typeID); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(`INSERT INTO catalog_products (product_type_id, sku, url, kind) VALUES ($1, $2, $2, 'simple') RETURNING id`, typeID, "qnamail-"+sfx).Scan(&productID); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`INSERT INTO catalog_product_channel_listings (product_id, channel_id, is_published) VALUES ($1, $2, TRUE)`, productID, ch); err != nil {
		t.Fatal(err)
	}
	q := db.New(conn)
	tx := txrunner.New(conn)
	r := &mutationResolver{&Resolver{QnaSvc: qna.NewService(q), CatalogSvc: catalog.NewService(catalog.Deps{Repo: catalog.NewRepository(q, tx), Tx: tx})}}
	ctx := channel.WithChannel(t.Context(), &channel.Channel{ID: ch})
	name := "Guest"
	for _, bad := range []string{"not-an-email", "Guest <g@example.test>"} {
		if ok, err := r.SubmitProductQuestion(ctx, productID.String(), "Does it run small?", &name, &bad); err == nil || ok {
			t.Errorf("%q accepted", bad)
		}
	}
	good := "g@example.test"
	if ok, err := r.SubmitProductQuestion(ctx, productID.String(), "Does it run small?", &name, &good); err != nil || !ok {
		t.Errorf("a valid address refused: %v", err)
	}
}
