package pluginerase

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

// A plugin whose system is down for an afternoon still erases: the erasure stays owed, is asked again on a widening
// schedule, and goes only when the plugin answers nil - where riding the event's redelivery it was dropped after an hour
func TestAnErasureStaysOwedUntilThePluginErases(t *testing.T) {
	q := db.New(testdb.Require(t))
	ctx := context.Background()
	sdk.ResetForTest()
	t.Cleanup(sdk.ResetForTest)

	down := true
	var asked []sdk.CustomerDataSubject
	sdk.NewPluginForTest("erp").RegisterCustomerData(sdk.CustomerData{Key: "ledger", Erase: func(_ context.Context, who sdk.CustomerDataSubject) error {
		asked = append(asked, who)
		if down {
			return errors.New("erp: connection refused " + strings.Repeat("é", 600))
		}
		return nil
	}})
	sdk.NewPluginForTest("notes").RegisterCustomerData(sdk.CustomerData{Key: "notes", Erase: func(context.Context, sdk.CustomerDataSubject) error { return nil }})
	sdk.NewPluginForTest("exports-only").RegisterCustomerData(sdk.CustomerData{Key: "x", Export: func(context.Context, sdk.CustomerDataSubject) (any, error) { return nil, nil }})

	customer, channel, now := uuid.New(), uuid.New(), time.Now()
	for range 2 { // the event delivered twice queues each data set once
		if err := Enqueue(ctx, q, customer, channel, "gone@example.test"); err != nil {
			t.Fatal(err)
		}
	}
	queued := func() []db.PluginCustomerErasure {
		t.Helper()
		rows, err := q.ListPluginCustomerErasuresForCustomer(ctx, customer)
		if err != nil {
			t.Fatal(err)
		}
		return rows
	}
	if rows := queued(); len(rows) != 2 {
		t.Fatalf("queued %+v - want erp's and notes', and nothing for a set with no Erase", rows)
	}

	DrainCustomer(ctx, q, customer, now)
	rows := queued()
	if len(rows) != 1 || rows[0].PluginSlug != "erp" || rows[0].Attempts != 1 || !strings.Contains(rows[0].LastError, "connection refused") {
		t.Fatalf("after the first go: %+v - want notes' cleared and erp's kept with its error", rows)
	}
	if len(asked) != 1 || asked[0].CustomerID != customer || asked[0].ChannelID != channel || asked[0].Email != "gone@example.test" {
		t.Errorf("erp was asked about %+v", asked)
	}
	if wait := rows[0].NextAttemptAt.Sub(now); wait < firstRetry-time.Second || wait > firstRetry+time.Second {
		t.Errorf("asked again in %s, want %s", wait, firstRetry)
	}

	// Not due yet - the job leaves it, and says something is owed
	if err := Drain(ctx, q, now.Add(time.Minute)); err == nil || len(asked) != 1 {
		t.Errorf("a pass before it's due: %v after %d asks - want it left alone and reported", err, len(asked))
	}
	if err := Drain(ctx, q, now.Add(firstRetry+time.Minute)); err == nil || len(asked) != 2 || queued()[0].Attempts != 2 {
		t.Errorf("a pass once it's due, erp still down: %v after %d asks", err, len(asked))
	}
	// A day later it's back - asked once more, and nothing is owed for this customer
	down = false
	_ = Drain(ctx, q, now.Add(24*time.Hour))
	if rows := queued(); len(rows) != 0 || len(asked) != 3 {
		t.Errorf("once erp answered: %+v after %d asks - want the queue empty", rows, len(asked))
	}
}

func TestRetryWidensToItsLongest(t *testing.T) {
	for failed, want := range map[int32]time.Duration{0: firstRetry, 1: 2 * firstRetry, 3: 8 * firstRetry, 9: longestRetry, 400: longestRetry} {
		if got := retryAfter(failed); got != want {
			t.Errorf("after %d failures before it: %s, want %s", failed, got, want)
		}
	}
}
