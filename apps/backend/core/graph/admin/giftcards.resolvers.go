package admin

import (
	"context"
	"strings"
	"time"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/giftcard"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func (r *mutationResolver) IssueGiftCard(ctx context.Context, currencyCode string, amount int32, recipientEmail *string, expiresAt *time.Time, channelID *string) (*model.GiftCard, error) {
	currency := strings.ToUpper(strings.TrimSpace(currencyCode))
	channel, err := r.giftCardIssueChannel(ctx, channelID, currency)
	if err != nil {
		return nil, err
	}
	email := ""
	if recipientEmail != nil {
		email = *recipientEmail
	}
	card, err := r.GiftcardSvc.Issue(ctx, giftcard.IssueInput{
		CurrencyCode:   currency,
		Amount:         int64(amount),
		RecipientEmail: email,
		ExpiresAt:      expiresAt,
		ChannelID:      channel,
	})
	if err != nil {
		return nil, err
	}
	return r.giftCardWithTxns(ctx, card)
}

func (r *mutationResolver) DisableGiftCard(ctx context.Context, id string) (*model.GiftCard, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "giftcards.manage"); err != nil {
		return nil, err
	}
	gid, err := parseID(id, "gift card")
	if err != nil {
		return nil, err
	}
	if err := r.requireGiftCardByID(ctx, "giftcards.manage", gid); err != nil {
		return nil, err
	}
	card, err := r.GiftcardSvc.Disable(ctx, gid)
	if err != nil {
		return nil, err
	}
	return r.giftCardWithTxns(ctx, card)
}

func (r *mutationResolver) AdjustGiftCard(ctx context.Context, id string, amount int32, note *string) (*model.GiftCard, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "giftcards.manage"); err != nil {
		return nil, err
	}
	gid, err := parseID(id, "gift card")
	if err != nil {
		return nil, err
	}
	if err := r.requireGiftCardByID(ctx, "giftcards.manage", gid); err != nil {
		return nil, err
	}
	n := ""
	if note != nil {
		n = *note
	}
	card, err := r.GiftcardSvc.Adjust(ctx, gid, int64(amount), n)
	if err != nil {
		return nil, err
	}
	return r.giftCardWithTxns(ctx, card)
}

func (r *queryResolver) GiftCards(ctx context.Context, limit *int32, offset *int32) ([]*model.GiftCard, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "giftcards.view"); err != nil {
		return nil, err
	}
	page, err := r.giftCardPage(ctx, "", limit, offset)
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

func (r *queryResolver) GiftCardsPaged(ctx context.Context, search *string, limit *int32, offset *int32) (*model.GiftCardPage, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "giftcards.view"); err != nil {
		return nil, err
	}
	s := ""
	if search != nil {
		s = *search
	}
	return r.giftCardPage(ctx, s, limit, offset)
}

func (r *queryResolver) GiftCard(ctx context.Context, code string) (*model.GiftCard, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "giftcards.view"); err != nil {
		return nil, err
	}
	card, err := r.GiftcardSvc.GetByCode(ctx, strings.TrimSpace(code))
	if err != nil {
		return nil, err
	}
	if err := r.requireGiftCard(ctx, "giftcards.view", card); err != nil {
		return nil, err
	}
	return r.giftCardWithTxns(ctx, card)
}
