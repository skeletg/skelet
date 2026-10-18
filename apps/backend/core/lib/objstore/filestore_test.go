package objstore

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func newTestFileStore(t *testing.T) *FileStore {
	t.Helper()
	s, err := New(context.Background(), Config{
		Driver: DriverFilesystem, Dir: t.TempDir(), SigningKey: strings.Repeat("k", 32),
		PublicURLBase: "https://cdn.shop", PresignEndpoint: "https://api.shop/files", MaxUploadBytes: 64,
	})
	if err != nil {
		t.Fatal(err)
	}
	fs := s.(*FileStore)
	t.Cleanup(func() { _ = fs.Close() })
	return fs
}

func TestFileStoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := newTestFileStore(t)
	if err := s.PutBytes(ctx, "catalog/p/1.png", []byte("png!"), "image/png"); err != nil {
		t.Fatal(err)
	}
	size, ct, ok, err := s.Head(ctx, "catalog/p/1.png")
	if err != nil || !ok || size != 4 || ct != "image/png" {
		t.Fatalf("head = %d %q %v %v", size, ct, ok, err)
	}
	if err := s.Copy(ctx, "catalog/p/1.png", "catalog/q/2.png"); err != nil {
		t.Fatal(err)
	}
	b, ct, err := s.GetBytesTyped(ctx, "catalog/q/2.png")
	if err != nil || string(b) != "png!" || ct != "image/png" {
		t.Fatalf("copy read back %q %q %v", b, ct, err)
	}
	if _, err := s.GetBytes(ctx, "catalog/nope.png"); !errors.Is(err, ErrObjectNotFound) {
		t.Fatalf("missing key = %v, want ErrObjectNotFound", err)
	}
	if _, _, ok, err := s.Head(ctx, ".skelet-health-probe"); ok || err != nil {
		t.Fatalf("health probe head = %v %v", ok, err)
	}
	if _, _, ok, _ := s.Head(ctx, "catalog/p"); ok {
		t.Fatal("a directory answered Head as an object")
	}
}

func TestFileStoreRefusesKeysOutsideItsRoot(t *testing.T) {
	ctx := context.Background()
	s := newTestFileStore(t)
	outside := filepath.Join(filepath.Dir(s.cfg.Dir), "escaped")
	for _, key := range []string{"../escaped", "a/../../escaped", "/etc/passwd", "a//b", "a/./b", `a\b`, "a\x00b", "", ".skelet/meta/x", ".skelet", "dir/"} {
		if err := s.PutBytes(ctx, key, []byte("x"), ""); !errors.Is(err, ErrInvalidKey) {
			t.Errorf("PutBytes(%q) = %v, want ErrInvalidKey", key, err)
		}
	}
	if _, err := os.Stat(outside); err == nil {
		t.Fatal("a key escaped the storage directory")
	}
	if err := os.Symlink(filepath.Dir(s.cfg.Dir), filepath.Join(s.cfg.Dir, "link")); err != nil {
		t.Fatal(err)
	}
	if err := s.PutBytes(ctx, "link/escaped", []byte("x"), ""); err == nil {
		t.Fatal("a write followed a symlink out of the root")
	}
}

func TestFileStoreListingMatchesS3(t *testing.T) {
	ctx := context.Background()
	s := newTestFileStore(t)
	for _, k := range []string{"a/b/photo.png", "a/b/phone/x.png", "a/b/z.png", "a/c.txt", "a-d.txt", "top.txt"} {
		if err := s.PutBytes(ctx, k, []byte(k), ""); err != nil {
			t.Fatal(err)
		}
	}
	keys, _ := s.ListPrefix(ctx, "a")
	if want := []string{"a-d.txt", "a/b/phone/x.png", "a/b/photo.png", "a/b/z.png", "a/c.txt"}; !slices.Equal(keys, want) {
		t.Fatalf("ListPrefix(a) = %v, want %v", keys, want)
	}
	keys, _ = s.ListPrefix(ctx, "a/b/ph")
	if want := []string{"a/b/phone/x.png", "a/b/photo.png"}; !slices.Equal(keys, want) {
		t.Fatalf("ListPrefix(a/b/ph) = %v, want %v", keys, want)
	}
	all, _ := s.ListPrefix(ctx, "")
	if len(all) != 6 {
		t.Fatalf("ListPrefix(\"\") = %v - the reserved tree leaked or objects went missing", all)
	}
	prefixes, objs, err := s.ListDirectoryContents(ctx, "a/b/")
	if err != nil || !slices.Equal(prefixes, []string{"a/b/phone/"}) || len(objs) != 2 || objs[0].Key != "a/b/photo.png" || objs[0].Size != int64(len("a/b/photo.png")) {
		t.Fatalf("ListDirectoryContents(a/b/) = %v %+v %v", prefixes, objs, err)
	}
	prefixes, objs, _ = s.ListDirectoryContents(ctx, "a/b/ph")
	if !slices.Equal(prefixes, []string{"a/b/phone/"}) || len(objs) != 1 || objs[0].Key != "a/b/photo.png" {
		t.Fatalf("ListDirectoryContents(a/b/ph) = %v %+v, want only the ph* entries", prefixes, objs)
	}
	top, _ := s.ListImmediatePrefixes(ctx, "")
	if !slices.Equal(top, []string{"a/"}) {
		t.Fatalf("ListImmediatePrefixes(\"\") = %v, want [a/] without the reserved tree", top)
	}
	if keys, err := s.ListPrefix(ctx, "missing/"); err != nil || len(keys) != 0 {
		t.Fatalf("an absent prefix = %v %v, want empty", keys, err)
	}
}

