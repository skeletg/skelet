package campaign

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"

	"github.com/skeletg/skelet/apps/backend/internal/analytics"
	db "github.com/skeletg/skelet/apps/backend/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/internal/emailtrack"
	"github.com/skeletg/skelet/apps/backend/internal/sendlog"
	"github.com/skeletg/skelet/apps/backend/internal/testdb"
)

// Gated on DATABASE_URL (disposable marker) - the send log is core's, so a receipt has to report as read with the analytics recorder switched off

func trackingFixture(t *testing.T) (*Service, *sendlog.Store, sendlog.Send, context.Context) {
	t.Helper()
	conn := testdb.Require(t)
	ctx := context.Background()
	q := db.New(conn)
	store := sendlog.NewStore(q)

	send, err := store.New(ctx, sendlog.Spec{
		ChannelID: uuid.New(), Source: sendlog.SourceTransactional, Email: "reader@example.test",
	})
	if err != nil {
		t.Fatalf("seed send: %v", err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM email_sends WHERE id = $1`, send.ID) })

	rec := analytics.NewRecorder(q, nil)
	rec.SetEnabled(false) // the whole point: no spine, and the send log still has to move
	return &Service{sends: store, recorder: rec, track: emailtrack.NewSigner([]byte("tracking-test-key"))}, store, send, ctx
}

// A UA with no machine marker, opened well past the prefetch window
const humanUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0 Safari/537.36"

func TestOpenStampsTheSendLogWithSignalsOff(t *testing.T) {
	svc, store, send, ctx := trackingFixture(t)
	token := svc.track.Sign(emailtrack.Payload{S: send.ID})

	svc.RecordOpen(ctx, token, analytics.Hit{UA: humanUA, At: send.SentAt.Add(time.Minute)})

	got, ok, err := store.Get(ctx, send.ID)
	if err != nil || !ok {
		t.Fatalf("re-read send: %v (found=%v)", err, ok)
	}
	if got.FirstOpenAt == nil {
		t.Fatal("first_open_at is unset, so the send reports as never opened")
	}
}

func TestAMachineOpenLeavesTheSendUnread(t *testing.T) {
	svc, store, send, ctx := trackingFixture(t)
	token := svc.track.Sign(emailtrack.Payload{S: send.ID})

	// Inside the prefetch window, which is what a scanner opening the pixel for the recipient looks like
	svc.RecordOpen(ctx, token, analytics.Hit{UA: humanUA, At: send.SentAt.Add(time.Second)})

	got, _, err := store.Get(ctx, send.ID)
	if err != nil {
		t.Fatalf("re-read send: %v", err)
	}
	if got.FirstOpenAt != nil {
		t.Error("a machine open stamped first_open_at, which reports the mail as read by a person")
	}
}

func TestClickStampsTheSendLogWithSignalsOff(t *testing.T) {
	svc, store, send, ctx := trackingFixture(t)
	token := svc.track.Sign(emailtrack.Payload{S: send.ID, U: "https://example.test/order", L: 0})

	dest, _ := svc.RecordClick(ctx, token, analytics.Hit{UA: humanUA, At: send.SentAt.Add(time.Minute)})
	if dest != "https://example.test/order" {
		t.Fatalf("redirect destination = %q, want the signed URL", dest)
	}

	got, _, err := store.Get(ctx, send.ID)
	if err != nil {
		t.Fatalf("re-read send: %v", err)
	}
	if got.FirstClickAt == nil {
		t.Fatal("first_click_at is unset, so the send reports as never clicked")
	}
}
