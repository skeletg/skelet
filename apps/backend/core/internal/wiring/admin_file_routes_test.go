package wiring

import (
	"context"
	"errors"
	"mime/multipart"
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
		"a library file":     {mediaFileHandler(nil, nil, nil, nil), http.MethodGet, mediaFilePath + id},
		"a smaller copy":     {mediaFileHandler(nil, nil, nil, nil), http.MethodGet, mediaFilePath + id + "?w=640"},
		"an SVG upload":      {mediaSVGHandler(nil, nil), http.MethodPost, mediaSVGPath},
		"a form upload":      {webformFileHandler(nil, nil), http.MethodGet, webformFilePath + id + "/files/cv"},
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

func TestTheSVGRouteRefusesAHalfItDoesNotKnow(t *testing.T) {
	for _, half := range []string{"PRIVATE", "secret", " private"} {
		body := &strings.Builder{}
		form := multipart.NewWriter(body)
		part, _ := form.CreateFormFile("file", "seal.svg")
		_, _ = part.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`))
		_ = form.WriteField("visibility", half)
		_ = form.Close()
		req := httptest.NewRequest(http.MethodPost, mediaSVGPath, strings.NewReader(body.String()))
		req.Header.Set("Content-Type", form.FormDataContentType())
		rec := httptest.NewRecorder()
		mediaSVGHandler(nil, grantsAll{}).ServeHTTP(rec, req.WithContext(auth.WithAdmin(req.Context(), &auth.Claims{AdminID: uuid.New()})))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("visibility %q: %d, want 400", half, rec.Code)
		}
	}
}

type grantsAll struct{}

func (grantsAll) HasPermission(context.Context, uuid.UUID, string) (bool, error) { return true, nil }
func (grantsAll) HasGlobalPermission(context.Context, uuid.UUID, string) (bool, error) {
	return true, nil
}
func (grantsAll) HasChannelPermission(context.Context, uuid.UUID, string, uuid.UUID, *string) (bool, error) {
	return true, nil
}
