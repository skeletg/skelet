package cart

import (
	"testing"
	"time"
)

func TestAssistedCreditLastsTheWindowFromTheLatestHelp(t *testing.T) {
	now := time.Now()
	recent, stale := now.Add(-AssistedCreditWindow+time.Hour), now.Add(-AssistedCreditWindow-time.Hour)
	for name, tc := range map[string]struct {
		c    Cart
		want string
	}{
		"fresh help":       {Cart{AssistedBy: AssistedAssistant, AssistedAt: &recent}, AssistedAssistant},
		"help gone stale":  {Cart{AssistedBy: AssistedAgent, AssistedAt: &stale}, ""},
		"no time recorded": {Cart{AssistedBy: AssistedAssistant}, ""},
		"nobody helped":    {Cart{}, ""},
	} {
		if got := tc.c.AssistedCredit(now); got != tc.want {
			t.Errorf("%s: %q, want %q", name, got, tc.want)
		}
	}
}
