package subscription

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

type retrySettings string

func (r retrySettings) GetString(context.Context, string, config.Scope) (string, error) {
	return string(r), nil
}

func TestTheRetryScheduleSetting(t *testing.T) {
	for v, want := range map[string]int{"1, 3, 7": 3, "2,5": 2, "0": 0, "": 0} {
		if got, err := parseRetryDays(v); err != nil || len(got) != want {
			t.Errorf("%q: %v, %v", v, got, err)
		}
	}
	for _, bad := range []string{"1,x", "0,3", "61", "1,1,1,1,1,1,1,1,1,1,1", "-2"} {
		if _, err := parseRetryDays(bad); err == nil {
			t.Errorf("%q was taken", bad)
		}
	}
}

func TestFailedRenewalsRetryOnTheChannelsSchedule(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	svc, repo, _, inv := newCollector(t, StatusActive, 1, errors.New("declined"))
	svc.SetSettings(retrySettings("2, 5"))
	if _, err := svc.collectOne(context.Background(), inv, now); err != nil {
		t.Fatal(err)
	}
	if next := repo.attempts[0].next; next == nil || !next.Equal(now.AddDate(0, 0, 5)) {
		t.Errorf("the second retry at %v, want five days on", next)
	}

	svc, repo, _, inv = newCollector(t, StatusActive, 0, errors.New("declined"))
	svc.SetSettings(retrySettings("0"))
	if _, err := svc.collectOne(context.Background(), inv, now); err != nil {
		t.Fatal(err)
	}
	if len(repo.statuses) != 1 || repo.statuses[0].status != StatusCanceled {
		t.Errorf("with no retries: %+v, want cancelled at the first failure", repo.statuses)
	}

	svc, repo, _, inv = newCollector(t, StatusActive, 0, errors.New("declined"))
	svc.SetSettings(retrySettings("  "))
	if _, err := svc.collectOne(context.Background(), inv, now); err != nil {
		t.Fatal(err)
	}
	cancelled := slices.ContainsFunc(repo.statuses, func(st recordedStatus) bool { return st.status == StatusCanceled })
	if cancelled || len(repo.attempts) == 0 || repo.attempts[0].next == nil || !repo.attempts[0].next.Equal(now.AddDate(0, 0, 1)) {
		t.Errorf("a cleared schedule: %+v / %+v, want the default's first retry a day on", repo.statuses, repo.attempts)
	}
}

func TestAFreePeriodIsPaidWithoutACharge(t *testing.T) {
	svc, repo, _, inv := newCollector(t, StatusPastDue, 1, errors.New("a gateway refuses zero"))
	inv.Amount = 0
	ok, err := svc.collectOne(context.Background(), inv, time.Now())
	if !ok || err != nil || len(repo.chargeIn) != 0 || len(repo.paid) != 1 {
		t.Fatalf("ok %v, %v, charged %d, paid %d", ok, err, len(repo.chargeIn), len(repo.paid))
	}
	if len(repo.statuses) != 1 || repo.statuses[0].status != StatusActive {
		t.Errorf("statuses %+v, want the past-due subscription back to active", repo.statuses)
	}
}

func TestARenewingWebhookAnswer(t *testing.T) {
	now := time.Now()
	r := &Renewing{AmountCents: 2500, now: now}
	later := now.Add(48 * time.Hour).UTC().Format(time.RFC3339)
	if rejected, _, err := r.ApplyWebhookResponse([]byte(`{"amountCents": 900, "skip": true, "delayUntil": "` + later + `"}`)); rejected || err != nil {
		t.Fatalf("rejected %v, %v", rejected, err)
	}
	if r.AmountCents != 900 || !r.Skip || r.DelayUntil == nil {
		t.Errorf("applied %+v", r)
	}
	if rejected, _, _ := (&Renewing{now: now}).ApplyWebhookResponse([]byte(`{"reject": true, "reason": "on hold"}`)); !rejected {
		t.Error("a reject didn't leave the period for the next run")
	}
	for _, body := range []string{`{"amountCents": -1}`, `{"delayUntil": "2001-01-01T00:00:00Z"}`} {
		if _, _, err := (&Renewing{now: now}).ApplyWebhookResponse([]byte(body)); err == nil {
			t.Errorf("%s was taken", body)
		}
	}
}

func TestARenewalCostsNoMoreThanTheCeiling(t *testing.T) {
	r := &Renewing{AmountCents: 100}
	if err := r.SetRenewalAmount(MaxRenewalCents + 1); err == nil || r.AmountCents != 100 {
		t.Errorf("a period priced past the ceiling: %v, %d", err, r.AmountCents)
	}
}
