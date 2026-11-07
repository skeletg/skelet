package embedding

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/ai"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/health"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

type fakeEmbedder struct {
	model    string
	texts    []string
	purposes []ai.Purpose
	err      error
	refuse   string
}

func (f *fakeEmbedder) Model(context.Context) string { return f.model }

func (f *fakeEmbedder) Embed(_ context.Context, purpose ai.Purpose, texts []string) ([][]float32, int64, error) {
	if f.err != nil {
		return nil, 0, f.err
	}
	for _, t := range texts {
		if f.refuse != "" && strings.Contains(t, f.refuse) {
			return nil, 0, fmt.Errorf("%w: 400 the input is too long", ai.ErrEmbeddingRefused)
		}
	}
	f.purposes = append(f.purposes, purpose)
	f.texts = append(f.texts, texts...)
	out := make([][]float32, len(texts))
	for i := range texts {
		v := make([]float32, ai.EmbeddingDimensions)
		v[len(f.texts)%ai.EmbeddingDimensions] = 1
		out[i] = v
	}
	return out, int64(len(texts)), nil
}

type seeded struct {
	channel                                 uuid.UUID
	product, variant, unlisted, page, draft uuid.UUID
}

func exec(t *testing.T, conn *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := conn.Exec(query, args...); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

func scanID(t *testing.T, conn *sql.DB, query string, args ...any) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := conn.QueryRow(query, args...).Scan(&id); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return id
}

func seed(t *testing.T, conn *sql.DB) seeded {
	t.Helper()
	sfx := uuid.NewString()[:8]
	var s seeded
	exec(t, conn, `INSERT INTO locales (code, name) VALUES ('de', 'German') ON CONFLICT (code) DO NOTHING`)
	channel := scanID(t, conn, `INSERT INTO channels (slug, name, currency_code) VALUES ($1, 'emb', 'USD') RETURNING id`, "emb-"+sfx)
	s.channel = channel
	kind := scanID(t, conn, `INSERT INTO catalog_product_types (slug, name) VALUES ($1, 'Bags') RETURNING id`, "bags-"+sfx)
	product := func(sku string, listed bool) uuid.UUID {
		id := scanID(t, conn, `INSERT INTO catalog_products (product_type_id, sku, url, kind, attributes) VALUES ($1, $2, $2, 'simple', '{"volume-`+sfx+`": 28, "supplier-`+sfx+`": "ACME-77"}') RETURNING id`, kind, sku+"-"+sfx)
		if listed {
			exec(t, conn, `INSERT INTO catalog_product_channel_listings (product_id, channel_id, is_published) VALUES ($1, $2, TRUE)`, id, channel)
		}
		return id
	}
	s.product, s.variant, s.unlisted = product("pack", true), product("pack-olive", true), product("hidden", false)
	exec(t, conn, `INSERT INTO catalog_product_children (parent_id, child_id, sort_order) VALUES ($1, $2, 0)`, s.product, s.variant)
	for _, id := range []uuid.UUID{s.product, s.variant, s.unlisted} {
		exec(t, conn, `INSERT INTO catalog_product_content (product_id, locale, name, description) VALUES ($1, 'en', 'Canvas Backpack', '{"plain": "A carry-on pack."}'), ($1, 'de', 'Canvas-Rucksack', '{"plain": "Ein Handgepäck-Rucksack."}')`, id)
	}
	colour := scanID(t, conn, `INSERT INTO catalog_attributes (slug, name, input_type, is_public) VALUES ($1, 'Colour', 'dropdown', TRUE) RETURNING id`, "colour-"+sfx)
	exec(t, conn, `INSERT INTO catalog_attribute_content (attribute_id, locale, name) VALUES ($1, 'de', 'Farbe')`, colour)
	olive := scanID(t, conn, `INSERT INTO catalog_attribute_values (attribute_id, slug, name) VALUES ($1, 'olive', 'Olive') RETURNING id`, colour)
	exec(t, conn, `INSERT INTO catalog_attribute_value_content (value_id, locale, name) VALUES ($1, 'de', 'Oliv')`, olive)
	exec(t, conn, `INSERT INTO catalog_product_attribute_values (product_id, attribute_id, attribute_value_id) VALUES ($1, $2, $3)`, s.product, colour, olive)
	exec(t, conn, `INSERT INTO catalog_attributes (slug, name, input_type, unit, is_public) VALUES ($1, 'Volume', 'numeric', 'l', TRUE)`, "volume-"+sfx)
	exec(t, conn, `INSERT INTO catalog_attributes (slug, name, input_type, is_public) VALUES ($1, 'Supplier code', 'plain_text', FALSE)`, "supplier-"+sfx)
	category := scanID(t, conn, `INSERT INTO catalog_categories (slug, url_path) VALUES ($1, $1) RETURNING id`, "travel-"+sfx)
	exec(t, conn, `INSERT INTO catalog_category_content (category_id, locale, name) VALUES ($1, 'en', 'Travel'), ($1, 'de', 'Reisen')`, category)
	exec(t, conn, `INSERT INTO catalog_product_categories (product_id, category_id, position) VALUES ($1, $2, 0)`, s.product, category)

	pageType := scanID(t, conn, `INSERT INTO cms_page_types (slug) VALUES ($1) RETURNING id`, "info-"+sfx)
	page := func(slug string, published bool) uuid.UUID {
		id := scanID(t, conn, `INSERT INTO cms_pages (page_type_id, slug, base_path, url_path, is_published) VALUES ($1, $2, $2, $2, $3) RETURNING id`, pageType, slug+"-"+sfx, published)
		exec(t, conn, `INSERT INTO cms_page_content (page_id, locale, title, body) VALUES ($1, 'en', 'Shipping', $2)`, id,
			`{"version": 1, "blocks": [{"type": "rich_text", "props": {"heading": "In the EU", "body": {"plain": "Two to four days."}}}, {"type": "rich_text", "props": {"heading": "Elsewhere", "body": {"plain": "A week."}}}]}`)
		return id
	}
	s.page, s.draft = page("shipping", true), page("draft", false)
	return s
}

