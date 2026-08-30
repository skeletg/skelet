package analytics

import "testing"

func TestAnonymizeIP(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"203.0.113.77", "203.0.113.0"},
		{"203.0.113.77:52110", "203.0.113.0"},
		{"2001:db8:abcd:1234:5678:9abc:def0:1234", "2001:db8:abcd::"},
		{"[2001:db8:abcd:1234::1]:443", "2001:db8:abcd::"},
		{"127.0.0.1", "127.0.0.0"},
		{"not-an-ip", ""},
		{"", ""},
		{"  ", ""},
	}
	for _, c := range cases {
		if got := AnonymizeIP(c.in); got != c.want {
			t.Errorf("AnonymizeIP(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
