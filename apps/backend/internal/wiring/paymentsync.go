package wiring

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/skeletg/skelet/apps/backend/internal/order"
	paymentpkg "github.com/skeletg/skelet/apps/backend/internal/payment"
)

// sweep window - orders younger than 30m are probably mid-checkout, older than 7d are abandoned for good
const (
	paymentSyncMinAge = 30 * time.Minute
	paymentSyncMaxAge = 7 * 24 * time.Hour
	paymentSyncBatch  = 100
)

// what the sweep should do with one gateway answer - a pure decision so the mapping stays table-testable
type paymentSyncAction int

const (
	syncNothing paymentSyncAction = iota
	syncRecordAuthorized
	syncSettleCaptured
	syncSettleRefunded
	syncSettleVoided
)

// paymentSyncActionFor maps gateway state to a settle action - pending parks for the next pass, failed stays unpaid so the shopper can retry
func paymentSyncActionFor(state paymentpkg.PaymentState) paymentSyncAction {
	switch state {
	case paymentpkg.PaymentStateAuthorized:
		return syncRecordAuthorized
	case paymentpkg.PaymentStateCaptured:
		return syncSettleCaptured
	case paymentpkg.PaymentStateRefunded:
		return syncSettleRefunded
	case paymentpkg.PaymentStateVoided:
		return syncSettleVoided
	}
	return syncNothing
}

func sweepUnsettledPayments(ctx context.Context, orders *order.Service, payments *paymentpkg.Service) error {
	now := time.Now()
	list, err := orders.ListAwaitingPaymentSync(ctx, now.Add(-paymentSyncMaxAge), now.Add(-paymentSyncMinAge), paymentSyncBatch)
	if err != nil {
		return err
	}
	for _, o := range list {
		res, err := payments.SyncPayment(ctx, o.PaymentProvider, paymentpkg.SyncPaymentRequest{
			OrderID:           o.ID,
			ChannelID:         o.ChannelID,
			ProviderReference: o.PaymentReference,
		})
		if err != nil {
			if errors.Is(err, paymentpkg.ErrSyncUnsupported) {
				continue // the provider has no synchronizer - webhooks are its only settlement path
			}
			log.Printf("[payment-sync] order %s via %s: %v", o.ID, o.PaymentProvider, err)
			continue // one gateway outage must not stall the rest of the batch
		}
		if err := applyPaymentSync(ctx, orders, o, res); err != nil {
			log.Printf("[payment-sync] settle order %s: %v", o.ID, err)
		}
	}
	return nil
}

func applyPaymentSync(ctx context.Context, orders *order.Service, o order.Order, res paymentpkg.SyncPaymentResult) error {
	switch paymentSyncActionFor(res.State) {
	case syncRecordAuthorized:

		amount := res.AuthorizedCents
		if amount == 0 {
			// what the gateway was asked to hold - the amount owed after gift-card/store-credit tenders
			amount = o.TotalAmount - o.GiftCardAmount - o.StoreCreditAmount
		}
		if _, err := orders.RecordAuthorization(ctx, o.ID, amount, o.PaymentProvider, o.PaymentReference); err != nil {
			return err
		}
		_, err := orders.SettlePayment(ctx, o.ID, order.PaymentAuthorized, o.PaymentReference)
		return err
	case syncSettleCaptured:
		if res.CapturedCents > 0 {
			_, err := orders.SettleGatewayAmount(ctx, o.ID, order.PaymentTxnCapture, res.CapturedCents, o.PaymentProvider, o.PaymentReference)
			return err
		}
		_, err := orders.SettlePayment(ctx, o.ID, order.PaymentPaid, o.PaymentReference)
		return err
	case syncSettleRefunded:
		if res.RefundedCents > 0 {
			_, err := orders.SettleGatewayAmount(ctx, o.ID, order.PaymentTxnRefund, res.RefundedCents, o.PaymentProvider, o.PaymentReference)
			return err
		}
		_, err := orders.SettlePayment(ctx, o.ID, order.PaymentRefunded, o.PaymentReference)
		return err
	case syncSettleVoided:
		_, err := orders.SettlePayment(ctx, o.ID, order.PaymentVoided, o.PaymentReference)
		return err
	}
	return nil
}
