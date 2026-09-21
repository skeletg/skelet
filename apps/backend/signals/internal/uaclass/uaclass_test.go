package uaclass

import (
	"testing"
	"time"
)

func TestClassify(t *testing.T) {
	hour := time.Hour
	cases := []struct {
		name        string
		ua          string
		since       time.Duration
		wantMachine bool
		wantDevice  string
		wantClient  string
	}{
		{"gmail proxy", "Mozilla/5.0 GoogleImageProxy", hour, true, "desktop", "Gmail (proxy)"},
		{"open too soon", "Mozilla/5.0 (Macintosh) Safari", 2 * time.Second, true, "desktop", "Safari"},
		{"scanner", "Mimecast Email Security", hour, true, "desktop", "other"},
		{"human chrome desktop", "Mozilla/5.0 (Windows NT 10.0) Chrome/120", hour, false, "desktop", "Chrome"},
		{"iphone safari", "Mozilla/5.0 (iPhone; CPU iPhone OS 17) Safari", hour, false, "mobile", "Safari"},
		{"empty ua", "", hour, false, "unknown", "unknown"},
	}
	for _, c := range cases {
		got := Classify(c.ua, c.since)
		if got.Machine != c.wantMachine || got.Device != c.wantDevice || got.Client != c.wantClient {
			t.Errorf("%s: Classify=%+v, want machine=%v device=%q client=%q", c.name, got, c.wantMachine, c.wantDevice, c.wantClient)
		}
	}
}
