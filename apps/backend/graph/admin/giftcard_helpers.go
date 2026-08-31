package admin

import (
	"context"

	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/internal/giftcard"
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
