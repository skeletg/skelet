package admin

import (
	"testing"

	"github.com/google/uuid"

	adminpkg "github.com/skeletg/skelet/apps/backend/core/internal/admin"
	"github.com/skeletg/skelet/apps/backend/core/internal/notification"
)

func TestTheInboxShowsEachCategoryOnlyWhereItsPermissionIsHeld(t *testing.T) {
	f := newEscalationFixture(t)
	a, b := f.channel, uuid.New()
	on := func(slug string, ch *uuid.UUID) adminpkg.RolePermissionScopeInput {
		return adminpkg.RolePermissionScopeInput{PermissionSlug: slug, ChannelID: ch}
	}
	_, ctx := f.admin(t, f.role(t, on("notifications.view", nil), on(notification.PermOrders, &a), on(notification.PermReviews, nil)))
	sc, err := f.r.notifScope(ctx)
	if err != nil {
		t.Fatal(err)
	}
	notice := func(perm string, ch *uuid.UUID) notification.Notification {
		return notification.Notification{Category: "x", RequiredPermission: perm, ChannelID: ch}
	}
	if !sc.CanSee(notice(notification.PermOrders, &a)) {
		t.Error("an order notification on the admin's orders channel was hidden")
	}
	if sc.CanSee(notice(notification.PermOrders, &b)) {
		t.Error("a global notifications.view showed another channel's order notification to an admin whose orders.view covers one channel")
	}
	if !sc.CanSee(notice(notification.PermReviews, &b)) {
		t.Error("a permission held everywhere was hidden on a channel")
	}
	if sc.CanSee(notice(notification.PermInventory, nil)) {
		t.Error("a global notifications.view showed inventory notices without inventory.view")
	}
}
