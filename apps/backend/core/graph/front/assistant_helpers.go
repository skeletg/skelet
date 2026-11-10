package front

import (
	"context"
	"maps"
	"math"
	"slices"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/front/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/shoptools"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func askAssistant(ctx context.Context, shop *shoptools.Service, messages []*model.AssistantMessage, cartID, page, conversation *string, notify shoptools.Notify) (*model.AssistantReply, error) {
	sv, ok := channel.FromContext(ctx)
	if !ok || sv == nil {
		return nil, storeRequired()
	}
	if shop == nil {
		return nil, shoptools.ErrAssistantUnavailable
	}
	msgs := make([]shoptools.Message, 0, len(messages))
	for _, m := range messages {
		if m != nil {
			msgs = append(msgs, shoptools.Message{Role: m.Role, Content: m.Content, Recap: ptrOr(m.Recap)})
		}
	}
	who := shoptools.Shopper{Customer: channel.OptionalCustomerID(ctx)}
	if cartID != nil {
		if id, err := uuid.Parse(*cartID); err == nil {
			who.CartID = &id
		}
	}
	if conversation != nil {
		who.Conversation, _ = uuid.Parse(*conversation)
	}
	if req, ok := auth.RequestFromContext(ctx); ok {
		who.Address = auth.ClientIP(req)
	}
	turn, err := shop.Assist(ctx, *sv, who, msgs, ptrOr(page), notify)
	if err != nil {
		return nil, err
	}
	reply := &model.AssistantReply{Reply: turn.Reply, Incomplete: turn.Incomplete, Recap: turn.Recap, FollowUps: append([]string{}, turn.FollowUps...),
		ProductIds: make([]string, len(turn.Products)), Links: assistantLinks(turn.Links), CartChanged: turn.CartChanged, Checkout: turn.Checkout}
	for i, id := range turn.Products {
		reply.ProductIds[i] = id.String()
	}
	if turn.Navigate != nil {
		reply.Navigate = assistantLinks([]shoptools.Link{*turn.Navigate})[0]
	}
	if turn.Detail != nil {
		reply.Detail = assistantDetail(*turn.Detail)
	}
	if turn.CartID != nil {
		id := turn.CartID.String()
		reply.CartID = &id
	}
	return reply, nil
}

// The filters go out slug-sorted, so one link always comes out as one address
func assistantLinks(links []shoptools.Link) []*model.AssistantLink {
	out := make([]*model.AssistantLink, len(links))
	for i, l := range links {
		link := &model.AssistantLink{Label: l.Label, Path: l.Path, Query: l.Query, Sort: l.Sort, Filters: []*model.AssistantLinkFilter{}}
		for _, slug := range slices.Sorted(maps.Keys(l.Filters)) {
			if len(l.Filters[slug]) > 0 {
				link.Filters = append(link.Filters, &model.AssistantLinkFilter{Slug: slug, Values: l.Filters[slug]})
			}
		}
		if l.Count != nil {
			n := int32(min(*l.Count, math.MaxInt32))
			link.Count = &n
		}
		out[i] = link
	}
	return out
}

func assistantDetail(d shoptools.Detail) *model.AssistantProductDetail {
	out := &model.AssistantProductDetail{ProductID: d.ProductID.String(), Summary: d.Summary, Highlights: make([]*model.AssistantPoint, len(d.Highlights)),
		GoodFor: append([]string{}, d.GoodFor...), KeepInMind: append([]string{}, d.KeepInMind...)}
	for i, p := range d.Highlights {
		out.Highlights[i] = &model.AssistantPoint{Title: p.Title, Detail: p.Detail}
	}
	return out
}
