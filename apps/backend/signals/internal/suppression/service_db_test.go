package suppression

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/google/uuid"
	_ "github.com/lib/pq"

	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	dbsqlc "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

// Suppression sits in front of every send, so both failure directions are expensive: a miss mails an address that already bounced or complained and burns sender reputation, and a false positive silently drops a customer's order receipt

func newTestService(t *testing.T) (*Service, *sql.DB) {
	t.Helper()
	conn := testdb.RequireSignals(t)
	return New(dbsqlc.New(conn)), conn
}

// A run-unique address, so a rerun against the same disposable database starts from a clean tally
func addr(t *testing.T, conn *sql.DB, label string) string {
	t.Helper()
	email := label + "-" + uuid.NewString() + "@example.test"
	t.Cleanup(func() {
		if _, err := conn.Exec(`DELETE FROM email_suppressions WHERE email = $1`, Normalize(email)); err != nil {
			t.Logf("cleanup %s: %v", email, err)
		}
	})
	return email
}

func TestNormalizeIsTheStoredForm(t *testing.T) {
	for name, tc := range map[string]struct{ in, want string }{
		"mixed case":         {"Foo@Example.COM", "foo@example.com"},
		"padded":             {"  foo@example.com\t", "foo@example.com"},
		"padded mixed case":  {" FOO@Example.com ", "foo@example.com"},
		"already normalized": {"foo@example.com", "foo@example.com"},
		"blank":              {"   ", ""},
	} {
		t.Run(name, func(t *testing.T) {
			if got := Normalize(tc.in); got != tc.want {
				t.Errorf("Normalize(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestSuppressionIgnoresTheCasingTheCallerHappensToHave(t *testing.T) {
	// Bounce webhooks, the admin form and the send path each carry their own casing for the same
	// mailbox - matching literally would let a suppressed address back through under any of them
	svc, conn := newTestService(t)
	ctx := context.Background()
	email := addr(t, conn, "casing")

	if err := svc.Suppress(ctx, " "+email+" ", ReasonComplaint, "spam report"); err != nil {
		t.Fatalf("Suppress: %v", err)
	}
	for _, spelling := range []string{email, Normalize(email), " " + email + " "} {
		if !svc.IsSuppressed(ctx, spelling) {
			t.Errorf("IsSuppressed(%q) = false; the send path would mail a complained address", spelling)
		}
	}
}

func TestSuppressIsIdempotentAndRefreshesTheReason(t *testing.T) {
	// The deliverability ingest replays, so re-suppressing has to update in place - a second row would
	// trip the unique index and abort the webhook
	svc, conn := newTestService(t)
	ctx := context.Background()
	email := addr(t, conn, "idempotent")

	if err := svc.Suppress(ctx, email, ReasonSoftBounce, "first"); err != nil {
		t.Fatalf("first Suppress: %v", err)
	}
	if err := svc.Suppress(ctx, email, ReasonComplaint, "escalated"); err != nil {
		t.Fatalf("second Suppress: %v", err)
	}

	var rows int
	var reason, detail string
	if err := conn.QueryRow(
		`SELECT count(*), max(reason), max(detail) FROM email_suppressions WHERE email = $1`,
		Normalize(email),
	).Scan(&rows, &reason, &detail); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if rows != 1 {
		t.Errorf("rows = %d, want 1", rows)
	}
	if reason != ReasonComplaint || detail != "escalated" {
		t.Errorf("row = (%q, %q), want the latest reason and detail", reason, detail)
	}
}

func TestBlankAddressesAreNeverStored(t *testing.T) { // an empty recipient is a caller bug, not a mailbox to suppress
	svc, conn := newTestService(t)
	ctx := context.Background()

	if err := svc.Suppress(ctx, "   ", ReasonManual, "oops"); err != nil {
		t.Fatalf("Suppress: %v", err)
	}
	if svc.IsSuppressed(ctx, "") {
		t.Error("the empty address reads as suppressed")
	}
	var rows int
	if err := conn.QueryRow(`SELECT count(*) FROM email_suppressions WHERE email = ''`).Scan(&rows); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if rows != 0 {
		t.Errorf("%d blank-address rows stored", rows)
	}
}

func TestSuppressedSetKeysByWhatTheCallerPassedIn(t *testing.T) {
	// The batch send path indexes this map by its own recipient strings, so returning normalized keys
	// would read as "nobody is suppressed" and mail the whole batch
	svc, conn := newTestService(t)
	ctx := context.Background()
	blocked := addr(t, conn, "blocked")
	clear := addr(t, conn, "clear")

	if err := svc.Suppress(ctx, blocked, ReasonHardBounce, "550"); err != nil {
		t.Fatalf("Suppress: %v", err)
	}
	upper := " " + strings.ToUpper(blocked) + " " // same mailbox as the recipient list happens to carry it
	got := svc.SuppressedSet(ctx, []string{blocked, upper, clear, "", "   "})

	for _, spelling := range []string{blocked, upper} {
		if !got[spelling] {
			t.Errorf("SuppressedSet missed %q; every spelling of one mailbox must map back", spelling)
		}
	}
	if got[clear] {
		t.Errorf("%q reads as suppressed", clear)
	}
	if len(got) != 2 {
		t.Errorf("set = %v, want only the two spellings of the suppressed address", got)
	}
}

func TestSuppressedSetIsEmptyWithoutRealAddresses(t *testing.T) { // no addresses means no query, not a query on an empty array
	svc, _ := newTestService(t)
	if got := svc.SuppressedSet(context.Background(), []string{"", "  "}); len(got) != 0 {
		t.Errorf("set = %v, want empty", got)
	}
}

func TestSoftBouncesTripSuppressionExactlyAtTheThreshold(t *testing.T) {
	// A single soft bounce is a full mailbox or a greylist, not a dead address - suppressing on the
	// first one drops a customer permanently, and never suppressing keeps retrying a dead mailbox
	svc, conn := newTestService(t)
	ctx := context.Background()
	email := addr(t, conn, "soft")
	const threshold = 3

	for bounce := 1; bounce <= threshold; bounce++ {
		nowSuppressed, count, err := svc.RecordSoftBounce(ctx, email, "mailbox full", threshold)
		if err != nil {
			t.Fatalf("bounce %d: %v", bounce, err)
		}
		if count != bounce {
			t.Errorf("bounce %d: tally = %d, want %d", bounce, count, bounce)
		}
		wantSuppressed := bounce >= threshold
		if nowSuppressed != wantSuppressed {
			t.Errorf("bounce %d: suppressed = %v, want %v", bounce, nowSuppressed, wantSuppressed)
		}
		if got := svc.IsSuppressed(ctx, email); got != wantSuppressed {
			t.Errorf("bounce %d: IsSuppressed = %v, want %v", bounce, got, wantSuppressed)
		}
	}
}

func TestASoftBounceThresholdUnderOneStillNeedsABounce(t *testing.T) {
	// An unset config knob arrives as 0 - treating that as "suppress at zero bounces" would blacklist
	// every address the ingest touches, so it clamps to one
	svc, conn := newTestService(t)
	ctx := context.Background()
	email := addr(t, conn, "zero-threshold")

	nowSuppressed, count, err := svc.RecordSoftBounce(ctx, email, "greylisted", 0)
	if err != nil {
		t.Fatalf("RecordSoftBounce: %v", err)
	}
	if !nowSuppressed || count != 1 {
		t.Errorf("got (%v, %d), want (true, 1)", nowSuppressed, count)
	}
}

func TestAHardSuppressionOutlivesLaterSoftBounces(t *testing.T) {
	// Retries against a complained address keep bouncing - letting a soft bounce rewrite the reason
	// would hide the complaint from the operator and lose why the address was blocked
	svc, conn := newTestService(t)
	ctx := context.Background()
	email := addr(t, conn, "hard-then-soft")

	if err := svc.Suppress(ctx, email, ReasonComplaint, "spam report"); err != nil {
		t.Fatalf("Suppress: %v", err)
	}
	nowSuppressed, _, err := svc.RecordSoftBounce(ctx, email, "mailbox full", 99)
	if err != nil {
		t.Fatalf("RecordSoftBounce: %v", err)
	}
	if !nowSuppressed {
		t.Error("a soft bounce lifted a hard suppression")
	}
	var reason string
	if err := conn.QueryRow(`SELECT reason FROM email_suppressions WHERE email = $1`, Normalize(email)).Scan(&reason); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if reason != ReasonComplaint {
		t.Errorf("reason = %q, want the complaint kept", reason)
	}
}

func TestUnsuppressResetsTheTallySoTheAddressStartsFresh(t *testing.T) {
	// An operator clearing a recovered mailbox expects a full new budget - keeping the old tally would
	// re-suppress it on the very next bounce
	svc, conn := newTestService(t)
	ctx := context.Background()
	email := addr(t, conn, "recovered")
	const threshold = 2

	for range threshold {
		if _, _, err := svc.RecordSoftBounce(ctx, email, "mailbox full", threshold); err != nil {
			t.Fatalf("RecordSoftBounce: %v", err)
		}
	}
	if !svc.IsSuppressed(ctx, email) {
		t.Fatal("the address never got suppressed; the rest of this test proves nothing")
	}
	if err := svc.Unsuppress(ctx, email); err != nil {
		t.Fatalf("Unsuppress: %v", err)
	}
	if svc.IsSuppressed(ctx, email) {
		t.Fatal("still suppressed after Unsuppress")
	}

	nowSuppressed, count, err := svc.RecordSoftBounce(ctx, email, "mailbox full", threshold)
	if err != nil {
		t.Fatalf("post-clear RecordSoftBounce: %v", err)
	}
	if count != 1 || nowSuppressed {
		t.Errorf("got (%v, %d), want (false, 1) - the tally carried over", nowSuppressed, count)
	}
}

func TestListPagesActiveRowsAndCountsThemAll(t *testing.T) {
	// The dashboard list is the operator's only view of who is blocked - a total that tracked the page
	// size would hide the tail, and cleared addresses reappearing would look like the clear failed
	svc, conn := newTestService(t)
	ctx := context.Background()
	reason := "test_" + uuid.NewString() // scopes the filter to this run, since the table is global
	t.Cleanup(func() {
		if _, err := conn.Exec(`DELETE FROM email_suppressions WHERE reason = $1`, reason); err != nil {
			t.Logf("cleanup: %v", err)
		}
	})

	const seeded = 60
	if _, err := conn.Exec(
		`INSERT INTO email_suppressions (email, reason) SELECT 'bulk-' || $1 || '-' || g || '@example.test', $1 FROM generate_series(1, $2) g`,
		reason, seeded,
	); err != nil {
		t.Fatalf("seed: %v", err)
	}

	for name, limit := range map[string]int32{"unset": 0, "negative": -1, "over the cap": 500} {
		t.Run(name, func(t *testing.T) {
			rows, total, err := svc.List(ctx, reason, limit, 0)
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if len(rows) != 50 {
				t.Errorf("page = %d rows, want the 50-row default", len(rows))
			}
			if total != seeded {
				t.Errorf("total = %d, want %d", total, seeded)
			}
		})
	}

	rows, total, err := svc.List(ctx, reason, 10, 55)
	if err != nil {
		t.Fatalf("List with an offset: %v", err)
	}
	if len(rows) != 5 || total != seeded {
		t.Errorf("tail page = %d rows of %d, want 5 of %d", len(rows), total, seeded)
	}

	if err := svc.Unsuppress(ctx, rows[0].Email); err != nil {
		t.Fatalf("Unsuppress: %v", err)
	}
	if _, total, err = svc.List(ctx, reason, 50, 0); err != nil {
		t.Fatalf("List after clearing: %v", err)
	}
	if total != seeded-1 {
		t.Errorf("total = %d after clearing one, want %d", total, seeded-1)
	}
}
