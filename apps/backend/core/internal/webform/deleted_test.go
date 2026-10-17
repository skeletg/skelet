package webform

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

type formsOn struct{}

func (formsOn) GetBool(context.Context, string, config.Scope) (bool, error)     { return true, nil }
func (formsOn) GetString(context.Context, string, config.Scope) (string, error) { return "", nil }

type recordingForms struct {
	Repository
	form Form
	subs []Submission
}

func (r *recordingForms) Get(context.Context, uuid.UUID) (Form, error) { return r.form, nil }
func (r *recordingForms) RecordSubmission(_ context.Context, s Submission) (Submission, error) {
	r.subs = append(r.subs, s)
	return s, nil
}

func newSubmitFixture(fields []Field) (*Service, *recordingForms) {
	repo := &recordingForms{form: Form{ID: uuid.New(), ChannelID: uuid.New(), Status: StatusActive, Fields: fields}}
	return New(repo, formsOn{}), repo
}

// A soft-deleted form keeps its active status, so the deletion alone has to take it off the storefront
func TestADeletedFormIsGoneFromTheStorefront(t *testing.T) {
	svc, repo := newSubmitFixture([]Field{{Key: "name", Type: FieldText}, {Key: "cv", Type: FieldFile}})
	gone := time.Now()
	repo.form.DeletedAt = &gone
	svc.SetDeps(nil, &presigner{})
	ctx := context.Background()
	if _, err := svc.Submit(ctx, SubmitInput{FormID: repo.form.ID, ChannelID: repo.form.ChannelID, Values: map[string]any{"name": "Ana"}}); !errors.Is(err, ErrFormNotFound) {
		t.Errorf("Submit = %v, want ErrFormNotFound", err)
	}
	if _, err := svc.PublicForm(ctx, repo.form.ID); !errors.Is(err, ErrFormNotFound) {
		t.Errorf("PublicForm = %v, want ErrFormNotFound", err)
	}
	if _, _, err := svc.PresignUpload(ctx, repo.form.ID, repo.form.ChannelID, "cv", "cv.pdf", "application/pdf", 10); !errors.Is(err, ErrFormNotFound) {
		t.Errorf("PresignUpload = %v, want ErrFormNotFound", err)
	}
	if len(repo.subs) != 0 {
		t.Fatal("a deleted form stored a submission")
	}
}

func TestTheConfirmationGoesOnlyToOnePlainAddress(t *testing.T) {
	form := Form{Fields: []Field{{Key: "to", Type: FieldText}, {Key: "email", Type: FieldEmail}}}
	form.Settings.Confirmation.ToFieldKey = "to"
	for _, bad := range []string{"a@b.test, victim@x.test", "Ana <a@b.test>", "not an address"} {
		if got := submitterEmail(form, map[string]any{"to": bad}); got != "" {
			t.Errorf("submitterEmail(%q) = %q, want none", bad, got)
		}
	}
	if got := submitterEmail(form, map[string]any{"to": " ana@b.test "}); got != "ana@b.test" {
		t.Errorf("a plain address = %q", got)
	}
}

func TestSubmittedAnswersKeepOnlyTheFormsShape(t *testing.T) {
	svc, repo := newSubmitFixture([]Field{
		{Key: "name", Type: FieldText},
		{Key: "kids", Type: FieldRepeater, SubFields: []Field{{Key: "age", Type: FieldNumber}}},
		{Key: "grid", Type: FieldMatrix, Rows: []MatrixRow{{Key: "q1"}}, Columns: []FieldOption{{Value: "yes"}}},
	})
	_, err := svc.Submit(context.Background(), SubmitInput{FormID: repo.form.ID, ChannelID: repo.form.ChannelID, Values: map[string]any{
		"name":  map[string]any{"nested": "blob"},
		"kids":  []any{map[string]any{"age": 4.0, "junk": strings.Repeat("x", 100)}},
		"grid":  map[string]any{"q1": "yes", "extra": "row"},
		"ghost": "not a field",
	}})
	if err != nil {
		t.Fatal(err)
	}
	got := repo.subs[0].Answers
	if _, ok := got["name"]; ok {
		t.Errorf("a non-scalar text answer was stored: %v", got["name"])
	}
	if item := got["kids"].([]any)[0].(map[string]any); len(item) != 1 || item["age"] != 4.0 {
		t.Errorf("repeater item = %v, want only age", item)
	}
	if grid := got["grid"].(map[string]any); len(grid) != 1 {
		t.Errorf("matrix = %v, want only q1", grid)
	}
	if _, ok := got["ghost"]; ok {
		t.Error("an unknown key was stored")
	}
}

func TestUnboundedAnswersAreCapped(t *testing.T) {
	svc, repo := newSubmitFixture([]Field{{Key: "note", Type: FieldTextarea}, {Key: "kids", Type: FieldRepeater, SubFields: []Field{{Key: "n", Type: FieldText}}}})
	items := make([]any, defaultMaxRepeatItems+1)
	for i := range items {
		items[i] = map[string]any{"n": "x"}
	}
	_, err := svc.Submit(context.Background(), SubmitInput{FormID: repo.form.ID, ChannelID: repo.form.ChannelID, Values: map[string]any{
		"note": strings.Repeat("a", defaultMaxAnswerRunes+1), "kids": items,
	}})
	var se *SubmissionError
	if !errors.As(err, &se) || len(se.Fields) != 2 {
		t.Fatalf("err = %v, want two field errors", err)
	}
}
