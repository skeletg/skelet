package shipping

import (
	"testing"
	"time"
)

func TestEstimatedArrival(t *testing.T) {
	day := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 15, 30, 0, 0, time.UTC) }
	fri, sat, mon := day(2026, time.October, 2), day(2026, time.October, 3), day(2026, time.October, 5)
	cases := []struct {
		name     string
		from     time.Time
		days     int
		weekends bool
		want     string
	}{
		{"friday plus one skips the weekend", fri, 1, false, "2026-10-05"},
		{"friday plus two lands tuesday", fri, 2, false, "2026-10-06"},
		{"friday plus one on a weekend carrier is saturday", fri, 1, true, "2026-10-03"},
		{"friday plus two on a weekend carrier is sunday", fri, 2, true, "2026-10-04"},
		{"an order on saturday counts from monday", sat, 1, false, "2026-10-05"},
		{"five days span one weekend", mon, 5, false, "2026-10-12"},
		{"five weekend days run straight through", mon, 5, true, "2026-10-10"},
		{"ten days span two weekends", fri, 10, false, "2026-10-16"},
		{"a weekday estimate ignores the flag", mon, 3, true, "2026-10-08"},
		{"no days, no estimate", fri, 0, false, ""},
		{"negative days, no estimate", fri, -2, true, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ArrivalDate(c.from, c.days, c.weekends); got != c.want {
				t.Errorf("ArrivalDate(%s, %d, %v) = %q, want %q", c.from.Format("Mon 2006-01-02"), c.days, c.weekends, got, c.want)
			}
		})
	}
}

// The calendar is the one from carries - 23:30 Friday in UTC is already Saturday in Belgrade, so a weekend carrier's 1-day estimate moves from Saturday to Sunday
func TestEstimatedArrivalReadsTheStoreCalendar(t *testing.T) {
	belgrade, err := time.LoadLocation("Europe/Belgrade")
	if err != nil {
		t.Skipf("no tzdata: %v", err)
	}
	lateFriday := time.Date(2026, time.October, 2, 23, 30, 0, 0, time.UTC)

	if got := ArrivalDate(lateFriday, 1, true); got != "2026-10-03" {
		t.Errorf("UTC: got %s, want Saturday", got)
	}
	if got := ArrivalDate(lateFriday.In(belgrade), 1, true); got != "2026-10-04" {
		t.Errorf("Belgrade: got %s, want Sunday - it is already Saturday there", got)
	}
	if got := EstimatedArrival(lateFriday.In(belgrade), 1, true); got.Location() != belgrade {
		t.Errorf("arrival lost the store's location: %v", got.Location())
	}
}
