package promotion_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
)

func TestTheStoreAdvertisesOnlyWhatThisShopperCouldGetNow(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC) // a Wednesday
	defer promotion.PinNowForTest(now)()
	past, future := now.Add(-time.Hour), now.Add(time.Hour)
	repo := &fakeRepo{conditions: map[uuid.UUID][]promotion.Condition{}, contents: map[uuid.UUID]promotion.RuleContent{}}
	rule := func(label string, edit func(*promotion.Rule), conds ...promotion.Condition) uuid.UUID {
		r := promotion.Rule{ID: uuid.New(), Name: label, PublicLabel: label, IsActive: true, Priority: int32(len(repo.rules))}
		if edit != nil {
			edit(&r)
		}
		repo.rules = append(repo.rules, r)
		for i := range conds {
			conds[i].RuleID = r.ID
		}
		repo.conditions[r.ID] = conds
		return r.ID
	}
	when := func(slug string, params any) promotion.Condition {
		raw, _ := json.Marshal(params)
		return promotion.Condition{Type: slug, Params: raw}
	}
	vip := when("core.customer_group_in", map[string]any{"group_slugs": []string{"vip"}})
	weekends := when("core.schedule", map[string]any{"days": []int{6, 7}, "timezone": "UTC"})

	storewide := rule("Storewide", nil)
	rule("Needs a code", func(r *promotion.Rule) { r.RequiresCode = true })
	rule("Over", func(r *promotion.Rule) { r.ValidTo = &past })
	rule("Other store", nil, when("core.channel_in", map[string]any{"slugs": []string{"eu"}}))
	rule("Weekends", nil, weekends)
	rule("VIPs", nil, vip)
	spend := rule("Spend 50", func(r *promotion.Rule) { r.ValidTo = &future }, when("core.cart_subtotal_range", map[string]any{"min_cents": 5000}))
	rule("Unloaded plugin", nil, when("acme.loyalty_tier", map[string]any{}))
	rule("Not on weekends", nil, when("core.not", map[string]any{"condition": weekends}))
	rule("VIPs or weekends", nil, when("core.any_of", map[string]any{"conditions": []promotion.Condition{vip, weekends}}))
	repo.contents[storewide] = promotion.RuleContent{Label: "Partout", Terms: "Sur tout"}

	svc := promotion.NewService(repo)
	got, err := svc.PublicPromotions(context.Background(), promotion.Audience{ChannelID: uuid.New(), ChannelSlug: "us", Locale: "fr"})
	if err != nil {
		t.Fatal(err)
	}
	var labels []string
	for _, p := range got {
		labels = append(labels, p.Label)
	}
	want := []string{"Partout", "Spend 50", "Not on weekends"}
	if len(labels) != len(want) {
		t.Fatalf("advertised %v, want %v", labels, want)
	}
	for i := range want {
		if labels[i] != want[i] {
			t.Fatalf("advertised %v, want %v", labels, want)
		}
	}
	if got[0].Terms != "Sur tout" || got[1].EndsAt == nil || !got[1].EndsAt.Equal(future) || repo.rules[6].ID != spend {
		t.Errorf("first %+v, second %+v", got[0], got[1])
	}

	vipID := uuid.New()
	member, err := svc.PublicPromotions(context.Background(), promotion.Audience{ChannelID: uuid.New(), ChannelSlug: "us", CustomerID: &vipID, CustomerGroupSlug: "vip"})
	if err != nil || len(member) != 5 {
		t.Errorf("a VIP sees %d promotions, %v - want the VIP ones too", len(member), err)
	}
}
