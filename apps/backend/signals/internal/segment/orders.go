package segment

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// A windowed order condition ("at least 3 orders in the last 90 days") cannot be answered from customer_order_agg, which
// holds lifetime totals - so it is answered by the spine, one read per channel and window, and handed to Compile as an id set

// WindowedOrders is who ordered what from an instant on, per channel - the spine engine in production, a fixture in tests
type WindowedOrders interface {
	CustomerOrdersSince(ctx context.Context, channelID uuid.UUID, since time.Time) ([]CustomerOrderStat, error)
}

type CustomerOrderStat struct {
	CustomerID    uuid.UUID
	OrderCount    int64
	MonetaryCents int64
}

// resolveWindowedConditions answers every windowed condition in the rules, by index - a customer the spine returns no row for
// has zero orders and zero spend in the window, which is why a condition zero satisfies comes back as an exclusion list rather
// than a selection: everyone on the roster is in, except those the spine shows exceeding it
func (s *Service) resolveWindowedConditions(ctx context.Context, rules *Rules) (map[int]Resolved, error) {
	conds := rules.WindowedConditions()
	if len(conds) == 0 {
		return nil, nil
	}
	if s.orders == nil {
		return nil, fmt.Errorf("segment has a windowed order condition but no spine is wired")
	}
	var channels []uuid.UUID
	now := time.Now()
	type window struct {
		days    int
		channel uuid.UUID
	}
	byWindow := map[window][]CustomerOrderStat{} // one spine read per distinct window and channel, shared by every condition on it
	out := make(map[int]Resolved, len(conds))
	for i, c := range conds {
		key := window{c.Days, c.channel()}
		stats, ok := byWindow[key]
		if !ok {
			since := now.Add(-time.Duration(c.Days) * 24 * time.Hour)
			reads := []uuid.UUID{key.channel}
			if key.channel == uuid.Nil {
				if channels == nil {
					var err error
					if channels, err = s.repo.RosterChannels(ctx); err != nil {
						return nil, fmt.Errorf("roster channels: %w", err)
					}
				}
				reads = channels
			}
			for _, ch := range reads {
				rows, err := s.orders.CustomerOrdersSince(ctx, ch, since)
				if err != nil {
					return nil, fmt.Errorf("orders since %s on channel %s: %w", since.Format(time.DateOnly), ch, err)
				}
				stats = append(stats, rows...)
			}
			byWindow[key] = stats
		}
		r := Resolved{Exclude: satisfies(c.Op, 0, c.Value)}
		for _, st := range stats {
			have := float64(st.OrderCount)
			if c.Kind == "total_spent_cents" {
				have = float64(st.MonetaryCents)
			}
			if satisfies(c.Op, have, c.Value) != r.Exclude { // selecting the matches, or excluding the non-matches
				r.IDs = append(r.IDs, st.CustomerID)
			}
		}
		out[i] = r
	}
	return out, nil
}

// The three operators validate() admits for order conditions
func satisfies(op string, have, want float64) bool {
	switch op {
	case "gte":
		return have >= want
	case "lte":
		return have <= want
	case "eq":
		return have == want
	}
	return false
}
