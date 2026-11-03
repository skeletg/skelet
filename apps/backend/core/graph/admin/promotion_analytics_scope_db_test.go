package admin

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	adminpkg "github.com/skeletg/skelet/apps/backend/core/internal/admin"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

func TestPromotionAnalyticsCountOnlyTheAdminsChannels(t *testing.T) {
	f := newEscalationFixture(t)
	conn := testdb.Require(t)
	own := f.channel
	var other uuid.UUID
	if err := conn.QueryRow(`INSERT INTO channels (slug, name, currency_code, default_locale)
		SELECT $1, $1, currency_code, default_locale FROM channels WHERE id = $2 RETURNING id`, "pa-"+uuid.NewString()[:8], own).Scan(&other); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM channels WHERE id = $1`, other) })
	rule := func(ch *uuid.UUID) uuid.UUID {
		var id uuid.UUID
		if err := conn.QueryRow(`INSERT INTO promotion_rules (slug, name, channel_id) VALUES ($1, $1, $2) RETURNING id`, "pa-"+uuid.NewString()[:8], ch).Scan(&id); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM promotion_rules WHERE id = $1`, id) })
		return id
	}
	redeem := func(ruleID, ch uuid.UUID) {
		var order uuid.UUID
		if err := conn.QueryRow(`INSERT INTO orders (channel_id, currency_code) VALUES ($1, 'USD') RETURNING id`, ch).Scan(&order); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM orders WHERE id = $1`, order) })
		if _, err := conn.Exec(`INSERT INTO promotion_redemptions (rule_id, order_id, discount_cents) VALUES ($1, $2, 500)`, ruleID, order); err != nil {
			t.Fatal(err)
		}
	}
	global := rule(nil)
	theirs := rule(&other)
	redeem(global, own)
	redeem(global, other)
	redeem(theirs, other)

	q := &queryResolver{&Resolver{AdminSvc: f.svc, PromotionSvc: promotion.NewService(promotion.NewRepository(db.New(conn))), Queries: db.New(conn)}}
	_, ctx := f.admin(t, f.role(t, adminpkg.RolePermissionScopeInput{PermissionSlug: "promotions.view", ChannelID: &own}))

	rows, err := q.PromotionAnalytics(ctx, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.RuleID == theirs.String() {
			t.Error("a channel-scoped admin saw another channel's rule in the analytics")
		}
		if r.RuleID == global.String() && r.RedemptionCount != 1 {
			t.Errorf("the global rule counted %d redemptions, want only the admin's channel's one", r.RedemptionCount)
		}
	}
	tot, err := q.PromotionAnalyticsTotals(ctx, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if tot.RedemptionCount != 1 {
		t.Errorf("totals counted %d redemptions, want only the admin's channel's", tot.RedemptionCount)
	}
	daily, err := q.PromotionAnalyticsDaily(ctx, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var n int32
	for _, d := range daily {
		n += d.RedemptionCount
	}
	if n != 1 {
		t.Errorf("the daily series counted %d redemptions, want only the admin's channel's", n)
	}

	everywhere := auth.WithAdmin(context.Background(), &auth.Claims{AdminID: uuid.New(), IsSuperuser: true})
	rows, err = q.PromotionAnalytics(everywhere, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int32{}
	for _, r := range rows {
		counts[r.RuleID] = r.RedemptionCount
	}
	if counts[global.String()] != 2 || counts[theirs.String()] != 1 {
		t.Errorf("an every-channel grant should count both channels, got global=%d theirs=%d", counts[global.String()], counts[theirs.String()])
	}
	if none, err := q.PromotionSvc.AnalyticsTotals(everywhere, time.Now().AddDate(0, 0, -1), time.Now().Add(time.Hour), promotion.AnalyticsScope{}); err != nil || none.RedemptionCount != 0 {
		t.Errorf("a scope naming no channels should read nothing, got %+v, %v", none, err)
	}
}
