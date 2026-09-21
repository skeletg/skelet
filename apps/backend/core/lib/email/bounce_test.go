package email

import (
	"errors"
	"testing"
)

func TestClassifyBounce(t *testing.T) {
	cases := []struct {
		in        string
		permanent bool
		matched   bool
	}{
		{"550 5.1.1 user unknown", true, true},
		{"smtp: 553 mailbox name not allowed", true, true},
		{"451 4.7.1 greylisted, try again later", false, true},
		{"421 service not available", false, true},
		{"5.1.1 recipient address rejected", true, true},
		{"4.2.2 mailbox full", false, true},
		{"dial tcp: i/o timeout", false, false},
		{"connection reset by peer", false, false},
		{"No such user here", true, true},
	}
	for _, c := range cases {
		perm, matched := ClassifyBounce(errors.New(c.in))
		if perm != c.permanent || matched != c.matched {
			t.Errorf("ClassifyBounce(%q) = (perm=%v, matched=%v), want (%v, %v)", c.in, perm, matched, c.permanent, c.matched)
		}
	}
	if perm, matched := ClassifyBounce(nil); perm || matched {
		t.Errorf("ClassifyBounce(nil) = (%v,%v), want (false,false)", perm, matched)
	}
}
