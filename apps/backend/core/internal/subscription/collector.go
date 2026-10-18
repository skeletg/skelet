package subscription

import (
	"context"
	"fmt"
	"log"
	"time"
)

// Collects one invoice against the sub's saved payment method - implemented in main so this package stays free of payment imports, and reference is the gateway transaction handle recorded on the paid invoice
type RenewalCharger interface {
	ChargeInvoice(ctx context.Context, sub Subscription, inv Invoice) (reference string, err error)
}

// Optional - without it invoices mint as 'due' and wait
func (s *Service) SetRenewalCharger(c RenewalCharger) { s.charger = c }

// The retry ladder - the Nth failure schedules the next attempt this far out, and exhausting it cancels the subscription
var dunningBackoff = []time.Duration{
	24 * time.Hour,
	3 * 24 * time.Hour,
	7 * 24 * time.Hour,
}

// Charges every due invoice whose dunning clock has arrived - one failure never aborts the sweep - Returns how many were collected
func (s *Service) CollectDue(ctx context.Context, now time.Time) (int, error) {
	if s.charger == nil {
		return 0, nil
	}
	invoices, err := s.repo.ListChargeableInvoices(ctx, now, dueBatchSize)
	if err != nil {
		return 0, err
	}
	collected := 0
	for _, inv := range invoices {
		ok, err := s.collectOne(ctx, inv, now)
		if err != nil {
			log.Printf("[subscription] collect invoice %s: %v", inv.ID, err)
			continue
		}
		if ok {
			collected++
		}
	}
	return collected, nil
}

func (s *Service) collectOne(ctx context.Context, inv Invoice, now time.Time) (bool, error) {
	sub, err := s.repo.GetByID(ctx, inv.SubscriptionID)
	if err != nil {
		return false, fmt.Errorf("load subscription: %w", err)
	}
	// Canceled, paused and expired subscriptions stop collecting - park the invoice with no next attempt, without burning a dunning step's meaning
	if sub.Status != StatusActive && sub.Status != StatusPastDue {
		_, err := s.repo.RecordInvoiceAttempt(ctx, inv.ID, nil, "subscription "+sub.Status)
		return false, err
	}

	reference, chargeErr := s.charger.ChargeInvoice(ctx, sub, inv)
	if chargeErr == nil {
		if err := s.repo.MarkInvoicePaid(ctx, inv.ID, reference); err != nil {
			return false, err
		}
		if sub.Status == StatusPastDue {
			if _, err := s.repo.SetStatus(ctx, sub.ID, StatusActive, nil); err != nil {
				return false, err
			}
		}
		return true, nil
	}

	// Failed - schedule the next dunning step, or cancel when the ladder is exhausted
	attempt := int(inv.AttemptCount) // attempts BEFORE this one
	if attempt >= len(dunningBackoff) {
		if _, err := s.repo.RecordInvoiceAttempt(ctx, inv.ID, nil, chargeErr.Error()); err != nil {
			return false, err
		}
		canceledAt := now
		if _, err := s.repo.SetStatus(ctx, sub.ID, StatusCanceled, &canceledAt); err != nil {
			return false, err
		}
		s.publishPaymentFailed(ctx, sub, inv, attempt+1, nil, true, chargeErr.Error())
		s.publishLifecycle(ctx, EventCanceled, sub)
		log.Printf("[subscription] %s canceled after %d failed charges", sub.ID, attempt+1)
		return false, nil
	}
	next := now.Add(dunningBackoff[attempt])
	if _, err := s.repo.RecordInvoiceAttempt(ctx, inv.ID, &next, chargeErr.Error()); err != nil {
		return false, err
	}
	if sub.Status != StatusPastDue {
		if _, err := s.repo.SetStatus(ctx, sub.ID, StatusPastDue, nil); err != nil {
			return false, err
		}
	}
	s.publishPaymentFailed(ctx, sub, inv, attempt+1, &next, false, chargeErr.Error())
	return false, nil
}

func (s *Service) publishPaymentFailed(ctx context.Context, sub Subscription, inv Invoice, attempt int, next *time.Time, willCancel bool, reason string) {
	if s.bus == nil {
		return
	}
	s.bus.Publish(ctx, &PaymentFailed{
		SubscriptionID: sub.ID,
		CustomerID:     sub.CustomerID,
		ChannelID:      chanOrNil(sub.ChannelID),
		InvoiceID:      inv.ID,
		Sequence:       inv.Sequence,
		AmountCents:    inv.Amount,
		Currency:       inv.CurrencyCode,
		Attempt:        attempt,
		NextRetryAt:    next,
		WillCancel:     willCancel,
		Reason:         reason,
	})
}
