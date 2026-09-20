// Package qna is product questions & answers: shoppers ask on the PDP, operators answer from the dashboard - Moderation mirrors reviews -
// questions sit pending until answered (which publishes) or rejected; only published rows reach the storefront
package qna

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/permission"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

const (
	StatusPending   = "pending"
	StatusPublished = "published"
	StatusRejected  = "rejected"
)

var (
	// ErrInvalidQuestion rejects empty or oversized questions
	ErrInvalidQuestion = apperr.New("QNA_INVALID_QUESTION", apperr.ClassValidation, "question must be between 10 and 1000 characters")
	// ErrInvalidAnswer rejects empty answers (an empty answer is a reject, not a publish)
	ErrInvalidAnswer = apperr.New("QNA_INVALID_ANSWER", apperr.ClassValidation, "answer must not be empty")
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
	ID          uuid.UUID
	ProductID   uuid.UUID
	ChannelID   uuid.UUID
	CustomerID  *uuid.UUID
	AuthorName  string
	AuthorEmail string
	Question    string
	Answer      string
	AnsweredBy  *uuid.UUID
	AnsweredAt  *time.Time
	Status      string
	CreatedAt   time.Time
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
	QuestionID uuid.UUID
	ProductID  uuid.UUID
	ChannelID  uuid.UUID
	AnsweredBy uuid.UUID
	Question   string
	Answer     string
}

func (*QuestionAnswered) Type() string                { return EventQuestionAnswered }
func (e *QuestionAnswered) EventChannelID() uuid.UUID { return e.ChannelID }

type Service struct {
	queries *db.Queries
	bus     *events.Bus
}

func NewService(queries *db.Queries) *Service { return &Service{queries: queries} }

// SetEventBus is optional and nil-safe
func (s *Service) SetEventBus(b *events.Bus) { s.bus = b }

func (s *Service) publish(ctx context.Context, e events.Event) {
	if s.bus != nil {
		s.bus.Publish(ctx, e)
	}
}

// Submit records a new pending question - rate limiting and channel resolution
// are the resolver's job, validation is authoritative here
func (s *Service) Submit(ctx context.Context, productID, channelID uuid.UUID, customerID *uuid.UUID, authorName, authorEmail, question string) (Question, error) {
	question = strings.TrimSpace(question)
	if len(question) < 10 || len(question) > 1000 {
		return Question{}, ErrInvalidQuestion
	}
	authorName = clampAuthorName(authorName)
	row, err := s.queries.InsertProductQuestion(ctx, db.InsertProductQuestionParams{
		ProductID:   productID,
		ChannelID:   channelID,
		CustomerID:  nullableUUID(customerID),
		AuthorName:  authorName,
		AuthorEmail: strings.TrimSpace(strings.ToLower(authorEmail)),
		Question:    question,
	})
	if err != nil {
		return Question{}, fmt.Errorf("insert question: %w", err)
	}
	q := toQuestion(row)
	s.publish(ctx, &QuestionSubmitted{
		QuestionID: q.ID,
		ProductID:  q.ProductID,
		ChannelID:  q.ChannelID,
		CustomerID: q.CustomerID,
		AuthorName: q.AuthorName,
		Question:   q.Question,
	})
	return q, nil
}

func (s *Service) Published(ctx context.Context, productID uuid.UUID, limit int32) ([]Question, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.queries.ListPublishedQuestions(ctx, db.ListPublishedQuestionsParams{
		ProductID: productID,
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
		return Question{}, ErrNotFound
	}
	return toQuestion(row), nil
}

// Answer publishes the question with the operator's answer
func (s *Service) Answer(ctx context.Context, id uuid.UUID, adminID uuid.UUID, answer string) (Question, error) {
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return Question{}, ErrInvalidAnswer
	}
	row, err := s.queries.AnswerProductQuestion(ctx, db.AnswerProductQuestionParams{
		ID:         id,
		Answer:     answer,
		AnsweredBy: uuid.NullUUID{UUID: adminID, Valid: true},
	})
	if err != nil {
		return Question{}, ErrNotFound
	}
	q := toQuestion(row)
	s.publish(ctx, &QuestionAnswered{
		QuestionID: q.ID,
		ProductID:  q.ProductID,
		ChannelID:  q.ChannelID,
		AnsweredBy: adminID,
		Question:   q.Question,
		Answer:     q.Answer,
	})
	return q, nil
}

// Reject hides the question from every surface but keeps the row
func (s *Service) Reject(ctx context.Context, id uuid.UUID) (Question, error) {
	row, err := s.queries.SetProductQuestionStatus(ctx, db.SetProductQuestionStatusParams{
		ID:     id,
		Status: StatusRejected,
	})
	if err != nil {
		return Question{}, ErrNotFound
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
	if r.AnsweredAt.Valid {
		t := r.AnsweredAt.Time
		q.AnsweredAt = &t
	}
	return q
}

func nullableUUID(p *uuid.UUID) uuid.NullUUID {
	if p == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *p, Valid: true}
}
