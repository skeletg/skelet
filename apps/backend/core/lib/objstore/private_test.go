package objstore

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func env(vars map[string]string) func(string) string {
	return func(name string) string { return vars[name] }
}

func TestThePrivateStoreIsTheMediaStoresSiblingWithNoPublicAddress(t *testing.T) {
	cfg, ok, err := PrivateConfigFromEnv(env(map[string]string{
		"S3_BUCKET_NAME": "media", "AWS_REGION": "eu-west-1", "S3_ENDPOINT_URL": "http://garage:3900", "S3_PRESIGN_ENDPOINT_URL": "http://localhost:9900",
		"S3_PUBLIC_URL_BASE": "https://cdn.shop.test", "S3_USE_PATH_STYLE": "true", PrivateBucketEnv: " vault ", PrivateDirEnv: "/ignored/on/s3",
	}))
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if cfg.Bucket != "vault" || cfg.InternalEndpoint != "http://garage:3900" || cfg.Region != "eu-west-1" || !cfg.UsePathStyle {
		t.Errorf("the private store lost its endpoint or got the wrong bucket: %+v", cfg)
	}
	if cfg.PublicURLBase != "" {
		t.Errorf("the private store was given an address to read from: %+v", cfg)
	}
	if cfg.PresignEndpoint != "http://localhost:9900" {
		t.Errorf("a browser's signed upload has nowhere to go: %+v", cfg)
	}

	dir := t.TempDir()
	cfg, ok, err = PrivateConfigFromEnv(env(map[string]string{
		"STORAGE_DRIVER": DriverFilesystem, "STORAGE_DIR": filepath.Join(dir, "media"), "STORAGE_UPLOAD_URL": "https://api.shop.test",
		PrivateDirEnv: filepath.Join(dir, "vault"), PrivateBucketEnv: "ignored-on-disk",
	}))
	if err != nil || !ok || cfg.Dir != filepath.Join(dir, "vault") || cfg.PublicURLBase != "" {
		t.Errorf("on the filesystem driver: ok=%v err=%v cfg=%+v", ok, err, cfg)
	}
	if cfg.PresignEndpoint != "https://api.shop.test/private-files" {
		t.Errorf("uploads to a private directory are signed for %q, want core's write-only route", cfg.PresignEndpoint)
	}
}

func TestAPrivateDirectoryIsWriteOnlyOverHTTP(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	getenv := env(map[string]string{
		"STORAGE_DRIVER": DriverFilesystem, "STORAGE_DIR": filepath.Join(dir, "media"), "STORAGE_UPLOAD_URL": "https://api.shop.test",
		"STORAGE_SIGNING_KEY": strings.Repeat("k", 32), PrivateDirEnv: filepath.Join(dir, "vault"),
	})
	open := func(cfg Config) *FileStore {
		st, err := New(ctx, cfg)
		if err != nil {
			t.Fatal(err)
		}
		return st.(*FileStore)
	}
	cfg, _, err := PrivateConfigFromEnv(getenv)
	if err != nil {
		t.Fatal(err)
	}
	media, vault := open(ConfigFromEnv(getenv)), open(cfg)
	private := vault.UploadHandler(PrivateUploadPath)
	const key = "webforms/f1/cv/resume.pdf"

	signed, err := vault.PresignUpload(ctx, key, "application/pdf", 4, time.Minute)
	if err != nil || !strings.HasPrefix(signed, "https://api.shop.test/private-files/"+key+"?t=") {
		t.Fatalf("presign = %q %v", signed, err)
	}
	forMedia, err := media.PresignUpload(ctx, key, "application/pdf", 4, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	_, mediaToken, _ := strings.Cut(forMedia, "?t=")
	_, vaultToken, _ := strings.Cut(signed, "?t=")
	if rec := put(t, private, "https://api.shop.test/private-files/"+key+"?t="+mediaToken, "application/pdf", []byte("%PDF")); rec.Code != http.StatusForbidden {
		t.Errorf("a link signed for the media store wrote to the private one: %d", rec.Code)
	}
	if rec := put(t, media.Handler(), "https://api.shop.test/files/"+key+"?t="+vaultToken, "application/pdf", []byte("%PDF")); rec.Code != http.StatusForbidden {
		t.Errorf("a link signed for the private store wrote to the media one, which is served: %d", rec.Code)
	}

	if rec := put(t, private, signed, "application/pdf", []byte("%PDF")); rec.Code != http.StatusOK {
		t.Fatalf("the signed upload got %d: %s", rec.Code, rec.Body)
	}
	if size, _, ok, _ := vault.Head(ctx, key); !ok || size != 4 {
		t.Fatalf("the upload isn't in the private store: %d %v", size, ok)
	}
	if _, _, ok, _ := media.Head(ctx, key); ok {
		t.Error("the upload reached the media store")
	}
	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodDelete} {
		rec := httptest.NewRecorder()
		private.ServeHTTP(rec, httptest.NewRequest(method, signed, nil))
		if rec.Code != http.StatusMethodNotAllowed || rec.Body.Len() > 64 {
			t.Errorf("%s on the private route got %d %q, want it refused with nothing of the file", method, rec.Code, rec.Body)
		}
	}
}

