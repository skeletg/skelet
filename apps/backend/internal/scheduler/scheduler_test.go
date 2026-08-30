package scheduler

import (
	"errors"
	"testing"
	"time"

	db "github.com/skeletg/skelet/internal/db/sqlc"
)

func TestParseSchedule(t *testing.T) {
	ok := []string{"@every 15m", "@daily", "@hourly", "0 2 * * *", "*/5 * * * *", "0 9 * * 1"}
	for _, s := range ok {
		if _, err := ParseSchedule(s); err != nil {
			t.Errorf("ParseSchedule(%q) = %v, want ok", s, err)
		}
	}
	bad := []string{"", "every 5m", "not a cron", "99 99 * * *", "@every", "* * *"}
	for _, s := range bad {
		if _, err := ParseSchedule(s); err == nil {
			t.Errorf("ParseSchedule(%q) = ok, want error", s)
		}
	}
}

func TestValidateWebhookURL(t *testing.T) {
	// Blocked (SSRF) when allowPrivate=false
	blocked := []string{
		"http://127.0.0.1/hook",
		"http://localhost/hook",
		"http://169.254.169.254/latest/meta-data/", // cloud metadata
		"http://10.0.0.5/x",
		"http://192.168.1.1/x",
		"http://[::1]/x",
		"ftp://example.com/x", // wrong scheme
		"not-a-url",
	}
	for _, u := range blocked {
		if err := ValidateWebhookURL(u, false); err == nil {
			t.Errorf("ValidateWebhookURL(%q, false) = ok, want blocked", u)
		}
	}
	// Public hosts allowed
	for _, u := range []string{"https://hooks.example.com/x", "https://api.stripe.com/v1/x"} {
		if err := ValidateWebhookURL(u, false); err != nil {
			t.Errorf("ValidateWebhookURL(%q, false) = %v, want ok", u, err)
		}
	}
	// allowPrivate (dev) lets loopback / RFC1918 through...
	for _, u := range []string{"http://127.0.0.1:9000/x", "http://10.0.0.5/x"} {
		if err := ValidateWebhookURL(u, true); err != nil {
			t.Errorf("ValidateWebhookURL(%q, allowPrivate) = %v, want ok", u, err)
		}
	}
	// ...but the cloud-metadata / link-local endpoint stays blocked even then
	if err := ValidateWebhookURL("http://169.254.169.254/latest/meta-data/", true); err == nil {
		t.Errorf("ValidateWebhookURL(metadata, allowPrivate) = ok, want blocked")
	}
}

func TestNextRunInTZ(t *testing.T) {
	from := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)

	// Cron in a zone fires at the local wall-clock time of that zone
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	next, ok := NextRunInTZ("0 2 * * *", "America/New_York", from)
	if !ok {
		t.Fatal("NextRunInTZ(daily 02:00, NY) not ok")
	}
	if inLoc := next.In(loc); inLoc.Hour() != 2 || inLoc.Minute() != 0 {
		t.Errorf("next run in NY = %v, want 02:00 local", inLoc)
	}

	// Same spec in server/UTC time fires at a different instant
	nUTC, ok := NextRunInTZ("0 2 * * *", "", from)
	if !ok {
		t.Fatal("NextRunInTZ(daily 02:00, server) not ok")
	}
	if nUTC.Equal(next) {
		t.Errorf("UTC (%v) and NY (%v) next-run should differ", nUTC, next)
	}

	// @every is timezone-independent - same instant with or without a zone
	e1, _ := NextRunInTZ("@every 1h", "America/New_York", from)
	e2, _ := NextRunInTZ("@every 1h", "", from)
	if !e1.Equal(e2) {
		t.Errorf("@every should be tz-independent: %v vs %v", e1, e2)
	}

	// Unknown zone and bad spec both fail
	if _, ok := NextRunInTZ("0 2 * * *", "Mars/Phobos", from); ok {
		t.Error("invalid timezone should fail")
	}
	if _, ok := NextRunInTZ("not a cron", "", from); ok {
		t.Error("invalid spec should fail")
	}
}

func TestValidateScheduleTZ(t *testing.T) {
	if err := ValidateScheduleTZ("0 2 * * *", "Europe/Paris"); err != nil {
		t.Errorf("valid cron+zone = %v, want ok", err)
	}
	if err := ValidateScheduleTZ("0 2 * * *", "Nope/Nope"); err == nil {
		t.Error("invalid zone should error")
	}
	if err := ValidateScheduleTZ("garbage", ""); err == nil {
		t.Error("invalid spec should error")
	}
}

