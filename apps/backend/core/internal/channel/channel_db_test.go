package channel_test

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

func dbService(t *testing.T) (*channel.Service, *sql.DB) {
	t.Helper()
	conn := testdb.Require(t)
	return channel.NewService(channel.NewRepository(db.New(conn), txrunner.New(conn))), conn
}

func uniq(prefix string) string { return prefix + "-" + uuid.NewString()[:8] }

func mustCreate(t *testing.T, svc *channel.Service, in channel.CreateChannelInput) channel.Channel {
	t.Helper()
	if in.Name == "" {
		in.Name = in.Slug
	}
	if in.CurrencyCode == "" {
		in.CurrencyCode = "USD"
	}
	ch, err := svc.Create(context.Background(), in)
	if err != nil {
		t.Fatalf("create %s: %v", in.Slug, err)
	}
	return ch
}

func TestTwoChannelsOnOneHostnameRouteByPort(t *testing.T) {
	svc, _ := dbService(t)
	ctx := context.Background()
	host := uniq("shop") + ".test"
	a := mustCreate(t, svc, channel.CreateChannelInput{Slug: uniq("a"), PublicURL: "http://" + host + ":3000"})
	b := mustCreate(t, svc, channel.CreateChannelInput{Slug: uniq("b"), PublicURL: "http://" + host + ":5174/"})
	c := mustCreate(t, svc, channel.CreateChannelInput{Slug: uniq("c"), PublicURL: "https://" + host + ":443/eu"})
	cases := map[string]uuid.UUID{
		host + ":3000":              a.ID,
		host + ":5174":              b.ID,
		host:                        c.ID,
		host + ":443":               c.ID,
		host + ":8080":              uuid.Nil,
		"SHOP" + host[4:] + ":3000": a.ID,
	}
	for reqHost, want := range cases {
		got, err := svc.GetByStorefrontHost(ctx, channel.NormalizeHost("", reqHost))
		if want == uuid.Nil {
			if !errors.Is(err, channel.ErrChannelNotFound) {
				t.Errorf("%s resolved %v, %v - want nothing", reqHost, got.Slug, err)
			}
			continue
		}
		if err != nil || got.ID != want {
			t.Errorf("%s resolved %v, %v - want %s", reqHost, got.Slug, err, want)
		}
	}
}

