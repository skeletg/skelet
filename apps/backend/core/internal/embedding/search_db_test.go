package embedding

import (
	"context"
	"database/sql"
	"math"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/ai"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

// A direction in the embedding space: weights on the first few axes, scaled to length one
func direction(weights ...float64) []float32 {
	v := make([]float32, ai.EmbeddingDimensions)
	var norm float64
	for _, w := range weights {
		norm += w * w
	}
	for i, w := range weights {
		v[i] = float32(w / math.Sqrt(norm))
	}
	return v
}

// Answers every question with the same direction, counting how often it's asked
type questionEmbedder struct {
	model   string
	vec     []float32
	asked   int
	purpose ai.Purpose
}

func (q *questionEmbedder) Model(context.Context) string { return q.model }

func (q *questionEmbedder) Embed(_ context.Context, purpose ai.Purpose, texts []string) ([][]float32, int64, error) {
	q.asked++
	q.purpose = purpose
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = q.vec
	}
	return out, 1, nil
}

func putVector(t *testing.T, conn *sql.DB, source string, id uuid.UUID, locale string, chunk int, content string, v []float32) {
	t.Helper()
	exec(t, conn, `INSERT INTO embeddings (source, source_id, locale, model, chunk, content, content_hash, embedding) VALUES ($1, $2, $3, 'm1', $4, $5, '\x00', CAST($6::text AS vector))`,
		source, id, locale, chunk, content, vectorLiteral(v))
}

