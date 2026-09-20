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

func TestMsToTime(t *testing.T) {
	if !msToTime(0).IsZero() {
		t.Error("msToTime(0) should be the zero time (RecordWeb's orNow handles it)")
	}
	if !msToTime(-1).IsZero() {
		t.Error("msToTime(negative) should be the zero time")
	}
	want := time.UnixMilli(1718000000000)
	if got := msToTime(1718000000000); !got.Equal(want) {
		t.Errorf("msToTime = %v, want %v", got, want)
	}
}
