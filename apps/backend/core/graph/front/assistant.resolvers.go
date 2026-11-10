package front

import (
	"context"

	"github.com/skeletg/skelet/apps/backend/core/graph/front/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/shoptools"
)

func (r *mutationResolver) AskShopAssistant(ctx context.Context, messages []*model.AssistantMessage, cartID *string, page *string, conversationID *string) (*model.AssistantReply, error) {
	return askAssistant(ctx, r.ShopAssistant, messages, cartID, page, conversationID, shoptools.Notify{})
}

func (r *queryResolver) AssistantAvailable(ctx context.Context) (bool, error) {
	sv, ok := channel.FromContext(ctx)
	return ok && sv != nil && r.ShopAssistant != nil && r.ShopAssistant.AssistantOpen(ctx, *sv), nil
}

func (r *queryResolver) AssistantPage(ctx context.Context, page string) (*model.AssistantPage, error) {
	sv, ok := channel.FromContext(ctx)
	if !ok || sv == nil {
		return nil, storeRequired()
	}
	if r.ShopAssistant == nil {
		return nil, shoptools.ErrAssistantUnavailable
	}
	info, err := r.ShopAssistant.PageContext(ctx, *sv, page)
	if err != nil {
		return nil, err
	}
	return &model.AssistantPage{Kind: info.Kind, Title: info.Title}, nil
}