func TestTheSearcherFindsTheStoresNearestInTheShoppersLanguage(t *testing.T) {
	conn := testdb.Require(t)
	ctx := context.Background()
	sfx := uuid.NewString()[:8]
	exec(t, conn, `INSERT INTO locales (code, name) VALUES ('de', 'German') ON CONFLICT (code) DO NOTHING`)
	ours := scanID(t, conn, `INSERT INTO channels (slug, name, currency_code) VALUES ($1, 'ours', 'USD') RETURNING id`, "ours-"+sfx)
	theirs := scanID(t, conn, `INSERT INTO channels (slug, name, currency_code) VALUES ($1, 'theirs', 'USD') RETURNING id`, "theirs-"+sfx)
	kind := scanID(t, conn, `INSERT INTO catalog_product_types (slug, name) VALUES ($1, 'k') RETURNING id`, "k-"+sfx)
	product := func(sku string, channel uuid.UUID) uuid.UUID {
		id := scanID(t, conn, `INSERT INTO catalog_products (product_type_id, sku, url, kind) VALUES ($1, $2, $2, 'simple') RETURNING id`, kind, sku+"-"+sfx)
		exec(t, conn, `INSERT INTO catalog_product_channel_listings (product_id, channel_id, is_published) VALUES ($1, $2, TRUE)`, id, channel)
		return id
	}
	jacket, beanie, towel, elsewhere := product("jacket", ours), product("beanie", ours), product("towel", ours), product("elsewhere", theirs)
	putVector(t, conn, SourceProduct, jacket, "en", 0, "jacket", direction(1))
	putVector(t, conn, SourceProduct, jacket, "de", 0, "Jacke", direction(1, 0.1))
	putVector(t, conn, SourceProduct, beanie, "en", 0, "beanie", direction(1, 1))
	putVector(t, conn, SourceProduct, towel, "en", 0, "towel", direction(0, 1))
	putVector(t, conn, SourceProduct, elsewhere, "en", 0, "nearest of all, another store's", direction(1))

	emb := &questionEmbedder{model: "m1", vec: direction(1)}
	s := NewSearcher(db.New(conn), txrunner.New(conn), emb)
	hits, err := s.Products(ctx, ours, []string{"de", "en"}, "something warm", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 3 || hits[0].ProductID != jacket || hits[1].ProductID != beanie || hits[2].ProductID != towel {
		t.Fatalf("hits = %+v - want this store's three, nearest first, the jacket once though it's in two languages", hits)
	}
	if hits[0].Distance > 0.01 || hits[1].Distance <= hits[0].Distance {
		t.Errorf("distances %v, %v", hits[0].Distance, hits[1].Distance)
	}
	if _, err := s.Products(ctx, ours, []string{"en"}, " something   warm ", 1); err != nil || emb.asked != 1 {
		t.Errorf("the same question asked again went to the provider (%d asks), %v", emb.asked, err)
	}
	if emb.purpose != ai.ForQuery {
		t.Errorf("the question went as %q - a provider that tells them apart should hear it's a search", emb.purpose)
	}
	emb.model = ""
	if none, err := s.Products(ctx, ours, []string{"en"}, "something warm", 3); err != nil || none != nil {
		t.Errorf("with embeddings off = %+v, %v", none, err)
	}
	emb.model = "m1"

	pageType := scanID(t, conn, `INSERT INTO cms_page_types (slug) VALUES ($1) RETURNING id`, "pt-"+sfx)
	page := func(slug string, listed bool) uuid.UUID {
		id := scanID(t, conn, `INSERT INTO cms_pages (page_type_id, slug, base_path, url_path, is_published) VALUES ($1, $2, $2, $2, TRUE) RETURNING id`, pageType, slug+"-"+sfx)
		exec(t, conn, `INSERT INTO cms_page_content (page_id, locale, title) VALUES ($1, 'en', $2), ($1, 'de', $2)`, id, slug)
		if listed {
			exec(t, conn, `INSERT INTO cms_page_channel_listings (page_id, channel_id, is_published) VALUES ($1, $2, TRUE)`, id, ours)
		}
		return id
	}
	shipping, unlisted := page("shipping", true), page("hidden", false)
	putVector(t, conn, SourcePage, shipping, "en", 0, "Shipping - EU\nTwo days.", direction(1, 0.2))
	putVector(t, conn, SourcePage, shipping, "en", 1, "Shipping - Elsewhere\nA week.", direction(0, 1))
	putVector(t, conn, SourcePage, shipping, "de", 0, "Versand - EU\nZwei Tage.", direction(1, 0.3))
	putVector(t, conn, SourcePage, unlisted, "en", 0, "Hidden\nNearest of all, not on this store.", direction(1))

	passages, err := s.Passages(ctx, ours, []string{"de", "en"}, "lieferung", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(passages) != 1 || passages[0].Text != "Versand - EU\nZwei Tage." || passages[0].Path != "shipping-"+sfx || passages[0].Title != "shipping" {
		t.Errorf("passages = %+v - want the German one, the English ones of the same page left out, and no unlisted page", passages)
	}
	passages, _ = s.Passages(ctx, ours, []string{"en"}, "delivery", 5)
	if len(passages) != 2 || passages[0].Text != "Shipping - EU\nTwo days." {
		t.Errorf("English passages = %+v - want both, nearest first", passages)
	}
}

func TestSimilarProductsShareTheValuesTheStoreSaysMustMatch(t *testing.T) {
	conn := testdb.Require(t)
	ctx := context.Background()
	sfx := uuid.NewString()[:8]
	ours := scanID(t, conn, `INSERT INTO channels (slug, name, currency_code) VALUES ($1, 'ours', 'USD') RETURNING id`, "sim-"+sfx)
	kind := scanID(t, conn, `INSERT INTO catalog_product_types (slug, name) VALUES ($1, 'k') RETURNING id`, "sk-"+sfx)
	brand := "brand-" + sfx
	gender := scanID(t, conn, `INSERT INTO catalog_attributes (slug, name, input_type, similar_match) VALUES ($1, 'Gender', 'dropdown', TRUE) RETURNING id`, "gender-"+sfx)
	exec(t, conn, `INSERT INTO catalog_attributes (slug, name, input_type, similar_match) VALUES ($1, 'Brand', 'plain_text', TRUE)`, brand)
	colour := scanID(t, conn, `INSERT INTO catalog_attributes (slug, name, input_type) VALUES ($1, 'Colour', 'swatch') RETURNING id`, "colour-"+sfx)
	women := scanID(t, conn, `INSERT INTO catalog_attribute_values (attribute_id, slug, name) VALUES ($1, 'women', 'Women') RETURNING id`, gender)
	men := scanID(t, conn, `INSERT INTO catalog_attribute_values (attribute_id, slug, name) VALUES ($1, 'men', 'Men') RETURNING id`, gender)
	unisex := scanID(t, conn, `INSERT INTO catalog_attribute_values (attribute_id, slug, name, matches_any) VALUES ($1, 'unisex', 'Unisex', TRUE) RETURNING id`, gender)
	black := scanID(t, conn, `INSERT INTO catalog_attribute_values (attribute_id, slug, name) VALUES ($1, 'black', 'Black') RETURNING id`, colour)

	product := func(sku, kindOf, maker string, sex *uuid.UUID, near []float32) uuid.UUID {
		id := scanID(t, conn, `INSERT INTO catalog_products (product_type_id, sku, url, kind, attributes) VALUES ($1, $2, $2, $3::catalog_product_kind, jsonb_build_object($4::text, $5::text)) RETURNING id`,
			kind, sku+"-"+sfx, kindOf, brand, maker)
		exec(t, conn, `INSERT INTO catalog_product_channel_listings (product_id, channel_id, is_published) VALUES ($1, $2, TRUE)`, id, ours)
		if sex != nil {
			exec(t, conn, `INSERT INTO catalog_product_attribute_values (product_id, attribute_id, attribute_value_id) VALUES ($1, $2, $3)`, id, gender, *sex)
		}
		if near != nil {
			putVector(t, conn, SourceProduct, id, "en", 0, sku, near)
		}
		return id
	}
	dress := product("dress", "configurable", "Saône", &women, direction(1))
	variant := product("dress-black", "simple", "Saône", nil, nil)
	exec(t, conn, `INSERT INTO catalog_product_children (parent_id, child_id, sort_order) VALUES ($1, $2, 0)`, dress, variant)
	exec(t, conn, `INSERT INTO catalog_product_attribute_values (product_id, attribute_id, attribute_value_id) VALUES ($1, $2, $3)`, variant, colour, black)
	skirt := product("skirt", "simple", "  SAÔNE ", &women, direction(1, 0.2))
	product("shirt", "simple", "Saône", &men, direction(1, 0.05))
	blouse := product("blouse", "simple", "Northfold", &women, direction(1, 0.1))
	product("tee", "simple", "Saône", nil, direction(1, 0.15))
	cardigan := product("cardigan", "simple", "Northfold", &women, direction(1, 0.3))
	scarf := product("scarf", "simple", "Saône", &unisex, direction(1, 0.25))

	s := NewSearcher(db.New(conn), txrunner.New(conn), &questionEmbedder{model: "m1"})
	one, err := s.Similar(ctx, ours, []string{"en"}, []uuid.UUID{variant}, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(one.Hits) != 2 || one.Hits[0].ProductID != skirt || one.Hits[1].ProductID != scarf {
		t.Errorf("like the black dress = %+v - want the skirt and the unisex scarf: the shirt is men's, the blouse another brand, the tee has no gender", one.Hits)
	}
	if len(one.Matched) != 2 || one.Matched[0] != "Brand" || one.Matched[1] != "Gender" {
		t.Errorf("matched on %v", one.Matched)
	}

	two, err := s.Similar(ctx, ours, []string{"en"}, []uuid.UUID{dress, blouse}, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(two.Hits) != 3 || two.Hits[0].ProductID != skirt || two.Hits[1].ProductID != scarf || two.Hits[2].ProductID != cardigan {
		t.Errorf("like the dress and the blouse = %+v - want either one's brand to count, and neither of them back", two.Hits)
	}

	byScarf, err := s.Similar(ctx, ours, []string{"en"}, []uuid.UUID{scarf}, 5)
	if err != nil {
		t.Fatal(err)
	}
	got := map[uuid.UUID]bool{}
	for _, h := range byScarf.Hits {
		got[h.ProductID] = true
	}
	if !got[dress] || !got[skirt] || len(byScarf.Hits) != 3 || got[blouse] {
		t.Errorf("like the unisex scarf = %+v - want women's and men's Saône alike, the shirt included, still not the other brand or the tee without a gender", byScarf.Hits)
	}

	exec(t, conn, `UPDATE catalog_attributes SET is_public = FALSE WHERE slug = $1`, brand)
	private, err := s.Similar(ctx, ours, []string{"en"}, []uuid.UUID{variant}, 5)
	if err != nil || len(private.Hits) != 2 || len(private.Matched) != 1 || private.Matched[0] != "Gender" {
		t.Errorf("with the brand private = %+v, %v - want it still matched on but never named to a shopper", private, err)
	}
	fresh := product("fresh", "simple", "Saône", &women, nil)
	if none, err := s.Similar(ctx, ours, []string{"en"}, []uuid.UUID{fresh}, 5); err != nil || !none.Unembedded || len(none.Hits) != 0 {
		t.Errorf("a product not embedded yet = %+v, %v - want it said, not passed off as nothing close", none, err)
	}
}
