package referral

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/internal/db/sqlc"
	"github.com/skeletg/skelet/internal/events"
	"github.com/skeletg/skelet/internal/order"
)

// SubscribeToOrderEvents wires the reward trigger: when a pending referee places their first order (durable via the outbox), both sides get
// store credit and the referral flips to 'rewarded' - The locked status flip makes at-least-once delivery safe
func (s *Service) SubscribeToOrderEvents(bus *events.Bus) {
	if bus == nil {
		return
	}
	bus.Subscribe(order.EventPlaced, s.onOrderPlaced)
	bus.Subscribe(order.EventCancelled, s.onOrderCancelled)
	bus.Subscribe(order.EventRefunded, s.onOrderRefunded)
}

func (s *Service) onOrderCancelled(ctx context.Context, e events.Event) error {
	cancelled, ok := e.(*order.OrderCancelled)
	if !ok {
		return nil
	}
	if err := s.reverseForOrder(ctx, cancelled.OrderID); err != nil {
		log.Printf("[referral] reverse reward for cancelled order %s: %v", cancelled.OrderID, err)
		return err // credit must be clawed back - the locked status flip keeps the replay idempotent
	}
	return nil
}

// A full refund of the qualifying order undoes the referral like a cancel does, else the cancel loop just becomes a refund loop
// FULL only - the reward is all-or-nothing, so a partial return on a genuine order must not cost the referrer their payout
func (s *Service) onOrderRefunded(ctx context.Context, e events.Event) error {
	refunded, ok := e.(*order.OrderRefunded)
	if !ok || !refunded.FullyRefunded {
		return nil
	}
	if err := s.reverseForOrder(ctx, refunded.OrderID); err != nil {
		log.Printf("[referral] reverse reward for refunded order %s: %v", refunded.OrderID, err)
		return err
	}
	return nil
}

// reverseForOrder takes back both sides of a referral payout when the qualifying
// first order is cancelled - without it, registering throwaway accounts under
// your own code, placing an unpaid order each and cancelling mints real credit
// on every loop, and the referral row stays 'rewarded' so nothing ever notices
// Clawbacks are clamped at the current balance, so a reward already spent leaves
// the customer whole rather than in hidden debt
func (s *Service) reverseForOrder(ctx context.Context, orderID uuid.UUID) error {
	return s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		q := s.queries.WithTx(t)
		ref, err := q.GetRewardedReferralByOrder(ctx, uuid.NullUUID{UUID: orderID, Valid: true})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil // not a referral-qualifying order, or already reversed
			}
			return fmt.Errorf("load rewarded referral: %w", err)
		}
		currency := ""
		if s.orders != nil {
			o, oerr := s.orders.GetByID(ctx, orderID)
			if oerr != nil {
				return fmt.Errorf("load order for reversal currency: %w", oerr)
			}
			currency = o.CurrencyCode
		}
		if currency == "" {
			return fmt.Errorf("no currency for referral reversal on order %s", orderID)
		}
		if ref.ReferrerRewardCents > 0 {
			if _, err := s.credit.ClawbackTx(ctx, t, ref.ReferrerCustomerID, ref.ReferrerRewardCents, currency,
				"referral reward reversed - the referred order was cancelled"); err != nil {
				return fmt.Errorf("claw back referrer reward: %w", err)
			}
		}
		if ref.RefereeRewardCents > 0 {
			if _, err := s.credit.ClawbackTx(ctx, t, ref.RefereeCustomerID, ref.RefereeRewardCents, currency,
				"welcome referral reward reversed - your first order was cancelled"); err != nil {
				return fmt.Errorf("claw back referee reward: %w", err)
			}
		}
		if err := q.MarkReferralReversed(ctx, ref.ID); err != nil {
			return fmt.Errorf("mark referral reversed: %w", err)
		}
		return nil
	})
}

func (s *Service) onOrderPlaced(ctx context.Context, e events.Event) error {
	placed, ok := e.(*order.OrderPlaced)
	if !ok || placed.CustomerID == nil {
		return nil
	}
	if err := s.rewardForOrder(ctx, placed); err != nil {
		log.Printf("[referral] reward for order %s: %v", placed.OrderID, err)
		return err
	}
	return nil
}

func (s *Service) rewardForOrder(ctx context.Context, placed *order.OrderPlaced) error {
	prog, err := s.ProgramFor(ctx, placed.ChannelID)
	if err != nil {
		return err
	}
	if !prog.Enabled {
		return nil
	}
	return s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		q := s.queries.WithTx(t)
		ref, err := q.GetPendingReferralByReferee(ctx, *placed.CustomerID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil // not a pending referee - nothing to do
			}
			return fmt.Errorf("load pending referral: %w", err)
		}
		if err := q.MarkReferralRewarded(ctx, db.MarkReferralRewardedParams{
			ID:                  ref.ID,
			OrderID:             uuid.NullUUID{UUID: placed.OrderID, Valid: true},
			ReferrerRewardCents: prog.ReferrerRewardCents,
			RefereeRewardCents:  prog.RefereeRewardCents,
		}); err != nil {
			return fmt.Errorf("mark rewarded: %w", err)
		}
		if prog.ReferrerRewardCents > 0 {
			if err := s.credit.GrantTx(ctx, t, ref.ReferrerCustomerID, prog.ReferrerRewardCents, placed.Currency,
				"referral reward - your invite placed their first order"); err != nil {
				return fmt.Errorf("grant referrer reward: %w", err)
			}
		}
		if prog.RefereeRewardCents > 0 {
			if err := s.credit.GrantTx(ctx, t, ref.RefereeCustomerID, prog.RefereeRewardCents, placed.Currency,
				"welcome referral reward"); err != nil {
				return fmt.Errorf("grant referee reward: %w", err)
			}
		}
		return nil
	})
}
