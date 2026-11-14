package admin

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	adminpkg "github.com/skeletg/skelet/apps/backend/core/internal/admin"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/medialibrary"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/objstore"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

type heldStore struct{ keys map[string]bool }

func (s *heldStore) PublicURL(key string) string { return "https://cdn.test/" + key }
func (s *heldStore) HasPublicBase() bool         { return true }
func (s *heldStore) MaxUploadBytes() int64       { return 1 << 20 }
func (s *heldStore) PresignUpload(_ context.Context, key, _ string, _ int64, _ time.Duration) (string, error) {
	return "https://put.test/" + key, nil
}
func (s *heldStore) PutBytes(_ context.Context, key string, _ []byte, _ string) error {
	s.keys[key] = true
	return nil
}
func (s *heldStore) PutEmptyObject(_ context.Context, key string) error {
	s.keys[key] = true
	return nil
}
func (s *heldStore) Head(_ context.Context, key string) (int64, string, bool, error) {
	return 1, "", true, nil
}
func (s *heldStore) GetStream(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("bytes")), nil
}
func (s *heldStore) UploadStream(_ context.Context, key, _ string, _ io.Reader) error {
	s.keys[key] = true
	return nil
}
func (s *heldStore) Copy(_ context.Context, _, to string) error           { s.keys[to] = true; return nil }
func (s *heldStore) Delete(_ context.Context, key string) error           { delete(s.keys, key); return nil }
func (s *heldStore) ListPrefix(context.Context, string) ([]string, error) { return nil, nil }
func (s *heldStore) ListImmediatePrefixes(context.Context, string) ([]string, error) {
	return nil, nil
}
func (s *heldStore) ListDirectoryContents(context.Context, string) ([]string, []objstore.StorageObject, error) {
	return nil, nil, nil
}

type libraryDoors struct {
	f      escalationFixture
	conn   *sql.DB
	q      *queryResolver
	m      *mutationResolver
	folder string
}

