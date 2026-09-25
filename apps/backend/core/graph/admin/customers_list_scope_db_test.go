package admin

import (
	"testing"

	"github.com/google/uuid"

	adminpkg "github.com/skeletg/skelet/apps/backend/core/internal/admin"
	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

func TestTheCustomerListReadsOnlyTheAdminsChannels(t *testing.T) {
	f := newEscalationFixture(t)
	conn := testdb.Require(t)
	own := f.channel
	var other uuid.UUID
	if err := conn.QueryRow(`INSERT INTO channels (slug, name, currency_code, default_locale)
		SELECT $1, $1, currency_code, default_locale FROM channels WHERE id = $2 RETURNING id`, "cl-"+uuid.NewString()[:8], own).Scan(&other); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM channels WHERE id = $1`, other) })
	mine, _ := seedChannelCustomer(t, conn, own)
	theirs, _ := seedChannelCustomer(t, conn, other)
	_, ctx := f.admin(t, f.role(t, adminpkg.RolePermissionScopeInput{PermissionSlug: "customers.view", ChannelID: &own}))
	q := &queryResolver{&Resolver{AdminSvc: f.svc, CustomerSvc: customer.NewService(customer.NewRepository(db.New(conn), nil, txrunner.New(conn)), nil, nil, nil, nil, nil, nil, nil)}}

	listed := func(channel *string) map[string]bool {
		got, err := q.Customers(ctx, channel)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]bool{}
		for _, c := range got {
			out[c.ID] = true
		}
		return out
	}
	if got := listed(nil); !got[mine.String()] || got[theirs.String()] {
		t.Errorf("scoped list has own=%v other=%v, want only the admin's channel", got[mine.String()], got[theirs.String()])
	}
	o := other.String()
	if got := listed(&o); len(got) != 0 {
		t.Errorf("naming another channel listed %d customers, want none - an explicit channel only narrows", len(got))
	}
}
