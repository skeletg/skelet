package spinech

import (
	"fmt"
	"strings"

	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
)

// filterSQL is the AND of each filter over e.properties, starting " AND ", with its args in the order the SQL binds
// them - the property text is propText, so a value compares the same way in every engine
func filterSQL(fs []analytics.PropertyFilter) (string, []any, error) {
	if len(fs) > analytics.MaxFilters {
		return "", nil, fmt.Errorf("%w: at most %d filters", analytics.ErrBadFilter, analytics.MaxFilters)
	}
	var b strings.Builder
	var args []any
	for _, f := range fs {
		if strings.TrimSpace(f.Key) == "" {
			return "", nil, fmt.Errorf("%w: a filter needs a property", analytics.ErrBadFilter)
		}
		prop, propArgs := propText(f.Key)
		switch f.Op {
		case analytics.FilterEq, "":
			if len(f.Values) != 1 {
				return "", nil, fmt.Errorf("%w: %s = takes one value", analytics.ErrBadFilter, f.Key)
			}
			b.WriteString(" AND " + prop + " = ?")
			args = append(append(args, propArgs...), f.Values[0])
		case analytics.FilterNeq:
			if len(f.Values) != 1 {
				return "", nil, fmt.Errorf("%w: %s ≠ takes one value", analytics.ErrBadFilter, f.Key)
			}
			b.WriteString(" AND (NOT JSONHas(e.properties, ?) OR " + prop + " != ?)")
			args = append(append(append(args, f.Key), propArgs...), f.Values[0])
		case analytics.FilterIn:
			if len(f.Values) == 0 || len(f.Values) > analytics.MaxFilterValues {
				return "", nil, fmt.Errorf("%w: %s is one of 1 to %d values", analytics.ErrBadFilter, f.Key, analytics.MaxFilterValues)
			}
			b.WriteString(" AND " + prop + " IN (" + strings.TrimSuffix(strings.Repeat("?, ", len(f.Values)), ", ") + ")")
			args = append(args, propArgs...)
			for _, v := range f.Values {
				args = append(args, v)
			}
		default:
			return "", nil, fmt.Errorf("%w: unknown filter operator %q", analytics.ErrBadFilter, f.Op)
		}
	}
	return b.String(), args, nil
}

// eventCond is " AND e.event_name IN (...)" for the events asked about (nothing for any event) and the filters after it
func eventCond(events []string, fs []analytics.PropertyFilter) (string, []any, error) {
	cond, args := "", []any(nil)
	if len(events) > 0 {
		cond = " AND e.event_name IN (" + strings.TrimSuffix(strings.Repeat("?, ", len(events)), ", ") + ")"
		for _, e := range events {
			args = append(args, e)
		}
	}
	fc, fa, err := filterSQL(fs)
	if err != nil {
		return "", nil, err
	}
	return cond + fc, append(args, fa...), nil
}
