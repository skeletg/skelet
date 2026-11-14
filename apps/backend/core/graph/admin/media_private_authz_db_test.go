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
	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/download"
	"github.com/skeletg/skelet/apps/backend/core/internal/medialibrary"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/objstore"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
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
		DownloadSvc:     download.NewService(download.NewRepository(q), txrunner.New(conn)),
		CustomerSvc:     customer.NewService(customer.NewRepository(q, nil, txrunner.New(conn)), nil, nil, nil, nil, nil, nil, nil),
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
		"who it is shared with": func(ctx context.Context) error {
			_, err := d.q.MediaAssetShares(ctx, d.file(t, "private", "ready"))
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

func TestSharingIsItsOwnGrantAndNeedsTheCustomersChannel(t *testing.T) {
	d := newLibraryDoors(t)
	own := d.f.channel
	var other uuid.UUID
	if err := d.conn.QueryRow(`INSERT INTO channels (slug, name, currency_code, default_locale)
		SELECT $1, $1, currency_code, default_locale FROM channels WHERE id = $2 RETURNING id`, "sh-"+uuid.NewString()[:8], own).Scan(&other); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = d.conn.Exec(`DELETE FROM channels WHERE id = $1`, other) })
	near, nearEmail := seedChannelCustomer(t, d.conn, own)
	far, _ := seedChannelCustomer(t, d.conn, other)

	view, privateView, share := everywhere("media.view"), everywhere(medialibrary.PermPrivateView), everywhere(medialibrary.PermShare)
	customersHere := adminpkg.RolePermissionScopeInput{PermissionSlug: "customers.view", ChannelID: &own}
	sharer := d.caller(t, view, privateView, share, customersHere)
	hidden := d.file(t, "private", "ready")
	give := func(ctx context.Context, file string, to uuid.UUID) (*model.MediaShare, error) {
		return d.m.ShareMediaAsset(ctx, model.ShareMediaAssetInput{AssetID: file, CustomerID: to.String()})
	}

	for name, tc := range map[string]struct {
		ctx  context.Context
		to   uuid.UUID
		want string
	}{
		"sees and manages both halves, but may not share": {d.caller(t, everywhere("media.manage"), everywhere(medialibrary.PermPrivateManage), customersHere), near, medialibrary.PermShare},
		"may share, but not see the private half":         {d.caller(t, view, share, customersHere), near, medialibrary.PermPrivateView},
		"may share on one channel only":                   {d.caller(t, view, privateView, customersHere, adminpkg.RolePermissionScopeInput{PermissionSlug: medialibrary.PermShare, ChannelID: &own}), near, medialibrary.PermShare},
		"may share, but not see customers":                {d.caller(t, view, privateView, share), near, "customers.view"},
		"a customer on a channel out of reach":            {sharer, far, "customers.view"},
	} {
		if _, err := give(tc.ctx, hidden, tc.to); stoppedAt(err) != tc.want {
			t.Errorf("%s: stopped at %q (%v), want %q", name, stoppedAt(err), err, tc.want)
		}
	}
	closed, _ := seedChannelCustomer(t, d.conn, own)
	if _, err := d.conn.Exec(`UPDATE customers SET is_active = false WHERE id = $1`, closed); err != nil {
		t.Fatal(err)
	}
	if _, err := give(sharer, hidden, closed); !errors.Is(err, download.ErrInactiveCustomer) {
		t.Errorf("sharing with a closed account: %v, want it refused", err)
	}
	var made int
	if err := d.conn.QueryRow(`SELECT count(*) FROM download_grants WHERE asset_id = $1`, hidden).Scan(&made); err != nil || made != 0 {
		t.Fatalf("%d shares were made by callers who were refused (%v)", made, err)
	}

	if _, err := give(sharer, d.file(t, "private", "pending"), near); !errors.Is(err, medialibrary.ErrUploadNotReceived) {
		t.Errorf("sharing a file whose upload never finished: %v, want it refused", err)
	}
	if _, err := give(sharer, d.file(t, "public", "ready"), near); !errors.Is(err, medialibrary.ErrSharePublic) {
		t.Errorf("sharing a public file: %v, want it refused", err)
	}
	mine, err := give(sharer, hidden, near)
	if err != nil || mine.Email != nearEmail {
		t.Fatalf("sharing with a customer in reach: %+v %v", mine, err)
	}
	if _, err := d.m.DownloadSvc.ShareAsset(context.Background(), download.ShareInput{AssetID: uuid.MustParse(hidden), CustomerID: far, Email: "far@example.invalid", Title: "deed"}); err != nil {
		t.Fatal(err)
	}

	listed := func(ctx context.Context) map[string]bool {
		shares, err := d.q.MediaAssetShares(ctx, hidden)
		if err != nil {
			t.Fatal(err)
		}
		if len(shares) != 2 {
			t.Fatalf("listed %d shares, want both", len(shares))
		}
		named := map[string]bool{}
		for _, s := range shares {
			if (s.Email == "") != (s.CustomerID == nil) {
				t.Errorf("a share kept back half of who it is for: %+v", s)
			}
			named[s.Email] = true
		}
		return named
	}
	if got := listed(sharer); !got[nearEmail] || got["far@example.invalid"] || !got[""] {
		t.Errorf("with customers in reach on one channel, the list names %v - want that channel's customer and the other kept back", got)
	}
	if got := listed(d.caller(t, view, privateView)); len(got) != 1 || !got[""] {
		t.Errorf("with no grant to see customers, the list names %v - want every customer kept back", got)
	}
	if got := listed(d.caller(t, view, privateView, everywhere("customers.view"))); !got[nearEmail] || !got["far@example.invalid"] {
		t.Errorf("with customers in reach everywhere, the list names %v - want both", got)
	}

	if _, err := d.m.RevokeMediaShare(d.caller(t, everywhere("media.manage"), everywhere(medialibrary.PermPrivateManage)), mine.ID); stoppedAt(err) != medialibrary.PermShare {
		t.Errorf("taking a share back without the grant to share: %v", err)
	}
	if ok, err := d.m.RevokeMediaShare(sharer, mine.ID); err != nil || !ok {
		t.Errorf("taking a share back: %v %v", ok, err)
	}
}
