package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/internal/auth"
)

// Update path is own-rows only, and a blank title auto-derives from the first user message
func (r *mutationResolver) SaveChatConversation(ctx context.Context, id *string, channelID *string, title *string, messages []*model.SignalsChatMessage) (string, error) {
	claims, ok := auth.AdminFromContext(ctx)
	if !ok || claims == nil {
		return "", errAdminRequired()
	}
	if r.ChatSvc == nil {
		return "", fmt.Errorf("conversation history not available")
	}
	var idPtr *uuid.UUID
	if id != nil && *id != "" {
		u, err := uuid.Parse(*id)
		if err != nil {
			return "", errInvalidID("conversation id", nil)
		}
		idPtr = &u
	}
	var ch uuid.UUID
	if channelID != nil && *channelID != "" {
		ch, _ = uuid.Parse(*channelID)
	}
	arr := make([]storedMsg, 0, len(messages))
	for _, m := range messages {
		if m != nil {
			arr = append(arr, storedMsg{Role: m.Role, Content: m.Content})
		}
	}
	const maxStoredMsgs = 200 // caps stored history so a runaway conversation can't bloat the row
	if len(arr) > maxStoredMsgs {
		arr = arr[len(arr)-maxStoredMsgs:]
	}
	raw, err := json.Marshal(arr)
	if err != nil {
		return "", fmt.Errorf("encode messages: %w", err)
	}
	t := ""
	if title != nil {
		t = strings.TrimSpace(*title)
	}
	if t == "" {
		for _, m := range arr {
			if m.Role == "user" {
				t = m.Content
				break
			}
		}
		if r := []rune(t); len(r) > 60 {
			t = string(r[:60])
		}
		if t == "" {
			t = "New chat"
		}
	}
	newID, err := r.ChatSvc.Save(ctx, idPtr, claims.AdminID, ch, t, raw)
	if err != nil {
		return "", err
	}
	return newID.String(), nil
}

func (r *mutationResolver) DeleteChatConversation(ctx context.Context, id string) (bool, error) {
	claims, ok := auth.AdminFromContext(ctx)
	if !ok || claims == nil {
		return false, errAdminRequired()
	}
	if r.ChatSvc == nil {
		return false, fmt.Errorf("conversation history not available")
	}
	u, err := uuid.Parse(id)
	if err != nil {
		return false, errInvalidID("conversation id", nil)
	}
	if err := r.ChatSvc.Delete(ctx, u, claims.AdminID); err != nil {
		return false, err
	}
	return true, nil
}

func (r *queryResolver) ChatConversations(ctx context.Context, limit *int32) ([]*model.ChatConversationSummary, error) {
	claims, ok := auth.AdminFromContext(ctx)
	if !ok || claims == nil {
		return nil, errAdminRequired()
	}
	if r.ChatSvc == nil {
		return []*model.ChatConversationSummary{}, nil
	}
	var l int32 = 20
	if limit != nil && *limit > 0 {
		l = *limit
	}
	rows, err := r.ChatSvc.List(ctx, claims.AdminID, l)
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
	claims, ok := auth.AdminFromContext(ctx)
	if !ok || claims == nil {
		return nil, errAdminRequired()
	}
	if r.ChatSvc == nil {
		return nil, nil
	}
	cid, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("conversation id", nil)
	}
	c, err := r.ChatSvc.Get(ctx, cid, claims.AdminID)
	if err != nil {
		return nil, nil // not found / not owned
	}
	var raw []storedMsg
	_ = json.Unmarshal(c.Messages, &raw)
	msgs := make([]*model.ChatMessage, 0, len(raw))
	for _, m := range raw {
		msgs = append(msgs, &model.ChatMessage{Role: m.Role, Content: m.Content})
	}
	return &model.ChatConversation{ID: c.ID.String(), Title: c.Title, Messages: msgs, UpdatedAt: c.UpdatedAt}, nil
}