func TestPreviewRuns(t *testing.T) {
	runs := PreviewRuns("@every 1h", "", 3)
	if len(runs) != 3 {
		t.Fatalf("PreviewRuns count = %d, want 3", len(runs))
	}
	for i := 1; i < len(runs); i++ {
		if !runs[i].After(runs[i-1]) {
			t.Errorf("preview run %d not strictly after previous", i)
		}
	}
	if PreviewRuns("garbage", "", 3) != nil {
		t.Error("invalid schedule should preview nil")
	}
	if got := len(PreviewRuns("@every 1h", "", 0)); got != 5 {
		t.Errorf("count<=0 should default to 5, got %d", got)
	}
}

func TestBackoffDuration(t *testing.T) {
	cases := []struct {
		base, attempt int32
		want          time.Duration
	}{
		{60, 1, 60 * time.Second},  // base
		{60, 2, 120 * time.Second}, // x2
		{60, 3, 240 * time.Second}, // x4
		{10, 1, 10 * time.Second},  // smaller base
		{0, 1, 60 * time.Second},   // base<=0 → default 60
		{3600, 20, maxBackoff},     // overflows the 6h cap
		{60, 30, maxBackoff},       // shift guard still caps
	}
	for _, c := range cases {
		if got := backoffDuration(c.base, c.attempt); got != c.want {
			t.Errorf("backoffDuration(%d, %d) = %v, want %v", c.base, c.attempt, got, c.want)
		}
	}
}

func TestDecideOutcome(t *testing.T) {
	boom := errors.New("boom")
	mk := func(curAttempt, consec, maxAtt, alertAfter int32, alerted bool) db.ScheduledJob {
		return db.ScheduledJob{
			CurrentAttempt:      curAttempt,
			ConsecutiveFailures: consec,
			MaxAttempts:         maxAtt,
			AlertAfterFailures:  alertAfter,
			Alerted:             alerted,
		}
	}
	cases := []struct {
		name    string
		row     db.ScheduledJob
		attempt int32
		err     error
		want    runDecision
	}{
		{
			name:    "success resets all counters",
			row:     mk(2, 3, 3, 2, true),
			attempt: 3,
			err:     nil,
			want:    runDecision{status: "success", nextAttempt: 0, consecFail: 0, alerted: false},
		},
		{
			name:    "failure with attempts left retries soon",
			row:     mk(0, 0, 3, 0, false),
			attempt: 1,
			err:     boom,
			want:    runDecision{status: "error", nextAttempt: 1, consecFail: 1, retrySoon: true},
		},
		{
			name:    "failure exhausted resumes cadence",
			row:     mk(2, 2, 3, 0, false),
			attempt: 3,
			err:     boom,
			want:    runDecision{status: "error", nextAttempt: 0, consecFail: 3, retrySoon: false},
		},
		{
			name:    "parked leaves counters untouched",
			row:     mk(0, 5, 1, 2, true),
			attempt: 1,
			err:     errNoHandler,
			want:    runDecision{status: "error", nextAttempt: 0, consecFail: 5, alerted: true, parked: true},
		},
		{
			name:    "alert fires once at threshold",
			row:     mk(0, 1, 1, 2, false),
			attempt: 1,
			err:     boom,
			want:    runDecision{status: "error", nextAttempt: 0, consecFail: 2, alerted: true, shouldAlert: true},
		},
		{
			name:    "alert latched does not refire",
			row:     mk(0, 5, 1, 2, true),
			attempt: 1,
			err:     boom,
			want:    runDecision{status: "error", nextAttempt: 0, consecFail: 6, alerted: true, shouldAlert: false},
		},
		{
			name:    "below alert threshold does not fire",
			row:     mk(0, 0, 1, 3, false),
			attempt: 1,
			err:     boom,
			want:    runDecision{status: "error", nextAttempt: 0, consecFail: 1, shouldAlert: false},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := decideOutcome(c.row, c.attempt, c.err)
			if got != c.want {
				t.Errorf("decideOutcome = %+v, want %+v", got, c.want)
			}
		})
	}
}
