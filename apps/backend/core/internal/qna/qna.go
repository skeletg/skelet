// Package qna is product questions & answers: shoppers ask on the PDP, operators answer from the dashboard - Moderation mirrors reviews -
// questions sit pending until answered (which publishes) or rejected; only published rows reach the storefront
package qna

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/db/sqlnull"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/permission"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

const (
	minQuestionRunes = 10
	maxQuestionRunes = 1000
	maxAnswerRunes   = 5000
)

const (
	StatusPending   = "pending"
	StatusPublished = "published"
	StatusRejected  = "rejected"
)

var (
	// ErrInvalidQuestion rejects empty or oversized questions
	ErrInvalidQuestion = apperr.New("QNA_INVALID_QUESTION", apperr.ClassValidation, "question must be between 10 and 1000 characters")
	// ErrInvalidAnswer rejects empty answers (an empty answer is a reject, not a publish) and oversized ones
	ErrInvalidAnswer = apperr.New("QNA_INVALID_ANSWER", apperr.ClassValidation, "answer must be between 1 and 5000 characters")
	// ErrNotFound is the generic missing-row error
	ErrNotFound = apperr.New("QNA_NOT_FOUND", apperr.ClassNotFound, "question not found")
)

func init() {
	permission.RegisterGroup(permission.Group{
		Slug:      "qna",
		Name:      "Product Q&A",
		SortOrder: 76,
	})
	permission.Register(permission.Definition{
		Group: "qna", Slug: "qna.view", Name: "View Product Questions", SortOrder: 1,
	})
	permission.Register(permission.Definition{
		Group: "qna", Slug: "qna.moderate", Name: "Answer / Reject Product Questions", SortOrder: 2,
	})
}

// Question is one shopper question with its (optional) staff answer
type Question struct {
	ID                    uuid.UUID
	ProductID             uuid.UUID
	ChannelID             uuid.UUID
	CustomerID            *uuid.UUID
	AuthorName            string
	AuthorEmail           string
	Question              string
	Answer                string
	AnsweredBy            *uuid.UUID
	AnsweredByIntegration *uuid.UUID
	AnsweredAt            *time.Time
	Status                string
	CreatedAt             time.Time
}

const (
	EventQuestionSubmitted = "qna.question_submitted"
	EventQuestionAnswered  = "qna.question_answered"
)

// QuestionSubmitted fires after a shopper submits a question (pending moderation) - Async, channel-scoped
type QuestionSubmitted struct {
	QuestionID uuid.UUID
	ProductID  uuid.UUID
	ChannelID  uuid.UUID
	CustomerID *uuid.UUID
	AuthorName string
	Question   string
}

func (*QuestionSubmitted) Type() string                { return EventQuestionSubmitted }
func (e *QuestionSubmitted) EventChannelID() uuid.UUID { return e.ChannelID }

// QuestionAnswered fires after an operator answers a question (which publishes
// it to the storefront) - Async, channel-scoped
type QuestionAnswered struct {
	QuestionID            uuid.UUID
	ProductID             uuid.UUID
	ChannelID             uuid.UUID
	AnsweredBy            uuid.UUID
	AnsweredByIntegration uuid.UUID
	Question              string
	Answer                string
}

func (*QuestionAnswered) Type() string                { return EventQuestionAnswered }
func (e *QuestionAnswered) EventChannelID() uuid.UUID { return e.ChannelID }

// OutboxEnqueuer writes an event through the caller's tx, satisfied by *outbox.Outbox
type OutboxEnqueuer interface {
	Enqueue(ctx context.Context, sqlTx *sql.Tx, e events.Event) error
}

// OutboxTypes is the registry half of the outbox
type OutboxTypes interface {
	RegisterType(eventType string, fn func() events.Event)
}

