package cms

import (
	"context"
	"testing"

	"github.com/google/uuid"
	_ "github.com/lib/pq"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

// The admin cmsAttributes list fills allContent from this batch load. It used to answer it empty - and the
// attribute pages read that list, so every attribute's translations came back blank there
func TestListAttributeContentForReturnsEveryLocale(t *testing.T) {
	conn := testdb.Require(t)
	suffix := uuid.NewString()[:6]
	a, b := "ta"+suffix, "tb"+suffix
	for _, code := range []string{a, b} {
		if _, err := conn.Exec(`INSERT INTO locales (code, name) VALUES ($1, $1)`, code); err != nil {
			t.Fatalf("seed locale: %v", err)
		}
		t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM locales WHERE code = $1`, code) }) // after the attributes below cascade their rows away
	}
	seed := func(slug string, locales ...string) uuid.UUID {
		var id uuid.UUID
		if err := conn.QueryRow(`INSERT INTO cms_attributes (slug, input_type) VALUES ($1, 'text') RETURNING id`, slug).Scan(&id); err != nil {
			t.Fatalf("seed attribute: %v", err)
		}
		t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM cms_attributes WHERE id = $1`, id) })
		for _, loc := range locales {
			if _, err := conn.Exec(`INSERT INTO cms_attribute_content (attribute_id, locale, name) VALUES ($1, $2, $3)`, id, loc, slug+" "+loc); err != nil {
				t.Fatalf("seed attribute content: %v", err)
			}
		}
		return id
	}
	both := seed("tr-both-"+suffix, a, b)
	none := seed("tr-none-" + suffix)

	got, err := NewRepository(db.New(conn), nil).ListAttributeContentFor(context.Background(), []uuid.UUID{both, none})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if rows := got[both]; len(rows) != 2 || rows[0].Locale != a || rows[1].Locale != b {
		t.Errorf("attribute with two translations came back with %+v, want the %s and %s rows", rows, a, b)
	}
	if rows, ok := got[none]; ok {
		t.Errorf("attribute with no translations came back with %+v, want no entry", rows)
	}
}
