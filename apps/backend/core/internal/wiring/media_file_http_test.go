package wiring

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/medialibrary"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

type libraryFiles struct {
	assets      map[uuid.UUID]medialibrary.Asset
	resized     []string
	fail        error
	noPrivate   bool
	lookupFails error
	movedTo     medialibrary.Visibility
}

func (l *libraryFiles) source() mediaFiles {
	return mediaFiles{
		open: func(_ context.Context, id uuid.UUID) (io.ReadCloser, medialibrary.Asset, error) {
			a, ok := l.assets[id]
			if l.fail != nil {
				return nil, medialibrary.Asset{}, l.fail
			}
			if !ok {
				return nil, medialibrary.Asset{}, medialibrary.ErrAssetNotFound
			}
			if l.movedTo != "" {
				a.Visibility = l.movedTo
			}
			return io.NopCloser(strings.NewReader("original of " + a.Filename)), a, nil
		},
		asset: func(_ context.Context, id uuid.UUID) (medialibrary.Asset, error) {
			if l.lookupFails != nil {
				return medialibrary.Asset{}, l.lookupFails
			}
			a, ok := l.assets[id]
			if !ok {
				return medialibrary.Asset{}, medialibrary.ErrAssetNotFound
			}
			return a, nil
		},
		allowed: func(_ context.Context, a medialibrary.Asset) error {
			if l.noPrivate && a.Visibility == medialibrary.Private {
				return &auth.ScopedDenial{Slug: medialibrary.PermPrivateView}
			}
			return nil
		},
		resize: func(_ context.Context, a medialibrary.Asset, width int) ([]byte, string, error) {
			if l.fail != nil {
				return nil, "", l.fail
			}
			l.resized = append(l.resized, a.Filename)
			return []byte("small " + a.Filename), "image/webp", nil
		},
	}
}

func newLibraryFiles() (*libraryFiles, map[string]uuid.UUID) {
	l, ids := &libraryFiles{assets: map[uuid.UUID]medialibrary.Asset{}}, map[string]uuid.UUID{}
	for name, mime := range map[string]string{"photo.jpg": "image/jpeg", "Sale \"banner\".png": "image/png", "contract.pdf": "application/pdf", "logo.svg": "image/svg+xml"} {
		id := uuid.New()
		ids[name] = id
		l.assets[id] = medialibrary.Asset{ID: id, Filename: name, MimeType: mime, Status: medialibrary.AssetStatusReady, Visibility: medialibrary.Private}
	}
	return l, ids
}

func get(h http.Handler, method, target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return rec
}

func TestALibraryFileIsHandedOverAsAnInertDownload(t *testing.T) {
	files, ids := newLibraryFiles()
	serve := serveMediaFile(files.source())
	for _, name := range []string{"Sale \"banner\".png", "contract.pdf", "logo.svg"} {
		rec := get(serve, http.MethodGet, mediaFilePath+ids[name].String())
		if rec.Code != http.StatusOK || rec.Body.String() != "original of "+name {
			t.Fatalf("%s: %d %q", name, rec.Code, rec.Body.String())
		}
		for header, want := range map[string]string{
			"Content-Type":            files.assets[ids[name]].MimeType,
			"X-Content-Type-Options":  "nosniff",
			"Content-Security-Policy": "default-src 'none'; sandbox",
			"Cache-Control":           "private, no-store",
		} {
			if got := rec.Header().Get(header); got != want {
				t.Errorf("%s: %s = %q, want %q", name, header, got, want)
			}
		}
		if cd := rec.Header().Get("Content-Disposition"); !strings.HasPrefix(cd, "attachment") || strings.Contains(cd, `"banner"`) {
			t.Errorf("%s: Content-Disposition = %q, want an attachment with the name made safe", name, cd)
		}
	}
}

