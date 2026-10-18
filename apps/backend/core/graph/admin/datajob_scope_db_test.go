package admin

import (
	"context"
	"testing"

	"github.com/google/uuid"

	adminpkg "github.com/skeletg/skelet/apps/backend/core/internal/admin"
	"github.com/skeletg/skelet/apps/backend/core/internal/dataio"
	"github.com/skeletg/skelet/apps/backend/core/internal/datajob"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

func TestTheJobListShowsOnlyTheAdminsChannelsRuns(t *testing.T) {
	f := newEscalationFixture(t)
	conn := testdb.Require(t)
	own := f.channel
	var other uuid.UUID
	if err := conn.QueryRow(`INSERT INTO channels (slug, name, currency_code, default_locale)
		SELECT $1, $1, currency_code, default_locale FROM channels WHERE id = $2 RETURNING id`, "dj-"+uuid.NewString()[:8], own).Scan(&other); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM channels WHERE id = $1`, other) })
	slug := "w1-" + uuid.NewString()[:8]
	reg := dataio.NewRegistry()
	reg.Register(dataio.Resource{Slug: slug, Label: slug, ViewPermission: "customers.view", Fields: []dataio.Field{{Name: "id", Label: "ID"}},
		NewExporter: func(context.Context, dataio.Opts) (dataio.Exporter, error) { return nil, nil }})
	for _, ch := range []uuid.UUID{own, other} {
		if _, err := conn.Exec(`INSERT INTO data_jobs (direction, resource, status, channel_id) VALUES ('export', $1, 'succeeded', $2)`, slug, ch); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM data_jobs WHERE resource = $1`, slug) })

	q := &queryResolver{&Resolver{AdminSvc: f.svc, DataJobSvc: datajob.New(db.New(conn), reg, nil)}}
	_, ctx := f.admin(t, f.role(t,
		adminpkg.RolePermissionScopeInput{PermissionSlug: "data.view", ChannelID: &own},
		adminpkg.RolePermissionScopeInput{PermissionSlug: "customers.view", ChannelID: &own}))
	list, err := q.DataJobs(ctx, []string{slug}, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 1 || list.TotalCount != 1 {
		t.Fatalf("listed %d jobs (total %d), want only the admin's channel's run", len(list.Items), list.TotalCount)
	}
}

func TestAChannelScopedGrantCannotDriveAnImportOfSharedRows(t *testing.T) {
	f := newEscalationFixture(t)
	own := f.channel
	slug := "w1-" + uuid.NewString()[:8]
	reg := dataio.NewRegistry()
	reg.Register(dataio.Resource{Slug: slug, Label: slug, ViewPermission: "catalog.view", ImportPermission: "catalog.manage", RequireGlobalImport: true,
		Fields:      []dataio.Field{{Name: "sku", Label: "SKU", Identifier: true}},
		NewExporter: func(context.Context, dataio.Opts) (dataio.Exporter, error) { return nil, nil },
		NewImporter: func(context.Context, dataio.Opts) (dataio.Importer, error) { return nil, nil }})
	r := &Resolver{AdminSvc: f.svc, DataJobSvc: datajob.New(nil, reg, nil)}
	_, ctx := f.admin(t, f.role(t,
		adminpkg.RolePermissionScopeInput{PermissionSlug: "catalog.manage", ChannelID: &own}))
	if err := r.requireJobAccess(ctx, datajob.Job{Resource: slug, Direction: dataio.DirectionImport, ChannelID: own}, true); !forbidden(err) {
		t.Errorf("a channel-scoped catalog.manage drove a products-style import = %v, want forbidden", err)
	}
	if err := r.requireJobAccess(ctx, datajob.Job{Resource: slug, Direction: dataio.DirectionExport, ChannelID: own}, false); err != nil {
		t.Errorf("a channel-scoped export read = %v, want allowed", err)
	}
}
