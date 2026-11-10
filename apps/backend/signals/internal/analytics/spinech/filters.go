package spinech

import (
	"strings"

	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
)

// filterSQL is the AND of each filter over e.properties, starting " AND ", with its args in the order the SQL binds
// them - the property text is propText, so a value compares the same way in every engine
func filterSQL(fs []analytics.PropertyFilter) (string, []any, error) {
	if err := analytics.ValidateFilters(fs); err != nil {
		return "", nil, err
	}
	var b strings.Builder
	var args []any
	for _, f := range fs {
		prop, propArgs := propText(f.Key)
		switch f.Op {
		case analytics.FilterEq:
			b.WriteString(" AND " + prop + " = ?")
			args = append(append(args, propArgs...), f.Values[0])
		case analytics.FilterNeq:
			b.WriteString(" AND (NOT JSONHas(e.properties, ?) OR " + prop + " != ?)")
			args = append(append(append(args, f.Key), propArgs...), f.Values[0])
		case analytics.FilterIn:
			b.WriteString(" AND " + prop + " IN (" + strings.TrimSuffix(strings.Repeat("?, ", len(f.Values)), ", ") + ")")
			args = append(args, propArgs...)
			for _, v := range f.Values {
				args = append(args, v)
			}
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

// propText is a property as text, for filters and breakdowns: numbers and bools as written, missing or JSON null as ”.
// It returns its own args because the key binds once per mention
func propText(key string) (string, []any) {
	return "multiIf(JSONType(e.properties, ?) = 'String', JSONExtractString(e.properties, ?), " +
			"JSONType(e.properties, ?) = 'Null', '', JSONExtractRaw(e.properties, ?))",
		[]any{key, key, key, key}
}

// propLabel is propText as a group label, '(none)' where it's empty
func propLabel(key string) (string, []any) {
	sql, args := propText(key)
	return "ifNull(nullIf(" + sql + ", ''), '(none)')", args
}
