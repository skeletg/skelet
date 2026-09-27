package signalsgrpc

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics/spinech"
	"github.com/skeletg/skelet/apps/backend/signals/internal/chtest"
	signalsdb "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/signals/internal/sendlog"
	"github.com/skeletg/skelet/apps/backend/signals/internal/suppression"
)

// The send log is written and read over the contract, so the handlers run against a real database here
// A send lands under the id core minted, an open stamps it when a human read it, and the answer carries the record core reacts to

func sendLogServer(t *testing.T) (signalsv1.IngestServiceClient, signalsv1.PulseServiceClient, signalsv1.SuppressionServiceClient, *sendlog.Store) {
	t.Helper()
	conn := testdb.RequireSignals(t)
	// Every send and open also lands on the spine, and ClickHouse is the only engine there - so this needs a live one, like spinech's own tests
	spine, err := spinech.New(context.Background(), chtest.DSN(t))
	if err != nil {
		t.Fatalf("clickhouse spine: %v", err)
	}
	t.Cleanup(func() { _ = spine.Close() })
	qs := signalsdb.New(conn)
	store := sendlog.NewStore(qs)
	deps := Deps{ServiceSecret: testSecret, Analytics: analytics.NewRecorder(qs, spine), SendLog: store, Suppression: suppression.New(qs)}
	cc := dial(t, deps)
	return signalsv1.NewIngestServiceClient(cc), signalsv1.NewPulseServiceClient(cc), signalsv1.NewSuppressionServiceClient(cc), store
}

// A UA with no machine marker, and a hit well past the prefetch window
const humanUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0 Safari/537.36"

func campaignSend(t *testing.T, ingest signalsv1.IngestServiceClient, store *sendlog.Store) (uuid.UUID, uuid.UUID, uuid.UUID) {
	t.Helper()
	sendID, campaignID, customerID := uuid.New(), uuid.New(), uuid.New()
	t.Cleanup(func() {
		conn := testdb.RequireSignals(t)
		_, _ = conn.Exec(`DELETE FROM email_sends WHERE id = $1`, sendID)
	})
	cid, cust := campaignID.String(), customerID.String()
	if _, err := ingest.RecordEmailSent(systemCtx(t), &signalsv1.RecordEmailSentRequest{Send: &signalsv1.EmailSendRef{
		SendId: sendID.String(), ChannelId: uuid.New().String(), Source: "campaign", Email: "reader@example.test",
		CampaignId: &cid, CustomerId: &cust, Variant: "B",
	}}); err != nil {
		t.Fatalf("record sent: %v", err)
	}
	if _, ok, err := store.Get(context.Background(), sendID); err != nil || !ok {
		t.Fatalf("the send is not in the log under core's id (found=%v, err=%v)", ok, err)
	}
	return sendID, campaignID, customerID
}

func TestACampaignSendLandsInTheLogUnderCoresId(t *testing.T) {
	ingest, _, _, store := sendLogServer(t)
	sendID, campaignID, _ := campaignSend(t, ingest, store)

	send, _, _ := store.Get(context.Background(), sendID)
	if send.CampaignID == nil || *send.CampaignID != campaignID || send.Variant != "B" || send.Email != "reader@example.test" {
		t.Errorf("logged send = %+v, want the campaign, variant and address core sent", send)
	}
}

// A transactional send carries no pixel and is never asked about again, so the spine alone hears of it
func TestATransactionalSendStaysOutOfTheLog(t *testing.T) {
	ingest, _, _, store := sendLogServer(t)
	sendID := uuid.New()
	if _, err := ingest.RecordEmailSent(systemCtx(t), &signalsv1.RecordEmailSentRequest{Send: &signalsv1.EmailSendRef{
		SendId: sendID.String(), ChannelId: uuid.New().String(), Source: "transactional", Email: "receipt@example.test",
	}}); err != nil {
		t.Fatalf("record sent: %v", err)
	}
	if _, ok, _ := store.Get(context.Background(), sendID); ok {
		t.Error("a transactional send was logged")
	}
}

// The suite decides what a human read is - a pixel loaded a minute after the send by a browser is one,
// and stamps the log; the answer carries the record so core can bump its counters and enrol a flow
func TestAHumanOpenStampsTheLogAndAnswersWithTheSend(t *testing.T) {
	ingest, _, _, store := sendLogServer(t)
	sendID, campaignID, customerID := campaignSend(t, ingest, store)
	before, _, _ := store.Get(context.Background(), sendID)

	res, err := ingest.RecordEmailOpen(systemCtx(t), &signalsv1.RecordEmailOpenRequest{
		Send: &signalsv1.EmailSendRef{SendId: sendID.String()},
		Hit:  &signalsv1.EmailHit{UserAgent: humanUA, At: timestamppb.New(before.SentAt.Add(time.Minute))},
	})
	if err != nil {
		t.Fatalf("record open: %v", err)
	}
	if !res.GetRecorded() || res.GetMachine() {
		t.Fatalf("recorded=%v machine=%v, want a recorded human open", res.GetRecorded(), res.GetMachine())
	}
	if res.GetSend().GetCampaignId() != campaignID.String() || res.GetSend().GetCustomerId() != customerID.String() {
		t.Errorf("answer = %+v, want the send's campaign and customer back", res.GetSend())
	}
	after, _, _ := store.Get(context.Background(), sendID)
	if after.FirstOpenAt == nil {
		t.Fatal("first_open_at is unset, so the send reports as never opened")
	}
}

