package admin

import (
	"context"
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

func (f rulesOnly) ListRules(context.Context, int32, int32) ([]promotion.Rule, int32, error) {
	return f.rules, int32(len(f.rules)), nil
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
