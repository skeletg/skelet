package action

import "strconv"

// Validated arg bag for a handler - values come from JSON so numbers arrive as float64, which is why you go through the accessors and never read raw
type Args struct {
	raw map[string]any
}

func (a Args) String(name string) (string, bool) {
	v, ok := a.raw[name]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

func (a Args) Int(name string) (int64, bool) { // also accepts a numeric string
	v, ok := a.raw[name]
	if !ok {
		return 0, false
	}
	switch x := v.(type) {
	case float64:
		return int64(x), true
	case int:
		return int64(x), true
	case int64:
		return x, true
	case string:
		n, err := strconv.ParseInt(x, 10, 64)
		return n, err == nil
	}
	return 0, false
}

func (a Args) Bool(name string) (bool, bool) {
	v, ok := a.raw[name]
	if !ok {
		return false, false
	}
	b, ok := v.(bool)
	return b, ok
}
