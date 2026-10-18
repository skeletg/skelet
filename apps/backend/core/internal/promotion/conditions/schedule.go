package conditions

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
)

// Matches when the request clock falls inside the rule's recurring window, from optional `days` (ISO weekday), optional `hours` ([startHour, endHour) pairs) and an IANA `timezone`
// A pair whose start is past its end is an overnight window evaluated against its own start day, both fields set means AND, an empty array matches nothing, and the clock is promotion.NowFunc()
type schedule struct{}

type scheduleParams struct {
	Days     []int   `json:"days,omitempty"`
	Hours    [][]int `json:"hours,omitempty"`
	Timezone string  `json:"timezone,omitempty"`
}

func (schedule) Slug() string  { return "core.schedule" }
func (schedule) Label() string { return "Recurring schedule" }
func (schedule) Description() string {
	return "Matches when the request time falls inside a day-of-week and/or hour-of-day window. Use for happy hour / weekday-only promotions."
}

func (schedule) ParamsSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "days": {
      "type": "array",
      "items": {"type": "integer", "minimum": 1, "maximum": 7},
      "title": "Days (ISO weekday: 1=Mon, 7=Sun)"
    },
    "hours": {
      "type": "array",
      "items": {
        "type": "array",
        "items": {"type": "integer", "minimum": 0, "maximum": 24},
        "minItems": 2,
        "maxItems": 2
      },
      "title": "Hour windows ([startHour, endHour) pairs, 24h clock; start > end wraps past midnight)"
    },
    "timezone": {
      "type": "string",
      "title": "IANA timezone (e.g. Europe/Berlin). Server TZ if blank."
    }
  },
  "additionalProperties": false
}`)
}

func (schedule) ValidateParams(raw json.RawMessage) error {
	var p scheduleParams
	if err := unmarshalParams(raw, &p); err != nil {
		return err
	}
	if len(p.Days) == 0 && len(p.Hours) == 0 {
		return fmt.Errorf("at least one of days or hours must be set")
	}
	for _, d := range p.Days {
		if d < 1 || d > 7 {
			return fmt.Errorf("day %d must be in 1..7", d)
		}
	}
	for i, pair := range p.Hours {
		if len(pair) != 2 {
			return fmt.Errorf("hours[%d] must be [start, end] pair", i)
		}
		s, e := pair[0], pair[1]
		if s < 0 || s > 24 || e < 0 || e > 24 {
			return fmt.Errorf("hours[%d]=[%d,%d]: both bounds must be in 0..24", i, s, e)
		}
		// start > end is the overnight form, so only the degenerate empty window is rejected
		if s == e {
			return fmt.Errorf("hours[%d]=[%d,%d] is an empty window", i, s, e)
		}
	}
	if p.Timezone != "" {
		if _, err := time.LoadLocation(p.Timezone); err != nil {
			return fmt.Errorf("timezone: %w", err)
		}
	}
	return nil
}

// LoadLocation reads tzdata from disk, and this runs on every cart evaluation
var locations sync.Map

func cachedLocation(name string) (*time.Location, error) {
	if l, ok := locations.Load(name); ok {
		return l.(*time.Location), nil
	}
	l, err := time.LoadLocation(name)
	if err != nil {
		return nil, err
	}
	locations.Store(name, l)
	return l, nil
}

func (schedule) Evaluate(ctx context.Context, env promotion.Environment, raw json.RawMessage) (bool, error) {
	var p scheduleParams
	if err := unmarshalParams(raw, &p); err != nil {
		return false, err
	}
	loc := time.Local
	if p.Timezone != "" {
		if l, err := cachedLocation(p.Timezone); err == nil {
			loc = l
		}
	}
	now := promotion.NowFunc().In(loc)
	hour := now.Hour()
	// time.Weekday(): 0=Sun..6=Sat - ISO: 1=Mon..7=Sun - Convert
	today := isoWeekday(now)
	// An overnight window that has wrapped belongs to YESTERDAY's schedule, so 01:00 on
	// Saturday is still inside a Friday 22:00-02:00 window
	yesterday := today - 1
	if yesterday == 0 {
		yesterday = 7
	}

	if len(p.Hours) == 0 {
		return matchesDay(p.Days, today), nil
	}
	for _, pair := range p.Hours {
		start, end := pair[0], pair[1]
		if start < end {
			if hour >= start && hour < end && matchesDay(p.Days, today) {
				return true, nil
			}
			continue
		}
		// Wrapped: the evening leg runs on the window's own day, the morning leg on the next
		if hour >= start && matchesDay(p.Days, today) {
			return true, nil
		}
		if hour < end && matchesDay(p.Days, yesterday) {
			return true, nil
		}
	}
	return false, nil
}

// An empty day list means every day
func matchesDay(days []int, iso int) bool {
	if len(days) == 0 {
		return true
	}
	return slices.Contains(days, iso)
}

func isoWeekday(t time.Time) int {
	if wd := int(t.Weekday()); wd != 0 {
		return wd
	}
	return 7
}

func init() {
	add(schedule{})
}
