package admin

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

func TestAnAdminCartOrDraftCannotTakeAnotherChannelsCustomer(t *testing.T) {
	conn := testdb.Require(t)
	var own uuid.UUID
	if err := conn.QueryRow(`SELECT id FROM channels ORDER BY created_at LIMIT 1`).Scan(&own); err != nil {
		t.Skipf("no channel: %v", err)
	}
	var other uuid.UUID
	if err := conn.QueryRow(`INSERT INTO channels (slug, name, currency_code, default_locale)
		SELECT $1, $1, currency_code, default_locale FROM channels WHERE id = $2 RETURNING id`, "cc-"+uuid.NewString()[:8], own).Scan(&other); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM channels WHERE id = $1`, other) })
	foreign, _ := seedChannelCustomer(t, conn, other)
	q := db.New(conn)
	r := &mutationResolver{&Resolver{CustomerSvc: customer.NewService(customer.NewRepository(q, nil, txrunner.New(conn)), nil, nil, nil, nil, nil, nil, nil)}}
	ctx := auth.WithAdmin(context.Background(), &auth.Claims{AdminID: uuid.New(), IsSuperuser: true})
	fid := foreign.String()

	wrong := func(err error) bool { return apperr.CodeOf(err) == "CUSTOMER_WRONG_CHANNEL" }
	if _, err := r.AdminCreateCart(ctx, own.String(), &fid); !wrong(err) {
		t.Errorf("a cart on channel A for channel B's customer = %v, want CUSTOMER_WRONG_CHANNEL", err)
	}
	if _, err := r.CreateDraftOrder(ctx, model.CreateDraftOrderInput{ChannelID: own.String(), CustomerID: &fid, Email: "x@example.invalid"}); !wrong(err) {
		t.Errorf("a draft order on channel A for channel B's customer = %v, want CUSTOMER_WRONG_CHANNEL", err)
	}
}
