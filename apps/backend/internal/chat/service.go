// Package chat persists chat chat conversations per admin - A thin service over the generated queries -
// conversations are stored whole (messages as a JSONB array) and every operation is scoped to the owning admin id
package chat

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	db "github.com/skeletg/skelet/apps/backend/internal/db/sqlc"
)

type Service struct{ q *db.Queries }

func NewService(q *db.Queries) *Service { return &Service{q: q} }

type Summary struct {
	ID        uuid.UUID
	Title     string
	UpdatedAt time.Time
}

type Conversation struct {
	ID        uuid.UUID
	Title     string
	Messages  json.RawMessage
	UpdatedAt time.Time
}

func (s *Service) List(ctx context.Context, adminID uuid.UUID, limit int32) ([]Summary, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	rows, err := s.q.ListChatConversations(ctx, db.ListChatConversationsParams{AdminID: adminID, Limit: limit})
	if err != nil {
		return nil, err
	}
	out := make([]Summary, len(rows))
	for i, r := range rows {
		out[i] = Summary{ID: r.ID, Title: r.Title, UpdatedAt: r.UpdatedAt}
	}
	return out, nil
}

func (s *Service) Get(ctx context.Context, id, adminID uuid.UUID) (Conversation, error) {
	r, err := s.q.GetChatConversation(ctx, db.GetChatConversationParams{ID: id, AdminID: adminID})
	if err != nil {
		return Conversation{}, err
	}
	return Conversation{ID: r.ID, Title: r.Title, Messages: r.Messages, UpdatedAt: r.UpdatedAt}, nil
}

// Save upserts: inserts when id is nil/zero, otherwise updates the admin's own row - Returns the conversation id -
// Updating a row the admin doesn't own affects nothing and surfaces as an error (RETURNING yields no row)
func (s *Service) Save(ctx context.Context, id *uuid.UUID, adminID, channelID uuid.UUID, title string, messages json.RawMessage) (uuid.UUID, error) {
	if len(messages) == 0 {
		messages = json.RawMessage("[]")
	}
	if id != nil && *id != uuid.Nil {
		return s.q.UpdateChatConversation(ctx, db.UpdateChatConversationParams{
			ID: *id, AdminID: adminID, Title: title, Messages: messages,
		})
	}
	return s.q.InsertChatConversation(ctx, db.InsertChatConversationParams{
		AdminID: adminID, ChannelID: channelID, Title: title, Messages: messages,
	})
}

func (s *Service) Delete(ctx context.Context, id, adminID uuid.UUID) error {
	return s.q.DeleteChatConversation(ctx, db.DeleteChatConversationParams{ID: id, AdminID: adminID})
}

func (s *Service) PruneBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	return s.q.DeleteChatConversationsBefore(ctx, cutoff)
}
