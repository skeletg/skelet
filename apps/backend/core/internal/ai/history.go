package ai

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
)

const HistoryRetentionPath = cfgHistoryRetention

const (
	RoleUser      = "user"
	RoleAssistant = "assistant"

	MaxHistoryMessages  = 200
	MaxMessageRunes     = 32000
	MaxTitleRunes       = 200
	DerivedTitleRunes   = 60
	DefaultTitle        = "New chat"
	DefaultHistoryLimit = 20
	MaxHistoryLimit     = 50
)

var (
	ErrConversationNotFound = errors.New("ai: conversation not found")
	ErrMessageTooLong       = errors.New("ai: chat message is too long")
	ErrInvalidRole          = errors.New("ai: chat message role must be user or assistant")
)

type HistoryMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ConversationSummary struct {
	ID        uuid.UUID
	Title     string
	UpdatedAt time.Time
}

type Conversation struct {
	ID        uuid.UUID
	Title     string
	Messages  []HistoryMessage
	UpdatedAt time.Time
}

// History persists the admin assistant's conversations, one admin's rows at a time and never channel-scoped
type History struct{ q *db.Queries }

func NewHistory(q *db.Queries) *History { return &History{q: q} }

func HistoryLimit(limit int32) int32 {
	switch {
	case limit <= 0:
		return DefaultHistoryLimit
	case limit > MaxHistoryLimit:
		return MaxHistoryLimit
	}
	return limit
}

func (h *History) List(ctx context.Context, adminID uuid.UUID, limit int32) ([]ConversationSummary, error) {
	rows, err := h.q.ListChatConversations(ctx, db.ListChatConversationsParams{AdminID: adminID, Limit: HistoryLimit(limit)})
	if err != nil {
		return nil, err
	}
	out := make([]ConversationSummary, len(rows))
	for i, r := range rows {
		out[i] = ConversationSummary{ID: r.ID, Title: r.Title, UpdatedAt: r.UpdatedAt}
	}
	return out, nil
}

func (h *History) Get(ctx context.Context, id, adminID uuid.UUID) (Conversation, error) {
	r, err := h.q.GetChatConversation(ctx, db.GetChatConversationParams{ID: id, AdminID: adminID})
	if errors.Is(err, sql.ErrNoRows) {
		return Conversation{}, ErrConversationNotFound
	}
	if err != nil {
		return Conversation{}, err
	}
	var msgs []HistoryMessage
	if err := json.Unmarshal(r.Messages, &msgs); err != nil {
		return Conversation{}, fmt.Errorf("decode conversation %s: %w", id, err)
	}
	if msgs == nil {
		msgs = []HistoryMessage{}
	}
	return Conversation{ID: r.ID, Title: r.Title, Messages: msgs, UpdatedAt: r.UpdatedAt}, nil
}

// Save inserts when id is nil, otherwise updates the admin's own row - keeps the newest MaxHistoryMessages
func (h *History) Save(ctx context.Context, id *uuid.UUID, adminID uuid.UUID, title string, msgs []HistoryMessage) (uuid.UUID, error) {
	if len(msgs) > MaxHistoryMessages {
		msgs = msgs[len(msgs)-MaxHistoryMessages:]
	}
	for _, m := range msgs {
		if m.Role != RoleUser && m.Role != RoleAssistant {
			return uuid.Nil, ErrInvalidRole
		}
		if utf8.RuneCountInString(m.Content) > MaxMessageRunes {
			return uuid.Nil, ErrMessageTooLong
		}
	}
	if msgs == nil {
		msgs = []HistoryMessage{}
	}
	raw, err := json.Marshal(msgs)
	if err != nil {
		return uuid.Nil, fmt.Errorf("encode messages: %w", err)
	}
	t := conversationTitle(title, msgs)
	if id == nil || *id == uuid.Nil {
		return h.q.InsertChatConversation(ctx, db.InsertChatConversationParams{AdminID: adminID, Title: t, Messages: raw})
	}
	got, err := h.q.UpdateChatConversation(ctx, db.UpdateChatConversationParams{ID: *id, AdminID: adminID, Title: t, Messages: raw})
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, ErrConversationNotFound
	}
	return got, err
}

func (h *History) Delete(ctx context.Context, id, adminID uuid.UUID) error {
	return h.q.DeleteChatConversation(ctx, db.DeleteChatConversationParams{ID: id, AdminID: adminID})
}

func (h *History) PruneBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	return h.q.DeleteChatConversationsBefore(ctx, cutoff)
}

func conversationTitle(title string, msgs []HistoryMessage) string {
	t := strings.TrimSpace(title)
	if t == "" {
		for _, m := range msgs {
			if m.Role == RoleUser {
				t = truncateRunes(strings.TrimSpace(m.Content), DerivedTitleRunes)
				break
			}
		}
	}
	if t == "" {
		return DefaultTitle
	}
	return truncateRunes(t, MaxTitleRunes)
}

func truncateRunes(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}
