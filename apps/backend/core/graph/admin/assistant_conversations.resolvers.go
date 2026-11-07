package admin

import (
	"context"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (r *queryResolver) AssistantConversations(ctx context.Context, channelID string, search *string, customerID *string, unanswered *model.AssistantUnansweredFilter, after *string, limit *int32) (*model.AssistantConversationPage, error) {
	authed, err := r.signalsChannel(ctx, "assistant.view", channelID)
	if err != nil {
		return nil, err
	}
	req := &signalsv1.ListAssistantConversationsRequest{ChannelId: channelID, CustomerId: customerID, WithCounts: countsSelected(ctx)}
	if unanswered != nil {
		req.Unanswered = unansweredWire[*unanswered]
	}
	if search != nil {
		req.Search = *search
	}
	if limit != nil {
		req.Limit = *limit
	}
	if after != nil && *after != "" {
		at, id, err := decodeConversationCursor(*after)
		if err != nil {
			return nil, err
		}
		req.BeforeAt, req.BeforeId = timestamppb.New(at), id
	}
	res, err := r.SignalsAPI.Assistant.ListAssistantConversations(authed, req)
	if err != nil {
		return nil, signalsErr(err)
	}
	page := &model.AssistantConversationPage{Items: make([]*model.AssistantConversation, len(res.GetConversations())), Counts: gapCounts(res.GetCounts())}
	for i, c := range res.GetConversations() {
		page.Items[i] = conversationSummary(c)
	}
	if n := len(page.Items); res.GetMore() && n > 0 {
		cursor := encodeConversationCursor(page.Items[n-1].LastAt, page.Items[n-1].ID)
		page.NextCursor = &cursor
	}
	return page, nil
}

func (r *queryResolver) AssistantConversation(ctx context.Context, channelID string, id string) (*model.AssistantConversationDetail, error) {
	authed, err := r.signalsChannel(ctx, "assistant.view", channelID)
	if err != nil {
		return nil, err
	}
	res, err := r.SignalsAPI.Assistant.GetAssistantConversation(authed, &signalsv1.GetAssistantConversationRequest{ChannelId: channelID, ConversationId: id})
	if err != nil {
		return nil, signalsErr(err)
	}
	if res.GetConversation() == nil {
		return nil, nil
	}
	out := &model.AssistantConversationDetail{Conversation: conversationSummary(res.GetConversation()), Turns: make([]*model.AssistantTranscriptTurn, len(res.GetTurns()))}
	for i, t := range res.GetTurns() {
		out.Turns[i] = &model.AssistantTranscriptTurn{
			ID: t.GetId(), Page: t.GetPage(), Question: t.GetQuestion(), Reply: t.GetReply(), Recap: t.GetRecap(), Showed: showedOut(t.GetRecap()), FollowUps: append([]string{}, t.GetFollowUps()...),
			Steps: t.GetSteps(), ToolCalls: t.GetToolCalls(), Refused: t.GetRefused(), Thought: t.GetThought(), Products: t.GetProducts(), CartChanged: t.GetCartChanged(), Checkout: t.GetCheckout(), Incomplete: t.GetIncomplete(), Model: t.GetModel(),
			InputTokens: int(t.GetInputTokens()), OutputTokens: int(t.GetOutputTokens()), CacheReadTokens: int(t.GetCacheReadTokens()),
			CacheWriteTokens: int(t.GetCacheWriteTokens()), At: t.GetAt().AsTime(),
		}
	}
	return out, nil
}

func (r *queryResolver) AssistantTurnRounds(ctx context.Context, channelID string, turnID string) ([]*model.AssistantRound, error) {
	authed, err := r.signalsChannel(ctx, "assistant.view", channelID)
	if err != nil {
		return nil, err
	}
	res, err := r.SignalsAPI.Assistant.GetAssistantTurnRounds(authed, &signalsv1.GetAssistantTurnRoundsRequest{ChannelId: channelID, TurnId: turnID})
	if err != nil {
		return nil, signalsErr(err)
	}
	return transcriptRounds(res.GetRounds()), nil
}