// S3 has no directories, so once the last object under a prefix goes the prefix must stop listing
func TestFileStoreDeletePrunesEmptyPrefixes(t *testing.T) {
	ctx := context.Background()
	s := newTestFileStore(t)
	_ = s.PutBytes(ctx, "lib/x/y/one.png", []byte("1"), "")
	_ = s.PutBytes(ctx, "lib/keep.png", []byte("1"), "")
	if err := s.Delete(ctx, "lib/x/y/one.png"); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, "lib/x/y/one.png"); err != nil {
		t.Fatalf("a second delete = %v, want nil", err)
	}
	prefixes, objs, _ := s.ListDirectoryContents(ctx, "lib/")
	if len(prefixes) != 0 || len(objs) != 1 {
		t.Fatalf("after delete lib/ lists %v %+v, want only keep.png", prefixes, objs)
	}
	if _, err := os.Stat(filepath.Join(s.cfg.Dir, metaDir, "lib", "x")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the metadata tree kept the empty prefix: %v", err)
	}
}

func put(t *testing.T, h http.Handler, url, contentType string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, url, bytes.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestFileStoreSignedUploads(t *testing.T) {
	ctx := context.Background()
	s := newTestFileStore(t)
	h := s.Handler()
	signed, err := s.PresignUpload(ctx, "media/a.png", "image/png", 5, time.Minute)
	if err != nil || !strings.HasPrefix(signed, "https://api.shop/files/media/a.png?t=") {
		t.Fatalf("presign = %q %v", signed, err)
	}
	if rec := put(t, h, signed, "text/html", []byte("12345")); rec.Code != http.StatusForbidden {
		t.Fatalf("a different content type got %d", rec.Code)
	}
	for _, body := range []string{"1234567890", "123"} {
		if rec := put(t, h, signed, "image/png", []byte(body)); rec.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("a %d-byte body on a 5-byte grant got %d", len(body), rec.Code)
		}
	}
	_, token, _ := strings.Cut(signed, "?t=")
	for name, u := range map[string]string{
		"a tampered token":        signed[:len(signed)-2] + "AA",
		"no token":                "https://api.shop/files/media/a.png",
		"another key's path":      "https://api.shop/files/media/evil.png?t=" + token,
		"a token without its sig": "https://api.shop/files/media/a.png?t=" + strings.SplitN(token, ".", 2)[0],
	} {
		if rec := put(t, h, u, "image/png", []byte("12345")); rec.Code != http.StatusForbidden {
			t.Fatalf("%s got %d", name, rec.Code)
		}
	}
	if _, _, ok, _ := s.Head(ctx, "media/evil.png"); ok {
		t.Fatal("a token wrote to a key it wasn't signed for")
	}
	if _, _, ok, _ := s.Head(ctx, "media/a.png"); ok {
		t.Fatal("a refused upload left an object behind")
	}
	if rec := put(t, h, signed, "image/png", []byte("12345")); rec.Code != http.StatusOK {
		t.Fatalf("the signed upload got %d: %s", rec.Code, rec.Body)
	}
	if size, ct, ok, _ := s.Head(ctx, "media/a.png"); !ok || size != 5 || ct != "image/png" {
		t.Fatalf("uploaded object head = %d %q %v", size, ct, ok)
	}

	expired, _ := s.PresignUpload(ctx, "media/b.png", "image/png", 5, -time.Minute)
	if rec := put(t, h, expired, "image/png", []byte("12345")); rec.Code != http.StatusForbidden {
		t.Fatalf("an expired token got %d", rec.Code)
	}
	unsized, _ := s.PresignUpload(ctx, "media/c.bin", "application/octet-stream", 0, time.Minute)
	if rec := put(t, h, unsized, "application/octet-stream", bytes.Repeat([]byte("x"), 65)); rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("an unsized upload past MaxUploadBytes got %d", rec.Code)
	}
	other := &FileStore{cfg: Config{SigningKey: strings.Repeat("o", 32)}}
	if _, err := other.verify(token, time.Now()); err == nil {
		t.Fatal("a token verified under a different signing key")
	}
	if got := s.PublicURL("media/a.png"); got != "https://cdn.shop/media/a.png" {
		t.Fatalf("PublicURL = %q, want the STORAGE_PUBLIC_URL base", got)
	}
}