// Both events feed webhooks, so a crash between the write and the publish must not lose them
func RegisterOutboxTypes(r OutboxTypes) {
	r.RegisterType(EventQuestionSubmitted, func() events.Event { return &QuestionSubmitted{} })
	r.RegisterType(EventQuestionAnswered, func() events.Event { return &QuestionAnswered{} })
}

type Service struct {
	queries *db.Queries
	bus     *events.Bus
	outbox  OutboxEnqueuer
	tx      *txrunner.Manager
}

func NewService(queries *db.Queries) *Service { return &Service{queries: queries} }

// SetEventBus is optional and nil-safe
func (s *Service) SetEventBus(b *events.Bus) { s.bus = b }

// SetOutbox commits each event with the row it describes, relayed at least once
func (s *Service) SetOutbox(o OutboxEnqueuer, txm *txrunner.Manager) { s.outbox, s.tx = o, txm }

// write runs fn and its event in one transaction where the outbox is wired, else publishes in-process after it
func (s *Service) write(ctx context.Context, fn func(q *db.Queries) (events.Event, error)) error {
	if s.outbox == nil || s.tx == nil {
		e, err := fn(s.queries)
		if err == nil && e != nil && s.bus != nil {
			s.bus.Publish(ctx, e)
		}
		return err
	}
	return s.tx.InTransaction(ctx, func(sqlTx *sql.Tx) error {
		e, err := fn(s.queries.WithTx(sqlTx))
		if err != nil || e == nil {
			return err
		}
		return s.outbox.Enqueue(ctx, sqlTx, e)
	})
}

func notFound(err error, op string) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return fmt.Errorf("%s: %w", op, err)
}

// Submit records a new pending question - rate limiting and channel resolution
// are the resolver's job, validation is authoritative here
func (s *Service) Submit(ctx context.Context, productID, channelID uuid.UUID, customerID *uuid.UUID, authorName, authorEmail, question string) (Question, error) {
	question = strings.TrimSpace(question)
	if n := utf8.RuneCountInString(question); n < minQuestionRunes || n > maxQuestionRunes {
		return Question{}, ErrInvalidQuestion
	}
	authorName = clampAuthorName(authorName)
	var q Question
	err := s.write(ctx, func(dbq *db.Queries) (events.Event, error) {
		row, err := dbq.InsertProductQuestion(ctx, db.InsertProductQuestionParams{
			ProductID:   productID,
			ChannelID:   channelID,
			CustomerID:  sqlnull.UUID(customerID),
			AuthorName:  authorName,
			AuthorEmail: strings.TrimSpace(strings.ToLower(authorEmail)),
			Question:    question,
		})
		if err != nil {
			return nil, fmt.Errorf("insert question: %w", err)
		}
		q = toQuestion(row)
		return &QuestionSubmitted{
			QuestionID: q.ID,
			ProductID:  q.ProductID,
			ChannelID:  q.ChannelID,
			CustomerID: q.CustomerID,
			AuthorName: q.AuthorName,
			Question:   q.Question,
		}, nil
	})
	if err != nil {
		return Question{}, err
	}
	return q, nil
}

