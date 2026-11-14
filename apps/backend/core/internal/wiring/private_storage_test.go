package wiring

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/skeletg/skelet/apps/backend/core/lib/objstore"
)

func privateRig(t *testing.T, privateDir string, serve http.Handler) (media objstore.Storage, mediaDir string) {
	t.Helper()
	mediaDir = filepath.Join(t.TempDir(), "media")
	t.Setenv("STORAGE_DRIVER", objstore.DriverFilesystem)
	t.Setenv("STORAGE_DIR", mediaDir)
	t.Setenv("STORAGE_SIGNING_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv(objstore.PrivateDirEnv, privateDir)
	if serve != nil {
		srv := httptest.NewServer(serve)
		t.Cleanup(srv.Close)
		t.Setenv("STORAGE_PUBLIC_URL", srv.URL)
	}
	media, err := objstore.New(context.Background(), objstore.FromEnv())
	if err != nil {
		t.Fatal(err)
	}
	return media, mediaDir
}

func TestWithNoPrivateStoreTheMediaStoreStandsIn(t *testing.T) {
	media, _ := privateRig(t, "", nil)
	got, err := newPrivateStorage(RootContext{context.Background()}, Config{Env: "production"}, media)
	if err != nil || got.Storage != media {
		t.Fatalf("err=%v, want the media store handed back as it always was", err)
	}
}

func TestAPrivateStoreKeepsItsFilesOutOfTheMediaStore(t *testing.T) {
	vault := filepath.Join(t.TempDir(), "vault")
	media, mediaDir := privateRig(t, vault, nil)
	got, err := newPrivateStorage(RootContext{context.Background()}, Config{Env: "production"}, media)
	if err != nil {
		t.Fatal(err)
	}
	if err := got.PutBytes(context.Background(), "data/exports/j1/customers.csv", []byte("pii"), "text/csv"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(vault, "data/exports/j1/customers.csv")); err != nil {
		t.Errorf("the export isn't in the private directory: %v", err)
	}
	if _, err := os.Stat(filepath.Join(mediaDir, "data")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the export reached the media directory, which is served: %v", err)
	}
	if got.HasPublicBase() {
		t.Error("the private store has a public address")
	}
}

func TestAMisplacedPrivateStoreStopsTheBoot(t *testing.T) {
	media, mediaDir := privateRig(t, "", nil)
	t.Setenv(objstore.PrivateDirEnv, filepath.Join(mediaDir, "private"))
	if _, err := newPrivateStorage(RootContext{context.Background()}, Config{Env: "development"}, media); !errors.Is(err, objstore.ErrPrivateIsPublic) {
		t.Fatalf("a private directory inside the served one: %v, want ErrPrivateIsPublic even in development", err)
	}
}

func TestAPrivateStoreAnyoneCanReadStopsProductionAndWarnsInDevelopment(t *testing.T) {
	vault := filepath.Join(t.TempDir(), "vault")
	media, _ := privateRig(t, vault, http.FileServer(http.Dir(vault)))

	if _, err := newPrivateStorage(RootContext{context.Background()}, Config{Env: "production"}, media); !errors.Is(err, objstore.ErrPrivateReadable) {
		t.Fatalf("production with a readable private store: %v, want the boot refused", err)
	}
	got, err := newPrivateStorage(RootContext{context.Background()}, Config{Env: "development"}, media)
	if err != nil || got.Storage == media || got.Storage == nil {
		t.Fatalf("development with a readable private store: err=%v, want it up, warned, and still on its own store", err)
	}
}

func TestAPrivateStoreThatCannotBeReachedStopsOnlyProduction(t *testing.T) {
	gone := httptest.NewServer(http.NotFoundHandler())
	gone.Close()
	for name, value := range map[string]string{
		"STORAGE_DRIVER": objstore.DriverS3, "S3_BUCKET_NAME": "media", "S3_ENDPOINT_URL": gone.URL, "S3_USE_PATH_STYLE": "true",
		objstore.PrivateBucketEnv: "private", "AWS_ACCESS_KEY_ID": "test", "AWS_SECRET_ACCESS_KEY": "test", "AWS_MAX_ATTEMPTS": "1",
	} {
		t.Setenv(name, value)
	}
	media, err := objstore.New(context.Background(), objstore.FromEnv())
	if err != nil {
		t.Fatal(err)
	}

	if _, err := newPrivateStorage(RootContext{context.Background()}, Config{Env: "production"}, media); err == nil {
		t.Fatal("production with a private store it can't reach started anyway")
	}
	got, err := newPrivateStorage(RootContext{context.Background()}, Config{Env: "development"}, media)
	if err != nil || got.Storage == media || got.Storage == nil {
		t.Fatalf("development with a private store that isn't up yet: err=%v, want it up, warned, and still on its own store", err)
	}
}

func TestOnlyAPrivateDirectoryGetsItsOwnUploadRoute(t *testing.T) {
	vault := filepath.Join(t.TempDir(), "vault")
	media, _ := privateRig(t, vault, nil)
	t.Setenv("STORAGE_UPLOAD_URL", "https://api.shop.test")
	private, err := newPrivateStorage(RootContext{context.Background()}, Config{Env: "development"}, media)
	if err != nil {
		t.Fatal(err)
	}
	route, ok := privateUploadRoute(private, media)
	if !ok {
		t.Fatal("a private directory has no route for a shopper's signed upload")
	}
	signed, err := private.PresignUpload(context.Background(), "webforms/f1/cv/a.pdf", "application/pdf", 4, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPut, signed, strings.NewReader("%PDF"))
	req.Header.Set("Content-Type", "application/pdf")
	rec := httptest.NewRecorder()
	route.ServeHTTP(rec, req)
	if _, err := os.Stat(filepath.Join(vault, "webforms/f1/cv/a.pdf")); rec.Code != http.StatusOK || err != nil {
		t.Fatalf("the signed upload got %d and the file: %v", rec.Code, err)
	}
	rec = httptest.NewRecorder()
	route.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, signed, nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("the upload route answered a read with %d", rec.Code)
	}

	if _, ok := privateUploadRoute(PrivateStorage{media}, media); ok {
		t.Error("with no private store the media store was given a second upload route")
	}
	if _, ok := privateUploadRoute(PrivateStorage{}, media); ok {
		t.Error("no storage at all got an upload route")
	}
}
