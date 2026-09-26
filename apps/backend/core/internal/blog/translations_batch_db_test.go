package blog

import (
	"context"
	"testing"

	"github.com/google/uuid"
	_ "github.com/lib/pq"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

// The admin blog lists fill allContent from these batch loads. They used to answer it empty, and the
// dashboard's cache let that empty list replace the rows the detail query had fetched - the next save
// wrote the blanks over the stored translations
func TestListTagContentForReturnsEveryLocale(t *testing.T) {
	conn := testdb.Require(t)
	suffix := uuid.NewString()[:6]
	a, b := "ta"+suffix, "tb"+suffix
	for _, code := range []string{a, b} {
		if _, err := conn.Exec(`INSERT INTO locales (code, name) VALUES ($1, $1)`, code); err != nil {
			t.Fatalf("seed locale: %v", err)
		}
		t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM locales WHERE code = $1`, code) }) // after the tags below cascade their rows away
	}
	seed := func(slug string, locales ...string) uuid.UUID {
		var id uuid.UUID
		if err := conn.QueryRow(`INSERT INTO blog_tags (slug) VALUES ($1) RETURNING id`, slug).Scan(&id); err != nil {
			t.Fatalf("seed tag: %v", err)
		}
		t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM blog_tags WHERE id = $1`, id) })
		for _, loc := range locales {
			if _, err := conn.Exec(`INSERT INTO blog_tag_content (tag_id, locale, name) VALUES ($1, $2, $3)`, id, loc, slug+" "+loc); err != nil {
				t.Fatalf("seed tag content: %v", err)
			}
		}
		return id
	}
	both := seed("tr-both-"+suffix, a, b)
	none := seed("tr-none-" + suffix)

	got, err := NewRepository(db.New(conn), nil).ListTagContentFor(context.Background(), []uuid.UUID{both, none})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if rows := got[both]; len(rows) != 2 || rows[0].Locale != a || rows[1].Locale != b {
		t.Errorf("tag with two translations came back with %+v, want the %s and %s rows", rows, a, b)
	}
	if rows, ok := got[none]; ok {
		t.Errorf("tag with no translations came back with %+v, want no entry", rows)
	}
}
