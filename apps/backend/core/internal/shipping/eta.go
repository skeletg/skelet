package shipping

import "time"

// EstimatedArrival is the day a delivery quoted at days lands, counting from the day after from - Saturday and Sunday are skipped unless the carrier delivers on weekends, so a Friday order quoted 2 days lands Tuesday. The zero time means no estimate. from carries the location the calendar is read in
func EstimatedArrival(from time.Time, days int, weekends bool) time.Time {
	if days <= 0 {
		return time.Time{}
	}
	d := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, from.Location())
	for left := days; left > 0; {
		d = d.AddDate(0, 0, 1)
		if weekends || (d.Weekday() != time.Saturday && d.Weekday() != time.Sunday) {
			left--
		}
	}
	return d
}

// ArrivalDate is EstimatedArrival as an ISO date for the storefront, empty when there is no estimate
func ArrivalDate(from time.Time, days int, weekends bool) string {
	d := EstimatedArrival(from, days, weekends)
	if d.IsZero() {
		return ""
	}
	return d.Format(time.DateOnly)
}
