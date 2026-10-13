package admin

import (
	"context"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/ai"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

// Chat history is self-scoped: every op needs the caller's admin id and nothing else
func (r *Resolver) chatHistory(ctx context.Context) (*ai.History, uuid.UUID, error) {
	claims, ok := auth.AdminFromContext(ctx)
	if !ok || claims == nil {
		return nil, uuid.Nil, errAdminRequired()
	}
	if r.ChatSvc == nil {
		return nil, uuid.Nil, apperr.New("CHAT_HISTORY_UNAVAILABLE", apperr.ClassUnavailable, "conversation history not available")
	}
	return r.ChatSvc, claims.AdminID, nil
}

func parseChatID(id string) (uuid.UUID, error) {
	u, err := uuid.Parse(id)
	if err != nil {
		return uuid.Nil, errInvalidID("conversation id", nil)
	}
	return u, nil
}

func chatMessagesToGQL(msgs []ai.HistoryMessage) []*model.ChatMessage {
	out := make([]*model.ChatMessage, len(msgs))
	for i, m := range msgs {
		out[i] = &model.ChatMessage{Role: m.Role, Content: m.Content}
	}
	return out
}
