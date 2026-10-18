package lens

import (
	"testing"
	"time"
)

func TestGzipRoundTrip(t *testing.T) {
	cases := []string{
		"",
		"{}",
		`{"type":0,"childNodes":[{"type":2,"tagName":"html"}]}`,
	}
	for _, in := range cases {
		got := string(gunzipBytes(gzipBytes([]byte(in))))
		if got != in {
			t.Errorf("round-trip mismatch: got %q want %q", got, in)
		}
	}
}

func TestGunzipPassThroughOnNonGzip(t *testing.T) {
	// Defensive: a non-gzip payload (e.g. plain JSON) is returned as-is
	raw := []byte(`{"type":0}`)
	if got := string(gunzipBytes(raw)); got != string(raw) {
		t.Errorf("expected pass-through, got %q", got)
	}
}

func TestClampDepth(t *testing.T) {
	for _, c := range []struct{ in, want int }{{-5, 0}, {0, 0}, {37, 37}, {100, 100}, {150, 100}} {
		if got := clampDepth(c.in); got != c.want {
			t.Errorf("clampDepth(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

// Rollup day keys come from it too, so a missing or far-off tracker clock takes the server's rather than year one or a forged day
func TestMsToTime(t *testing.T) {
	for _, ms := range []int64{0, -1, 1718000000000} {
		if got := msToTime(ms); time.Since(got) > time.Minute {
			t.Errorf("msToTime(%d) = %v, want the server clock", ms, got)
		}
	}
	recent := time.Now().Add(-time.Hour).Truncate(time.Millisecond)
	if got := msToTime(recent.UnixMilli()); !got.Equal(recent) {
		t.Errorf("msToTime(an hour ago) = %v, want %v", got, recent)
	}
}