func TestAPictureCanBeAskedForSmaller(t *testing.T) {
	files, ids := newLibraryFiles()
	serve := serveMediaFile(files.source())
	rec := get(serve, http.MethodGet, mediaFilePath+ids["photo.jpg"].String()+"?w=640")
	if rec.Code != http.StatusOK || rec.Body.String() != "small photo.jpg" || rec.Header().Get("Content-Type") != "image/webp" {
		t.Fatalf("got %d %q %q", rec.Code, rec.Body.String(), rec.Header().Get("Content-Type"))
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "private, no-store" {
		t.Errorf("a private picture's copy may be kept by the browser, or by something in between: Cache-Control %q", cc)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("the resized copy can be sniffed into something else")
	}

	for name, tc := range map[string]struct {
		target string
		want   int
	}{
		"a document":      {mediaFilePath + ids["contract.pdf"].String() + "?w=640", http.StatusUnsupportedMediaType},
		"a vector":        {mediaFilePath + ids["logo.svg"].String() + "?w=640", http.StatusUnsupportedMediaType},
		"no such file":    {mediaFilePath + uuid.NewString() + "?w=640", http.StatusNotFound},
		"a width of zero": {mediaFilePath + ids["photo.jpg"].String() + "?w=0", http.StatusBadRequest},
		"a huge width":    {mediaFilePath + ids["photo.jpg"].String() + "?w=99999", http.StatusBadRequest},
		"not a number":    {mediaFilePath + ids["photo.jpg"].String() + "?w=wide", http.StatusBadRequest},
	} {
		if rec := get(serve, http.MethodGet, tc.target); rec.Code != tc.want {
			t.Errorf("%s: %d, want %d", name, rec.Code, tc.want)
		}
	}
	if len(files.resized) != 1 {
		t.Errorf("resized %v, want only the one picture that could be", files.resized)
	}
}

func TestTheFileRouteRefusesWhatItCannotServe(t *testing.T) {
	files, ids := newLibraryFiles()
	serve := serveMediaFile(files.source())
	photo := mediaFilePath + ids["photo.jpg"].String()
	for name, tc := range map[string]struct {
		method, target string
		want           int
	}{
		"a write":       {http.MethodPost, photo, http.StatusMethodNotAllowed},
		"not an id":     {http.MethodGet, mediaFilePath + "../../etc/passwd", http.StatusBadRequest},
		"no such asset": {http.MethodGet, mediaFilePath + uuid.NewString(), http.StatusNotFound},
	} {
		if rec := get(serve, tc.method, tc.target); rec.Code != tc.want {
			t.Errorf("%s: %d, want %d", name, rec.Code, tc.want)
		}
	}

	files.fail = errors.New("dial tcp: refused")
	for _, target := range []string{photo, photo + "?w=640"} {
		rec := get(serve, http.MethodGet, target)
		if rec.Code != http.StatusBadGateway || strings.Contains(rec.Body.String(), "dial tcp") {
			t.Errorf("storage down on %s: %d %q, want a 502 that keeps the storage error to itself", target, rec.Code, rec.Body.String())
		}
	}
}

func TestTheFileRouteDoesNotGuessWhenItCannotTell(t *testing.T) {
	files, ids := newLibraryFiles()
	serve := serveMediaFile(files.source())
	photo := mediaFilePath + ids["photo.jpg"].String()

	files.movedTo = medialibrary.Public
	if rec := get(serve, http.MethodGet, photo); rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), "original of") {
		t.Errorf("a file that changed halves mid-request: %d %q, want a 404 with none of it", rec.Code, rec.Body.String())
	}
	files.movedTo = ""
	files.lookupFails = errors.New("pq: connection refused")
	for _, target := range []string{photo, photo + "?w=640"} {
		rec := get(serve, http.MethodGet, target)
		if rec.Code != http.StatusBadGateway || strings.Contains(rec.Body.String(), "pq:") {
			t.Errorf("the database down on %s: %d %q, want a 502 that keeps the error to itself", target, rec.Code, rec.Body.String())
		}
	}
}

func TestAPrivateFileNeedsThePrivateHalfsGrant(t *testing.T) {
	files, ids := newLibraryFiles()
	files.noPrivate = true
	open := uuid.New()
	files.assets[open] = medialibrary.Asset{ID: open, Filename: "banner.jpg", MimeType: "image/jpeg", Status: medialibrary.AssetStatusReady, Visibility: medialibrary.Public}
	serve := serveMediaFile(files.source())

	for _, target := range []string{mediaFilePath + ids["contract.pdf"].String(), mediaFilePath + ids["photo.jpg"].String() + "?w=640"} {
		rec := get(serve, http.MethodGet, target)
		if rec.Code != http.StatusForbidden || strings.Contains(rec.Body.String(), "contract.pdf") || strings.Contains(rec.Body.String(), "photo.jpg") {
			t.Errorf("%s: %d %q, want a bare 403", target, rec.Code, rec.Body.String())
		}
		if rec.Header().Get("Content-Disposition") != "" {
			t.Errorf("%s: the refusal names the file: %q", target, rec.Header().Get("Content-Disposition"))
		}
	}
	if len(files.resized) != 0 {
		t.Errorf("resized %v for a caller who may not see them", files.resized)
	}
	for _, target := range []string{mediaFilePath + open.String(), mediaFilePath + open.String() + "?w=640"} {
		if rec := get(serve, http.MethodGet, target); rec.Code != http.StatusOK {
			t.Errorf("a public file was refused with the private one: %s -> %d", target, rec.Code)
		}
	}
}
