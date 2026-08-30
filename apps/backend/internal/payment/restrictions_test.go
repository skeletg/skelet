package payment

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// fakePayRepo embeds Repository (nil) so it satisfies the interface;
// only the methods the country filter touches are implemented
type fakePayRepo struct {
	Repository
	methods         []PaymentMethod
	methodCountries map[uuid.UUID][]string
}

func (f fakePayRepo) GetPaymentMethod(_ context.Context, _ uuid.UUID, slug string) (PaymentMethod, error) {
	for _, m := range f.methods {
		if m.ProviderSlug == slug {
			return m, nil
		}
	}
	return PaymentMethod{}, ErrProviderNotInstalled
}

func (f fakePayRepo) ListEnabledPaymentMethodsByChannel(_ context.Context, _ uuid.UUID) ([]PaymentMethod, error) {
	out := make([]PaymentMethod, 0, len(f.methods))
	for _, m := range f.methods {
		if m.IsEnabled {
			out = append(out, m)
		}
	}
	return out, nil
}

func (f fakePayRepo) ListCountriesForPaymentMethod(_ context.Context, id uuid.UUID) ([]string, error) {
	return f.methodCountries[id], nil
}

func newPayFixture() (*Service, PaymentMethod, PaymentMethod, uuid.UUID) {
	ch := uuid.New()
	open := PaymentMethod{ID: uuid.New(), ChannelID: ch, ProviderSlug: "open", DisplayName: "Open", IsEnabled: true, CountryMode: "all"}
	usOnly := PaymentMethod{ID: uuid.New(), ChannelID: ch, ProviderSlug: "us-only", DisplayName: "US Only", IsEnabled: true, CountryMode: "specific"}
	disabled := PaymentMethod{ID: uuid.New(), ChannelID: ch, ProviderSlug: "off", DisplayName: "Off", IsEnabled: false, CountryMode: "all"}
	repo := fakePayRepo{
		methods:         []PaymentMethod{open, usOnly, disabled},
		methodCountries: map[uuid.UUID][]string{usOnly.ID: {"US"}},
	}
	// The eligible-methods read drops rows whose provider isn't registered, so the
	// fixture has to install one per slug or every case filters out before the country check
	ResetForTest()
	for _, slug := range []string{"open", "us-only", "off"} {
		RegisterProvider(Registration{Slug: slug, Provider: &fakeProvider{}})
	}
	return NewService(repo, nil, nil), open, usOnly, ch
}

func TestIsMethodAllowedForCountry(t *testing.T) {
	svc, open, usOnly, ch := newPayFixture()
	ctx := context.Background()

	cases := []struct {
		name    string
		slug    string
		country string
		want    bool
	}{
		{"all-mode accepts any country", open.ProviderSlug, "CA", true},
		{"specific in list", usOnly.ProviderSlug, "US", true},
		{"specific case-insensitive", usOnly.ProviderSlug, "us", true},
		{"specific not in list", usOnly.ProviderSlug, "CA", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := svc.IsMethodAllowedForCountry(ctx, ch, c.slug, c.country)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != c.want {
				t.Fatalf("IsMethodAllowedForCountry(%s, %s) = %v, want %v", c.slug, c.country, got, c.want)
			}
		})
	}
}

func TestValidatePaymentForChannel(t *testing.T) {
	svc, _, _, ch := newPayFixture()
	ctx := context.Background()

	cases := []struct {
		name    string
		slug    string
		country string
		wantErr error
		ok      bool
	}{
		{name: "enabled all-mode", slug: "open", country: "CA", ok: true},
		{name: "enabled specific in list", slug: "us-only", country: "US", ok: true},
		{name: "enabled specific not in list", slug: "us-only", country: "CA", ok: false},
		{name: "disabled provider", slug: "off", country: "US", wantErr: ErrProviderDisabled},
		{name: "unknown provider", slug: "nope", country: "US", wantErr: ErrProviderNotInstalled},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := svc.ValidatePaymentForChannel(ctx, ch, c.slug, c.country)
			if c.wantErr != nil {
				if !errors.Is(err, c.wantErr) {
					t.Fatalf("want error %v, got %v", c.wantErr, err)
				}
				return
			}
			if c.ok && err != nil {
				t.Fatalf("want ok, got error: %v", err)
			}
			if !c.ok && err == nil {
				t.Fatalf("want error, got nil")
			}
		})
	}
}

func TestListEligiblePaymentMethods(t *testing.T) {
	svc, _, _, ch := newPayFixture()
	ctx := context.Background()

	// US: both the all-mode and the US-specific method qualify
	us, err := svc.ListEligiblePaymentMethods(ctx, ch, "US")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(us) != 2 {
		t.Fatalf("US: got %d methods, want 2", len(us))
	}

	// CA: the US-specific method drops out, only the all-mode remains
	ca, err := svc.ListEligiblePaymentMethods(ctx, ch, "CA")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ca) != 1 || ca[0].ProviderSlug != "open" {
		t.Fatalf("CA: got %+v, want only [open]", ca)
	}
}

// A row left behind by an uninstalled plugin must not reach checkout - offering it
// only gets the shopper an ErrProviderNotFound at placement
func TestListEligiblePaymentMethodsDropsUnregisteredProvider(t *testing.T) {
	_, _, _, ch := newPayFixture()
	orphan := PaymentMethod{ID: uuid.New(), ChannelID: ch, ProviderSlug: "uninstalled", DisplayName: "Gone", IsEnabled: true, CountryMode: "all"}
	svc := NewService(fakePayRepo{methods: []PaymentMethod{orphan}}, nil, nil)

	got, err := svc.ListEligiblePaymentMethods(context.Background(), ch, "US")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %+v, want the orphaned row filtered out", got)
	}
}
