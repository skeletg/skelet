package wiring

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/medialibrary"
)

func TestTheEditorIsHandedAPictureAsAnInertDownload(t *testing.T) {
	id := uuid.New()
	serve := serveMediaFile(func(_ *http.Request, got uuid.UUID) (io.ReadCloser, medialibrary.Asset, error) {
		if got != id {
			t.Errorf("opened %s, want %s", got, id)
		}
		return io.NopCloser(strings.NewReader("pixels")), medialibrary.Asset{MimeType: "image/png", Filename: "Sale \"banner\".png"}, nil
	})
	rec := httptest.NewRecorder()
	serve(rec, httptest.NewRequest(http.MethodGet, mediaFilePath+id.String(), nil))

	if rec.Code != http.StatusOK || rec.Body.String() != "pixels" {
		t.Fatalf("got %d %q, want the file's bytes", rec.Code, rec.Body.String())
	}
	for header, want := range map[string]string{
		"Content-Type":            "image/png",
		"X-Content-Type-Options":  "nosniff",
		"Content-Security-Policy": "default-src 'none'; sandbox",
		"Cache-Control":           "private, no-store",
	} {
		if got := rec.Header().Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.HasPrefix(cd, "attachment") || strings.Contains(cd, `"banner"`) {
		t.Errorf("Content-Disposition = %q, want an attachment with the name made safe", cd)
	}
}

func TestTheFileRouteRefusesWhatItCannotServe(t *testing.T) {
	id := uuid.New()
	opens := 0
	failing := func(err error) mediaFileOpener {
		return func(*http.Request, uuid.UUID) (io.ReadCloser, medialibrary.Asset, error) {
			opens++
			return nil, medialibrary.Asset{}, err
		}
	}
	for name, tc := range map[string]struct {
		method, path string
		open         mediaFileOpener
		want         int
	}{
		"a write":             {http.MethodPost, mediaFilePath + id.String(), failing(nil), http.StatusMethodNotAllowed},
		"not an id":           {http.MethodGet, mediaFilePath + "../../etc/passwd", failing(nil), http.StatusBadRequest},
		"no such asset":       {http.MethodGet, mediaFilePath + id.String(), failing(medialibrary.ErrAssetNotFound), http.StatusNotFound},
		"a document or svg":   {http.MethodGet, mediaFilePath + id.String(), failing(medialibrary.ErrNotEditable), http.StatusUnsupportedMediaType},
		"storage unreachable": {http.MethodGet, mediaFilePath + id.String(), failing(errors.New("dial tcp: refused")), http.StatusBadGateway},
	} {
		rec := httptest.NewRecorder()
		serveMediaFile(tc.open)(rec, httptest.NewRequest(tc.method, tc.path, nil))
		if rec.Code != tc.want {
			t.Errorf("%s: %d, want %d", name, rec.Code, tc.want)
		}
		if strings.Contains(rec.Body.String(), "dial tcp") {
			t.Errorf("%s: the storage error reached the caller: %q", name, rec.Body.String())
		}
	}
	if opens != 3 {
		t.Errorf("the file was opened %d times, want only for the three requests that named an asset", opens)
	}
}
