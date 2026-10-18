package subscription

import (
	"testing"
	"time"
)

func date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func TestNextBillingTime(t *testing.T) {
	cases := []struct {
		name  string
		from  time.Time
		unit  string
		count int32
		want  time.Time
	}{
		{"daily", date(2026, 1, 1), "day", 1, date(2026, 1, 2)},
		{"every 3 days", date(2026, 1, 1), "day", 3, date(2026, 1, 4)},
		{"weekly", date(2026, 1, 1), "week", 1, date(2026, 1, 8)},
		{"biweekly", date(2026, 1, 1), "week", 2, date(2026, 1, 15)},
		{"monthly", date(2026, 1, 15), "month", 1, date(2026, 2, 15)},
		{"quarterly", date(2026, 1, 15), "month", 3, date(2026, 4, 15)},
		{"yearly", date(2026, 1, 1), "year", 1, date(2027, 1, 1)},
		// Go's AddDate normalizes overflow: Jan 31 + 1mo → Mar 3 (no Feb 31)
		{"month-end rollover", date(2026, 1, 31), "month", 1, date(2026, 3, 3)},
		// 2028 is a leap year - Feb 29 exists
		{"leap day", date(2028, 1, 29), "month", 1, date(2028, 2, 29)},
		{"zero count defaults to 1", date(2026, 1, 1), "month", 0, date(2026, 2, 1)},
		{"unknown unit falls back to month", date(2026, 1, 1), "fortnight", 1, date(2026, 2, 1)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := nextBillingTime(c.from, c.unit, c.count)
			if !got.Equal(c.want) {
				t.Fatalf("nextBillingTime(%v, %q, %d) = %v, want %v", c.from, c.unit, c.count, got, c.want)
			}
		})
	}
}

func monthlySub() Subscription {
	return Subscription{
		IntervalUnit:       "month",
		IntervalCount:      1,
		CurrentPeriodStart: date(2026, 1, 1),
		CurrentPeriodEnd:   date(2026, 2, 1),
		NextBillingAt:      date(2026, 2, 1),
	}
}

func TestPlanBilling_NotYetDue(t *testing.T) {
	sub := monthlySub()
	// now is before next_billing_at → nothing to bill
	periods, start, end, next := planBilling(sub, date(2026, 1, 20), 0)
	if len(periods) != 0 {
		t.Fatalf("expected no invoices, got %d", len(periods))
	}
	if !start.Equal(sub.CurrentPeriodStart) || !end.Equal(sub.CurrentPeriodEnd) || !next.Equal(sub.NextBillingAt) {
		t.Fatalf("period should be unchanged when not due")
	}
}

func TestPlanBilling_OnePeriod(t *testing.T) {
	sub := monthlySub()
	// now just past the first boundary → exactly one invoice
	periods, start, end, next := planBilling(sub, date(2026, 2, 2), 0)
	if len(periods) != 1 {
		t.Fatalf("expected 1 invoice, got %d", len(periods))
	}
	p := periods[0]
	if !p.PeriodStart.Equal(date(2026, 2, 1)) || !p.PeriodEnd.Equal(date(2026, 3, 1)) {
		t.Fatalf("invoice period = [%v, %v], want [Feb 1, Mar 1]", p.PeriodStart, p.PeriodEnd)
	}
	if p.Sequence != 1 {
		t.Fatalf("sequence = %d, want 1", p.Sequence)
	}
	if !start.Equal(date(2026, 2, 1)) || !end.Equal(date(2026, 3, 1)) || !next.Equal(date(2026, 3, 1)) {
		t.Fatalf("advanced period = [%v, %v] next %v, want [Feb 1, Mar 1] next Mar 1", start, end, next)
	}
}

func TestPlanBilling_CatchUpMultiplePeriods(t *testing.T) {
	sub := monthlySub()
	// now is ~3.5 months past period start → catch up Feb, Mar, Apr (3 invoices)
	periods, _, end, next := planBilling(sub, date(2026, 4, 15), 0)
	if len(periods) != 3 {
		t.Fatalf("expected 3 catch-up invoices, got %d", len(periods))
	}
	// Sequences are consecutive and periods are contiguous
	for i, p := range periods {
		if p.Sequence != int32(i+1) {
			t.Fatalf("invoice %d sequence = %d, want %d", i, p.Sequence, i+1)
		}
	}
	if !periods[0].PeriodStart.Equal(date(2026, 2, 1)) ||
		!periods[1].PeriodStart.Equal(date(2026, 3, 1)) ||
		!periods[2].PeriodStart.Equal(date(2026, 4, 1)) {
		t.Fatalf("catch-up period starts wrong: %v", periods)
	}
	// After catch-up, next billing is in the future relative to now
	if !next.After(date(2026, 4, 15)) {
		t.Fatalf("next billing %v should be after now", next)
	}
	if !end.Equal(date(2026, 5, 1)) {
		t.Fatalf("final period end = %v, want May 1", end)
	}
}

func TestPlanBilling_SequenceContinuesFromExisting(t *testing.T) {
	sub := monthlySub()
	// Two invoices already on record → next is #3
	periods, _, _, _ := planBilling(sub, date(2026, 2, 2), 2)
	if len(periods) != 1 || periods[0].Sequence != 3 {
		t.Fatalf("expected one invoice with sequence 3, got %+v", periods)
	}
}
