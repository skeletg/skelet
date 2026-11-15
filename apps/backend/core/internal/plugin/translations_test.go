package plugin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/skeletg/skelet/apps/backend/sdk"
)

func resetTranslations(t *testing.T) *Host {
	t.Helper()
	clear := func() {
		adminTranslationsRegistry.mu.Lock()
		adminTranslationsRegistry.byLocale = map[string]adminTranslation{}
		adminTranslationsRegistry.mu.Unlock()
	}
	clear()
	t.Cleanup(clear)
	return &Host{}
}

func plRegistration() sdk.AdminTranslationsRegistration {
	return sdk.AdminTranslationsRegistration{
		Locale:      "pl",
		DisplayName: "Polski",
		Messages:    json.RawMessage(`{"vXIe7J":"Język"}`),
	}
}

func callTranslations(t *testing.T, method, path string, header http.Header) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if header != nil {
		req.Header = header
	}
	rec := httptest.NewRecorder()
	AdminTranslationsHandler().ServeHTTP(rec, req)
	return rec
}

func TestAdminTranslationsDuplicateLocaleRefused(t *testing.T) {
	h := resetTranslations(t)
	if err := h.RegisterAdminTranslations("lang-pl", plRegistration()); err != nil {
		t.Fatalf("first registration: %v", err)
	}
	err := h.RegisterAdminTranslations("other-pack", plRegistration())
	if err == nil {
		t.Fatal("second pack claiming the same locale must be refused")
	}
	// The error names the first claimant, since that's what the operator has to uninstall
	if !strings.Contains(err.Error(), "lang-pl") {
		t.Fatalf("error should name the earlier pack, got: %v", err)
	}
}

func TestAdminTranslationsLocalesList(t *testing.T) {
	h := resetTranslations(t)
	if err := h.RegisterAdminTranslations("lang-pl", plRegistration()); err != nil {
		t.Fatalf("register: %v", err)
	}
	rec := callTranslations(t, http.MethodGet, "/api/admin-ui/i18n/locales", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("locales: got %d", rec.Code)
	}
	var body struct {
		Locales []struct {
			Locale, Name, Direction, Plugin string
		}
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Locales) != 1 {
		t.Fatalf("want 1 locale, got %d", len(body.Locales))
	}
	l := body.Locales[0]
	if l.Locale != "pl" || l.Name != "Polski" || l.Plugin != "lang-pl" {
		t.Fatalf("unexpected entry: %+v", l)
	}
	// Empty Direction defaults at registration, not per request - the dashboard applies it verbatim
	if l.Direction != "ltr" {
		t.Fatalf("direction should default to ltr, got %q", l.Direction)
	}
}

func TestAdminTranslationsCatalogETag(t *testing.T) {
	h := resetTranslations(t)
	if err := h.RegisterAdminTranslations("lang-pl", plRegistration()); err != nil {
		t.Fatalf("register: %v", err)
	}
	rec := callTranslations(t, http.MethodGet, "/api/admin-ui/i18n/catalog/pl", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("catalog: got %d", rec.Code)
	}
	var msgs map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &msgs); err != nil {
		t.Fatalf("catalog body isn't the raw messages object: %v", err)
	}
	if msgs["vXIe7J"] != "Język" {
		t.Fatalf("unexpected catalog: %v", msgs)
	}
	etag := rec.Header().Get("ETag")
	if etag == "" {
		t.Fatal("catalog response must carry an ETag")
	}
	hdr := http.Header{}
	hdr.Set("If-None-Match", etag)
	rec = callTranslations(t, http.MethodGet, "/api/admin-ui/i18n/catalog/pl", hdr)
	if rec.Code != http.StatusNotModified {
		t.Fatalf("matching If-None-Match should 304, got %d", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Fatal("304 must not carry a body")
	}
}

func TestAdminTranslationsUnknownPathsAndMethods(t *testing.T) {
	h := resetTranslations(t)
	if err := h.RegisterAdminTranslations("lang-pl", plRegistration()); err != nil {
		t.Fatalf("register: %v", err)
	}
	if rec := callTranslations(t, http.MethodGet, "/api/admin-ui/i18n/catalog/de", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown locale: want 404, got %d", rec.Code)
	}
	if rec := callTranslations(t, http.MethodGet, "/api/admin-ui/i18n/nope", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown path: want 404, got %d", rec.Code)
	}
	if rec := callTranslations(t, http.MethodPost, "/api/admin-ui/i18n/locales", nil); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST: want 405, got %d", rec.Code)
	}
}
