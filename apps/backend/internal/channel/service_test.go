package channel_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/internal/channel"
	"github.com/skeletg/skelet/apps/backend/internal/locale"
)

// Extends the middleware fake with just enough write behavior for Service tests
type svcRepo struct {
	fakeRepo
	localeSets map[uuid.UUID][]string
}

func newSvcRepo(channels ...channel.Channel) *svcRepo {
	return &svcRepo{channels: channels, localeSets: map[uuid.UUID][]string{}}
}

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

func (r *svcRepo) Update(_ context.Context, in channel.UpdateChannelInput) (channel.Channel, error) {
	for i, ch := range r.channels {
		if ch.ID != in.ID {
			continue
		}
		if in.Name != nil {
			ch.Name = *in.Name
		}
		if in.CurrencyCode != nil {
			ch.CurrencyCode = *in.CurrencyCode
		}
		r.channels[i] = ch
		return ch, nil
	}
	return channel.Channel{}, channel.ErrChannelNotFound
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

type fakeCounter struct {
	count int64
	calls int
}

func (f *fakeCounter) CountInheritingListingsForChannel(context.Context, uuid.UUID) (int64, error) {
	f.calls++
	return f.count, nil
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
			svc := channel.NewService(newSvcRepo(), nil)
			if _, err := svc.Create(context.Background(), tc.input); err == nil {
				t.Fatalf("Create(%+v) should have been rejected", tc.input)
			}
		})
	}
}

func TestServiceCreateNormalization(t *testing.T) {
	svc := channel.NewService(newSvcRepo(), nil)
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

	setup := func(count int64) (*channel.Service, *fakeCounter, uuid.UUID) {
		store := active(usID, "us", "https://shop.example.com", true)
		store.CurrencyCode = "USD"
		repo := newSvcRepo(store)
		svc := channel.NewService(repo, nil)
		counter := &fakeCounter{count: count}
		svc.SetListingCounter(counter)
		return svc, counter, store.ID
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
		if counter.calls != 0 {
			t.Fatalf("counter consulted %d times for a no-op currency write", counter.calls)
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
