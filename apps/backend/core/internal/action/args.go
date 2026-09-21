package action

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
