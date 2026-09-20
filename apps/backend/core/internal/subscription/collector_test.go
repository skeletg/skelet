package subscription

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Recurring billing charges people while nobody is watching, so its failures are silent and they repeat on a schedule - the two that matter
// are charging a subscription that should have stopped, and a dunning ladder that either gives up early or never gives up

type collectorRepo struct {
	Repository
	subs     map[uuid.UUID]Subscription
	attempts []recordedAttempt
	paid     []uuid.UUID
	statuses []recordedStatus
	chargeIn []uuid.UUID // invoice ids handed to the charger, in order
}

type recordedAttempt struct {
	invoiceID uuid.UUID
	next      *time.Time
	reason    string
}

type recordedStatus struct {
	subID  uuid.UUID
	status string
}

func (r *collectorRepo) GetByID(_ context.Context, id uuid.UUID) (Subscription, error) {
	if s, ok := r.subs[id]; ok {
		return s, nil
	}
	return Subscription{}, ErrNotFound
}

func (r *collectorRepo) MarkInvoicePaid(_ context.Context, id uuid.UUID, _ string) error {
	r.paid = append(r.paid, id)
	return nil
}

func (r *collectorRepo) RecordInvoiceAttempt(_ context.Context, id uuid.UUID, next *time.Time, reason string) (Invoice, error) {
	r.attempts = append(r.attempts, recordedAttempt{invoiceID: id, next: next, reason: reason})
	return Invoice{ID: id}, nil
}

func (r *collectorRepo) SetStatus(_ context.Context, id uuid.UUID, status string, _ *time.Time) (Subscription, error) {
	r.statuses = append(r.statuses, recordedStatus{subID: id, status: status})
	sub := r.subs[id]
	sub.Status = status
	r.subs[id] = sub
	return sub, nil
}

// Records what it was asked to charge, and fails on demand
type fakeCharger struct {
	repo *collectorRepo
	err  error
}

func (c *fakeCharger) ChargeInvoice(_ context.Context, _ Subscription, inv Invoice) (string, error) {
	c.repo.chargeIn = append(c.repo.chargeIn, inv.ID)
	if c.err != nil {
		return "", c.err
	}
	return "ref_" + inv.ID.String(), nil
}

func newCollector(t *testing.T, status string, attemptCount int32, chargeErr error) (*Service, *collectorRepo, Subscription, Invoice) {
	t.Helper()
	sub := Subscription{ID: uuid.New(), Status: status}
	inv := Invoice{ID: uuid.New(), SubscriptionID: sub.ID, Amount: 2500, AttemptCount: attemptCount}
	repo := &collectorRepo{subs: map[uuid.UUID]Subscription{sub.ID: sub}}
	svc := &Service{repo: repo}
	svc.SetRenewalCharger(&fakeCharger{repo: repo, err: chargeErr})
	return svc, repo, sub, inv
}

// The money question: a subscription that is no longer active must never reach the gateway
func TestCollectDoesNotChargeInactiveSubscriptions(t *testing.T) {
	for _, status := range []string{StatusCanceled, StatusPaused, StatusExpired} {
		t.Run(status, func(t *testing.T) {
			svc, repo, _, inv := newCollector(t, status, 0, nil)

			if _, err := svc.collectOne(context.Background(), inv, time.Now()); err != nil {
				t.Fatalf("collectOne: %v", err)
			}
			if len(repo.chargeIn) != 0 {
				t.Fatalf("a %s subscription was charged - the customer is billed after stopping", status)
			}
			if len(repo.attempts) != 1 {
				t.Fatalf("recorded %d attempts, want 1 parking row", len(repo.attempts))
			}
			// parked with no next attempt, so it doesn't burn a dunning step it never used
			if repo.attempts[0].next != nil {
				t.Errorf("parked invoice scheduled a retry at %v, want none", repo.attempts[0].next)
			}
		})
	}
}