func (s *Service) Published(ctx context.Context, productID, channelID uuid.UUID, limit int32) ([]Question, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.queries.ListPublishedQuestions(ctx, db.ListPublishedQuestionsParams{
		ProductID: productID,
		ChannelID: channelID,
		Limit:     limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list published questions: %w", err)
	}
	return toQuestions(rows), nil
}

// ListForModeration returns the admin queue, optionally filtered by status
// channelIDs empty = all channels (the resolver passes the admin's scope)
func (s *Service) ListForModeration(ctx context.Context, channelIDs []uuid.UUID, status string, limit, offset int32) ([]Question, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.queries.ListQuestionsForModeration(ctx, db.ListQuestionsForModerationParams{
		Limit:       limit,
		Offset:      offset,
		AllChannels: len(channelIDs) == 0,
		ChannelIds:  channelIDs,
		Status:      status,
	})
	if err != nil {
		return nil, fmt.Errorf("list questions: %w", err)
	}
	return toQuestions(rows), nil
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (Question, error) {
	row, err := s.queries.GetProductQuestion(ctx, id)
	if err != nil {
		return Question{}, notFound(err, "get question")
	}
	return toQuestion(row), nil
}

// Answer publishes the question with the operator's answer - editing an answer keeps it published without announcing it again
func (s *Service) Answer(ctx context.Context, id uuid.UUID, by auth.Actor, answer string) (Question, error) {
	answer = strings.TrimSpace(answer)
	if answer == "" || utf8.RuneCountInString(answer) > maxAnswerRunes {
		return Question{}, ErrInvalidAnswer
	}
	var q Question
	err := s.write(ctx, func(dbq *db.Queries) (events.Event, error) {
		row, err := dbq.AnswerProductQuestion(ctx, db.AnswerProductQuestionParams{
			ID:                      id,
			Answer:                  answer,
			AnsweredBy:              by.AdminID(),
			AnsweredByIntegrationID: by.IntegrationID(),
		})
		if err != nil {
			return nil, notFound(err, "answer question")
		}
		q = toQuestion(row.ProductQuestion)
		if row.PrevStatus == StatusPublished {
			return nil, nil
		}
		return &QuestionAnswered{
			QuestionID:            q.ID,
			ProductID:             q.ProductID,
			ChannelID:             q.ChannelID,
			AnsweredBy:            by.AdminID().UUID,
			AnsweredByIntegration: by.IntegrationID().UUID,
			Question:              q.Question,
			Answer:                q.Answer,
		}, nil
	})
	if err != nil {
		return Question{}, err
	}
	return q, nil
}

// Reject hides the question from every surface but keeps the row
func (s *Service) Reject(ctx context.Context, id uuid.UUID) (Question, error) {
	row, err := s.queries.SetProductQuestionStatus(ctx, db.SetProductQuestionStatusParams{
		ID:     id,
		Status: StatusRejected,
	})
	if err != nil {
		return Question{}, notFound(err, "reject question")
	}
	return toQuestion(row), nil
}

// Delete removes the question permanently
func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	if err := s.queries.DeleteProductQuestion(ctx, id); err != nil {
		return fmt.Errorf("delete question: %w", err)
	}
	return nil
}

// clampAuthorName trims and caps the display name at 120 bytes, backing the cut off to a rune boundary - a mid-rune cut is invalid UTF-8 Postgres refuses, failing the whole submit
func clampAuthorName(name string) string {
	name = strings.TrimSpace(name)
	if len(name) <= 120 {
		return name
	}
	cut := 120
	for cut > 0 && !utf8.RuneStart(name[cut]) {
		cut--
	}
	return name[:cut]
}

func toQuestions(rows []db.ProductQuestion) []Question {
	out := make([]Question, 0, len(rows))
	for _, r := range rows {
		out = append(out, toQuestion(r))
	}
	return out
}

func toQuestion(r db.ProductQuestion) Question {
	q := Question{
		ID:          r.ID,
		ProductID:   r.ProductID,
		ChannelID:   r.ChannelID,
		AuthorName:  r.AuthorName,
		AuthorEmail: r.AuthorEmail,
		Question:    r.Question,
		Answer:      r.Answer,
		Status:      r.Status,
		CreatedAt:   r.CreatedAt,
	}
	if r.CustomerID.Valid {
		id := r.CustomerID.UUID
		q.CustomerID = &id
	}
	if r.AnsweredBy.Valid {
		id := r.AnsweredBy.UUID
		q.AnsweredBy = &id
	}
	if r.AnsweredByIntegrationID.Valid {
		id := r.AnsweredByIntegrationID.UUID
		q.AnsweredByIntegration = &id
	}
	if r.AnsweredAt.Valid {
		t := r.AnsweredAt.Time
		q.AnsweredAt = &t
	}
	return q
}
