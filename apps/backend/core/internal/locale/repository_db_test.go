package locale

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

// Columns that record what language something was written in, not content to keep in it - counting them would make a locale undeletable forever
var historicalLocaleColumns = []string{"reviews", "cms_page_comments", "cms_page_revisions"}

// A new locale column must join the delete guard or be named historical here - otherwise deleting a locale orphans its rows, as blog, CMS, menu and promotion content once were
func TestTheDeleteGuardCountsEveryLocaleColumn(t *testing.T) {
	conn := testdb.Require(t)
	src, err := os.ReadFile("../db/query/locales.sql")
	if err != nil {
		t.Fatal(err)
	}
	counted := map[string]bool{}
	for _, m := range regexp.MustCompile(`FROM\s+(\w+)\s+WHERE\s+(\w+)\s*=\s*LOWER\(\$1\)`).FindAllStringSubmatch(string(src), -1) {
		counted[m[1]+"."+m[2]] = true
	}
	rows, err := conn.Query(`SELECT c.table_name || '.' || c.column_name FROM information_schema.columns c
		JOIN information_schema.tables t ON t.table_schema = c.table_schema AND t.table_name = c.table_name AND t.table_type = 'BASE TABLE'
		WHERE c.table_schema = 'public' AND c.column_name IN ('locale', 'locale_code', 'default_locale')`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var col string
		if err := rows.Scan(&col); err != nil {
			t.Fatal(err)
		}
		table, _, _ := strings.Cut(col, ".")
		if !counted[col] && !slices.Contains(historicalLocaleColumns, table) {
			t.Errorf("%s holds a locale but the delete guard does not count it", col)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}

func newDBService(t *testing.T) (*Service, *sql.DB, string) {
	t.Helper()
	conn := testdb.Require(t)
	code := "qq-" + uuid.NewString()[:8]
	svc := NewService(NewRepository(db.New(conn)))
	if _, err := svc.Create(context.Background(), CreateLocaleInput{Code: code, Name: "Fixture", IsActive: true, Metadata: map[string]string{"k": "v"}}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(`DELETE FROM blog_tags WHERE slug LIKE $1`, code+"%")
		_, _ = conn.Exec(`DELETE FROM locales WHERE code = $1`, code)
	})
	return svc, conn, code
}

// Blog content has no FK to locales, so the old guard waved the delete through and left the post's translation pointing at nothing
func TestALocaleWithBlogContentCannotBeDeleted(t *testing.T) {
	svc, conn, code := newDBService(t)
	var tagID uuid.UUID
	if err := conn.QueryRow(`INSERT INTO blog_tags (slug) VALUES ($1) RETURNING id`, code+"-tag").Scan(&tagID); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`INSERT INTO blog_tag_content (tag_id, locale, name) VALUES ($1, $2, 'Tag')`, tagID, code); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(context.Background(), code); !errors.Is(err, ErrLocaleInUse) {
		t.Fatalf("delete = %v, want ErrLocaleInUse", err)
	}
	if _, err := conn.Exec(`DELETE FROM blog_tag_content WHERE tag_id = $1`, tagID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(context.Background(), code); err != nil {
		t.Fatalf("delete once unreferenced = %v", err)
	}
}

func TestUpdateChangesOnlyWhatItCarries(t *testing.T) {
	svc, _, code := newDBService(t)
	ctx := context.Background()
	var flips []bool
	svc.SetStatusChangeHook(func(_ context.Context, _ string, active bool) { flips = append(flips, active) })

	name := "Renamed"
	got, err := svc.Update(ctx, UpdateLocaleInput{Code: code, Name: &name})
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != name || got.NativeName != "Fixture" || got.DecimalSeparator != "." || got.Metadata["k"] != "v" || !got.IsActive {
		t.Fatalf("after a name-only edit: %+v", got)
	}

	off, on := false, true
	for _, v := range []*bool{&off, &off, &on} {
		if _, err := svc.Update(ctx, UpdateLocaleInput{Code: code, IsActive: v}); err != nil {
			t.Fatal(err)
		}
	}
	if !slices.Equal(flips, []bool{false, true}) {
		t.Errorf("status hook fired %v, want once off and once on - a repeat is not a flip", flips)
	}

	missing := "x"
	if _, err := svc.Update(ctx, UpdateLocaleInput{Code: "qq-missing", Name: &missing}); !errors.Is(err, ErrLocaleNotFound) {
		t.Errorf("update of a missing locale = %v, want ErrLocaleNotFound", err)
	}
}

func TestCreatingATakenCodeIsAConflict(t *testing.T) {
	svc, _, code := newDBService(t)
	if _, err := svc.Create(context.Background(), CreateLocaleInput{Code: code, Name: "Again"}); !errors.Is(err, ErrLocaleCodeTaken) {
		t.Fatalf("duplicate create = %v, want ErrLocaleCodeTaken", err)
	}
}
