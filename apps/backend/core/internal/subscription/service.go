package subscription

import (
	"context"
	"database/sql"
	"log"
	"math"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

// Caps how many due subscriptions one ProcessDue pass handles, so a large backlog can't hold a tick open indefinitely - the next tick picks up the rest
const dueBatchSize = 200

// The most periods one billing run mints for a subscription - a daily plan after a long outage, or a row with a zero billing date, would otherwise mint thousands in one transaction. The rest are minted by the following runs
const maxCatchUpPeriods = 366

// cycleAmount is what one period costs - the snapshotted unit price times the subscribed quantity, rounded to cents
func cycleAmount(sub Subscription) int64 {
	if sub.Quantity <= 0 {
		return sub.Price
	}
	return int64(math.Round(float64(sub.Price) * sub.Quantity))
}

// Runs the recurring-billing engine - it starts subscriptions on purchase, through the OrderPlaced subscriber, and advances active ones through billing periods
type Service struct {
	repo   Repository
	tx     *txrunner.Manager
	now    func() time.Time
	orders OrderSubscriptionLookup
	// Publishes lifecycle events for the Synapse activity tracker - nil means none
	bus *events.Bus
	// Collects due invoices for renewal and dunning - nil leaves invoices at 'due'
	charger RenewalCharger
}

func NewService(repo Repository, txMgr *txrunner.Manager) *Service {
	return &Service{repo: repo, tx: txMgr, now: time.Now}
}

// Optional - without it, start-on-purchase is disabled
func (s *Service) SetOrderLookup(o OrderSubscriptionLookup) { s.orders = o }

// Advances from t by one billing interval - calendar-aware through AddDate, so month and year intervals land on the same day-of-month and handle month-length and leap-year rollovers the way Go's time does
func nextBillingTime(t time.Time, unit string, count int32) time.Time {
	n := int(count)
	if n <= 0 {
		n = 1
	}
	switch unit {
	case "day":
		return t.AddDate(0, 0, n)
	case "week":
		return t.AddDate(0, 0, 7*n)
	case "month":
		return t.AddDate(0, n, 0)
	case "year":
		return t.AddDate(n, 0, 0)
	default:
		// An unknown unit falls back to a month, so a misconfigured row still makes forward progress rather than billing every tick forever
		return t.AddDate(0, 1, 0)
	}
}

// Bills every active subscription due as of now and returns how many advanced - each runs in its own row-locked transaction, so concurrent schedulers and customer actions can't double-bill or race period advancement
func (s *Service) ProcessDue(ctx context.Context, now time.Time) (int, error) {
	due, err := s.repo.ListDue(ctx, now, dueBatchSize)
	if err != nil {
		return 0, err
	}
	billed := 0
	for _, d := range due {
		if err := s.billOne(ctx, d.ID, now); err != nil {
			log.Printf("[subscription] bill %s: %v", d.ID, err)
			continue
		}
		billed++
	}
	return billed, nil
}

func (s *Service) billOne(ctx context.Context, id uuid.UUID, now time.Time) error {
	var billedSub Subscription
	var minted []plannedPeriod
	err := s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		minted = nil // reset so a tx retry doesn't double-publish invoices
		r := s.repo.WithTx(t)
		sub, err := r.GetForUpdate(ctx, id)
		if err != nil {
			return err
		}
		// Re-check under the lock - a concurrent run may already have billed and advanced it, or it may have been canceled or paused since the snapshot
		if sub.Status != StatusActive || sub.NextBillingAt.After(now) {
			return nil
		}
		count, err := r.CountInvoices(ctx, sub.ID)
		if err != nil {
			return err
		}
		periods, start, end, next := planBilling(sub, now, count)
		if len(periods) == maxCatchUpPeriods {
			log.Printf("[subscription] %s is more than %d periods behind - billing the rest on the next runs", sub.ID, maxCatchUpPeriods)
		}
		for _, p := range periods {
			created, err := r.CreateInvoice(ctx, sub.ID, p.PeriodStart, p.PeriodEnd, cycleAmount(sub), sub.CurrencyCode, p.Sequence)
			if err != nil {
				return err
			}
			if created {
				minted = append(minted, p)
			}
		}
		if _, err := r.AdvancePeriod(ctx, sub.ID, start, end, next); err != nil {
			return err
		}
		billedSub = sub
		return nil
	})
	if err != nil {
		return err
	}
	for _, p := range minted {
		s.publishInvoiced(ctx, billedSub, p)
	}
	return nil
}

type plannedPeriod struct {
	PeriodStart time.Time
	PeriodEnd   time.Time
	Sequence    int32
}