type passage struct {
	locale  string
	model   string
	chunk   int
	content string
}

func passages(t *testing.T, conn *sql.DB, id uuid.UUID) []passage {
	t.Helper()
	rows, err := conn.Query(`SELECT locale, model, chunk, content FROM embeddings WHERE source_id = $1 ORDER BY model, locale, chunk`, id)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []passage
	for rows.Next() {
		var p passage
		if err := rows.Scan(&p.locale, &p.model, &p.chunk, &p.content); err != nil {
			t.Fatal(err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestTheIndexerEmbedsWhatShoppersCanFindAndOnlyWhatChanged(t *testing.T) {
	conn := testdb.Require(t)
	ctx := context.Background()
	s := seed(t, conn)
	emb := &fakeEmbedder{model: "m1"}
	x := New(db.New(conn), txrunner.New(conn), emb)

	if err := x.Run(ctx); err != nil {
		t.Fatal(err)
	}
	product := passages(t, conn, s.product)
	if len(product) != 2 || product[0].locale != "de" || product[1].locale != "en" {
		t.Fatalf("product passages = %+v", product)
	}
	en, de := product[1].content, product[0].content
	for _, want := range []string{"Canvas Backpack", "Type: Bags", "Categories: Travel", "Colour: Olive", "Volume: 28 l", "A carry-on pack."} {
		if !strings.Contains(en, want) {
			t.Errorf("the English passage lacks %q:\n%s", want, en)
		}
	}
	if !strings.Contains(de, "Farbe: Oliv") || !strings.Contains(de, "Categories: Reisen") {
		t.Errorf("the German passage didn't read the translations:\n%s", de)
	}
	if strings.Contains(en+de, "ACME-77") || strings.Contains(en+de, "Supplier") {
		t.Error("a private attribute went to the provider")
	}
	for name, id := range map[string]uuid.UUID{"a variant": s.variant, "an unlisted product": s.unlisted, "an unpublished page": s.draft} {
		if got := passages(t, conn, id); len(got) != 0 {
			t.Errorf("%s was embedded: %+v", name, got)
		}
	}
	page := passages(t, conn, s.page)
	if len(page) != 2 || !strings.HasPrefix(page[0].content, "Shipping - In the EU\n") || !strings.HasPrefix(page[1].content, "Shipping - Elsewhere\n") {
		t.Fatalf("page passages = %+v", page)
	}

	for _, p := range emb.purposes {
		if p != ai.ForDocument {
			t.Fatalf("the indexer embedded as %q - what's stored is what gets searched", p)
		}
	}
	sent := len(emb.texts)
	if err := x.Run(ctx); err != nil || len(emb.texts) != sent {
		t.Fatalf("a run with nothing changed sent %d more texts, %v", len(emb.texts)-sent, err)
	}
	exec(t, conn, `UPDATE catalog_product_content SET name = 'Canvas Travel Backpack' WHERE product_id = $1 AND locale = 'en'`, s.product)
	if err := x.Enqueue(ctx, SourceProduct, s.product); err != nil {
		t.Fatal(err)
	}
	if err := x.Run(ctx); err != nil || len(emb.texts) != sent+1 || !strings.HasPrefix(emb.texts[sent], "Canvas Travel Backpack") {
		t.Fatalf("an English rename sent %v, %v - want the one changed passage", emb.texts[sent:], err)
	}

	exec(t, conn, `UPDATE cms_pages SET is_published = FALSE WHERE id = $1`, s.page)
	if err := x.Enqueue(ctx, SourcePage, s.page); err != nil {
		t.Fatal(err)
	}
	if err := x.Run(ctx); err != nil || len(passages(t, conn, s.page)) != 0 {
		t.Errorf("an unpublished page kept its passages: %v", err)
	}
	if backlog, err := db.New(conn).EmbeddingBacklog(ctx); err != nil || backlog.Queued != 0 {
		t.Errorf("the unpublished page stayed queued, so every run would redo it: %+v, %v", backlog, err)
	}

	emb.model = "m2"
	if err := x.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if got := passages(t, conn, s.product); len(got) != 2 || got[0].model != "m2" || got[1].model != "m2" {
		t.Errorf("after a model change = %+v - want the new model's passages and none of the old", got)
	}
}

func TestAFailedProviderLeavesTheQueueAndShowsInHealth(t *testing.T) {
	conn := testdb.Require(t)
	ctx := context.Background()
	s := seed(t, conn)
	q := db.New(conn)
	emb := &fakeEmbedder{}
	x := New(q, txrunner.New(conn), emb)
	if r := x.Health(ctx); r.Status != health.StatusOff {
		t.Errorf("no model = %+v, want off", r)
	}

	emb.model, emb.err = "m1", errors.New("429 slow down")
	if err := x.Enqueue(ctx, SourceProduct, s.product); err != nil {
		t.Fatal(err)
	}
	// The scheduler records each run on the job's row, and health reads it there - whichever process ran it
	ran := func(err error) {
		t.Helper()
		status, msg := "success", ""
		if err != nil {
			status, msg = "error", err.Error()
		}
		exec(t, conn, `INSERT INTO scheduled_jobs (name, kind, schedule, description, next_run_at, timeout_seconds, enabled, last_status, last_error, last_run_at)
			VALUES ('embeddings', 'code', '@every 1m', 'embed', now(), 60, TRUE, $1, $2, now())
			ON CONFLICT (name) DO UPDATE SET last_status = EXCLUDED.last_status, last_error = EXCLUDED.last_error, last_run_at = now()`, status, msg)
	}
	err := x.Run(ctx)
	if err == nil {
		t.Fatal("a provider failure passed silently")
	}
	ran(err)
	backlog, err := q.EmbeddingBacklog(ctx)
	if err != nil || backlog.Queued == 0 {
		t.Errorf("the failed product left the queue: %+v, %v", backlog, err)
	}
	if r := x.Health(ctx); r.Status != health.StatusWarn || !strings.Contains(r.Detail, "429") {
		t.Errorf("health after a failure = %+v", r)
	}

	emb.err = nil
	err = x.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ran(nil)
	if r := x.Health(ctx); r.Status != health.StatusOK {
		t.Errorf("health once caught up = %+v", r)
	}
	st, err := x.Status(ctx, &s.channel)
	if err != nil || st.Products == 0 || st.EmbeddedProducts != st.Products || st.Pages != 0 {
		t.Errorf("coverage = %+v, %v - want every product the store shows embedded, and the page unlisted on it uncounted", st, err)
	}
}

// Attributes tied on sort order used to interleave, so a multiselect's values split across two lines; and an attribute
// switched out of search by meaning stays on the storefront but out of the text, whether it holds values or a scalar
func TestEachAttributeReadsAsOneLineAndASwitchedOffOneStaysOut(t *testing.T) {
	conn := testdb.Require(t)
	ctx := context.Background()
	s := seed(t, conn)
	sfx := uuid.NewString()[:8]
	season := scanID(t, conn, `INSERT INTO catalog_attributes (slug, name, input_type, is_public) VALUES ($1, 'Season', 'multiselect', TRUE) RETURNING id`, "season-"+sfx)
	spring := scanID(t, conn, `INSERT INTO catalog_attribute_values (attribute_id, slug, name) VALUES ($1, 'spring', 'Spring') RETURNING id`, season)
	winter := scanID(t, conn, `INSERT INTO catalog_attribute_values (attribute_id, slug, name) VALUES ($1, 'winter', 'Winter') RETURNING id`, season)
	exec(t, conn, `UPDATE catalog_product_attribute_values SET sort_order = 1 WHERE product_id = $1`, s.product)
	exec(t, conn, `INSERT INTO catalog_product_attribute_values (product_id, attribute_id, attribute_value_id, sort_order) VALUES ($1, $2, $3, 0), ($1, $2, $4, 2)`, s.product, season, spring, winter)

	origin := scanID(t, conn, `INSERT INTO catalog_attributes (slug, name, input_type, is_public, embedded) VALUES ($1, 'Origin', 'dropdown', TRUE, FALSE) RETURNING id`, "origin-"+sfx)
	italy := scanID(t, conn, `INSERT INTO catalog_attribute_values (attribute_id, slug, name) VALUES ($1, 'italy', 'Italy') RETURNING id`, origin)
	exec(t, conn, `INSERT INTO catalog_product_attribute_values (product_id, attribute_id, attribute_value_id) VALUES ($1, $2, $3)`, s.product, origin, italy)
	exec(t, conn, `INSERT INTO catalog_attributes (slug, name, input_type, is_public, embedded) VALUES ($1, 'Care', 'plain_text', TRUE, FALSE)`, "care-"+sfx)
	exec(t, conn, `UPDATE catalog_products SET attributes = attributes || jsonb_build_object($2::text, 'Wash cold') WHERE id = $1`, s.product, "care-"+sfx)

	if err := New(db.New(conn), txrunner.New(conn), &fakeEmbedder{model: "m1"}).Run(ctx); err != nil {
		t.Fatal(err)
	}
	var en string
	for _, p := range passages(t, conn, s.product) {
		if p.locale == "en" {
			en = p.content
		}
	}
	if !strings.Contains(en, "Season: Spring, Winter") || strings.Count(en, "Season:") != 1 {
		t.Errorf("the season's values didn't read as one line:\n%s", en)
	}
	if !strings.Contains(en, "Colour: Olive") {
		t.Errorf("the colour went missing:\n%s", en)
	}
	if strings.Contains(en, "Italy") || strings.Contains(en, "Wash cold") {
		t.Errorf("an attribute switched out of search by meaning was embedded:\n%s", en)
	}
}

func TestARefusedTextWaitsWithoutHoldingTheQueue(t *testing.T) {
	conn := testdb.Require(t)
	ctx := context.Background()
	s := seed(t, conn)
	exec(t, conn, `UPDATE catalog_product_content SET description = '{"plain": "UNSENDABLE"}' WHERE product_id = $1`, s.product)
	emb := &fakeEmbedder{model: "m1", refuse: "UNSENDABLE"}
	x := New(db.New(conn), txrunner.New(conn), emb)

	if err := x.Run(ctx); err == nil || !errors.Is(err, ai.ErrEmbeddingRefused) {
		t.Errorf("a refused text = %v - want the run to say so", err)
	}
	if len(passages(t, conn, s.page)) != 2 || len(passages(t, conn, s.product)) != 0 {
		t.Fatalf("page %d passages, product %d - want the page embedded beside the refused product", len(passages(t, conn, s.page)), len(passages(t, conn, s.product)))
	}
	var attempts int
	var later bool
	if err := conn.QueryRow(`SELECT attempts, queued_at > now() FROM embedding_queue WHERE source_id = $1`, s.product).Scan(&attempts, &later); err != nil || attempts != 1 || !later {
		t.Fatalf("the refused product's queue row: attempts %d, waiting %v, %v", attempts, later, err)
	}
	sent := len(emb.texts)
	if err := x.Run(ctx); err != nil || len(emb.texts) != sent {
		t.Errorf("the next run = %v and sent %d more texts - want the refused one left to wait", err, len(emb.texts)-sent)
	}
}

func TestATextRefusedForWeeksStillWaitsADay(t *testing.T) {
	conn := testdb.Require(t)
	ctx := context.Background()
	s := seed(t, conn)
	exec(t, conn, `UPDATE catalog_product_content SET description = '{"plain": "UNSENDABLE"}' WHERE product_id = $1`, s.product)
	x := New(db.New(conn), txrunner.New(conn), &fakeEmbedder{model: "m1", refuse: "UNSENDABLE"})
	_ = x.Run(ctx)
	exec(t, conn, `UPDATE embedding_queue SET attempts = 40, queued_at = now() - interval '1 minute' WHERE source_id = $1`, s.product)
	if err := x.Run(ctx); !errors.Is(err, ai.ErrEmbeddingRefused) {
		t.Fatalf("the 41st refusal = %v - want it deferred like the rest", err)
	}
	var attempts int
	var aDay bool
	if err := conn.QueryRow(`SELECT attempts, queued_at > now() + interval '23 hours' FROM embedding_queue WHERE source_id = $1`, s.product).Scan(&attempts, &aDay); err != nil || attempts != 41 || !aDay {
		t.Errorf("attempts %d, a day away %v, %v - want the wait capped at a day", attempts, aDay, err)
	}
}
