package suppression

import "testing"

func TestTruncateBoundsTheStoredBounceReason(t *testing.T) { // SMTP rejections can carry a whole message body back
	for name, tc := range map[string]struct {
		in   string
		n    int
		want string
	}{
		"shorter":      {"550 no", 240, "550 no"},
		"exact length": {"abcde", 5, "abcde"},
		"longer":       {"abcdefghij", 4, "abcd"},
		"zero":         {"abc", 0, ""},
		"mid-rune":     {"550 žž", 5, "550 "},
	} {
		t.Run(name, func(t *testing.T) {
			if got := truncate(tc.in, tc.n); got != tc.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tc.in, tc.n, got, tc.want)
			}
		})
	}
}
