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

	"github.com/skeletg/skelet/apps/backend/core/internal/webform"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

type formFiles struct {
	sub    webform.Submission
	form   webform.Form
	opened []string
}

func (f *formFiles) GetSubmission(_ context.Context, id uuid.UUID) (webform.Submission, error) {
	if id != f.sub.ID {
		return webform.Submission{}, webform.ErrSubmissionNotFound
	}
	return f.sub, nil
}
func (f *formFiles) Get(context.Context, uuid.UUID) (webform.Form, error) { return f.form, nil }
func (f *formFiles) OpenSubmissionFile(_ context.Context, _ webform.Submission, fieldKey string) (io.ReadCloser, webform.SubmissionFile, error) {
	f.opened = append(f.opened, fieldKey)
	if fieldKey != "cv" {
		return nil, webform.SubmissionFile{}, webform.ErrFileNotFound
	}
	return io.NopCloser(strings.NewReader("<script>alert(1)</script>")), webform.SubmissionFile{Name: "my \"cv\".html", MimeType: "text/html"}, nil
}

func formFileRig() (*formFiles, string) {
	files := &formFiles{sub: webform.Submission{ID: uuid.New(), FormID: uuid.New()}, form: webform.Form{ChannelID: uuid.New()}}
	return files, webformFilePath + files.sub.ID.String() + "/files/"
}

func TestAShoppersUploadIsOnlyEverADownload(t *testing.T) {
	files, base := formFileRig()
	var asked uuid.UUID
	serve := serveWebformFile(files, func(_ *http.Request, channelID uuid.UUID) error {
		asked = channelID
		return nil
	})
	rec := httptest.NewRecorder()
	serve(rec, httptest.NewRequest(http.MethodGet, base+"cv", nil))

	if rec.Code != http.StatusOK || rec.Body.String() != "<script>alert(1)</script>" {
		t.Fatalf("got %d %q", rec.Code, rec.Body.String())
	}
	if asked != files.form.ChannelID {
		t.Errorf("the grant was checked for channel %s, want the form's own %s", asked, files.form.ChannelID)
	}
	for header, want := range map[string]string{
		"X-Content-Type-Options":  "nosniff",
		"Content-Security-Policy": "default-src 'none'; sandbox",
		"Cache-Control":           "private, no-store",
	} {
		if got := rec.Header().Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.HasPrefix(cd, "attachment") || strings.Contains(cd, `"cv"`) {
		t.Errorf("Content-Disposition = %q, want an attachment under a name made safe", cd)
	}
}

func TestTheFormFileRouteGivesNothingAwayToTheWrongCaller(t *testing.T) {
	files, base := formFileRig()
	yes := func(*http.Request, uuid.UUID) error { return nil }
	no := func(*http.Request, uuid.UUID) error { return &auth.ScopedDenial{Slug: "webforms.submissions_view"} }
	for name, tc := range map[string]struct {
		method, path string
		allowed      func(*http.Request, uuid.UUID) error
		want         int
	}{
		"no grant on the form's channel":  {http.MethodGet, base + "cv", no, http.StatusForbidden},
		"a submission that doesn't exist": {http.MethodGet, webformFilePath + uuid.NewString() + "/files/cv", yes, http.StatusForbidden},
		"a field with no file":            {http.MethodGet, base + "photo", yes, http.StatusNotFound},
		"not an id":                       {http.MethodGet, webformFilePath + "x/files/cv", yes, http.StatusNotFound},
		"no field named":                  {http.MethodGet, base, yes, http.StatusNotFound},
		"a path under the field":          {http.MethodGet, base + "cv/../../x", yes, http.StatusNotFound},
		"a write":                         {http.MethodPost, base + "cv", yes, http.StatusMethodNotAllowed},
	} {
		before := len(files.opened)
		rec := httptest.NewRecorder()
		serveWebformFile(files, tc.allowed)(rec, httptest.NewRequest(tc.method, tc.path, nil))
		if rec.Code != tc.want {
			t.Errorf("%s: %d, want %d", name, rec.Code, tc.want)
		}
		if strings.Contains(rec.Body.String(), "script") {
			t.Errorf("%s: the file's bytes were sent", name)
		}
		if tc.want == http.StatusForbidden && len(files.opened) != before {
			t.Errorf("%s: the file was opened before the caller was cleared", name)
		}
	}
}

func TestTheFormFileRouteSaysWhenItCouldNotLook(t *testing.T) {
	files, base := formFileRig()
	yes := func(*http.Request, uuid.UUID) error { return nil }
	broken := &brokenFormFiles{inner: files}
	for name, fail := range map[string]func(error){
		"the submission lookup": func(err error) { broken.subErr = err },
		"the form lookup":       func(err error) { broken.formErr = err },
		"the store":             func(err error) { broken.openErr = err },
	} {
		fail(errors.New("pq: connection refused"))
		rec := httptest.NewRecorder()
		serveWebformFile(broken, yes)(rec, httptest.NewRequest(http.MethodGet, base+"cv", nil))
		if rec.Code != http.StatusBadGateway || strings.Contains(rec.Body.String(), "pq:") {
			t.Errorf("%s failing: %d %q, want a 502 that keeps the error to itself", name, rec.Code, rec.Body.String())
		}
		fail(nil)
	}
}

type brokenFormFiles struct {
	inner                    *formFiles
	subErr, formErr, openErr error
}

func (b *brokenFormFiles) GetSubmission(ctx context.Context, id uuid.UUID) (webform.Submission, error) {
	if b.subErr != nil {
		return webform.Submission{}, b.subErr
	}
	return b.inner.GetSubmission(ctx, id)
}
func (b *brokenFormFiles) Get(ctx context.Context, id uuid.UUID) (webform.Form, error) {
	if b.formErr != nil {
		return webform.Form{}, b.formErr
	}
	return b.inner.Get(ctx, id)
}
func (b *brokenFormFiles) OpenSubmissionFile(ctx context.Context, sub webform.Submission, key string) (io.ReadCloser, webform.SubmissionFile, error) {
	if b.openErr != nil {
		return nil, webform.SubmissionFile{}, b.openErr
	}
	return b.inner.OpenSubmissionFile(ctx, sub, key)
}
