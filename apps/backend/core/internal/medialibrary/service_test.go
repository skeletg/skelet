package medialibrary

import (
	"context"
	"errors"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/objstore"
)

type memStore struct {
	objects    map[string]bool
	failDelete map[string]bool
	deleted    []string
}

func newMemStore(keys ...string) *memStore {
	s := &memStore{objects: map[string]bool{}, failDelete: map[string]bool{}}
	for _, k := range keys {
		s.objects[k] = true
	}
	return s
}

func (m *memStore) PublicURL(key string) string { return "https://cdn.test/" + key }
func (m *memStore) MaxUploadBytes() int64       { return 1 << 20 }
func (m *memStore) PresignUpload(context.Context, string, string, int64, time.Duration) (string, error) {
	return "https://upload.test", nil
}
func (m *memStore) PutBytes(_ context.Context, key string, _ []byte, _ string) error {
	m.objects[key] = true
	return nil
}
func (m *memStore) PutEmptyObject(_ context.Context, key string) error {
	m.objects[key] = true
	return nil
}
func (m *memStore) Head(_ context.Context, key string) (int64, string, bool, error) {
	return 1, "", m.objects[key], nil
}
func (m *memStore) Copy(_ context.Context, from, to string) error {
	if !m.objects[from] {
		return objstore.ErrObjectNotFound
	}
	m.objects[to] = true
	return nil
}
func (m *memStore) Delete(_ context.Context, key string) error {
	if m.failDelete[key] {
		return errors.New("storage down")
	}
	delete(m.objects, key)
	m.deleted = append(m.deleted, key)
	return nil
}
func (m *memStore) ListPrefix(_ context.Context, prefix string) ([]string, error) {
	var out []string
	for k := range m.objects {
		if strings.HasPrefix(k, prefix) {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out, nil
}
func (m *memStore) ListImmediatePrefixes(_ context.Context, prefix string) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	for k := range m.objects {
		rest, ok := strings.CutPrefix(k, prefix)
		if !ok {
			continue
		}
		if dir, _, found := strings.Cut(rest, "/"); found && !seen[dir] {
			seen[dir] = true
			out = append(out, prefix+dir+"/")
		}
	}
	sort.Strings(out)
	return out, nil
}
func (m *memStore) ListDirectoryContents(ctx context.Context, prefix string) ([]string, []objstore.StorageObject, error) {
	dirs, _ := m.ListImmediatePrefixes(ctx, prefix)
	var objs []objstore.StorageObject
	for k := range m.objects {
		if rest, ok := strings.CutPrefix(k, prefix); ok && !strings.Contains(rest, "/") {
			objs = append(objs, objstore.StorageObject{Key: k})
		}
	}
	return dirs, objs, nil
}

type memRepo struct {
	Repository
	assets    map[uuid.UUID]Asset
	failMove  bool
	deletedBy []string
}

func (r *memRepo) GetAssetByID(_ context.Context, id uuid.UUID) (Asset, error) {
	a, ok := r.assets[id]
	if !ok {
		return Asset{}, ErrAssetNotFound
	}
	return a, nil
}
func (r *memRepo) CreateAsset(_ context.Context, a Asset) (Asset, error) {
	r.assets[a.ID] = a
	return a, nil
}
func (r *memRepo) DeleteAsset(_ context.Context, id uuid.UUID) error {
	delete(r.assets, id)
	return nil
}
func (r *memRepo) UpdateAssetLocation(_ context.Context, id uuid.UUID, p, filename, key string, _ *uuid.UUID) (Asset, error) {
	if r.failMove {
		return Asset{}, errors.New("db down")
	}
	a := r.assets[id]
	a.DirectoryPrefix, a.Filename, a.ObjectKey = p, filename, key
	r.assets[id] = a
	return a, nil
}
func (r *memRepo) ListAssetsByPathRecursive(_ context.Context, prefix string) ([]Asset, error) {
	var out []Asset
	for _, a := range r.assets {
		if strings.HasPrefix(a.ObjectKey, prefix) {
			out = append(out, a)
		}
	}
	return out, nil
}
func (r *memRepo) ListAssetsByDirectory(context.Context, string) ([]Asset, error) { return nil, nil }
func (r *memRepo) DeleteAssetsByDirectoryPrefix(_ context.Context, prefix string) error {
	r.deletedBy = append(r.deletedBy, prefix)
	return nil
}

func fixture(keys ...string) (*Service, *memStore, *memRepo) {
	st, repo := newMemStore(keys...), &memRepo{assets: map[uuid.UUID]Asset{}}
	return NewService(repo, st, ""), st, repo
}

// With no root prefix the library spans the bucket the platform writes everything into
var systemKeys = []string{
	"catalog/products/p1/images/a.jpg", "webforms/f1/cv/x-cv.pdf", "data/exports/j1/customers.csv",
	"derived/w320/catalog/products/p1/images/a.jpg", "banners/summer.jpg",
}

func TestPlatformPrefixesAreOffLimits(t *testing.T) {
	svc, st, _ := fixture(systemKeys...)
	ctx := context.Background()

	dir, err := svc.GetDirectory(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, d := range dir.Directories {
		names = append(names, d.Path)
	}
	if !slices.Equal(names, []string{"banners"}) {
		t.Errorf("root lists %v, want only the library's own banners", names)
	}
	all, err := svc.ListAllDirectories(ctx)
	if err != nil || len(all) != 1 || all[0].Path != "banners" {
		t.Errorf("directory tree = %+v (%v), want banners alone", all, err)
	}

	for name, err := range map[string]error{
		"browse exports":   func() error { _, err := svc.GetDirectory(ctx, "data/exports"); return err }(),
		"delete catalog":   svc.DeleteDirectory(ctx, "catalog", true),
		"rename webforms":  func() error { _, err := svc.RenameDirectory(ctx, "webforms", "x"); return err }(),
		"rename onto data": func() error { _, err := svc.RenameDirectory(ctx, "banners", "data"); return err }(),
		"upload into catalog": func() error {
			_, err := svc.RequestUpload(ctx, RequestUploadInput{Path: "catalog/x", Filename: "a.jpg", MimeType: "image/jpeg", SizeBytes: 10})
			return err
		}(),
		"make derived": func() error { _, err := svc.CreateDirectory(ctx, "", "derived"); return err }(),
	} {
		if !errors.Is(err, ErrReservedPath) {
			t.Errorf("%s = %v, want ErrReservedPath", name, err)
		}
	}
	if !st.objects["catalog/products/p1/images/a.jpg"] || !st.objects["data/exports/j1/customers.csv"] {
		t.Fatal("a refused operation still touched platform objects")
	}
}

// One recursive delete at the root used to empty the whole bucket
func TestTheRootCannotBeDeletedRecursively(t *testing.T) {
	svc, st, _ := fixture(systemKeys...)
	if err := svc.DeleteDirectory(context.Background(), "", true); !errors.Is(err, ErrRootDeleteNotAllowed) {
		t.Fatalf("root delete = %v, want ErrRootDeleteNotAllowed", err)
	}
	if len(st.deleted) != 0 {
		t.Fatalf("deleted %v", st.deleted)
	}
}

// A rooted library has no platform neighbours to protect
func TestARootedLibraryReservesNothing(t *testing.T) {
	st, repo := newMemStore("library/catalog/x.jpg"), &memRepo{assets: map[uuid.UUID]Asset{}}
	svc := NewService(repo, st, "library")
	if _, err := svc.GetDirectory(context.Background(), "catalog"); err != nil {
		t.Fatalf("a library folder named catalog = %v", err)
	}
}

func seedAsset(repo *memRepo, st *memStore, dir, file string) Asset {
	a := Asset{ID: uuid.New(), DirectoryPrefix: dir, Filename: file, ObjectKey: dir + "/" + file, MimeType: "image/jpeg"}
	repo.assets[a.ID] = a
	st.objects[a.ObjectKey] = true
	return a
}

// Deleting the old object before repointing the row left a failed update pointing at a key that no longer existed
func TestAFailedMoveKeepsTheAssetReadable(t *testing.T) {
	svc, st, repo := fixture()
	a := seedAsset(repo, st, "banners", "a.jpg")
	repo.failMove = true
	if _, err := svc.MoveAsset(context.Background(), MoveAssetInput{ID: a.ID, TargetPath: "archive"}); err == nil {
		t.Fatal("the move reported success")
	}
	if got := repo.assets[a.ID].ObjectKey; !st.objects[got] {
		t.Fatalf("the row points at %s, which is gone", got)
	}
}

func TestAMoveDropsTheOldObjectAfterRepointing(t *testing.T) {
	svc, st, repo := fixture()
	a := seedAsset(repo, st, "banners", "a.jpg")
	moved, err := svc.MoveAsset(context.Background(), MoveAssetInput{ID: a.ID, TargetPath: "archive"})
	if err != nil {
		t.Fatal(err)
	}
	if st.objects[a.ObjectKey] || !st.objects[moved.ObjectKey] || repo.assets[a.ID].ObjectKey != moved.ObjectKey {
		t.Fatalf("after the move: old %v new %v row %s", st.objects[a.ObjectKey], st.objects[moved.ObjectKey], repo.assets[a.ID].ObjectKey)
	}
}

// Dropping the row first leaked the object for good when the storage delete then failed
func TestAFailedStorageDeleteKeepsTheRowToRetryFrom(t *testing.T) {
	svc, st, repo := fixture()
	a := seedAsset(repo, st, "banners", "a.jpg")
	st.failDelete[a.ObjectKey] = true
	if err := svc.DeleteAsset(context.Background(), a.ID, true); err == nil {
		t.Fatal("the delete reported success")
	}
	if _, ok := repo.assets[a.ID]; !ok {
		t.Fatal("the row is gone with the object still stored")
	}
}

func TestRenamingOntoAnExistingDirectoryIsRefused(t *testing.T) {
	svc, st, repo := fixture("archive/old.jpg")
	seedAsset(repo, st, "banners", "a.jpg")
	if _, err := svc.RenameDirectory(context.Background(), "banners", "archive"); !errors.Is(err, ErrPathTaken) {
		t.Fatalf("rename onto archive = %v, want ErrPathTaken", err)
	}
	if !st.objects["banners/a.jpg"] {
		t.Fatal("the refused rename moved files")
	}
}

func TestARenameRepointsEveryRowBeforeDroppingOriginals(t *testing.T) {
	svc, st, repo := fixture()
	a := seedAsset(repo, st, "banners", "a.jpg")
	if _, err := svc.RenameDirectory(context.Background(), "banners", "promos"); err != nil {
		t.Fatal(err)
	}
	got := repo.assets[a.ID]
	if got.ObjectKey != "promos/a.jpg" || !st.objects["promos/a.jpg"] || st.objects["banners/a.jpg"] {
		t.Fatalf("after rename: row %s, new %v, old %v", got.ObjectKey, st.objects["promos/a.jpg"], st.objects["banners/a.jpg"])
	}
}

func TestInputErrorsAreTyped(t *testing.T) {
	svc, st, repo := fixture()
	a := seedAsset(repo, st, "banners", "a.jpg")
	repo.assets[a.ID] = Asset{ID: a.ID, ObjectKey: a.ObjectKey, MimeType: "image/jpeg", Status: AssetStatusPending}
	ctx := context.Background()
	if _, err := svc.ConfirmUpload(ctx, ConfirmUploadInput{AssetID: a.ID}); !errors.Is(err, ErrInvalidDimensions) {
		t.Errorf("confirm without dimensions = %v", err)
	}
	x := 2.0
	if _, err := svc.SetFocalPoint(ctx, a.ID, &x, &x); !errors.Is(err, ErrInvalidFocalPoint) {
		t.Errorf("focal 2,2 = %v", err)
	}
	if _, err := svc.RequestUpload(ctx, RequestUploadInput{Path: "banners", Filename: "a.jpg", MimeType: "image/jpeg"}); !errors.Is(err, ErrInvalidSize) {
		t.Errorf("zero size = %v", err)
	}
}
