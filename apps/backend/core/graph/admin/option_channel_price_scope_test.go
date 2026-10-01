package admin

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/admin"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func TestOptionChannelPriceNeedsTheChannelAndEveryListing(t *testing.T) {
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	onlyA := func() context.Context {
		return adminWithCachedGrants(auth.Grant{Slug: "catalog.manage", ChannelID: &a})
	}
	both := func() context.Context {
		return adminWithCachedGrants(auth.Grant{Slug: "catalog.manage", ChannelID: &a}, auth.Grant{Slug: "catalog.manage", ChannelID: &b})
	}
	global := func() context.Context { return adminWithCachedGrants(auth.Grant{Slug: "catalog.manage"}) }
	for _, tc := range []struct {
		name    string
		ctx     func() context.Context
		target  uuid.UUID
		listed  []uuid.UUID
		allowed bool
	}{
		{"own channel, only listing", onlyA, a, []uuid.UUID{a}, true},
		{"own channel, product also listed elsewhere", onlyA, a, []uuid.UUID{a, b}, false},
		{"a channel not held", onlyA, b, []uuid.UUID{b}, false},
		{"holding every listing", both, b, []uuid.UUID{a, b}, true},
		{"global grant", global, a, []uuid.UUID{a, b}, true},
		{"a channel the product isn't listed on", global, c, []uuid.UUID{a, b}, false},
	} {
		err := requireOptionChannelPrice(tc.ctx(), (*admin.Service)(nil), tc.target, tc.listed)
		if (err == nil) != tc.allowed {
			t.Errorf("%s: err = %v, want allowed=%v", tc.name, err, tc.allowed)
		}
	}
}

func TestOptionChannelPricesShowOnlyVisibleChannels(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	ctx := adminWithCachedGrants(auth.Grant{Slug: "catalog.view", ChannelID: &a})
	perm, err := auth.LoadPermittedChannels(ctx, (*admin.Service)(nil), "catalog.view")
	if err != nil {
		t.Fatal(err)
	}
	got := toOptionChannelPriceModels([]catalog.OptionChannelPrice{{ChannelID: a, PriceValue: 1}, {ChannelID: b, PriceValue: 2}}, perm)
	if len(got) != 1 || got[0].ChannelID != a.String() {
		t.Fatalf("channel prices = %+v, want only channel %s", got, a)
	}
}