func newLibraryDoors(t *testing.T) libraryDoors {
	t.Helper()
	f := newEscalationFixture(t)
	conn := testdb.Require(t)
	q := db.New(conn)
	lib := medialibrary.NewService(medialibrary.NewRepository(q), &heldStore{keys: map[string]bool{}}, "")
	lib.SetPrivateStore(&heldStore{keys: map[string]bool{}})
	root := &Resolver{
		AdminSvc:        f.svc,
		MediaLibrarySvc: lib,
	}
	folder := "authz-" + uuid.NewString()[:8]
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM media_assets WHERE starts_with(object_key, $1)`, folder+"/") })
	return libraryDoors{f: f, conn: conn, q: &queryResolver{root}, m: &mutationResolver{root}, folder: folder}
}

func (d libraryDoors) file(t *testing.T, half, status string) string {
	t.Helper()
	id := uuid.New()
	name := id.String()[:8] + ".jpg"
	if _, err := d.conn.Exec(`INSERT INTO media_assets (id, filename, directory_prefix, object_key, mime_type, size_bytes, status, visibility) VALUES ($1, $2, $3, $4, 'image/jpeg', 1, $5, $6)`,
		id, name, d.folder, d.folder+"/"+name, status, half); err != nil {
		t.Fatal(err)
	}
	return id.String()
}

func (d libraryDoors) caller(t *testing.T, scopes ...adminpkg.RolePermissionScopeInput) context.Context {
	t.Helper()
	_, ctx := d.f.admin(t, d.f.role(t, scopes...))
	return ctx
}

func everywhere(slug string) adminpkg.RolePermissionScopeInput {
	return adminpkg.RolePermissionScopeInput{PermissionSlug: slug}
}

type door func(context.Context) error

func (d libraryDoors) privateReads(t *testing.T) map[string]door {
	private := model.MediaVisibilityPrivate
	return map[string]door{
		"browse a folder": func(ctx context.Context) error { _, err := d.q.MediaDirectory(ctx, d.folder, &private); return err },
		"search": func(ctx context.Context) error {
			_, err := d.q.SearchMediaAssets(ctx, nil, &d.folder, nil, nil, nil, nil, &private)
			return err
		},
		"list the folders": func(ctx context.Context) error { _, err := d.q.AllMediaDirectories(ctx, &private); return err },
		"one file by id": func(ctx context.Context) error {
			_, err := d.q.MediaAsset(ctx, d.file(t, "private", "ready"))
			return err
		},
	}
}

func (d libraryDoors) privateWrites(t *testing.T) map[string]door {
	private, public := model.MediaVisibilityPrivate, model.MediaVisibilityPublic
	hidden := func() string { return d.file(t, "private", "ready") }
	return map[string]door{
		"make a folder": func(ctx context.Context) error {
			_, err := d.m.CreateMediaDirectory(ctx, d.folder, "new", &private)
			return err
		},
		"ask to upload": func(ctx context.Context) error {
			_, err := d.m.RequestMediaAssetUpload(ctx, model.RequestMediaAssetUploadInput{Path: d.folder, Visibility: &private, Filename: uuid.NewString()[:8] + ".jpg", MimeType: "image/jpeg", SizeBytes: 1})
			return err
		},
		"confirm an upload": func(ctx context.Context) error {
			size := int32(10)
			_, err := d.m.ConfirmMediaAssetUpload(ctx, d.file(t, "private", "pending"), &size, &size)
			return err
		},
		"edit its words": func(ctx context.Context) error {
			title := "Deed"
			_, err := d.m.UpdateMediaAsset(ctx, model.UpdateMediaAssetInput{ID: hidden(), Title: &title})
			return err
		},
		"set its focal point": func(ctx context.Context) error {
			x, y := 0.5, 0.5
			_, err := d.m.SetMediaFocalPoint(ctx, hidden(), &x, &y)
			return err
		},
		"rename it": func(ctx context.Context) error {
			name := uuid.NewString()[:8] + ".jpg"
			_, err := d.m.MoveMediaAsset(ctx, hidden(), d.folder, &name, nil)
			return err
		},
		"make it public": func(ctx context.Context) error {
			_, err := d.m.MoveMediaAsset(ctx, hidden(), d.folder, nil, &public)
			return err
		},
		"make a public one private": func(ctx context.Context) error {
			_, err := d.m.MoveMediaAsset(ctx, d.file(t, "public", "ready"), d.folder, nil, &private)
			return err
		},
		"delete it": func(ctx context.Context) error { _, err := d.m.DeleteMediaAsset(ctx, hidden(), nil); return err },
		"delete a folder": func(ctx context.Context) error {
			_, err := d.m.DeleteMediaDirectory(ctx, d.folder+"/gone", true, &private)
			return err
		},
		"rename a folder": func(ctx context.Context) error {
			_, err := d.m.RenameMediaDirectory(ctx, d.folder+"/old", uuid.NewString()[:8], &private)
			return err
		},
		"write alt text with AI": func(ctx context.Context) error {
			_, err := d.m.GenerateMediaAltText(ctx, hidden())
			return err
		},
	}
}

func stoppedAt(err error) string {
	if denied, ok := errors.AsType[*auth.ScopedDenial](err); ok {
		return denied.Slug
	}
	if errors.Is(err, auth.ErrForbidden) || errors.Is(err, auth.ErrUnauthenticated) {
		return "an unnamed refusal: " + err.Error()
	}
	return ""
}

func TestThePrivateHalfAsksForItsOwnGrant(t *testing.T) {
	d := newLibraryDoors(t)
	view, manage := everywhere("media.view"), everywhere("media.manage")
	privateView, privateManage := everywhere(medialibrary.PermPrivateView), everywhere(medialibrary.PermPrivateManage)
	onOneChannel := adminpkg.RolePermissionScopeInput{PermissionSlug: medialibrary.PermPrivateManage, ChannelID: &d.f.channel}

	refusedAnyway := map[string]error{"write alt text with AI": medialibrary.ErrPrivateAsset}
	for name, tc := range map[string]struct {
		grants        []adminpkg.RolePermissionScopeInput
		reads, writes string
	}{
		"sees the library":                    {[]adminpkg.RolePermissionScopeInput{view}, medialibrary.PermPrivateView, "media.manage"},
		"manages the library":                 {[]adminpkg.RolePermissionScopeInput{manage}, medialibrary.PermPrivateView, medialibrary.PermPrivateManage},
		"sees the private half":               {[]adminpkg.RolePermissionScopeInput{view, privateView}, "", "media.manage"},
		"sees private, manages public":        {[]adminpkg.RolePermissionScopeInput{manage, privateView}, "", medialibrary.PermPrivateManage},
		"manages both halves":                 {[]adminpkg.RolePermissionScopeInput{manage, privateManage}, "", ""},
		"the private grant on one channel":    {[]adminpkg.RolePermissionScopeInput{manage, onOneChannel}, medialibrary.PermPrivateView, medialibrary.PermPrivateManage},
		"the private grant without the other": {[]adminpkg.RolePermissionScopeInput{privateManage}, "media.view", "media.manage"},
	} {
		ctx := d.caller(t, tc.grants...)
		for what, open := range d.privateReads(t) {
			err := open(ctx)
			if got := stoppedAt(err); got != tc.reads {
				t.Errorf("%s, %s: stopped at %q, want %q", name, what, got, tc.reads)
			} else if got == "" && err != nil {
				t.Errorf("%s, %s: let through, then failed: %v", name, what, err)
			}
		}
		for what, open := range d.privateWrites(t) {
			err := open(ctx)
			if got := stoppedAt(err); got != tc.writes {
				t.Errorf("%s, %s: stopped at %q, want %q", name, what, got, tc.writes)
			} else if got == "" && !errors.Is(err, refusedAnyway[what]) {
				t.Errorf("%s, %s: let through, then failed: %v", name, what, err)
			}
		}
	}

	anonymous := context.Background()
	for what, open := range d.privateReads(t) {
		if err := open(anonymous); !errors.Is(err, auth.ErrUnauthenticated) {
			t.Errorf("no one signed in, %s: %v, want unauthenticated", what, err)
		}
	}
	for what, open := range d.privateWrites(t) {
		if err := open(anonymous); !errors.Is(err, auth.ErrUnauthenticated) {
			t.Errorf("no one signed in, %s: %v, want unauthenticated", what, err)
		}
	}
}

func TestThePublicHalfRunsOnTheLibrarysGrantsAlone(t *testing.T) {
	d := newLibraryDoors(t)
	viewer := d.caller(t, everywhere("media.view"))
	manager := d.caller(t, everywhere("media.manage"))
	open := d.file(t, "public", "ready")

	if _, err := d.q.MediaDirectory(viewer, d.folder, nil); err != nil {
		t.Errorf("browse: %v", err)
	}
	res, err := d.q.SearchMediaAssets(viewer, nil, &d.folder, nil, nil, nil, nil, nil)
	if err != nil || len(res.Items) != 1 || res.Items[0].ID != open {
		t.Errorf("search: %+v %v, want the one public file", res, err)
	}
	if got, err := d.q.MediaAsset(viewer, open); err != nil || got.URL == "" {
		t.Errorf("one file by id: %+v %v", got, err)
	}
	title := "Banner"
	if _, err := d.m.UpdateMediaAsset(manager, model.UpdateMediaAssetInput{ID: open, Title: &title}); err != nil {
		t.Errorf("edit its words: %v", err)
	}
	if _, err := d.m.DeleteMediaAsset(manager, open, nil); err != nil {
		t.Errorf("delete: %v", err)
	}
}

func TestAPrivatePictureIsNotSentToTheAIProvider(t *testing.T) {
	d := newLibraryDoors(t)
	both := d.caller(t, everywhere("media.manage"), everywhere(medialibrary.PermPrivateManage))
	if _, err := d.m.GenerateMediaAltText(both, d.file(t, "private", "ready")); !errors.Is(err, medialibrary.ErrPrivateAsset) {
		t.Errorf("a private picture: %v, want ErrPrivateAsset", err)
	}
	if _, err := d.m.GenerateMediaAltText(both, d.file(t, "public", "ready")); stoppedAt(err) != "" || errors.Is(err, medialibrary.ErrPrivateAsset) {
		t.Errorf("a public picture was refused: %v", err)
	}
}
