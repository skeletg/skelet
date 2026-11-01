package subscription

import (
	"testing"
	"time"
)

func date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// The boundary after `after`, counted from the anchor - a short month clamps only its own boundary, and the next one
// is back on the anchor's day
func TestNextBoundary(t *testing.T) {
	at := func(y int, m time.Month, d, h int) time.Time { return time.Date(y, m, d, h, 30, 0, 0, time.UTC) }
	cases := []struct {
		name          string
		anchor, after time.Time
		unit          string
		count         int32
		want          time.Time
	}{
		{"daily", date(2026, 1, 1), date(2026, 1, 1), "day", 1, date(2026, 1, 2)},
		{"every 3 days", date(2026, 1, 1), date(2026, 1, 1), "day", 3, date(2026, 1, 4)},
		{"weekly", date(2026, 1, 1), date(2026, 1, 1), "week", 1, date(2026, 1, 8)},
		{"biweekly", date(2026, 1, 1), date(2026, 1, 1), "week", 2, date(2026, 1, 15)},
		{"monthly", date(2026, 1, 15), date(2026, 1, 15), "month", 1, date(2026, 2, 15)},
		{"quarterly", date(2026, 1, 15), date(2026, 1, 15), "month", 3, date(2026, 4, 15)},
		{"yearly", date(2026, 1, 1), date(2026, 1, 1), "year", 1, date(2027, 1, 1)},
		{"the 31st clamps to February's end", date(2026, 1, 31), date(2026, 1, 31), "month", 1, date(2026, 2, 28)},
		{"and comes back to the 31st", date(2026, 1, 31), date(2026, 2, 28), "month", 1, date(2026, 3, 31)},
		{"a 30-day month after a 31st", date(2026, 1, 31), date(2026, 3, 31), "month", 1, date(2026, 4, 30)},
		{"a leap February keeps the 29th", date(2028, 1, 31), date(2028, 1, 31), "month", 1, date(2028, 2, 29)},
		{"quarterly from Nov 30 into February", date(2025, 11, 30), date(2025, 11, 30), "month", 3, date(2026, 2, 28)},
		{"quarterly back to the 30th", date(2025, 11, 30), date(2026, 2, 28), "month", 3, date(2026, 5, 30)},
		{"yearly from a leap day", date(2028, 2, 29), date(2028, 2, 29), "year", 1, date(2029, 2, 28)},
		{"yearly back to a leap day", date(2028, 2, 29), date(2031, 2, 28), "year", 1, date(2032, 2, 29)},
		{"the time of day stays", at(2026, 1, 31, 14), at(2026, 1, 31, 14), "month", 1, at(2026, 2, 28, 14)},
		{"before the anchor, the anchor - a trial's first renewal", date(2026, 3, 10), date(2026, 3, 1), "month", 1, date(2026, 3, 10)},
		{"zero count defaults to 1", date(2026, 1, 1), date(2026, 1, 1), "month", 0, date(2026, 2, 1)},
		{"unknown unit falls back to month", date(2026, 1, 1), date(2026, 1, 1), "fortnight", 1, date(2026, 2, 1)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := nextBoundary(c.anchor, c.after, c.unit, c.count); !got.Equal(c.want) {
				t.Fatalf("nextBoundary(%v, %v, %q, %d) = %v, want %v", c.anchor, c.after, c.unit, c.count, got, c.want)
			}
		})
	}
}

// Forty years of a subscription started on the 31st: one boundary every calendar month, each on the 31st or the
// month's last day - chaining from the previous boundary drifted to the 3rd after the first February and lost a month
func TestMonthlyBoundariesNeverDrift(t *testing.T) {
	anchor := date(1986, 1, 31)
	b := anchor
	for i := 1; i <= 40*12; i++ {
		b = nextBoundary(anchor, b, "month", 1)
		want := addMonthsClamped(anchor, i)
		if !b.Equal(want) || b.Day() != min(31, want.AddDate(0, 1, -want.Day()).Day()) {
			t.Fatalf("boundary %d = %v, want %v", i, b, want)
		}
	}
	if !b.Equal(date(2026, 1, 31)) {
		t.Fatalf("after 480 months the schedule is on %v, want 2026-01-31", b)
	}
}

func monthlySub() Subscription {
	return Subscription{
		IntervalUnit:       "month",
		IntervalCount:      1,
		CurrentPeriodStart: date(2026, 1, 1),
		CurrentPeriodEnd:   date(2026, 2, 1),
		NextBillingAt:      date(2026, 2, 1),
		BillingAnchor:      date(2026, 1, 1),
	}
}

// Catching up across a month end mints one period per month, each ending on the 31st or the month's last day
func TestPlanBilling_MonthEndAnchor(t *testing.T) {
	sub := Subscription{
		IntervalUnit:       "month",
		IntervalCount:      1,
		CurrentPeriodStart: date(2026, 1, 31),
		CurrentPeriodEnd:   date(2026, 2, 28),
		NextBillingAt:      date(2026, 2, 28),
		BillingAnchor:      date(2026, 1, 31),
	}
	periods, _, end, next := planBilling(sub, date(2026, 5, 1), 1)
	want := [][2]time.Time{
		{date(2026, 2, 28), date(2026, 3, 31)},
		{date(2026, 3, 31), date(2026, 4, 30)},
		{date(2026, 4, 30), date(2026, 5, 31)},
	}
	if len(periods) != len(want) {
		t.Fatalf("minted %d periods, want %d", len(periods), len(want))
	}
	for i, p := range periods {
		if !p.PeriodStart.Equal(want[i][0]) || !p.PeriodEnd.Equal(want[i][1]) || p.Sequence != int32(i+2) {
			t.Fatalf("period %d = [%v, %v] #%d, want [%v, %v] #%d", i, p.PeriodStart, p.PeriodEnd, p.Sequence, want[i][0], want[i][1], i+2)
		}
	}
	if !end.Equal(date(2026, 5, 31)) || !next.Equal(date(2026, 5, 31)) {
		t.Fatalf("current period ends %v, next billing %v - want both May 31", end, next)
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
