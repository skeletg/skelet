package boot

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
	"github.com/skeletg/skelet/apps/backend/signals/internal/assistantlog"
	signalsdb "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/signals/internal/lens"
)

type stitchedSpine struct {
	analytics.SpineStore
	anon map[uuid.UUID][]string
}

func (s stitchedSpine) AnonIDsForCustomers(_ context.Context, ids []uuid.UUID) ([]string, error) {
	var out []string
	for _, id := range ids {
		out = append(out, s.anon[id]...)
	}
	return out, nil
}

func (stitchedSpine) DeleteCustomerIdentities(context.Context, []uuid.UUID) error { return nil }

func TestTheErasureSweepTakesTheGuestChatsWithIt(t *testing.T) {
	t.Setenv("LENS_S3_BUCKET", "")
	t.Setenv("LENS_STORAGE_DIR", "")
	conn := testdb.RequireSignals(t)
	ctx := context.Background()
	qs := signalsdb.New(conn)
	cust, ch, conv := uuid.New(), uuid.New(), uuid.New()
	rec := analytics.NewRecorder(qs, stitchedSpine{anon: map[uuid.UUID][]string{cust: {"guest-" + cust.String()[:8]}}})
	conversations := assistantlog.New(qs)
	if err := wire(RootContext{ctx}, lens.NewService(qs, rec, nil, nil), analytics.NewChannelRoster(conn), ReadQueries{}, rec, conversations); err != nil {
		t.Fatal(err)
	}

	if ok, err := conversations.Record(ctx, signalsdb.RecordAssistantTurnParams{ID: uuid.New(), ConversationID: conv, ChannelID: ch, AnonID: "guest-" + cust.String()[:8],
		Question: "do you ship to Oslo", FollowUps: []string{}, Rounds: json.RawMessage(`[]`), At: time.Now()}); err != nil || !ok {
		t.Fatalf("record = %v, %v", ok, err)
	}
	if err := qs.EnqueueIdentityErasure(ctx, cust); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`UPDATE gdpr_identity_erasures SET requested_at = now() - interval '1 hour' WHERE customer_id = $1`, cust); err != nil {
		t.Fatal(err)
	}
	if err := rec.Maintain(ctx); err != nil {
		t.Fatal(err)
	}
	if _, found, err := conversations.Conversation(ctx, ch, conv); err != nil || found {
		t.Errorf("the guest chat stitched to the erased customer = %v, %v - want it swept with them", found, err)
	}
}
