package webform

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/objstore"
)

type oneForm struct {
	Repository
	form Form
}

func (o oneForm) Get(context.Context, uuid.UUID) (Form, error) { return o.form, nil }

type presigner struct {
	issued int
	stored map[string]string
	opened []string
	down   string
}

func (p *presigner) PresignUpload(context.Context, string, string, int64, time.Duration) (string, error) {
	p.issued++
	return "https://upload.test", nil
}
func (p *presigner) Head(context.Context, string) (int64, string, error) { return 0, "", nil }
func (p *presigner) Open(_ context.Context, key string) (io.ReadCloser, error) {
	p.opened = append(p.opened, key)
	body, ok := p.stored[key]
	if !ok {
		if key == p.down {
			return nil, errors.New("dial tcp: refused")
		}
		return nil, fmt.Errorf("%w: %s", objstore.ErrObjectNotFound, key)
	}
	return io.NopCloser(strings.NewReader(body)), nil
}
func (p *presigner) MaxUploadBytes() int64 { return 1 << 20 }

func TestAnUploadTicketIsOnlyIssuedForTheStoresOwnForm(t *testing.T) {
	home, other := uuid.New(), uuid.New()
	form := Form{ID: uuid.New(), ChannelID: home, Status: StatusActive, Fields: []Field{{Key: "cv", Type: FieldFile}}}
	files := &presigner{}
	svc := New(oneForm{form: form}, nil)
	svc.SetDeps(nil, files)
	if _, _, err := svc.PresignUpload(context.Background(), form.ID, other, "cv", "cv.pdf", "application/pdf", 10); !errors.Is(err, ErrFormNotFound) {
		t.Fatalf("another store's form issued a ticket: %v", err)
	}
	if files.issued != 0 {
		t.Fatal("a presigned URL was minted for another store's form")
	}
	if _, _, err := svc.PresignUpload(context.Background(), form.ID, home, "cv", "cv.pdf", "application/pdf", 10); err != nil {
		t.Fatalf("own form: %v", err)
	}
}

func TestASubmissionOpensOnlyItsOwnFormsUploads(t *testing.T) {
	formID := uuid.New()
	mine := uploadPrefix(formID, "cv") + "u1-resume.pdf"
	files := &presigner{stored: map[string]string{mine: "%PDF", "data/exports/j1/customers.csv": "pii", uploadPrefix(uuid.New(), "cv") + "theirs.pdf": "other form"}}
	svc := New(&recordingForms{}, formsOn{})
	svc.SetDeps(nil, files)
	ctx := context.Background()

	body, file, err := svc.OpenSubmissionFile(ctx, Submission{FormID: formID, Files: []SubmissionFile{{FieldKey: "cv", ObjectKey: mine, Name: "Resume.pdf", MimeType: "application/pdf"}}}, "cv")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(body)
	if string(raw) != "%PDF" || file.Name != "Resume.pdf" {
		t.Fatalf("opened %q as %q", raw, file.Name)
	}

	for name, sub := range map[string]Submission{
		"an export":               {FormID: formID, Files: []SubmissionFile{{FieldKey: "cv", ObjectKey: "data/exports/j1/customers.csv"}}},
		"another form's upload":   {FormID: formID, Files: []SubmissionFile{{FieldKey: "cv", ObjectKey: uploadPrefix(uuid.New(), "cv") + "theirs.pdf"}}},
		"another field's upload":  {FormID: formID, Files: []SubmissionFile{{FieldKey: "photo", ObjectKey: mine}}},
		"a field with no file":    {FormID: formID},
		"a file storage has lost": {FormID: formID, Files: []SubmissionFile{{FieldKey: "cv", ObjectKey: uploadPrefix(formID, "cv") + "gone.pdf"}}},
	} {
		if _, _, err := svc.OpenSubmissionFile(ctx, sub, "cv"); !errors.Is(err, ErrFileNotFound) {
			t.Errorf("%s: %v, want ErrFileNotFound", name, err)
		}
	}
	if want := []string{mine, uploadPrefix(formID, "cv") + "gone.pdf"}; strings.Join(files.opened, " ") != strings.Join(want, " ") {
		t.Errorf("storage was asked for %v, want only this form's own keys %v", files.opened, want)
	}

	files.down = uploadPrefix(formID, "cv") + "unreachable.pdf"
	down := Submission{FormID: formID, Files: []SubmissionFile{{FieldKey: "cv", ObjectKey: files.down}}}
	if _, _, err := svc.OpenSubmissionFile(ctx, down, "cv"); err == nil || errors.Is(err, ErrFileNotFound) {
		t.Errorf("storage down: %v, want an error that isn't ErrFileNotFound", err)
	}
}
