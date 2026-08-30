package admin

import (
	"context"
	"strings"
	"time"

	"github.com/skeletg/skelet/graph/admin/model"
	"github.com/skeletg/skelet/internal/auth"
	"github.com/skeletg/skelet/internal/giftcard"
)

func (r *mutationResolver) IssueGiftCard(ctx context.Context, currencyCode string, amount int32, recipientEmail *string, expiresAt *time.Time) (*model.GiftCard, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "giftcards.manage"); err != nil {
		return nil, err
	}
	email := ""
	if recipientEmail != nil {
		email = *recipientEmail
	}
	card, err := r.GiftcardSvc.Issue(ctx, giftcard.IssueInput{
		CurrencyCode:   strings.ToUpper(strings.TrimSpace(currencyCode)),
		Amount:         int64(amount),
		RecipientEmail: email,
		ExpiresAt:      expiresAt,
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
	cards, err := r.GiftcardSvc.List(ctx, int32Or(limit, 50), int32Or(offset, 0))
	if err != nil {
		return nil, err
	}
	out := make([]*model.GiftCard, len(cards))
	for i, c := range cards {
		out[i] = toGiftCardModel(c, nil)
	}
	return out, nil
}

func (r *queryResolver) GiftCard(ctx context.Context, code string) (*model.GiftCard, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "giftcards.view"); err != nil {
		return nil, err
	}
	card, err := r.GiftcardSvc.GetByCode(ctx, strings.TrimSpace(code))
	if err != nil {
		return nil, err
	}
	return r.giftCardWithTxns(ctx, card)
}
