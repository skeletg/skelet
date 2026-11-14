package wiring

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func TestADownloadTicketIsOnlyMadeForAFileRoute(t *testing.T) {
	keys, _ := auth.NewAdminKeys("download-ticket-admin-secret-0123456789")
	admin := auth.WithAdmin(t.Context(), &auth.Claims{AdminID: uuid.New(), SessionID: uuid.New()})
	ask := func(ctx_ bool, method, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, downloadTicketPath, strings.NewReader(body))
		if ctx_ {
			req = req.WithContext(admin)
		}
		rec := httptest.NewRecorder()
		downloadTicketHandler(keys)(rec, req)
		return rec
	}

	file := mediaFilePath + uuid.NewString()
	rec := ask(true, http.MethodPost, `{"path": "`+file+`?w=640"}`)
	var got struct{ URL string }
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &got) != nil {
		t.Fatalf("a library file: %d %s", rec.Code, rec.Body.String())
	}
	u, _ := url.Parse(got.URL)
	if u.Path != file || u.Query().Get("w") != "640" || u.Query().Get("ticket") == "" {
		t.Errorf("the link %q, want the same path and query with a ticket on it", got.URL)
	}
	for _, p := range []string{webformFilePath + uuid.NewString() + "/files/cv", dataJobsPath + uuid.NewString() + "/download", dataTemplatePath + "?resource=products&format=csv"} {
		if rec := ask(true, http.MethodPost, `{"path": "`+p+`"}`); rec.Code != http.StatusOK {
			t.Errorf("%s: %d, want a link", p, rec.Code)
		}
	}

	for name, p := range map[string]string{
		"the admin GraphQL endpoint": "/api/admin/admin-x/graphql",
		"the SVG upload":             mediaSVGPath,
		"the ticket route itself":    downloadTicketPath,
		"the file route's bare root": mediaFilePath,
		"a way out of the route":     mediaFilePath + "../graphql",
		"another host":               "https://evil.test" + mediaFilePath + uuid.NewString(),
	} {
		if rec := ask(true, http.MethodPost, `{"path": "`+p+`"}`); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", name, rec.Code)
		}
	}
	if rec := ask(false, http.MethodPost, `{"path": "`+file+`"}`); rec.Code != http.StatusUnauthorized {
		t.Errorf("no session: %d, want 401", rec.Code)
	}
	token := auth.WithApiToken(t.Context(), &auth.ApiTokenClaims{})
	req := httptest.NewRequest(http.MethodPost, downloadTicketPath, strings.NewReader(`{"path": "`+file+`"}`)).WithContext(token)
	rec = httptest.NewRecorder()
	downloadTicketHandler(keys)(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("an integration token: %d, want 403 - it has no browser to hand a link to", rec.Code)
	}
	if rec := ask(true, http.MethodGet, ""); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("a GET: %d, want 405", rec.Code)
	}
}
