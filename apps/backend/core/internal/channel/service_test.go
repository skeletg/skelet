package channel_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/locale"
)

// Extends the middleware fake with just enough write behavior for Service tests
type svcRepo struct {
	fakeRepo
	localeSets map[uuid.UUID][]string
	inheriting int64
	counted    int
}

func newSvcRepo(channels ...channel.Channel) *svcRepo {
	return &svcRepo{fakeRepo: fakeRepo{channels}, localeSets: map[uuid.UUID][]string{}}
}

func (r *svcRepo) Atomic(_ context.Context, fn func(channel.Repository) error) error { return fn(r) }

func (r *svcRepo) Create(_ context.Context, in channel.CreateChannelInput) (channel.Channel, error) {
	ch := channel.Channel{
		ID:            uuid.New(),
		Slug:          in.Slug,
		Name:          in.Name,
		DefaultLocale: in.DefaultLocale,
		CurrencyCode:  in.CurrencyCode,
		PublicURL:     in.PublicURL,
		IsActive:      true,
	}
	r.channels = append(r.channels, ch)
	return ch, nil
}

func (r *svcRepo) Update(_ context.Context, ch channel.Channel, _ bool) (channel.Channel, error) {
	for i := range r.channels {
		if r.channels[i].ID == ch.ID {
			ch.Locales = nil
			r.channels[i] = ch
			return ch, nil
		}
	}
	return channel.Channel{}, channel.ErrChannelNotFound
}

func (r *svcRepo) SetDefault(_ context.Context, id uuid.UUID) (channel.Channel, []uuid.UUID, error) {
	var out channel.Channel
	var cleared []uuid.UUID
	for i := range r.channels {
		switch {
		case r.channels[i].ID == id:
			r.channels[i].IsDefault = true
			out = r.channels[i]
		case r.channels[i].IsDefault:
			r.channels[i].IsDefault = false
			cleared = append(cleared, r.channels[i].ID)
		}
	}
	return out, cleared, nil
}

func (r *svcRepo) CountInheritingListings(context.Context, uuid.UUID) (int64, error) {
	r.counted++
	return r.inheriting, nil
}

func (r *svcRepo) SetLocales(_ context.Context, id uuid.UUID, codes []string) error {
	r.localeSets[id] = codes
	return nil
}

func (r *svcRepo) ListLocales(_ context.Context, id uuid.UUID) ([]locale.Locale, error) {
	out := make([]locale.Locale, 0, len(r.localeSets[id]))
	for _, c := range r.localeSets[id] {
		out = append(out, locale.Locale{Code: c, IsActive: true})
	}
	return out, nil
}

func TestServiceCreateValidation(t *testing.T) {
	cases := []struct {
		name  string
		input channel.CreateChannelInput
	}{
		{"empty slug", channel.CreateChannelInput{Name: "Store", CurrencyCode: "USD"}},
		{"uppercase slug", channel.CreateChannelInput{Slug: "US", Name: "Store", CurrencyCode: "USD"}},
		{"slug with space", channel.CreateChannelInput{Slug: "us store", Name: "Store", CurrencyCode: "USD"}},
		{"blank name", channel.CreateChannelInput{Slug: "us", Name: "  ", CurrencyCode: "USD"}},
		{"missing currency", channel.CreateChannelInput{Slug: "us", Name: "Store"}},
		{"two-letter currency", channel.CreateChannelInput{Slug: "us", Name: "Store", CurrencyCode: "US"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := channel.NewService(newSvcRepo())
			if _, err := svc.Create(context.Background(), tc.input); err == nil {
				t.Fatalf("Create(%+v) should have been rejected", tc.input)
			}
		})
	}
}

