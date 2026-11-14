package wiring

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func TestTheAdminFileRoutesAskForASessionFirst(t *testing.T) {
	id := uuid.NewString()
	for name, tc := range map[string]struct {
		h              http.Handler
		method, target string
	}{
		"a library file":     {mediaFileHandler(nil, nil), http.MethodGet, mediaFilePath + id},
		"an SVG upload":      {mediaSVGHandler(nil, nil), http.MethodPost, mediaSVGPath},
		"an export":          {dataDownloadHandler(nil, nil), http.MethodGet, "/api/admin/data/jobs/" + id + "/file"},
		"an import":          {dataImportHandler(nil, nil), http.MethodPost, "/api/admin/data/import"},
		"an import template": {dataTemplateHandler(nil, nil), http.MethodGet, "/api/admin/data/template?resource=products"},
	} {
		rec := httptest.NewRecorder()
		tc.h.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.target, strings.NewReader("")))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s with no session: %d, want 401", name, rec.Code)
		}
	}
}

func TestARefusalSaysWhetherASessionWouldHelp(t *testing.T) {
	for err, want := range map[error]int{
		auth.ErrUnauthenticated:                         http.StatusUnauthorized,
		&auth.ScopedDenial{Slug: "media.private.view"}:  http.StatusForbidden,
		auth.ErrForbidden:                               http.StatusForbidden,
		errors.New("check permission: pq: conn closed"): http.StatusBadGateway,
	} {
		rec := httptest.NewRecorder()
		refuse(rec, err)
		if rec.Code != want {
			t.Errorf("%v: %d, want %d", err, rec.Code, want)
		}
	}
}
