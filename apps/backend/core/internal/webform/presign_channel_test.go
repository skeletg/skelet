package webform

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

type oneForm struct {
	Repository
	form Form
}

func (o oneForm) Get(context.Context, uuid.UUID) (Form, error) { return o.form, nil }

type presigner struct{ issued int }

func (p *presigner) PresignUpload(context.Context, string, string, int64, time.Duration) (string, error) {
	p.issued++
	return "https://upload.test", nil
}
func (p *presigner) Head(context.Context, string) (int64, string, error) { return 0, "", nil }
func (p *presigner) MaxUploadBytes() int64                               { return 1 << 20 }

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