func TestServiceCreateNormalization(t *testing.T) {
	svc := channel.NewService(newSvcRepo())
	ch, err := svc.Create(context.Background(), channel.CreateChannelInput{
		Slug: "eu", Name: "EU Store", CurrencyCode: " eur ", DefaultLocale: "DE",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if ch.CurrencyCode != "EUR" {
		t.Fatalf("currency should normalize to uppercase, got %q", ch.CurrencyCode)
	}
	if ch.DefaultLocale != "de" {
		t.Fatalf("default locale should normalize to lowercase, got %q", ch.DefaultLocale)
	}
	// The supported set seeds from the default so a fresh store serves its own locale
	if len(ch.Locales) != 1 || ch.Locales[0].Code != "de" {
		t.Fatalf("locales should seed from the default, got %+v", ch.Locales)
	}
}

func TestServiceCurrencyChangeGuard(t *testing.T) {
	eur := "EUR"

	setup := func(count int64) (*channel.Service, *svcRepo, uuid.UUID) {
		store := active(usID, "us", "https://shop.example.com", true)
		store.CurrencyCode = "USD"
		repo := newSvcRepo(store)
		repo.inheriting = count
		return channel.NewService(repo), repo, store.ID
	}

	t.Run("blocked while listings inherit price", func(t *testing.T) {
		svc, _, id := setup(3)
		_, err := svc.Update(context.Background(), channel.UpdateChannelInput{ID: id, CurrencyCode: &eur})
		if !errors.Is(err, channel.ErrStoreCurrencyChangeBlocked) {
			t.Fatalf("expected ErrStoreCurrencyChangeBlocked, got %v", err)
		}
	})

	t.Run("allowed once every listing has an explicit price", func(t *testing.T) {
		svc, _, id := setup(0)
		ch, err := svc.Update(context.Background(), channel.UpdateChannelInput{ID: id, CurrencyCode: &eur})
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		if ch.CurrencyCode != "EUR" {
			t.Fatalf("currency not updated, got %q", ch.CurrencyCode)
		}
	})

	t.Run("same currency skips the listing count", func(t *testing.T) {
		svc, counter, id := setup(3)
		usd := "usd" // normalizes to the current USD - not a change
		if _, err := svc.Update(context.Background(), channel.UpdateChannelInput{ID: id, CurrencyCode: &usd}); err != nil {
			t.Fatalf("Update: %v", err)
		}
		if counter.counted != 0 {
			t.Fatalf("counter consulted %d times for a no-op currency write", counter.counted)
		}
	})

	t.Run("invalid currency rejected before any repo write", func(t *testing.T) {
		svc, _, id := setup(0)
		bad := "EU"
		if _, err := svc.Update(context.Background(), channel.UpdateChannelInput{ID: id, CurrencyCode: &bad}); !errors.Is(err, channel.ErrInvalidCurrency) {
			t.Fatalf("expected ErrInvalidCurrency, got %v", err)
		}
	})
}

// The storefront header caches a channel's own fields, so an edit has to announce itself for anything to purge it
func TestUpdatePublishesChannelUpdated(t *testing.T) {
	svc := channel.NewService(newSvcRepo(active(usID, "us", "https://shop.example.com", true)))
	bus := events.NewBus()
	svc.SetEventBus(bus)
	got := make(chan uuid.UUID, 1)
	bus.Subscribe(channel.EventUpdated, func(_ context.Context, e events.Event) error {
		got <- e.(*channel.Updated).ChannelID
		return nil
	})
	name := "Renamed"
	if _, err := svc.Update(context.Background(), channel.UpdateChannelInput{ID: usID, Name: &name}); err != nil {
		t.Fatal(err)
	}
	select {
	case id := <-got:
		if id != usID {
			t.Errorf("event names %s, want %s", id, usID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no channel.updated after an edit")
	}
}

func TestValidationFailuresAreTypedSentinels(t *testing.T) {
	long := make(channel.Metadata, 51)
	for range 51 {
		long[uuid.NewString()] = "v"
	}
	cases := []struct {
		name  string
		input channel.CreateChannelInput
		want  error
	}{
		{"blank name", channel.CreateChannelInput{Slug: "us", Name: " ", CurrencyCode: "USD"}, channel.ErrNameRequired},
		{"empty slug", channel.CreateChannelInput{Name: "S", CurrencyCode: "USD"}, channel.ErrInvalidSlug},
		{"bad slug", channel.CreateChannelInput{Slug: "U S", Name: "S", CurrencyCode: "USD"}, channel.ErrInvalidSlug},
		{"long slug", channel.CreateChannelInput{Slug: strings.Repeat("a", 65), Name: "S", CurrencyCode: "USD"}, channel.ErrInvalidSlug},
		{"too many metadata keys", channel.CreateChannelInput{Slug: "us", Name: "S", CurrencyCode: "USD", Metadata: long}, channel.ErrMetadataTooLarge},
		{"long metadata value", channel.CreateChannelInput{Slug: "us", Name: "S", CurrencyCode: "USD", Metadata: channel.Metadata{"k": strings.Repeat("v", 1025)}}, channel.ErrMetadataTooLarge},
		{"bad public url", channel.CreateChannelInput{Slug: "us", Name: "S", CurrencyCode: "USD", PublicURL: "https://shop.example.com/?x=1"}, channel.ErrInvalidPublicURL},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := channel.NewService(newSvcRepo()).Create(context.Background(), tc.input); !errors.Is(err, tc.want) {
				t.Fatalf("Create = %v, want %v", err, tc.want)
			}
		})
	}
	if _, err := channel.NewService(newSvcRepo()).Create(context.Background(), channel.CreateChannelInput{Slug: strings.Repeat("a", 64), Name: "S", CurrencyCode: "USD", Metadata: channel.Metadata{"k": strings.Repeat("v", 1024)}}); err != nil {
		t.Fatalf("limits are inclusive: %v", err)
	}
}

func codes(ch channel.Channel) []string {
	out := []string{}
	for _, l := range ch.Locales {
		out = append(out, l.Code)
	}
	return out
}

func TestTheDefaultLocaleAlwaysStaysSupported(t *testing.T) {
	ctx := context.Background()
	repo := newSvcRepo()
	svc := channel.NewService(repo)
	ch, err := svc.Create(ctx, channel.CreateChannelInput{Slug: "eu", Name: "EU", CurrencyCode: "EUR", DefaultLocale: "de", Locales: []string{"FR", "fr"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := codes(ch); !slices.Equal(got, []string{"fr", "de"}) {
		t.Fatalf("create locales = %v, want [fr de]", got)
	}
	it := "it"
	if ch, err = svc.Update(ctx, channel.UpdateChannelInput{ID: ch.ID, DefaultLocale: &it}); err != nil {
		t.Fatal(err)
	}
	if got := codes(ch); !slices.Equal(got, []string{"fr", "de", "it"}) {
		t.Fatalf("after moving the default, locales = %v, want [fr de it]", got)
	}
	if ch, err = svc.Update(ctx, channel.UpdateChannelInput{ID: ch.ID, Locales: []string{"fr"}}); err != nil {
		t.Fatal(err)
	}
	if got := codes(ch); !slices.Equal(got, []string{"fr", "it"}) {
		t.Fatalf("update dropping the default kept %v, want [fr it]", got)
	}
	if ch, err = svc.SetLocales(ctx, ch.ID, nil); err != nil {
		t.Fatal(err)
	}
	if got := codes(ch); !slices.Equal(got, []string{"it"}) {
		t.Fatalf("SetLocales(nil) left %v, want [it]", got)
	}
}

func TestANewChannelInheritsTheInstallDefaultLocale(t *testing.T) {
	ctx := context.Background()
	if ch, err := channel.NewService(newSvcRepo()).Create(ctx, channel.CreateChannelInput{Slug: "a", Name: "A", CurrencyCode: "USD"}); err != nil || ch.DefaultLocale != channel.FallbackLocale {
		t.Fatalf("no default channel: locale %q, %v, want the fallback", ch.DefaultLocale, err)
	}
	def := active(usID, "us", "", true)
	def.DefaultLocale = "sr"
	if ch, err := channel.NewService(newSvcRepo(def)).Create(ctx, channel.CreateChannelInput{Slug: "a", Name: "A", CurrencyCode: "USD"}); err != nil || ch.DefaultLocale != "sr" {
		t.Fatalf("locale %q, %v, want the default channel's sr", ch.DefaultLocale, err)
	}
}

func TestTheDefaultChannelStaysActive(t *testing.T) {
	ctx := context.Background()
	repo := newSvcRepo(active(usID, "us", "", true), inactive(euID, "eu", "", false), active(b2bID, "b2b", "", false))
	svc := channel.NewService(repo)
	if _, err := svc.SetDefault(ctx, euID); !errors.Is(err, channel.ErrDefaultChannelInactive) {
		t.Fatalf("defaulting an inactive channel = %v", err)
	}
	off := false
	if _, err := svc.Update(ctx, channel.UpdateChannelInput{ID: usID, IsActive: &off}); !errors.Is(err, channel.ErrDeactivateDefaultChannel) {
		t.Fatalf("deactivating the default = %v", err)
	}
	if _, err := svc.Update(ctx, channel.UpdateChannelInput{ID: b2bID, IsActive: &off}); err != nil {
		t.Fatalf("deactivating a sibling: %v", err)
	}
}

func TestSetDefaultAnnouncesBothChannels(t *testing.T) {
	repo := newSvcRepo(active(usID, "us", "", true), active(euID, "eu", "", false))
	svc := channel.NewService(repo)
	bus := events.NewBus()
	svc.SetEventBus(bus)
	got := make(chan uuid.UUID, 4)
	bus.Subscribe(channel.EventUpdated, func(_ context.Context, e events.Event) error {
		got <- e.(*channel.Updated).ChannelID
		return nil
	})
	ch, err := svc.SetDefault(context.Background(), euID)
	if err != nil || !ch.IsDefault {
		t.Fatalf("SetDefault = %+v, %v", ch, err)
	}
	seen := map[uuid.UUID]bool{}
	for range 2 {
		select {
		case id := <-got:
			seen[id] = true
		case <-time.After(2 * time.Second):
			t.Fatalf("only saw %v", seen)
		}
	}
	if !seen[usID] || !seen[euID] {
		t.Fatalf("updated events for %v, want both channels", seen)
	}
}
