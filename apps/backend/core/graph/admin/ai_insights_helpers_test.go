package admin

import (
	"testing"

	"github.com/google/uuid"

	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
)

func TestAIInsightsKeepTheirWindowAndNameTheProducts(t *testing.T) {
	for in, want := range map[int32]int32{0: 30, -4: 30, 7: 7, 400: 365} {
		if got := reportDays(&in); got != want {
			t.Errorf("days %d = %d, want %d", in, got, want)
		}
	}
	if reportDays(nil) != 30 {
		t.Error("no window asked for isn't the 30 day default")
	}
	known, gone := uuid.New(), uuid.New()
	out := aiInsightsOut(30, "EUR", &signalsv1.GetAIInsightsResponse{
		Chatted:  &signalsv1.AICohort{People: 40, Buyers: 6, Orders: 7, RevenueCents: 70000, RevenueOrders: 7, ReturnedOrders: 1},
		Products: []*signalsv1.AIProduct{{ProductId: known.String(), Shown: 9, People: 5, Carted: 2, Ordered: 1}, {ProductId: gone.String(), Shown: 3}},
		Agents:   []*signalsv1.AIAgent{{Agent: "ShopBot", Calls: 12, Searches: 4, EmptySearches: 1, Tools: []*signalsv1.AIToolCount{{Tool: "search_products", Calls: 4}}}},
	}, map[uuid.UUID]string{known: "Linen Shirt"})
	if out.Chatted.Revenue != 70000 || out.DidNotChat == nil || out.DidNotChat.People != 0 || out.CurrencyCode != "EUR" {
		t.Errorf("cohorts = %+v / %+v - want both, the missing one empty", out.Chatted, out.DidNotChat)
	}
	if *out.Products[0].Name != "Linen Shirt" || out.Products[1].Name != nil || out.Products[0].Carted != 2 {
		t.Errorf("products = %+v, %+v - want the known one named and the gone one nameless", out.Products[0], out.Products[1])
	}
	if out.Agents[0].Agent != "ShopBot" || out.Agents[0].Tools[0].Calls != 4 {
		t.Errorf("agents = %+v", out.Agents[0])
	}
}
