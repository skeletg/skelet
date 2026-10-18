package admin

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/ai"
)

func (r *mutationResolver) SaveChatConversation(ctx context.Context, id *string, title *string, messages []*model.SignalsChatMessage) (string, error) {
	hist, adminID, err := r.chatHistory(ctx)
	if err != nil {
		return "", err
	}
	var idPtr *uuid.UUID
	if id != nil && *id != "" {
		u, err := parseChatID(*id)
		if err != nil {
			return "", err
		}
		idPtr = &u
	}
	msgs := make([]ai.HistoryMessage, 0, len(messages))
	for _, m := range messages {
		if m != nil {
			msgs = append(msgs, ai.HistoryMessage{Role: m.Role, Content: m.Content})
		}
	}
	t := ""
	if title != nil {
		t = *title
	}
	newID, err := hist.Save(ctx, idPtr, adminID, t, msgs)
	if err != nil {
		return "", err
	}
	return newID.String(), nil
}

func (r *mutationResolver) DeleteChatConversation(ctx context.Context, id string) (bool, error) {
	hist, adminID, err := r.chatHistory(ctx)
	if err != nil {
		return false, err
	}
	u, err := parseChatID(id)
	if err != nil {
		return false, err
	}
	if err := hist.Delete(ctx, u, adminID); err != nil {
		return false, err
	}
	return true, nil
}

func (r *queryResolver) ChatConversations(ctx context.Context, limit *int32) ([]*model.ChatConversationSummary, error) {
	hist, adminID, err := r.chatHistory(ctx)
	if err != nil {
		return nil, err
	}
	var l int32
	if limit != nil {
		l = *limit
	}
	rows, err := hist.List(ctx, adminID, l)
	if err != nil {
		return nil, err
	}
	out := make([]*model.ChatConversationSummary, 0, len(rows))
	for _, c := range rows {
		out = append(out, &model.ChatConversationSummary{ID: c.ID.String(), Title: c.Title, UpdatedAt: c.UpdatedAt})
	}
	return out, nil
}

func (r *queryResolver) ChatConversation(ctx context.Context, id string) (*model.ChatConversation, error) {
	hist, adminID, err := r.chatHistory(ctx)
	if err != nil {
		return nil, err
	}
	cid, err := parseChatID(id)
	if err != nil {
		return nil, err
	}
	c, err := hist.Get(ctx, cid, adminID)
	if errors.Is(err, ai.ErrConversationNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &model.ChatConversation{ID: c.ID.String(), Title: c.Title, Messages: chatMessagesToGQL(c.Messages), UpdatedAt: c.UpdatedAt}, nil
}
