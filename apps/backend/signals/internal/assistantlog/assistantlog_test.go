package assistantlog

import "testing"

func TestASearchNeedsThreeLettersOrDigitsToNarrowAnything(t *testing.T) {
	for raw, want := range map[string]string{
		"ge":        "",
		" a b ":     "",
		"...":       "",
		"1 2":       "",
		"%_%":       "",
		"ab cd":     "ab cd",
		"100%":      `100\%`,
		" čaj ":     "čaj",
		"a_b c":     `a\_b c`,
		"mugs   ok": "mugs   ok",
	} {
		if got := searchTerm(raw); got != want {
			t.Errorf("searchTerm(%q) = %q, want %q", raw, got, want)
		}
	}
}
