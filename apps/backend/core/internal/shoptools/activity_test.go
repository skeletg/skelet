package shoptools

import (
	"context"
	"encoding/json"
	"testing"
)

func TestAnAgentsCallIsCountedWithWhatItsSearchFound(t *testing.T) {
	bus := &fakeBus{}
	s := New(Deps{GraphQL: searchAnswering(nil, nil), Bus: bus})
	args, _ := json.Marshal(map[string]any{"query": "Wide Fit Boots"})
	if res := s.AgentToolSet(shop, "ShopBot").Call(context.Background(), "search_products", args); res.IsError {
		t.Fatal(res.Content)
	}
	if len(bus.published) != 1 {
		t.Fatalf("published %d events", len(bus.published))
	}
	call, ok := bus.published[0].(*AgentCall)
	if !ok || call.Agent != "ShopBot" || call.Tool != "search_products" || call.ChannelID != shop.ID || len(call.Searches) != 1 ||
		call.Searches[0].Term != "Wide Fit Boots" || call.Searches[0].Results != 0 {
		t.Errorf("agent call = %+v", bus.published[0])
	}
	if s.ToolSet(shop).Call(context.Background(), "search_products", args); len(bus.published) != 1 {
		t.Error("the store's own callers were counted as a shopper's agent")
	}
}
