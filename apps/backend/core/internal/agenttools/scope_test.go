package agenttools

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
)

type rulesOnly struct {
	promotion.Repository
	rules  []promotion.Rule
	scopes []promotion.AnalyticsScope
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
	deps := &Deps{Promotion: promotion.NewService(&rulesOnly{rules: []promotion.Rule{
		{ID: uuid.New(), Name: "Own sale", ChannelID: &a},
		{ID: uuid.New(), Name: "Their sale", ChannelID: &b},
		{ID: uuid.New(), Name: "Everywhere"},
	}})}
	got := toolListPromotions(context.Background(), deps, a)
	if strings.Contains(got, "Their sale") {
		t.Errorf("the chat on channel A listed channel B's rule:\n%s", got)
	}
	if !strings.Contains(got, "Own sale") || !strings.Contains(got, "Everywhere") {
		t.Errorf("the chat dropped a rule that applies on its channel:\n%s", got)
	}
}

func (f *rulesOnly) AnalyticsTotals(_ context.Context, _, _ time.Time, scope promotion.AnalyticsScope) (promotion.AnalyticsTotals, error) {
	f.scopes = append(f.scopes, scope)
	return promotion.AnalyticsTotals{RedemptionCount: 3, DiscountTotalCents: 120000}, nil
}

func (f *rulesOnly) AnalyticsByRule(_ context.Context, _, _ time.Time, scope promotion.AnalyticsScope) ([]promotion.RuleAnalytics, error) {
	f.scopes = append(f.scopes, scope)
	return []promotion.RuleAnalytics{{RuleName: "Spring", DiscountTotalCents: 5000, OrderRevenueCents: 90000}}, nil
}

func TestPromotionPerformanceReadsItsOwnChannelInItsCurrency(t *testing.T) {
	cid := uuid.New()
	repo := &rulesOnly{}
	ctx := channel.WithChannel(context.Background(), &channel.Channel{ID: cid, CurrencyCode: "JPY"})
	got := toolPromotionPerformance(ctx, &Deps{Promotion: promotion.NewService(repo)}, cid, time.Now().AddDate(0, 0, -30))
	if len(repo.scopes) != 2 {
		t.Fatalf("want the totals and the by-rule reads, got %d", len(repo.scopes))
	}
	for _, sc := range repo.scopes {
		if sc.All || len(sc.ChannelIDs) != 1 || sc.ChannelIDs[0] != cid {
			t.Errorf("the tool must read only its own channel, asked for %+v", sc)
		}
	}
	for _, want := range []string{"JPY 120000", "JPY 5000", "JPY 90000"} {
		if !strings.Contains(got, want) {
			t.Errorf("yen have no minor unit, so %q should appear in:\n%s", want, got)
		}
	}
}