func TestNoPrivateStoreUnlessOneIsAskedFor(t *testing.T) {
	for name, vars := range map[string]map[string]string{
		"s3":                     {"S3_BUCKET_NAME": "media"},
		"s3, blank":              {"S3_BUCKET_NAME": "media", PrivateBucketEnv: "  "},
		"filesystem":             {"STORAGE_DRIVER": DriverFilesystem, "STORAGE_DIR": "/data/media"},
		"the other driver's var": {"STORAGE_DRIVER": DriverFilesystem, "STORAGE_DIR": "/data/media", PrivateBucketEnv: "vault"},
	} {
		if _, ok, err := PrivateConfigFromEnv(env(vars)); ok || err != nil {
			t.Errorf("%s: ok=%v err=%v, want no private store and no complaint", name, ok, err)
		}
	}
}

func TestAPrivateStoreThatWouldBeServedIsRefused(t *testing.T) {
	for name, vars := range map[string]map[string]string{
		"the media bucket itself":     {"S3_BUCKET_NAME": "media", PrivateBucketEnv: "media"},
		"the media directory itself":  {"STORAGE_DRIVER": DriverFilesystem, "STORAGE_DIR": "/data/media", PrivateDirEnv: "/data/media/"},
		"inside the media directory":  {"STORAGE_DRIVER": DriverFilesystem, "STORAGE_DIR": "/data/media", PrivateDirEnv: "/data/media/private"},
		"reached by climbing back in": {"STORAGE_DRIVER": DriverFilesystem, "STORAGE_DIR": "/data/media", PrivateDirEnv: "/data/other/../media/x"},
		"around the media directory":  {"STORAGE_DRIVER": DriverFilesystem, "STORAGE_DIR": "/data/media", PrivateDirEnv: "/data"},
	} {
		if _, ok, err := PrivateConfigFromEnv(env(vars)); ok || !errors.Is(err, ErrPrivateIsPublic) {
			t.Errorf("%s: ok=%v err=%v, want ErrPrivateIsPublic", name, ok, err)
		}
	}
	if _, ok, err := PrivateConfigFromEnv(env(map[string]string{"STORAGE_DRIVER": DriverFilesystem, "STORAGE_DIR": "/data/media", PrivateDirEnv: "/data/media-private"})); !ok || err != nil {
		t.Errorf("a sibling whose name starts the same way was refused: ok=%v err=%v", ok, err)
	}
}

type probeRig struct {
	store          Storage
	cfg, media     Config
	asked          []string
	publicEndpoint *httptest.Server
}

func newProbeRig(t *testing.T, serve func(rig *probeRig, key string) (int, string)) *probeRig {
	t.Helper()
	rig := &probeRig{}
	rig.publicEndpoint = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Errorf("the probe sent credentials to %s", r.URL)
		}
		rig.asked = append(rig.asked, r.URL.Path)
		status, body := serve(rig, strings.TrimPrefix(r.URL.Path, "/"))
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(rig.publicEndpoint.Close)
	rig.media = Config{Driver: DriverFilesystem, Dir: t.TempDir(), PublicURLBase: rig.publicEndpoint.URL}
	rig.cfg = rig.media.Private(t.TempDir())
	store, err := New(context.Background(), Config{Driver: DriverFilesystem, Dir: rig.cfg.Dir, SigningKey: strings.Repeat("k", 32)})
	if err != nil {
		t.Fatal(err)
	}
	rig.store = store
	return rig
}

func (r *probeRig) stored(t *testing.T, key string) string {
	t.Helper()
	raw, err := r.store.GetBytes(context.Background(), key)
	if err != nil {
		return ""
	}
	return string(raw)
}

