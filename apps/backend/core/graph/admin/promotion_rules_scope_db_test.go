package admin

import (
	"testing"

	"github.com/google/uuid"

	adminpkg "github.com/skeletg/skelet/apps/backend/core/internal/admin"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

// Scoped in SQL, so a channel admin's pages and total count only their rules - filtering after LIMIT left page 1 empty when other channels' rules sorted first
func TestAScopedAdminPagesThroughOnlyTheirRules(t *testing.T) {
	f := newEscalationFixture(t)
	conn := testdb.Require(t)
	own := f.channel
	var other uuid.UUID
	if err := conn.QueryRow(`INSERT INTO channels (slug, name, currency_code, default_locale)
		SELECT $1, $1, currency_code, default_locale FROM channels WHERE id = $2 RETURNING id`, "pr-"+uuid.NewString()[:8], own).Scan(&other); err != nil {
		t.Fatal(err)
	}
	rule := func(ch *uuid.UUID, priority int) uuid.UUID {
		var id uuid.UUID
		if err := conn.QueryRow(`INSERT INTO promotion_rules (slug, name, channel_id, priority) VALUES ($1, $1, $2, $3) RETURNING id`, "pr-"+uuid.NewString()[:8], ch, priority).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	for range 3 {
		rule(&other, -1000) // sorts ahead of the admin's own
	}
	rule(nil, -1000)
	mine := rule(&own, 0)

	q := &queryResolver{&Resolver{AdminSvc: f.svc, PromotionSvc: promotion.NewService(promotion.NewRepository(db.New(conn)))}}
	_, ctx := f.admin(t, f.role(t, adminpkg.RolePermissionScopeInput{PermissionSlug: "promotions.view", ChannelID: &own}))
	one := int32(1)
	list, err := q.PromotionRules(ctx, &one, nil)
	if err != nil {
		t.Fatal(err)
	}
	if list.TotalCount != 1 || len(list.Items) != 1 || list.Items[0].ID != mine.String() {
		t.Errorf("first page: total %d, %d items - want just the admin's own rule", list.TotalCount, len(list.Items))
	}
}