// Both collectible states DO charge - past_due is mid-dunning, not stopped
func TestCollectChargesActiveAndPastDue(t *testing.T) {
	for _, status := range []string{StatusActive, StatusPastDue} {
		t.Run(status, func(t *testing.T) {
			svc, repo, _, inv := newCollector(t, status, 0, nil)

			ok, err := svc.collectOne(context.Background(), inv, time.Now())
			if err != nil {
				t.Fatalf("collectOne: %v", err)
			}
			if !ok {
				t.Fatal("a successful charge was not reported as collected")
			}
			if len(repo.chargeIn) != 1 {
				t.Fatalf("charged %d times, want 1", len(repo.chargeIn))
			}
			if len(repo.paid) != 1 || repo.paid[0] != inv.ID {
				t.Errorf("invoice not marked paid: %v", repo.paid)
			}
		})
	}
}

// Recovering from dunning has to put the subscription back, or a customer who paid stays flagged
// past_due forever
func TestSuccessfulChargeRevivesPastDue(t *testing.T) {
	svc, repo, sub, inv := newCollector(t, StatusPastDue, 1, nil)

	if _, err := svc.collectOne(context.Background(), inv, time.Now()); err != nil {
		t.Fatalf("collectOne: %v", err)
	}
	if len(repo.statuses) != 1 || repo.statuses[0].status != StatusActive {
		t.Fatalf("status transitions = %+v, want one back to active", repo.statuses)
	}
	if repo.statuses[0].subID != sub.ID {
		t.Errorf("revived the wrong subscription")
	}
}

// An already-active subscription that pays must not be pointlessly re-set, which would churn events
func TestSuccessfulChargeOnActiveLeavesStatusAlone(t *testing.T) {
	svc, repo, _, inv := newCollector(t, StatusActive, 0, nil)

	if _, err := svc.collectOne(context.Background(), inv, time.Now()); err != nil {
		t.Fatalf("collectOne: %v", err)
	}
	if len(repo.statuses) != 0 {
		t.Errorf("an active subscription was re-set to %+v", repo.statuses)
	}
}

// The ladder: each failure schedules the next attempt further out, indexed by attempts ALREADY made
func TestDunningLadderSchedulesEachStep(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	for attempt, wantDelay := range dunningBackoff {
		t.Run(wantDelay.String(), func(t *testing.T) {
			svc, repo, _, inv := newCollector(t, StatusActive, int32(attempt), errors.New("card declined"))

			ok, err := svc.collectOne(context.Background(), inv, now)
			if err != nil {
				t.Fatalf("collectOne: %v", err)
			}
			if ok {
				t.Fatal("a declined charge was reported as collected")
			}
			if len(repo.attempts) != 1 || repo.attempts[0].next == nil {
				t.Fatalf("attempts = %+v, want one with a next attempt scheduled", repo.attempts)
			}
			if got := *repo.attempts[0].next; !got.Equal(now.Add(wantDelay)) {
				t.Errorf("next attempt at %v, want %v", got, now.Add(wantDelay))
			}
			// the reason is what support reads when a customer asks why they were dropped
			if repo.attempts[0].reason != "card declined" {
				t.Errorf("failure reason = %q, want the gateway's", repo.attempts[0].reason)
			}
		})
	}
}

// A first failure moves the subscription into dunning, and later failures leave it there
func TestFirstFailureMovesToPastDue(t *testing.T) {
	svc, repo, _, inv := newCollector(t, StatusActive, 0, errors.New("declined"))
	if _, err := svc.collectOne(context.Background(), inv, time.Now()); err != nil {
		t.Fatalf("collectOne: %v", err)
	}
	if len(repo.statuses) != 1 || repo.statuses[0].status != StatusPastDue {
		t.Fatalf("status transitions = %+v, want one to past_due", repo.statuses)
	}

	svc2, repo2, _, inv2 := newCollector(t, StatusPastDue, 1, errors.New("declined"))
	if _, err := svc2.collectOne(context.Background(), inv2, time.Now()); err != nil {
		t.Fatalf("collectOne: %v", err)
	}
	if len(repo2.statuses) != 0 {
		t.Errorf("an already past_due subscription was re-set to %+v", repo2.statuses)
	}
}

