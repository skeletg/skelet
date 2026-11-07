package signalsgrpc

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
	"github.com/skeletg/skelet/apps/backend/signals/internal/assistantlog"
	signalsdb "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

type stitched struct {
	analytics.SpineStore
	anon map[uuid.UUID][]string
}

func (s stitched) AnonIDsForCustomers(_ context.Context, ids []uuid.UUID) ([]string, error) {
	var out []string
	for _, id := range ids {
		out = append(out, s.anon[id]...)
	}
	return out, nil
}

func TestAKeptTurnReadsBackAndGoesWithItsCustomerAndTheirGuestChats(t *testing.T) {
	conn := testdb.RequireSignals(t)
	qs := signalsdb.New(conn)
	cust, ch := uuid.New(), uuid.New()
	guest := "guest-" + cust.String()[:8]
	cc := testConnWith(t, Deps{ServiceKeys: testKeys, Analytics: analytics.NewRecorder(qs, stitched{anon: map[uuid.UUID][]string{cust: {guest}}}),
		CustomerRoster: analytics.NewRoster(qs), Channels: analytics.NewChannelRoster(conn), Assistant: assistantlog.New(qs)})
	ingest, reader := signalsv1.NewIngestServiceClient(cc), signalsv1.NewAssistantServiceClient(cc)
	sys, staff := systemCtx(t), tokenCtx(t, grantOn("assistant.view", ch))

	asGuest, asCustomer := uuid.NewString(), uuid.NewString()
	at := time.Now().Add(-time.Minute).UTC().Truncate(time.Microsecond)
	first := &signalsv1.AssistantTurnRecord{Id: uuid.NewString(), ConversationId: asGuest, ChannelId: ch.String(), AnonId: guest, Page: "/en/shirts",
		Question: "a linen shirt for a wedding", Reply: "Here are three.", FollowUps: []string{"in white"}, FoundNothing: true, Model: "m", At: timestamppb.New(at)}
	for i, turn := range []*signalsv1.AssistantTurnRecord{first, first} {
		res, err := ingest.RecordAssistantTurn(sys, &signalsv1.RecordAssistantTurnRequest{Turn: turn})
		if err != nil || res.GetRecorded() != (i == 0) {
			t.Fatalf("record #%d = %v, %v - want kept once", i+1, res.GetRecorded(), err)
		}
	}
	id := cust.String()
	if _, err := ingest.RecordAssistantTurn(sys, &signalsv1.RecordAssistantTurnRequest{Turn: &signalsv1.AssistantTurnRecord{
		Id: uuid.NewString(), ConversationId: asCustomer, ChannelId: ch.String(), CustomerId: &id, Question: "where's my order", At: timestamppb.New(at.Add(time.Hour))}}); err != nil {
		t.Fatal(err)
	}

	got, err := reader.GetAssistantConversation(staff, &signalsv1.GetAssistantConversationRequest{ChannelId: ch.String(), ConversationId: asGuest})
	if err != nil || len(got.GetTurns()) != 1 {
		t.Fatalf("read back = %+v, %v", got, err)
	}
	if tr := got.GetTurns()[0]; tr.GetAnonId() != guest || !tr.GetFoundNothing() || tr.GetUnsure() || tr.GetPage() != "/en/shirts" || !tr.GetAt().AsTime().Equal(at) {
		t.Errorf("turn = %+v - want what was sent, anon id and found-nothing included", tr)
	}
	late, err := reader.GetAssistantConversation(staff, &signalsv1.GetAssistantConversationRequest{ChannelId: ch.String(), ConversationId: asCustomer})
	if err != nil || len(late.GetTurns()) != 1 {
		t.Fatalf("the customer's conversation = %+v, %v", late, err)
	}
	if !late.GetTurns()[0].GetAt().AsTime().Before(time.Now().Add(time.Second)) {
		t.Error("a turn stamped an hour ahead kept its future time")
	}

	if _, err := ingest.EraseCustomerIdentities(sys, &signalsv1.EraseCustomerIdentitiesRequest{CustomerId: id}); err != nil {
		t.Fatal(err)
	}
	for _, conv := range []string{asGuest, asCustomer} {
		if left, err := reader.GetAssistantConversation(staff, &signalsv1.GetAssistantConversationRequest{ChannelId: ch.String(), ConversationId: conv}); err != nil || left.GetConversation() != nil {
			t.Errorf("conversation %s after erasure = %+v, %v", conv, left.GetConversation(), err)
		}
	}
	after, err := ingest.RecordAssistantTurn(sys, &signalsv1.RecordAssistantTurnRequest{Turn: &signalsv1.AssistantTurnRecord{
		Id: uuid.NewString(), ConversationId: asCustomer, ChannelId: ch.String(), CustomerId: &id, Question: "hello again"}})
	if err != nil || after.GetRecorded() {
		t.Errorf("a turn after erasure = %v, %v - want it refused quietly", after.GetRecorded(), err)
	}

	// the stitch is gone with the erasure, so a later guest turn on the same visitor id is a stranger's chat, tied to nobody
	browsing := uuid.NewString()
	if res, err := ingest.RecordAssistantTurn(sys, &signalsv1.RecordAssistantTurnRequest{Turn: &signalsv1.AssistantTurnRecord{
		Id: uuid.NewString(), ConversationId: browsing, ChannelId: ch.String(), AnonId: guest, Question: "just looking"}}); err != nil || !res.GetRecorded() {
		t.Fatalf("a guest turn after erasure = %v, %v", res.GetRecorded(), err)
	}
	again, err := reader.GetAssistantConversation(staff, &signalsv1.GetAssistantConversationRequest{ChannelId: ch.String(), ConversationId: browsing})
	if err != nil || len(again.GetTurns()) != 1 || again.GetTurns()[0].CustomerId != nil || again.GetConversation().CustomerId != nil {
		t.Errorf("the guest's chat after erasure = %+v, %v - want it kept with no customer on it", again, err)
	}
}