func TestTheProbePassesAStoreNobodyCanReadAndCleansUp(t *testing.T) {
	rig := newProbeRig(t, func(*probeRig, string) (int, string) { return http.StatusForbidden, "AccessDenied" })
	unchecked, err := ProbePrivate(context.Background(), rig.store, rig.cfg, rig.media, nil)
	if err != nil || len(unchecked) != 0 {
		t.Fatalf("err=%v unchecked=%v", err, unchecked)
	}
	if len(rig.asked) != 1 || !strings.HasPrefix(rig.asked[0], "/"+probePrefix) {
		t.Fatalf("asked %v, want one request for the marker at the public address", rig.asked)
	}
	if left, _ := rig.store.ListPrefix(context.Background(), probePrefix); len(left) != 0 {
		t.Errorf("the marker was left behind: %v", left)
	}
}

// Only the marker itself is proof - plenty of CDNs answer 200 with the shop's own page for a path that isn't there
func TestTheProbeTellsAReadableStoreFromAPageThatAnswersEverything(t *testing.T) {
	leaky := newProbeRig(t, func(rig *probeRig, key string) (int, string) { return http.StatusOK, rig.stored(t, key) + "\n" })
	_, err := ProbePrivate(context.Background(), leaky.store, leaky.cfg, leaky.media, nil)
	if !errors.Is(err, ErrPrivateReadable) || !strings.Contains(err.Error(), leaky.publicEndpoint.URL) {
		t.Fatalf("a store served at the public address: %v, want ErrPrivateReadable naming where", err)
	}
	if left, _ := leaky.store.ListPrefix(context.Background(), probePrefix); len(left) != 0 {
		t.Errorf("the marker was left behind in a store anyone can read: %v", left)
	}

	spa := newProbeRig(t, func(*probeRig, string) (int, string) { return http.StatusOK, "<html>the shop's home page</html>" })
	if unchecked, err := ProbePrivate(context.Background(), spa.store, spa.cfg, spa.media, nil); err != nil || len(unchecked) != 0 {
		t.Fatalf("a page that answers every path was taken for the file: err=%v unchecked=%v", err, unchecked)
	}
}

func TestTheProbeSaysWhatItCouldNotCheckAndWhenItCannotWrite(t *testing.T) {
	rig := newProbeRig(t, func(*probeRig, string) (int, string) { return http.StatusForbidden, "" })
	rig.publicEndpoint.Close()
	unchecked, err := ProbePrivate(context.Background(), rig.store, rig.cfg, rig.media, nil)
	if err != nil || len(unchecked) != 1 || !strings.HasPrefix(unchecked[0], rig.media.PublicURLBase) {
		t.Fatalf("an address that doesn't answer: err=%v unchecked=%v, want it reported and no failure", err, unchecked)
	}

	if _, err := ProbePrivate(context.Background(), unwritable{rig.store}, rig.cfg, rig.media, nil); err == nil || errors.Is(err, ErrPrivateReadable) || !strings.Contains(err.Error(), "can't be written") {
		t.Fatalf("a store that refuses the marker: %v, want a plain \"can't be written\"", err)
	}
}

type unwritable struct{ Storage }

func (unwritable) PutBytes(context.Context, string, []byte, string) error {
	return errors.New("NoSuchBucket")
}

func TestAStrangerIsLookedForAtEveryAddressTheFileCouldHave(t *testing.T) {
	garage := Config{Bucket: "media", InternalEndpoint: "http://garage:3900/", PublicURLBase: "http://media.localhost:9902/", UsePathStyle: true}
	aws := Config{Bucket: "shop-media", Region: "eu-west-1", PublicURLBase: "https://cdn.shop.test"}
	for name, tc := range map[string]struct {
		media Config
		place string
		want  []string
	}{
		"an S3-compatible store with buckets named by host": {garage, "vault", []string{
			"http://garage:3900/vault/k", "http://media.localhost:9902/k", "http://vault.localhost:9902/k",
		}},
		"AWS behind a CDN":                     {aws, "shop-private", []string{"https://shop-private.s3.eu-west-1.amazonaws.com/k", "https://cdn.shop.test/k"}},
		"a directory served by core":           {Config{Driver: DriverFilesystem, Dir: "/data/media", PublicURLBase: "https://api.shop.test/files"}, "/data/vault", []string{"https://api.shop.test/files/k"}},
		"a media store with no public address": {Config{Bucket: "media", InternalEndpoint: "http://minio:9000"}, "vault", []string{"http://minio:9000/vault/k"}},
	} {
		got := strangerAddresses(tc.media.Private(tc.place), tc.media, "k")
		if strings.Join(got, " ") != strings.Join(tc.want, " ") {
			t.Errorf("%s:\n got  %v\n want %v", name, got, tc.want)
		}
	}
}
