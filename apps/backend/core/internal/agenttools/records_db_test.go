package agenttools

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

// Telling "elsewhere" from "nowhere" would confirm an id exists on a channel the caller can't read
func TestRecordToolsAnswerAlikeForMissingAndForeignIDs(t *testing.T) {
	conn := testdb.Require(t)
	var other, group, cust uuid.UUID
	slug := "rec-" + uuid.NewString()[:8]
	if err := conn.QueryRow(`INSERT INTO channels (slug, name, currency_code, default_locale) VALUES ($1, $1, 'USD', 'en') RETURNING id`, slug).Scan(&other); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM channels WHERE id = $1`, other) })
	if err := conn.QueryRow(`SELECT id FROM customer_groups LIMIT 1`).Scan(&group); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(`INSERT INTO customers (email, password_hash, customer_group_id, channel_id) VALUES ($1, '', $2, $3) RETURNING id`, slug+"@example.com", group, other).Scan(&cust); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM customers WHERE id = $1`, cust) })

	deps := &Deps{Customer: customer.NewService(customer.NewRepository(db.New(conn), nil, txrunner.New(conn)), nil, nil, nil, nil, nil, nil, nil)}
	own := uuid.New()
	foreign := toolCustomer(context.Background(), deps, own, toolArgs{CustomerID: cust.String()})
	missing := toolCustomer(context.Background(), deps, own, toolArgs{CustomerID: uuid.NewString()})
	if foreign != missing || !strings.Contains(missing, "No customer") {
		t.Errorf("a customer on another channel and no customer at all should read the same, got %q and %q", foreign, missing)
	}
}
