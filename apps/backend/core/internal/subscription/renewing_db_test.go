package subscription

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/skeletg/skelet/apps/backend/core/internal/events"
)

func withRenewalRule(svc *Service, rule func(*Renewing) error) {
	svc.bus = events.NewBus()
	svc.bus.SubscribeSync(EventRenewing, 100, func(_ context.Context, e events.Event) error { return rule(e.(*Renewing)) })
}

func TestARenewalRuleSetsWhatAPeriodCosts(t *testing.T) {
	svc, _, insert := newLifecycleDB(t)
	ctx, now := context.Background(), time.Now()
	id := insert(StatusActive, 2, now.Add(-time.Minute))
	withRenewalRule(svc, func(r *Renewing) error {
		if r.AmountCents != 2500 || r.Sequence != 1 || r.Quantity != 2 {
			return errors.New("seeded wrong")
		}
		return r.SetRenewalAmount(1999)
	})
	if err := svc.billOne(ctx, id, now); err != nil {
		t.Fatal(err)
	}
	if invs, err := svc.Invoices(ctx, id); err != nil || len(invs) != 1 || invs[0].Amount != 1999 {
		t.Fatalf("invoices %+v, %v - want one of 1999", invs, err)
	}
}

// Two periods behind and the first skipped - one invoice, numbered 1, and the subscription moves past both
func TestASkippedPeriodPassesUnbilled(t *testing.T) {
	svc, _, insert := newLifecycleDB(t)
	ctx, now := context.Background(), time.Now()
	id := insert(StatusActive, 1, now.AddDate(0, -1, 0).Add(-time.Minute))
	calls := 0
	withRenewalRule(svc, func(r *Renewing) error {
		calls++
		if calls == 1 {
			r.SkipRenewal()
		}
		return nil
	})
	if err := svc.billOne(ctx, id, now); err != nil {
		t.Fatal(err)
	}
	invs, err := svc.Invoices(ctx, id)
	if err != nil || len(invs) != 1 || invs[0].Sequence != 1 || !invs[0].PeriodStart.After(now.Add(-2*time.Minute)) {
		t.Fatalf("invoices %+v, %v - want only the second period's, numbered 1", invs, err)
	}
	sub, _ := svc.Get(ctx, id)
	if !sub.NextBillingAt.After(now) {
		t.Errorf("next billing %v, want past both periods", sub.NextBillingAt)
	}
}

func TestADelayedRenewalMovesTheBillingDate(t *testing.T) {
	svc, _, insert := newLifecycleDB(t)
	ctx, now := context.Background(), time.Now()
	id := insert(StatusActive, 1, now.Add(-time.Minute))
	until := now.Add(10 * 24 * time.Hour)
	withRenewalRule(svc, func(r *Renewing) error {
		if r.DelayRenewal(now.Add(-time.Hour)) == nil {
			return errors.New("a delay into the past was taken")
		}
		r.SkipRenewal()
		return r.DelayRenewal(until)
	})
	if err := svc.billOne(ctx, id, now); err != nil {
		t.Fatal(err)
	}
	invs, _ := svc.Invoices(ctx, id)
	sub, _ := svc.Get(ctx, id)
	if len(invs) != 0 || sub.NextBillingAt.Sub(until).Abs() > time.Millisecond || !sub.BillingAnchor.Equal(sub.NextBillingAt) {
		t.Errorf("%d invoices, next billing %v, anchor %v - want none, both at %v", len(invs), sub.NextBillingAt, sub.BillingAnchor, until)
	}
}

func TestAFailingRenewalRuleLeavesThePeriodForTheNextRun(t *testing.T) {
	svc, _, insert := newLifecycleDB(t)
	ctx, now := context.Background(), time.Now()
	id := insert(StatusActive, 1, now.Add(-time.Minute))
	before, _ := svc.Get(ctx, id)
	withRenewalRule(svc, func(*Renewing) error { return errors.New("pricing service down") })
	if err := svc.billOne(ctx, id, now); err == nil {
		t.Fatal("a failing rule billed anyway")
	}
	invs, _ := svc.Invoices(ctx, id)
	after, _ := svc.Get(ctx, id)
	if len(invs) != 0 || !after.NextBillingAt.Equal(before.NextBillingAt) {
		t.Errorf("%d invoices, next billing moved from %v to %v", len(invs), before.NextBillingAt, after.NextBillingAt)
	}
}
