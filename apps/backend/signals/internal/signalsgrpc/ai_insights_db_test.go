package signalsgrpc

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
	"github.com/skeletg/skelet/apps/backend/signals/internal/aiactivity"
	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
	signalsdb "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

type aiSpine struct {
	analytics.SpineStore
	currency string
}

func (s *aiSpine) AIImpact(_ context.Context, _ uuid.UUID, _ time.Time, currency string) (analytics.AIImpact, error) {
	s.currency = currency
	return analytics.AIImpact{Chatted: analytics.AICohort{People: 40, Buyers: 6}, DidNotChat: analytics.AICohort{People: 900, Buyers: 18}, AgentOrders: 2, AgentRevenueCents: 9000}, nil
}

func (s *aiSpine) AIProducts(context.Context, uuid.UUID, time.Time, int) ([]analytics.AIProductStat, error) {
	return []analytics.AIProductStat{{ProductID: "p1", Shown: 12, People: 9, Carted: 3, Ordered: 1}}, nil
}

func TestTheAIsActivityIsCountedAndReadBackAsInsights(t *testing.T) {
	conn := testdb.RequireSignals(t)
	qs := signalsdb.New(conn)
	ch := uuid.New()
	spine := &aiSpine{}
	cc := testConnWith(t, Deps{ServiceKeys: testKeys, Analytics: analytics.NewRecorder(qs, spine), AIActivity: aiactivity.New(qs)})
	ingest, reader := signalsv1.NewIngestServiceClient(cc), signalsv1.NewAssistantServiceClient(cc)
	sys := systemCtx(t)

	call := &signalsv1.RecordAIActivityRequest{ChannelId: ch.String(), Source: "agent", Agent: "Claude", Tools: []string{"search_products"},
		Searches: []*signalsv1.AISearch{{Term: "wide fit boots"}}, DedupeKey: "call-" + ch.String()}
	for i := range 2 {
		res, err := ingest.RecordAIActivity(sys, call)
		if err != nil || res.GetRecorded() != (i == 0) {
			t.Fatalf("record #%d = %v, %v - want it counted once", i+1, res.GetRecorded(), err)
		}
	}
	if _, err := ingest.RecordAIActivity(sys, &signalsv1.RecordAIActivityRequest{ChannelId: ch.String(), Source: "robot", DedupeKey: "x"}); status.Code(err) != codes.InvalidArgument {
		t.Errorf("an unknown source = %v, want refused", err)
	}

	if _, err := reader.GetAIInsights(tokenCtx(t, grantOn("lens.view", ch)), &signalsv1.GetAIInsightsRequest{ChannelId: ch.String()}); status.Code(err) != codes.PermissionDenied {
		t.Errorf("without assistant.view = %v, want refused", err)
	}
	got, err := reader.GetAIInsights(tokenCtx(t, grantOn("assistant.view", ch)), &signalsv1.GetAIInsightsRequest{ChannelId: ch.String(), SinceDays: 30, Currency: " eur "})
	if err != nil {
		t.Fatal(err)
	}
	if got.GetChatted().GetBuyers() != 6 || got.GetDidNotChat().GetPeople() != 900 || got.GetAgentOrders() != 2 || spine.currency != "EUR" {
		t.Errorf("cohorts %+v / %+v, agent orders %d, asked in %q", got.GetChatted(), got.GetDidNotChat(), got.GetAgentOrders(), spine.currency)
	}
	if len(got.GetProducts()) != 1 || got.GetProducts()[0].GetCarted() != 3 {
		t.Errorf("products = %+v", got.GetProducts())
	}
	if len(got.GetAgents()) != 1 || got.GetAgents()[0].GetAgent() != "Claude" || got.GetAgents()[0].GetEmptySearches() != 1 || len(got.GetAgents()[0].GetTools()) != 1 {
		t.Errorf("agents = %+v - want Claude with its one empty search", got.GetAgents())
	}
}