// Exhausting the ladder cancels - and must NOT schedule another attempt, or a cancelled
// subscription keeps being charged forever
func TestExhaustedLadderCancelsAndStopsRetrying(t *testing.T) {
	svc, repo, sub, inv := newCollector(t, StatusPastDue, int32(len(dunningBackoff)), errors.New("declined"))

	ok, err := svc.collectOne(context.Background(), inv, time.Now())
	if err != nil {
		t.Fatalf("collectOne: %v", err)
	}
	if ok {
		t.Fatal("a final failure was reported as collected")
	}
	if len(repo.attempts) != 1 || repo.attempts[0].next != nil {
		t.Fatalf("attempts = %+v, want one with NO next attempt", repo.attempts)
	}
	if len(repo.statuses) != 1 || repo.statuses[0].status != StatusCanceled {
		t.Fatalf("status transitions = %+v, want one to canceled", repo.statuses)
	}
	if repo.statuses[0].subID != sub.ID {
		t.Errorf("cancelled the wrong subscription")
	}
}

// The boundary either side of the ladder's end - one before still retries, one past still cancels
func TestLadderBoundary(t *testing.T) {
	last := len(dunningBackoff) - 1

	svc, repo, _, inv := newCollector(t, StatusPastDue, int32(last), errors.New("declined"))
	if _, err := svc.collectOne(context.Background(), inv, time.Now()); err != nil {
		t.Fatal(err)
	}
	if repo.attempts[0].next == nil {
		t.Errorf("the last ladder step cancelled instead of retrying")
	}

	svc2, repo2, _, inv2 := newCollector(t, StatusPastDue, int32(last+5), errors.New("declined"))
	if _, err := svc2.collectOne(context.Background(), inv2, time.Now()); err != nil {
		t.Fatal(err)
	}
	if repo2.attempts[0].next != nil {
		t.Errorf("an attempt count past the ladder still scheduled a retry")
	}
}

// The sweep is a batch job - one customer's dead card must not stop everyone else's renewals
func TestCollectDueContinuesPastAFailure(t *testing.T) {
	good1, bad, good2 := uuid.New(), uuid.New(), uuid.New()
	repo := &sweepRepo{
		subs: map[uuid.UUID]Subscription{},
		queue: []Invoice{
			{ID: uuid.New(), SubscriptionID: good1},
			{ID: uuid.New(), SubscriptionID: bad},
			{ID: uuid.New(), SubscriptionID: good2},
		},
	}
	for _, id := range []uuid.UUID{good1, good2} {
		repo.subs[id] = Subscription{ID: id, Status: StatusActive}
	}
	// `bad` is deliberately absent, so loading it errors mid-sweep

	svc := &Service{repo: repo}
	svc.SetRenewalCharger(&fakeCharger{repo: &repo.collectorRepo})

	collected, err := svc.CollectDue(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("CollectDue: %v", err)
	}
	if collected != 2 {
		t.Errorf("collected %d, want 2 - a single broken subscription aborted the sweep", collected)
	}
}

type sweepRepo struct {
	collectorRepo
	queue []Invoice
}

func (r *sweepRepo) ListChargeableInvoices(context.Context, time.Time, int32) ([]Invoice, error) {
	return r.queue, nil
}

// Without a charger wired the sweep must do nothing rather than mark invoices paid
func TestCollectDueWithoutChargerIsNoop(t *testing.T) {
	repo := &sweepRepo{
		subs:  map[uuid.UUID]Subscription{},
		queue: []Invoice{{ID: uuid.New(), SubscriptionID: uuid.New()}},
	}
	svc := &Service{repo: repo}

	collected, err := svc.CollectDue(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("CollectDue: %v", err)
	}
	if collected != 0 || len(repo.paid) != 0 {
		t.Errorf("collected %d and paid %d with no charger wired, want 0 / 0", collected, len(repo.paid))
	}
}
