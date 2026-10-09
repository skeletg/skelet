package admin

import (
	"context"

	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	adminpkg "github.com/skeletg/skelet/apps/backend/core/internal/admin"
	"github.com/skeletg/skelet/apps/backend/core/internal/dataio"
	"github.com/skeletg/skelet/apps/backend/core/internal/datajob"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/orderio"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

func scoped(slug string, ch *uuid.UUID) adminpkg.RolePermissionScopeInput {
	return adminpkg.RolePermissionScopeInput{PermissionSlug: slug, ChannelID: ch}
}

// Order lines carry customer emails, so orders.view alone must not reach them on any surface
func TestOrderLinesNeedBothGrantsEverywhere(t *testing.T) {
	f := newEscalationFixture(t)
	conn := testdb.Require(t)
	own := f.channel
	var other uuid.UUID
	if err := conn.QueryRow(`INSERT INTO channels (slug, name, currency_code, default_locale)
		SELECT $1, $1, currency_code, default_locale FROM channels WHERE id = $2 RETURNING id`, "va-"+uuid.NewString()[:8], own).Scan(&other); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM channels WHERE id = $1`, other) })
	reg := dataio.NewRegistry()
	reg.Register(orderio.OrderLineResource(nil))
	const slug = "order_lines"
	if _, err := conn.Exec(`INSERT INTO data_jobs (direction, resource, status, channel_id) VALUES ('export', $1, 'succeeded', $2)`, slug, own); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM data_jobs WHERE resource = $1 AND channel_id = $2`, slug, own) })
	base := &Resolver{AdminSvc: f.svc, DataJobSvc: datajob.New(db.New(conn), reg, nil)}
	q, m := &queryResolver{base}, &mutationResolver{base}
	ownStr := own.String()
	job := datajob.Job{Resource: slug, Direction: dataio.DirectionExport, ChannelID: own}

	listed := func(t *testing.T, ctx context.Context) int {
		t.Helper()
		list, err := q.DataJobs(ctx, []string{slug}, nil, nil, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(list.Items) != int(list.TotalCount) {
			t.Fatalf("listed %d jobs but counted %d", len(list.Items), list.TotalCount)
		}
		return len(list.Items)
	}

	_, ordersOnly := f.admin(t, f.role(t, scoped("data.view", nil), scoped("orders.view", nil)))
	if res, _ := q.DataResources(ordersOnly); hasResource(res, slug) {
		t.Error("orders.view alone listed the order-lines resource")
	}
	if _, err := q.DataResource(ordersOnly, slug); !forbidden(err) {
		t.Errorf("orders.view alone read the resource = %v, want forbidden", err)
	}
	if _, err := m.StartDataExport(ordersOnly, model.StartDataExportInput{Resource: slug, Format: "csv", ChannelID: &ownStr}); !forbidden(err) {
		t.Errorf("orders.view alone started an export = %v, want forbidden", err)
	}
	if err := base.requireJobAccess(ordersOnly, job, false); !forbidden(err) {
		t.Errorf("orders.view alone read an order-lines job = %v, want forbidden", err)
	}
	if n := listed(t, ordersOnly); n != 0 {
		t.Errorf("orders.view alone listed %d order-lines jobs", n)
	}

	_, split := f.admin(t, f.role(t, scoped("data.view", nil), scoped("orders.view", &own), scoped("customers.manage", &other)))
	if n := listed(t, split); n != 0 {
		t.Errorf("grants on disjoint channels listed %d jobs, want none", n)
	}
	if v, err := base.jobVisibility(split, []string{slug}); err != nil || len(v.Pairs)+len(v.AnyChannel) != 0 {
		t.Errorf("grants on disjoint channels made the job filter %+v, %v - want it empty, not the union", v, err)
	}
	if err := base.requireJobAccess(split, job, false); !forbidden(err) {
		t.Errorf("grants on disjoint channels read the job = %v, want forbidden", err)
	}

	_, both := f.admin(t, f.role(t, scoped("data.view", nil), scoped("orders.view", nil), scoped("customers.manage", &own)))
	if res, _ := q.DataResources(both); !hasResource(res, slug) {
		t.Error("both grants did not list the resource")
	}
	if err := base.requireJobAccess(both, job, false); err != nil {
		t.Errorf("both grants on the job's channel = %v, want allowed", err)
	}
	if v, err := base.jobVisibility(both, []string{slug}); err != nil || len(v.AnyChannel) != 0 || len(v.Pairs) != 1 || v.Pairs[0].ChannelID != own {
		t.Errorf("a global orders.view with a scoped customers.manage made the job filter %+v, %v - want only %s", v, err, own)
	}
	if n := listed(t, both); n != 1 {
		t.Errorf("both grants listed %d jobs, want the one on their channel", n)
	}
	if _, err := m.StartDataExport(both, model.StartDataExportInput{Resource: slug, Format: "csv", ChannelID: &ownStr}); forbidden(err) {
		t.Error("both grants were refused the export on their channel")
	}
	otherStr := other.String()
	if _, err := m.StartDataExport(both, model.StartDataExportInput{Resource: slug, Format: "csv", ChannelID: &otherStr}); !forbidden(err) {
		t.Errorf("customers.manage scoped elsewhere started an export on %s = %v, want forbidden", other, err)
	}
}

func hasResource(list []*model.DataResource, slug string) bool {
	for _, r := range list {
		if r.Slug == slug {
			return true
		}
	}
	return false
}
