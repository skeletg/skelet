package qna

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/db/sqlnull"
)

// bare returns a Service with nil queries, so any code path reaching the database panics - the recover in guarded turns that into a named failure
func bare() *Service { return NewService(nil) }

func guarded(t *testing.T, what string, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("%s reached the database instead of being refused (%v)", what, r)
		}
	}()
	fn()
}

func TestSubmitRefusesOutOfBoundsQuestions(t *testing.T) { // the PDP form is public, and validation is authoritative here rather than in the resolver
	ctx := context.Background()
	for name, q := range map[string]string{
		"empty":              "",
		"whitespace":         "         ",
		"too short":          "why?",
		"nine chars":         strings.Repeat("a", 9),
		"padding is trimmed": "   why?   ",
		"over the cap":       strings.Repeat("a", 1001),
	} {
		var err error
		guarded(t, name, func() { _, err = bare().Submit(ctx, uuid.New(), uuid.New(), nil, "A", "a@b.test", q) })
		if !errors.Is(err, ErrInvalidQuestion) {
			t.Errorf("%s: err = %v, want ErrInvalidQuestion", name, err)
		}
	}
}

func TestClampAuthorNameCutsOnARuneBoundary(t *testing.T) { // a mid-rune cut is invalid UTF-8 Postgres refuses, so a long enough name would fail the whole submit
	long := strings.Repeat("a", 119) + "\u00e9" // 121 bytes, the byte cap falls inside the two-byte rune

	got := clampAuthorName(long)
	if !utf8.ValidString(got) {
		t.Errorf("clamped name is invalid UTF-8: % x", got[115:])
	}
	if len(got) > 120 {
		t.Errorf("name is %d bytes, want at most 120", len(got))
	}
	if got != strings.Repeat("a", 119) {
		t.Errorf("name = %q, want the half-cut rune dropped entirely", got)
	}
}

func TestClampAuthorNamePassesShortNamesThrough(t *testing.T) {
	for in, want := range map[string]string{
		"Ana":                        "Ana",
		"  Ana  ":                    "Ana",
		"":                           "",
		"   ":                        "",
		strings.Repeat("b", 120):     strings.Repeat("b", 120),
		strings.Repeat("b", 121):     strings.Repeat("b", 120),
		strings.Repeat("\u00e9", 60): strings.Repeat("\u00e9", 60), // exactly 120 bytes of multibyte
	} {
		if got := clampAuthorName(in); got != want {
			t.Errorf("clampAuthorName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAnswerRefusesAnEmptyAnswer(t *testing.T) { // an empty answer is a reject, not a publish - letting it through would flip the question to published with nothing to show
	ctx := context.Background()
	for name, a := range map[string]string{"empty": "", "whitespace": "  \t\n  "} {
		var err error
		guarded(t, name, func() { _, err = bare().Answer(ctx, uuid.New(), uuid.New(), a) })
		if !errors.Is(err, ErrInvalidAnswer) {
			t.Errorf("%s: err = %v, want ErrInvalidAnswer", name, err)
		}
	}
}

func TestQnaEventShapes(t *testing.T) { // the bus routes on Type() and channel-scoped subscribers gate on EventChannelID()
	ch := uuid.New()
	sub := &QuestionSubmitted{ChannelID: ch, QuestionID: uuid.New()}
	if sub.Type() != "qna.question_submitted" {
		t.Errorf("QuestionSubmitted.Type() = %q", sub.Type())
	}
	ans := &QuestionAnswered{ChannelID: ch, QuestionID: uuid.New()}
	if ans.Type() != "qna.question_answered" {
		t.Errorf("QuestionAnswered.Type() = %q", ans.Type())
	}
	for _, e := range []interface{ EventChannelID() uuid.UUID }{sub, ans} {
		if e.EventChannelID() != ch {
			t.Errorf("EventChannelID() = %s, want %s", e.EventChannelID(), ch)
		}
	}
}

func TestPublishWithoutABusIsANoOp(t *testing.T) { // SetEventBus is optional, and Submit publishes unconditionally after the insert
	guarded(t, "publish", func() { bare().publish(context.Background(), &QuestionSubmitted{}) })
}

func TestToQuestionMapsTheNullables(t *testing.T) { // a pending question has no answer metadata, and a shared pointer bug here would attribute one admin's answer to every row
	now := time.Now()
	cust, admin := uuid.New(), uuid.New()

	full := toQuestion(db.ProductQuestion{
		ID: uuid.New(), ProductID: uuid.New(), ChannelID: uuid.New(),
		CustomerID: uuid.NullUUID{UUID: cust, Valid: true},
		AnsweredBy: uuid.NullUUID{UUID: admin, Valid: true},
		AnsweredAt: sql.NullTime{Time: now, Valid: true},
		AuthorName: "Ana", Question: "Does it fit a 2019 Golf?", Answer: "Yes.", Status: StatusPublished,
	})
	if full.CustomerID == nil || *full.CustomerID != cust {
		t.Errorf("CustomerID = %v, want %s", full.CustomerID, cust)
	}
	if full.AnsweredBy == nil || *full.AnsweredBy != admin {
		t.Errorf("AnsweredBy = %v, want %s", full.AnsweredBy, admin)
	}
	if full.AnsweredAt == nil || !full.AnsweredAt.Equal(now) {
		t.Errorf("AnsweredAt = %v, want %s", full.AnsweredAt, now)
	}

	pending := toQuestion(db.ProductQuestion{ID: uuid.New(), Status: StatusPending})
	if pending.CustomerID != nil || pending.AnsweredBy != nil || pending.AnsweredAt != nil {
		t.Errorf("a pending guest question = %+v, want every nullable nil", pending)
	}
}

func TestToQuestionsCopiesEveryRowDistinctly(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	got := toQuestions([]db.ProductQuestion{
		{ID: uuid.New(), CustomerID: uuid.NullUUID{UUID: a, Valid: true}},
		{ID: uuid.New(), CustomerID: uuid.NullUUID{UUID: b, Valid: true}},
	})
	if len(got) != 2 {
		t.Fatalf("%d questions, want 2", len(got))
	}
	if *got[0].CustomerID == *got[1].CustomerID {
		t.Error("two rows share a customer id, so the loop reused one pointer target")
	}
	if empty := toQuestions(nil); empty == nil || len(empty) != 0 {
		t.Errorf("toQuestions(nil) = %v, want an empty non-nil slice for the resolvers", empty)
	}
}

func TestNullableUUID(t *testing.T) {
	if got := sqlnull.UUID(nil); got.Valid {
		t.Errorf("nullableUUID(nil) = %+v, want invalid", got)
	}
	id := uuid.New()
	got := sqlnull.UUID(&id)
	if !got.Valid || got.UUID != id {
		t.Errorf("nullableUUID(&id) = %+v, want the id marked valid", got)
	}
}

type argSpy struct {
	db.DBTX
	args []any
}

func (s *argSpy) QueryContext(_ context.Context, _ string, args ...any) (*sql.Rows, error) {
	s.args = args
	return nil, errors.New("spy")
}

func TestPublishedQuestionsAreReadForOneChannel(t *testing.T) {
	spy := &argSpy{}
	product, store := uuid.New(), uuid.New()
	_, _ = NewService(db.New(spy)).Published(context.Background(), product, store, 5)
	if len(spy.args) < 2 || spy.args[0] != product || spy.args[1] != store {
		t.Fatalf("query args = %v, want the product then the store", spy.args)
	}
}
