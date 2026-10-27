package medialibrary

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

// What these pin: each sort the media picker offers orders the way its label says - names ignoring case - a search
// returns at most 200 while counting them all, and paging by offset through any sort returns every asset once,
// ties included, since that's how the picker loads more

type searchRig struct {
	t      *testing.T
	svc    *Service
	exec   func(query string, args ...any)
	folder string
}

func newSearchRig(t *testing.T) searchRig {
	t.Helper()
	tx, err := testdb.Require(t).Begin()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback() })
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := tx.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	return searchRig{t: t, svc: NewService(NewRepository(db.New(tx)), newMemStore(), ""), exec: exec, folder: "sorttest-" + uuid.NewString()[:8]}
}

func (r searchRig) add(filename string, size int64, created, updated time.Time) {
	r.exec(`INSERT INTO media_assets (id, filename, object_key, mime_type, size_bytes, status, created_at, updated_at) VALUES ($1, $2, $3, 'image/png', $4, 'ready', $5, $6)`,
		uuid.New(), filename, r.folder+"/"+uuid.NewString()+".png", size, created, updated)
}

func (r searchRig) search(sort string, limit, offset int32) SearchResult {
	r.t.Helper()
	res, err := r.svc.SearchAssets(context.Background(), SearchInput{Path: &r.folder, Sort: sort, Limit: limit, Offset: offset})
	if err != nil {
		r.t.Fatal(err)
	}
	return res
}

func names(items []Asset) []string {
	out := make([]string, len(items))
	for i, a := range items {
		out[i] = a.Filename
	}
	return out
}

func TestEachSortOrdersTheWayItsLabelSays(t *testing.T) {
	r := newSearchRig(t)
	day := func(n int) time.Time { return time.Date(2026, 10, n, 12, 0, 0, 0, time.UTC) }
	// Byte order would put Banana before apple - the name sort mustn't. Every sort lands differently, so one that
	// silently falls back to newest shows
	r.add("Banana.png", 100, day(1), day(4))
	r.add("apple.png", 200, day(2), day(2))
	r.add("cherry.png", 300, day(3), day(3))

	for sort, want := range map[string][]string{
		"":         {"Banana.png", "cherry.png", "apple.png"},
		"newest":   {"Banana.png", "cherry.png", "apple.png"},
		"oldest":   {"Banana.png", "apple.png", "cherry.png"},
		"name":     {"apple.png", "Banana.png", "cherry.png"},
		"largest":  {"cherry.png", "apple.png", "Banana.png"},
		"sideways": {"Banana.png", "cherry.png", "apple.png"},
	} {
		if got := names(r.search(sort, 10, 0).Items); !slices.Equal(got, want) {
			t.Errorf("sort %q = %v, want %v", sort, got, want)
		}
	}
}

func TestPagingThroughASortReturnsEveryAssetOnce(t *testing.T) {
	r := newSearchRig(t)
	same := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	for range 7 {
		r.add("twin.png", 100, same, same)
	}
	for _, sort := range []string{"newest", "oldest", "name", "largest"} {
		seen := map[uuid.UUID]bool{}
		for offset := int32(0); offset < 7; offset += 2 {
			for _, a := range r.search(sort, 2, offset).Items {
				if seen[a.ID] {
					t.Fatalf("sort %q showed %s twice across pages", sort, a.ID)
				}
				seen[a.ID] = true
			}
		}
		if len(seen) != 7 {
			t.Errorf("sort %q paged through %d of 7", sort, len(seen))
		}
	}
}

func TestASearchReturnsAtMost200ButCountsThemAll(t *testing.T) {
	r := newSearchRig(t)
	r.exec(`INSERT INTO media_assets (id, filename, object_key, mime_type, size_bytes, status)
		SELECT gen_random_uuid(), 'bulk-' || n || '.png', $1 || '/bulk-' || n || '.png', 'image/png', n, 'ready' FROM generate_series(1, 205) n`, r.folder)
	res := r.search("largest", 500, 0)
	if len(res.Items) != 200 || res.TotalCount != 205 {
		t.Fatalf("got %d items of %d, want 200 of 205", len(res.Items), res.TotalCount)
	}
	if rest := r.search("largest", 500, 200); len(rest.Items) != 5 || rest.Items[4].SizeBytes != 1 {
		t.Fatalf("the next page held %d, want the last 5 ending on the smallest", len(rest.Items))
	}
}