func TestFileStoreServesWithoutRunningContent(t *testing.T) {
	ctx := context.Background()
	s := newTestFileStore(t)
	_ = s.PutBytes(ctx, "m/pic.png", []byte("png"), "image/png")
	_ = s.PutBytes(ctx, "m/evil.html", []byte("<script>alert(1)</script>"), "text/html")
	_ = s.PutBytes(ctx, "m/logo.svg", []byte("<svg/>"), "image/svg+xml")
	serve := func(method, key string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, httptest.NewRequest(method, "/files/"+key, nil))
		return rec
	}
	rec := serve(http.MethodGet, "m/pic.png")
	if rec.Code != 200 || rec.Body.String() != "png" || rec.Header().Get("Content-Type") != "image/png" || rec.Header().Get("Content-Disposition") != "" {
		t.Fatalf("image: %d %q %v", rec.Code, rec.Body, rec.Header())
	}
	for _, key := range []string{"m/evil.html", "m/logo.svg"} {
		rec := serve(http.MethodGet, key)
		if rec.Header().Get("Content-Disposition") != "attachment" || rec.Header().Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(rec.Header().Get("Content-Security-Policy"), "sandbox") {
			t.Fatalf("%s served inline or unsandboxed: %v", key, rec.Header())
		}
	}
	if rec := serve(http.MethodHead, "m/pic.png"); rec.Code != 200 || rec.Body.Len() != 0 {
		t.Fatalf("HEAD = %d with %d body bytes", rec.Code, rec.Body.Len())
	}
	for _, key := range []string{"m/none.png", ".skelet/meta/m/pic.png", "m"} {
		if rec := serve(http.MethodGet, key); rec.Code != http.StatusNotFound {
			t.Fatalf("GET %s = %d, want 404", key, rec.Code)
		}
	}
	for _, method := range []string{http.MethodDelete, http.MethodPost, http.MethodPatch} {
		if rec := serve(method, "m/pic.png"); rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s = %d, want 405", method, rec.Code)
		}
	}
}

func TestMigrateBetweenDrivers(t *testing.T) {
	ctx := context.Background()
	src, dst := newTestFileStore(t), newTestFileStore(t)
	_ = src.PutBytes(ctx, "a/1.png", []byte("one"), "image/png")
	_ = src.PutBytes(ctx, "a/2.txt", []byte("two"), "text/plain")
	_ = dst.PutBytes(ctx, "a/2.txt", []byte("TWO"), "text/plain")
	stats, err := Migrate(ctx, src, dst, MigrateOptions{})
	if err != nil || stats.Scanned != 2 || stats.Copied != 1 || stats.Skipped != 1 {
		t.Fatalf("migrate = %+v %v", stats, err)
	}
	if b, ct, _ := dst.GetBytesTyped(ctx, "a/1.png"); string(b) != "one" || ct != "image/png" {
		t.Fatalf("copied object = %q %q", b, ct)
	}
}

func TestConfigFromEnvFilesystem(t *testing.T) {
	env := map[string]string{"STORAGE_DRIVER": "filesystem", "STORAGE_DIR": "/data", "STORAGE_UPLOAD_URL": "https://api.shop/", "S3_BUCKET_NAME": "media"}
	cfg := ConfigFromEnv(func(k string) string { return env[k] })
	if cfg.Bucket != "" || cfg.Dir != "/data" || cfg.PublicURLBase != "https://api.shop/files" || cfg.PresignEndpoint != "https://api.shop/files" {
		t.Fatalf("cfg = %+v, want core serving its own files", cfg)
	}
	env["STORAGE_PUBLIC_URL"] = "https://cdn.shop/"
	if cfg := ConfigFromEnv(func(k string) string { return env[k] }); cfg.PublicURLBase != "https://cdn.shop" || cfg.PresignEndpoint != "https://api.shop/files" {
		t.Fatalf("cfg = %+v, want reads from the CDN and uploads still to core", cfg)
	}
	if cfg.SigningKey != "" {
		t.Fatal("outside development the dev signing key filled in")
	}
	env["APP_ENV"] = "development"
	if cfg := ConfigFromEnv(func(k string) string { return env[k] }); cfg.SigningKey == "" {
		t.Fatal("development got no signing key")
	}
	if _, err := New(context.Background(), Config{Driver: "ftp", Bucket: "media", AccessKeyID: "k", SecretAccessKey: "s"}); err == nil {
		t.Fatal("an unknown driver was accepted")
	}
}