// Computes the invoices to mint and the resulting current period, numbering after existingInvoices - bill-in-advance, so each elapsed boundary mints one invoice and catches up across periods missed to downtime - Pure, so testable
func planBilling(sub Subscription, now time.Time, existingInvoices int64) (periods []plannedPeriod, newStart, newEnd, newNext time.Time) {
	seq := int32(existingInvoices)
	start, end, next := sub.CurrentPeriodStart, sub.CurrentPeriodEnd, sub.NextBillingAt
	for !next.After(now) && len(periods) < maxCatchUpPeriods {
		ps := end
		pe := nextBillingTime(ps, sub.IntervalUnit, sub.IntervalCount)
		seq++
		periods = append(periods, plannedPeriod{PeriodStart: ps, PeriodEnd: pe, Sequence: seq})
		start, end, next = ps, pe, pe
	}
	return periods, start, end, next
}

// Stops future billing - customerID is the ownership guard for customer-initiated cancels, and admins pass nil to skip the check
// Idempotent, so cancelling an already-cancelled subscription is a no-op
func (s *Service) Cancel(ctx context.Context, id uuid.UUID, customerID *uuid.UUID) (Subscription, error) {
	sub, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return Subscription{}, err
	}
	if customerID != nil {
		if sub.CustomerID == nil || *sub.CustomerID != *customerID {
			return Subscription{}, ErrNotOwned
		}
	}
	if sub.Status == StatusCanceled {
		return sub, nil
	}
	now := s.now()
	out, err := s.repo.SetStatus(ctx, id, StatusCanceled, &now)
	if err == nil {
		s.publishLifecycle(ctx, EventCanceled, out)
	}
	return out, err
}

// Suspends billing without cancelling - the scheduler skips a paused subscription until it resumes. Only a live subscription pauses, so a cancelled one can't come back through pause and resume
func (s *Service) Pause(ctx context.Context, id uuid.UUID) (Subscription, error) {
	sub, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return Subscription{}, err
	}
	if sub.Status != StatusActive && sub.Status != StatusPastDue {
		return Subscription{}, ErrNotActive
	}
	out, err := s.repo.SetStatus(ctx, id, StatusPaused, nil)
	if err == nil {
		s.publishLifecycle(ctx, EventPaused, out)
	}
	return out, err
}

// Reactivates a paused subscription - the time spent paused is never billed: a billing date that passed while paused restarts the period now, so the next scheduler pass mints one fresh period
func (s *Service) Resume(ctx context.Context, id uuid.UUID) (Subscription, error) {
	sub, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return Subscription{}, err
	}
	if sub.Status != StatusPaused {
		return Subscription{}, ErrNotActive
	}
	now := s.now()
	var out Subscription
	err = s.inTx(ctx, func(r Repository) error {
		if !sub.NextBillingAt.After(now) {
			if _, err := r.AdvancePeriod(ctx, id, now, now, now); err != nil {
				return err
			}
		}
		var err error
		out, err = r.SetStatus(ctx, id, StatusActive, nil)
		return err
	})
	if err != nil {
		return Subscription{}, err
	}
	s.publishLifecycle(ctx, EventResumed, out)
	return out, nil
}

// inTx runs fn on a transactional repository, or on the plain one when no tx manager is wired
func (s *Service) inTx(ctx context.Context, fn func(Repository) error) error {
	if s.tx == nil {
		return fn(s.repo)
	}
	return s.tx.InTransaction(ctx, func(t *sql.Tx) error { return fn(s.repo.WithTx(t)) })
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (Subscription, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *Service) ListForCustomer(ctx context.Context, customerID uuid.UUID) ([]Subscription, error) {
	return s.repo.ListByCustomer(ctx, customerID)
}

// Picks the saved method renewals charge, nil clearing it - ownership is enforced here, while the method row's own ownership is the resolver's job
func (s *Service) SetPaymentMethodForCustomer(ctx context.Context, customerID, id uuid.UUID, paymentMethodID *uuid.UUID) (Subscription, error) {
	sub, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return Subscription{}, err
	}
	if sub.CustomerID == nil || *sub.CustomerID != customerID {
		return Subscription{}, ErrNotFound
	}
	return s.repo.SetPaymentMethod(ctx, id, paymentMethodID)
}

func (s *Service) List(ctx context.Context, status string, limit, offset int32) ([]Subscription, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.repo.List(ctx, status, limit, offset)
}

func (s *Service) Invoices(ctx context.Context, id uuid.UUID) ([]Invoice, error) {
	return s.repo.ListInvoices(ctx, id)
}
