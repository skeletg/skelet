package admin

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/giftcard"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func (r *Resolver) giftCardWithTxns(ctx context.Context, g giftcard.GiftCard) (*model.GiftCard, error) {
	txns, err := r.GiftcardSvc.Transactions(ctx, g.ID)
	if err != nil {
		return nil, err
	}
	return toGiftCardModel(g, txns), nil
}

func toGiftCardModel(g giftcard.GiftCard, txns []giftcard.Transaction) *model.GiftCard { // nil txns for list views that don't need the ledger
	m := &model.GiftCard{
		ID:             g.ID.String(),
		Code:           g.Code,
		CurrencyCode:   g.CurrencyCode,
		InitialBalance: int32(g.InitialBalance),
		Balance:        int32(g.Balance),
		Status:         g.Status,
		RecipientEmail: g.RecipientEmail,
		CreatedAt:      g.CreatedAt,
		Transactions:   make([]*model.GiftCardTransaction, len(txns)),
	}
	if g.IssuedOrderID != nil {
		s := g.IssuedOrderID.String()
		m.IssuedOrderID = &s
	}
	if g.ExpiresAt != nil {
		t := *g.ExpiresAt
		m.ExpiresAt = &t
	}
	if g.ChannelID != nil {
		s := g.ChannelID.String()
		m.ChannelID = &s
	}
	for i, tx := range txns {
		tm := &model.GiftCardTransaction{
			ID:        tx.ID.String(),
			Amount:    int32(tx.Amount),
			Reason:    tx.Reason,
			Note:      tx.Note,
			CreatedAt: tx.CreatedAt,
		}
		if tx.OrderID != nil {
			s := tx.OrderID.String()
			tm.OrderID = &s
		}
		m.Transactions[i] = tm
	}
	return m
}

func (r *Resolver) requireGiftCard(ctx context.Context, slug string, g giftcard.GiftCard) error {
	if g.ChannelID == nil {
		return auth.RequireGlobalPermission(ctx, r.AdminSvc, slug)
	}
	return auth.RequireChannelPermission(ctx, r.AdminSvc, slug, *g.ChannelID, nil)
}

func (r *Resolver) requireGiftCardByID(ctx context.Context, slug string, id uuid.UUID) error {
	if err := auth.RequirePermission(ctx, r.AdminSvc, slug); err != nil {
		return err
	}
	g, err := r.GiftcardSvc.GetByID(ctx, id)
	if err != nil {
		return err
	}
	return r.requireGiftCard(ctx, slug, g)
}

// A card in another currency than its store's could never be spent there
func (r *mutationResolver) giftCardIssueChannel(ctx context.Context, raw *string, currency string) (*uuid.UUID, error) {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil, auth.RequireGlobalPermission(ctx, r.AdminSvc, "giftcards.manage")
	}
	id, err := parseID(*raw, "channel")
	if err != nil {
		return nil, err
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "giftcards.manage", id, nil); err != nil {
		return nil, err
	}
	ch, err := r.ChannelSvc.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(ch.CurrencyCode, currency) {
		return nil, apperr.New("GIFT_CARD_CURRENCY_MISMATCH", apperr.ClassValidation, fmt.Sprintf("%s sells in %s, so a %s card could never be spent there", ch.Name, ch.CurrencyCode, currency))
	}
	return &id, nil
}

// One page of the cards the caller may view - callers gate on giftcards.view first
func (r *queryResolver) giftCardPage(ctx context.Context, search string, limit, offset *int32) (*model.GiftCardPage, error) {
	permitted, err := auth.LoadPermittedChannels(ctx, r.AdminSvc, "giftcards.view")
	if err != nil {
		return nil, err
	}
	cards, total, err := r.GiftcardSvc.Page(ctx, permitted.IDs(), search, int32Or(limit, 50), int32Or(offset, 0))
	if err != nil {
		return nil, err
	}
	out := make([]*model.GiftCard, len(cards))
	for i, c := range cards {
		out[i] = toGiftCardModel(c, nil)
	}
	return &model.GiftCardPage{TotalCount: total, Items: out}, nil
}