func TestTheIPv6LoopbackRoutesWithItsPort(t *testing.T) {
	svc, conn := dbService(t)
	ch := mustCreate(t, svc, channel.CreateChannelInput{Slug: uniq("v6"), PublicURL: "http://[::1]:8080/"})
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM channels WHERE id = $1`, ch.ID) })
	got, err := svc.GetByStorefrontHost(context.Background(), channel.NormalizeHost("", "[::1]:8080"))
	if err != nil || got.ID != ch.ID {
		t.Fatalf("resolved %v, %v", got.Slug, err)
	}
}

func TestAStorefrontHostBelongsToOneChannel(t *testing.T) {
	svc, _ := dbService(t)
	ctx := context.Background()
	host := uniq("dup") + ".test"
	first := mustCreate(t, svc, channel.CreateChannelInput{Slug: uniq("d"), PublicURL: "https://" + host})
	if _, err := svc.Create(ctx, channel.CreateChannelInput{Slug: uniq("d"), Name: "x", CurrencyCode: "USD", PublicURL: "https://" + host + ":443/other"}); !errors.Is(err, channel.ErrStorefrontHostTaken) {
		t.Fatalf("second channel on the same host = %v, want ErrStorefrontHostTaken", err)
	}
	if _, err := svc.Create(ctx, channel.CreateChannelInput{Slug: first.Slug, Name: "x", CurrencyCode: "USD"}); !errors.Is(err, channel.ErrSlugTaken) {
		t.Fatalf("duplicate slug = %v, want ErrSlugTaken", err)
	}
	other := mustCreate(t, svc, channel.CreateChannelInput{Slug: uniq("d")})
	taken := "HTTPS://" + host + "/"
	if _, err := svc.Update(ctx, channel.UpdateChannelInput{ID: other.ID, PublicURL: &taken}); !errors.Is(err, channel.ErrStorefrontHostTaken) {
		t.Fatalf("moving onto a taken host = %v, want ErrStorefrontHostTaken", err)
	}
	free := "https://" + uniq("free") + ".test"
	if got, err := svc.Update(ctx, channel.UpdateChannelInput{ID: other.ID, PublicURL: &free}); err != nil || got.PublicURL != free {
		t.Fatalf("moving onto a free host = %v, %v", got.PublicURL, err)
	}
}

func TestAFailedLocaleWriteRollsTheChannelBack(t *testing.T) {
	svc, _ := dbService(t)
	ctx := context.Background()
	slug := uniq("rb")
	if _, err := svc.Create(ctx, channel.CreateChannelInput{Slug: slug, Name: "rb", CurrencyCode: "USD", Locales: []string{"zz-nope"}}); err == nil {
		t.Fatal("a locale with no row was accepted")
	}
	if _, err := svc.GetBySlug(ctx, slug); !errors.Is(err, channel.ErrChannelNotFound) {
		t.Fatalf("the channel survived its failed locale write: %v", err)
	}
	ch := mustCreate(t, svc, channel.CreateChannelInput{Slug: uniq("rb")})
	name := "renamed"
	if _, err := svc.Update(ctx, channel.UpdateChannelInput{ID: ch.ID, Name: &name, Locales: []string{"zz-nope"}}); err == nil {
		t.Fatal("a locale with no row was accepted")
	}
	got, err := svc.GetByID(ctx, ch.ID)
	if err != nil || got.Name != ch.Name {
		t.Fatalf("name = %q, %v after a failed update, want %q", got.Name, err, ch.Name)
	}
	if !slices.Equal(codes(got), []string{channel.FallbackLocale}) {
		t.Fatalf("locales = %v after a failed update", codes(got))
	}
}

func TestTheCurrencyGuardCountsInsideTheUpdate(t *testing.T) {
	svc, conn := dbService(t)
	ctx := context.Background()
	ch := mustCreate(t, svc, channel.CreateChannelInput{Slug: uniq("cur")})
	var typeID, productID uuid.UUID
	if err := conn.QueryRow(`INSERT INTO catalog_product_types (slug, name) VALUES ($1, 'fx') RETURNING id`, uniq("cur")).Scan(&typeID); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(`INSERT INTO catalog_products (product_type_id, sku, url, kind) VALUES ($1, $2, $2, 'simple') RETURNING id`, typeID, uniq("CUR")).Scan(&productID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(`DELETE FROM catalog_products WHERE id = $1`, productID)
		_, _ = conn.Exec(`DELETE FROM catalog_product_types WHERE id = $1`, typeID)
	})
	if _, err := conn.Exec(`INSERT INTO catalog_product_channel_listings (product_id, channel_id, is_published) VALUES ($1, $2, true)`, productID, ch.ID); err != nil {
		t.Fatal(err)
	}
	eur := "EUR"
	if _, err := svc.Update(ctx, channel.UpdateChannelInput{ID: ch.ID, CurrencyCode: &eur}); !errors.Is(err, channel.ErrStoreCurrencyChangeBlocked) {
		t.Fatalf("currency change over an inheriting listing = %v", err)
	}
	if _, err := conn.Exec(`UPDATE catalog_product_channel_listings SET price = 100 WHERE channel_id = $1`, ch.ID); err != nil {
		t.Fatal(err)
	}
	if got, err := svc.Update(ctx, channel.UpdateChannelInput{ID: ch.ID, CurrencyCode: &eur}); err != nil || got.CurrencyCode != "EUR" {
		t.Fatalf("currency change once priced = %v, %v", got.CurrencyCode, err)
	}
}

func TestTheDefaultFlagMovesOnlyToAnActiveChannel(t *testing.T) {
	svc, conn := dbService(t)
	ctx := context.Background()
	var prev uuid.UUID
	_ = conn.QueryRow(`SELECT id FROM channels WHERE is_default`).Scan(&prev)
	t.Cleanup(func() {
		_, _ = conn.Exec(`UPDATE channels SET is_default = (id = $1)`, prev)
	})
	a := mustCreate(t, svc, channel.CreateChannelInput{Slug: uniq("df")})
	b := mustCreate(t, svc, channel.CreateChannelInput{Slug: uniq("df")})
	off := false
	if _, err := svc.Update(ctx, channel.UpdateChannelInput{ID: b.ID, IsActive: &off}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetDefault(ctx, b.ID); !errors.Is(err, channel.ErrDefaultChannelInactive) {
		t.Fatalf("inactive default = %v", err)
	}
	got, err := svc.SetDefault(ctx, a.ID)
	if err != nil || !got.IsDefault || len(got.Locales) == 0 {
		t.Fatalf("SetDefault = %+v, %v", got, err)
	}
	if _, err := svc.Update(ctx, channel.UpdateChannelInput{ID: a.ID, IsActive: &off}); !errors.Is(err, channel.ErrDeactivateDefaultChannel) {
		t.Fatalf("deactivating the default = %v", err)
	}
	if def, err := svc.GetDefault(ctx); err != nil || def.ID != a.ID {
		t.Fatalf("default = %v, %v", def.Slug, err)
	}
}
