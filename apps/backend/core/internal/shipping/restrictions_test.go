package shipping

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// fakeShipRepo embeds Repository (nil) so it satisfies the interface;
// only the methods the country filter touches are implemented
type fakeShipRepo struct {
	Repository
	zoneID  uuid.UUID
	methods []Method
}

func (f fakeShipRepo) FindZoneForCountry(context.Context, uuid.UUID, string) (Zone, error) {
	return Zone{ID: f.zoneID}, nil
}
func (f fakeShipRepo) ListActiveMethodsByZone(context.Context, uuid.UUID) ([]Method, error) {
	return f.methods, nil
}
func (f fakeShipRepo) GetMethod(_ context.Context, id uuid.UUID) (Method, error) {
	for _, m := range f.methods {
		if m.ID == id {
			return m, nil
		}
	}
	return Method{}, ErrMethodNotFound
}

func newShipFixture() (*Service, Method, Method, uuid.UUID) {
	zone := uuid.New()
	all := Method{ID: uuid.New(), ZoneID: zone, Code: "std", CalcType: CalcFlat, FlatCents: 500, IsActive: true, CountryMode: "all"}
	usOnly := Method{ID: uuid.New(), ZoneID: zone, Code: "us-only", CalcType: CalcFlat, FlatCents: 700, IsActive: true, CountryMode: "specific", AllowedCountries: []string{"US"}}
	repo := fakeShipRepo{
		zoneID:  zone,
		methods: []Method{all, usOnly},
	}
	return NewService(repo), all, usOnly, uuid.New()
}

func TestQuote_PerMethodCountryFilter(t *testing.T) {
	svc, _, _, ch := newShipFixture()

	us, err := svc.Quote(context.Background(), QuoteInput{ChannelID: ch, CountryCode: "US", SubtotalCents: 1000})
	if err != nil {
		t.Fatalf("Quote(US): %v", err)
	}
	if len(us) != 2 {
		t.Errorf("US: got %d methods, want 2 (both offered)", len(us))
	}

	ca, err := svc.Quote(context.Background(), QuoteInput{ChannelID: ch, CountryCode: "CA", SubtotalCents: 1000})
	if err != nil {
		t.Fatalf("Quote(CA): %v", err)
	}
	if len(ca) != 1 || ca[0].Code != "std" {
		t.Errorf("CA: got %v, want only the all-countries method", ca)
	}
}

func TestComputeForOrderByID_CountryFilter(t *testing.T) {
	svc, _, usOnly, ch := newShipFixture()

	if _, _, err := svc.ComputeForOrderByID(context.Background(), usOnly.ID,
		QuoteInput{ChannelID: ch, CountryCode: "us", SubtotalCents: 1000}); err != nil {
		t.Errorf("US (case-insensitive) should price: %v", err)
	}

	if _, _, err := svc.ComputeForOrderByID(context.Background(), usOnly.ID,
		QuoteInput{ChannelID: ch, CountryCode: "CA", SubtotalCents: 1000}); !errors.Is(err, ErrMethodNotFound) {
		t.Errorf("CA should be rejected: got %v, want ErrMethodNotFound", err)
	}
}
