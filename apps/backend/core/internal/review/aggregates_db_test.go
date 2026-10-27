package review

import (
	"context"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

func TestOverallAggregatesAreTheFullOnesWithoutPerQuestion(t *testing.T) {
	conn := testdb.Require(t)
	ctx := context.Background()
	ch := uuid.New()
	if _, err := conn.Exec(`INSERT INTO channels (id, slug, name, currency_code, is_active) VALUES ($1,$2,'R','USD',true)`, ch, "ag-"+ch.String()[:8]); err != nil {
		t.Fatal(err)
	}
	var typeID uuid.UUID
	if err := conn.QueryRow(`INSERT INTO catalog_product_types (slug, name) VALUES ($1, 'T') RETURNING id`, "ag-"+uuid.NewString()[:8]).Scan(&typeID); err != nil {
		t.Fatal(err)
	}
	product := func() uuid.UUID {
		var id uuid.UUID
		sku := "ag-" + uuid.NewString()[:8]
		if err := conn.QueryRow(`INSERT INTO catalog_products (product_type_id, sku, url, kind) VALUES ($1, $2, $2, 'simple') RETURNING id`, typeID, sku).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	reviewed, unreviewed := product(), product()
	var questions [2]uuid.UUID
	for i := range questions {
		if err := conn.QueryRow(`INSERT INTO review_questions (slug, input_type) VALUES ($1, 'rating_1_5') RETURNING id`, "ag-"+uuid.NewString()[:8]).Scan(&questions[i]); err != nil {
			t.Fatal(err)
		}
	}
	for _, ratings := range [][2]int{{5, 3}, {4, 4}} {
		var reviewID uuid.UUID
		if err := conn.QueryRow(`INSERT INTO reviews (product_id, guest_name, channel_id, locale, title, status) VALUES ($1, 'G', $2, 'en', 'r', 'approved') RETURNING id`, reviewed, ch).Scan(&reviewID); err != nil {
			t.Fatal(err)
		}
		for i, q := range questions {
			if _, err := conn.Exec(`INSERT INTO review_answers (review_id, question_id, rating_value) VALUES ($1, $2, $3)`, reviewID, q, ratings[i]); err != nil {
				t.Fatal(err)
			}
		}
	}
	svc := NewService(NewRepository(db.New(conn), txrunner.New(conn)), txrunner.New(conn))
	onCh := channel.WithChannel(ctx, &channel.Channel{ID: ch})
	ids := []uuid.UUID{reviewed, unreviewed}

	full, err := svc.GetAggregatesByProductIDs(onCh, ids)
	if err != nil {
		t.Fatal(err)
	}
	overall, err := svc.GetOverallAggregatesByProductIDs(onCh, ids)
	if err != nil {
		t.Fatal(err)
	}
	if len(full[reviewed].PerQuestion) != 2 || full[reviewed].Count != 2 {
		t.Fatalf("fixture didn't land: full aggregates %+v", full[reviewed])
	}
	want := full[reviewed]
	want.PerQuestion = nil
	if !reflect.DeepEqual(overall[reviewed], want) {
		t.Fatalf("overall = %+v, want the full aggregates minus PerQuestion %+v", overall[reviewed], want)
	}
	_, inFull := full[unreviewed]
	if got, inOverall := overall[unreviewed]; inOverall != inFull || got.Count != 0 {
		t.Fatalf("unreviewed product: overall %+v (present %v), full present %v", got, inOverall, inFull)
	}
}
