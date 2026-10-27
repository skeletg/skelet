package admin

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	adminpkg "github.com/skeletg/skelet/apps/backend/core/internal/admin"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/medialibrary"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

// What this pins: every MediaAssetSort the picker sends reaches the query as the order it names - the resolver
// lowercases the enum into the strings the SQL switches on, so a renamed value would quietly fall back to newest - and
// searching still needs media.view

// A search only builds urls - the rest of storage is never reached here
type urlOnlyStorage struct{ medialibrary.Storage }

func (urlOnlyStorage) PublicURL(key string) string { return "https://cdn.test/" + key }

func TestEverySearchSortReachesTheQuery(t *testing.T) {
	f := newEscalationFixture(t)
	conn := testdb.Require(t)
	folder := "sortgql-" + uuid.NewString()[:8]
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM media_assets WHERE starts_with(object_key, $1)`, folder+"/") })
	day := func(n int) time.Time { return time.Date(2026, 10, n, 12, 0, 0, 0, time.UTC) }
	for _, a := range []struct {
		name             string
		size             int64
		created, updated time.Time
	}{{"Banana.png", 100, day(1), day(4)}, {"apple.png", 200, day(2), day(2)}, {"cherry.png", 300, day(3), day(3)}} {
		if _, err := conn.Exec(`INSERT INTO media_assets (id, filename, object_key, mime_type, size_bytes, status, created_at, updated_at) VALUES ($1, $2, $3, 'image/png', $4, 'ready', $5, $6)`,
			uuid.New(), a.name, folder+"/"+a.name, a.size, a.created, a.updated); err != nil {
			t.Fatal(err)
		}
	}
	r := &queryResolver{&Resolver{AdminSvc: f.svc, MediaLibrarySvc: medialibrary.NewService(medialibrary.NewRepository(db.New(conn)), urlOnlyStorage{}, "")}}
	_, ctx := f.admin(t, f.role(t, adminpkg.RolePermissionScopeInput{PermissionSlug: "media.view"}))

	search := func(ctx context.Context, sort *model.MediaAssetSort) ([]string, error) {
		res, err := r.SearchMediaAssets(ctx, nil, &folder, nil, sort, nil, nil)
		if err != nil {
			return nil, err
		}
		out := make([]string, len(res.Items))
		for i, a := range res.Items {
			out[i] = a.Filename
		}
		return out, nil
	}
	want := map[model.MediaAssetSort][]string{
		model.MediaAssetSortNewest:  {"Banana.png", "cherry.png", "apple.png"},
		model.MediaAssetSortOldest:  {"Banana.png", "apple.png", "cherry.png"},
		model.MediaAssetSortName:    {"apple.png", "Banana.png", "cherry.png"},
		model.MediaAssetSortLargest: {"cherry.png", "apple.png", "Banana.png"},
	}
	for _, sort := range model.AllMediaAssetSort {
		got, err := search(ctx, &sort)
		if err != nil {
			t.Fatalf("%s: %v", sort, err)
		}
		if !slices.Equal(got, want[sort]) {
			t.Errorf("%s = %v, want %v", sort, got, want[sort])
		}
	}
	if got, err := search(ctx, nil); err != nil || !slices.Equal(got, want[model.MediaAssetSortNewest]) {
		t.Errorf("no sort = %v %v, want newest first", got, err)
	}

	_, outsider := f.admin(t, f.role(t, adminpkg.RolePermissionScopeInput{PermissionSlug: "catalog.view"}))
	if _, err := search(outsider, nil); err == nil {
		t.Error("an admin without media.view searched the library")
	}
}
