package admin

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/inventory"
	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
)

type rulesOnly struct {
	promotion.Repository
	rules []promotion.Rule
}

// Applies the filter the way the SQL does, so the test still proves the tool asks for the right scope
func (f rulesOnly) ListRules(_ context.Context, rf promotion.RuleFilter, _, _ int32) ([]promotion.Rule, int32, error) {
	var out []promotion.Rule
	for _, r := range f.rules {
		if rf.AllChannels || (r.ChannelID == nil && rf.IncludeGlobal) || (r.ChannelID != nil && slices.Contains(rf.ChannelIDs, *r.ChannelID)) {
			out = append(out, r)
		}
	}
	return out, int32(len(out)), nil
}

func TestTheChatListsOnlyItsChannelsPromotions(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	r := &Resolver{PromotionSvc: promotion.NewService(rulesOnly{rules: []promotion.Rule{
		{ID: uuid.New(), Name: "Own sale", ChannelID: &a},
		{ID: uuid.New(), Name: "Their sale", ChannelID: &b},
		{ID: uuid.New(), Name: "Everywhere"},
	}})}
	got := toolListPromotions(context.Background(), r, a)
	if strings.Contains(got, "Their sale") {
		t.Errorf("the chat on channel A listed channel B's rule:\n%s", got)
	}
	if !strings.Contains(got, "Own sale") || !strings.Contains(got, "Everywhere") {
		t.Errorf("the chat dropped a rule that applies on its channel:\n%s", got)
	}
}

func TestLowStockKeepsOnlyTheChannelsWarehouses(t *testing.T) {
	own, foreign := uuid.New(), uuid.New()
	levels := []inventory.LowStockLevel{{ProductSKU: "OWN", LocationID: &own}, {ProductSKU: "FOREIGN", LocationID: &foreign}, {ProductSKU: "UNSTOCKED"}}
	got := channelLowStock(levels, []inventory.Location{{ID: own}})
	if len(got) != 1 || got[0].ProductSKU != "OWN" {
		t.Fatalf("kept %v, want only the channel's own warehouse", got)
	}
}