func TestAMachineOpenLeavesTheSendUnread(t *testing.T) {
	ingest, _, _, store := sendLogServer(t)
	sendID, _, _ := campaignSend(t, ingest, store)
	before, _, _ := store.Get(context.Background(), sendID)

	// A load within seconds of the send is a proxy prefetch, whatever the user agent says
	res, err := ingest.RecordEmailOpen(systemCtx(t), &signalsv1.RecordEmailOpenRequest{
		Send: &signalsv1.EmailSendRef{SendId: sendID.String()},
		Hit:  &signalsv1.EmailHit{UserAgent: humanUA, At: timestamppb.New(before.SentAt.Add(time.Second))},
	})
	if err != nil {
		t.Fatalf("record open: %v", err)
	}
	if !res.GetMachine() {
		t.Fatal("an open seconds after the send was classified as human")
	}
	after, _, _ := store.Get(context.Background(), sendID)
	if after.FirstOpenAt != nil {
		t.Error("a machine open stamped the send as read")
	}
}

// A send the log does not hold answers recorded=false rather than an error, so the pixel still serves
func TestAnUnknownSendIsNotAnError(t *testing.T) {
	ingest, _, _, _ := sendLogServer(t)
	res, err := ingest.RecordEmailOpen(systemCtx(t), &signalsv1.RecordEmailOpenRequest{
		Send: &signalsv1.EmailSendRef{SendId: uuid.New().String()},
		Hit:  &signalsv1.EmailHit{UserAgent: humanUA, At: timestamppb.Now()},
	})
	if err != nil {
		t.Fatalf("record open: %v", err)
	}
	if res.GetRecorded() || res.GetSend() != nil {
		t.Errorf("answer = %+v, want recorded=false and no send", res)
	}
}

// The A/B tally groups on the denormalized variant, human hits only
func TestVariantStatsCountHumanEngagementPerArm(t *testing.T) {
	ingest, pulse, _, store := sendLogServer(t)
	sendID, campaignID, _ := campaignSend(t, ingest, store)
	before, _, _ := store.Get(context.Background(), sendID)
	if _, err := ingest.RecordEmailOpen(systemCtx(t), &signalsv1.RecordEmailOpenRequest{
		Send: &signalsv1.EmailSendRef{SendId: sendID.String()},
		Hit:  &signalsv1.EmailHit{UserAgent: humanUA, At: timestamppb.New(before.SentAt.Add(time.Minute))},
	}); err != nil {
		t.Fatalf("record open: %v", err)
	}

	res, err := pulse.GetCampaignVariantStats(systemCtx(t), &signalsv1.GetCampaignVariantStatsRequest{CampaignId: campaignID.String()})
	if err != nil {
		t.Fatalf("variant stats: %v", err)
	}
	if len(res.GetVariants()) != 1 || res.GetVariants()[0].GetVariant() != "B" || res.GetVariants()[0].GetSent() != 1 || res.GetVariants()[0].GetOpened() != 1 {
		t.Errorf("stats = %+v, want one B arm, sent 1, opened 1", res.GetVariants())
	}
}

// The suppression list over the contract - the shapes the send loops depend on
func TestSuppressionRoundTripsOverTheContract(t *testing.T) {
	_, _, sup, _ := sendLogServer(t)
	ctx := systemCtx(t)
	addr := "Bounced-" + uuid.NewString()[:8] + "@Example.test"
	t.Cleanup(func() {
		conn := testdb.RequireSignals(t)
		_, _ = conn.Exec(`DELETE FROM email_suppressions WHERE email = $1`, suppression.Normalize(addr))
	})

	if _, err := sup.Suppress(ctx, &signalsv1.SuppressRequest{Email: addr, Reason: "hard_bounce", Detail: "550"}); err != nil {
		t.Fatalf("suppress: %v", err)
	}
	is, err := sup.IsSuppressed(ctx, &signalsv1.IsSuppressedRequest{Email: addr})
	if err != nil || !is.GetSuppressed() {
		t.Fatalf("suppressed = %v (%v), want true whatever the casing the caller has", is.GetSuppressed(), err)
	}
	set, err := sup.ListSuppressed(ctx, &signalsv1.ListSuppressedRequest{Emails: []string{addr, "clean-" + uuid.NewString()[:8] + "@example.test"}})
	if err != nil || len(set.GetEmails()) != 1 || set.GetEmails()[0] != suppression.Normalize(addr) {
		t.Fatalf("set = %v (%v), want the one suppressed address, normalized", set.GetEmails(), err)
	}
	list, err := sup.ListSuppressions(ctx, &signalsv1.ListSuppressionsRequest{Reason: "hard_bounce", Limit: 200})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var listed bool
	for _, s := range list.GetSuppressions() {
		if s.GetEmail() == suppression.Normalize(addr) && s.GetReason() == "hard_bounce" {
			listed = true
		}
	}
	if !listed {
		t.Error("the suppressed address is not on the hard_bounce list")
	}
	if _, err := sup.Unsuppress(ctx, &signalsv1.UnsuppressRequest{Email: addr}); err != nil {
		t.Fatalf("unsuppress: %v", err)
	}
	if is, _ := sup.IsSuppressed(ctx, &signalsv1.IsSuppressedRequest{Email: addr}); is.GetSuppressed() {
		t.Error("still suppressed after an operator cleared it")
	}
}

// dial serves the given deps over an in-memory listener, torn down with the test
func dial(t *testing.T, deps Deps) *grpc.ClientConn {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := NewServer(deps)
	go func() { _ = srv.Serve(lis) }()
	cc, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = cc.Close(); srv.Stop() })
	return cc
}
