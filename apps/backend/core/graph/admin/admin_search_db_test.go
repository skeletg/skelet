package admin

import (
	"context"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/locale"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

// What this pins: ⌘K finds an order by number, email or the full name it ships to, a customer by email or full name and
// a product by name or SKU - only in the channels and kinds the admin may read, with a typed _ matching itself
func TestAdminSearchFindsWhatTheListsWould(t *testing.T) {
	conn := testdb.Require(t)
	ctx := context.Background()
	sfx := uuid.NewString()[:8]
	exec := func(q string, args ...any) uuid.UUID {
		t.Helper()
		var id uuid.UUID
		if err := conn.QueryRowContext(ctx, q, args...).Scan(&id); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		return id
	}
	chA := exec(`INSERT INTO channels (slug, name, currency_code) VALUES ($1, $1, 'USD') RETURNING id`, "qs-a-"+sfx)
	chB := exec(`INSERT INTO channels (slug, name, currency_code) VALUES ($1, $1, 'USD') RETURNING id`, "qs-b-"+sfx)
	order := func(ch uuid.UUID, number, email, first, last string) uuid.UUID {
		return exec(`INSERT INTO orders (channel_id, currency_code, number, email, shipping_first_name, shipping_last_name)
			VALUES ($1, 'USD', $2, $3, $4, $5) RETURNING id`, ch, number, email, first, last)
	}
	inA := order(chA, "QS"+sfx+"-1", "grace-"+sfx+"@example.test", "Grace", "Hopper"+sfx)
	inB := order(chB, "QS"+sfx+"-2", "kelly-"+sfx+"@example.test", "Grace", "Kelly"+sfx)
	group := exec(`INSERT INTO customer_groups (name, slug) VALUES ($1, $1) RETURNING id`, "qs-"+sfx)
	ada := exec(`INSERT INTO customers (channel_id, email, password_hash, customer_group_id, first_name, last_name)
		VALUES ($1, $2, 'x', $3, 'Ada', $4) RETURNING id`, chA, "ada-"+sfx+"@example.test", group, "Lovelace"+sfx)
	ptype := exec(`INSERT INTO catalog_product_types (slug, name) VALUES ($1, $1) RETURNING id`, "qs-"+sfx)
	mug := exec(`INSERT INTO catalog_products (product_type_id, sku, url, kind) VALUES ($1, $2, $2, 'simple') RETURNING id`, ptype, "QS-"+sfx+"-MUG")
	if _, err := conn.ExecContext(ctx, `INSERT INTO catalog_product_content (product_id, locale, name) VALUES ($1, 'en', $2)`, mug, "Search mug "+sfx); err != nil {
		t.Fatal(err)
	}

	r := &queryResolver{&Resolver{Queries: db.New(conn)}}
	search := func(ctx context.Context, term string) *model.AdminSearchResult {
		t.Helper()
		res, err := r.AdminSearch(locale.WithLocale(ctx, "en"), term)
		if err != nil {
			t.Fatalf("search %q: %v", term, err)
		}
		return res
	}
	orderIDs := func(res *model.AdminSearchResult) []string {
		out := []string{}
		for _, o := range res.Orders {
			out = append(out, o.ID)
		}
		return out
	}
	everything := adminWithCachedGrants(auth.Grant{Slug: "orders.view"}, auth.Grant{Slug: "customers.view"}, auth.Grant{Slug: "catalog.view"})

	for _, term := range []string{"QS" + sfx + "-1", "grace-" + sfx, "Grace Hopper" + sfx} {
		if got := orderIDs(search(everything, term)); len(got) != 1 || got[0] != inA.String() {
			t.Errorf("%q found orders %v, want just %s", term, got, inA)
		}
	}
	if got := search(everything, "Grace Hopper"+sfx).Orders; len(got) != 1 || got[0].Name != "Grace Hopper"+sfx || got[0].Number != "QS"+sfx+"-1" {
		t.Errorf("order row %+v, want its number and the name it ships to", got)
	}
	if got := search(everything, "Ada Lovelace"+sfx).Customers; len(got) != 1 || got[0].ID != ada.String() || got[0].Name != "Ada Lovelace"+sfx {
		t.Errorf("a customer by full name: %+v", got)
	}
	for _, term := range []string{"-" + sfx + "-MUG", "mug " + sfx} {
		if got := search(everything, term).Products; len(got) != 1 || got[0].ID != mug.String() || got[0].Name != "Search mug "+sfx {
			t.Errorf("%q found products %+v, want the mug with its name", term, got)
		}
	}
	// a typed _ is a character, not a wildcard, and a term too short for the trigram indexes finds nothing
	if res := search(everything, "QS"); len(res.Orders)+len(res.Customers)+len(res.Products) != 0 {
		t.Errorf("a two-character term found %+v", res)
	}
	if got := orderIDs(search(everything, "___")); slices.Contains(got, inA.String()) || slices.Contains(got, inB.String()) {
		t.Errorf("___ matched the test orders %v as wildcards", got)
	}

	// An admin reading orders in one channel finds that channel's orders only, and nothing else they can't read
	scopedA := adminWithCachedGrants(auth.Grant{Slug: "orders.view", ChannelID: &chA})
	res := search(scopedA, "Grace")
	if got := orderIDs(res); slices.Contains(got, inB.String()) || !slices.Contains(got, inA.String()) {
		t.Errorf("a channel-A admin found orders %v - want %s and never %s", got, inA, inB)
	}
	if len(res.Customers) != 0 || len(res.Products) != 0 {
		t.Errorf("an admin without customer or catalog access found %+v / %+v", res.Customers, res.Products)
	}
	// With the header on a channel they can't read orders in, that kind drops out rather than failing the search
	onB := channel.WithChannel(scopedA, &channel.Channel{ID: chB})
	if got := search(onB, "Grace").Orders; len(got) != 0 {
		t.Errorf("a channel-A admin scoped to channel B found orders %+v", got)
	}

	if _, err := r.AdminSearch(context.Background(), "Grace"); err == nil {
		t.Error("an anonymous caller searched")
	}
}
