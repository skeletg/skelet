package inventory

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/phone"
)

// Records what the service hands the repository - the phone is normalized before it gets there
type locationRepo struct {
	Repository
	current   Location
	lookupErr error
	created   *CreateLocationInput
	updated   *UpdateLocationInput
}

func (r *locationRepo) CreateLocation(_ context.Context, in CreateLocationInput) (Location, error) {
	r.created = &in
	return Location{Phone: in.Phone, CountryCode: in.CountryCode}, nil
}

func (r *locationRepo) UpdateLocation(_ context.Context, in UpdateLocationInput) (Location, error) {
	r.updated = &in
	return r.current, nil
}

func (r *locationRepo) GetLocationByID(context.Context, uuid.UUID) (Location, error) {
	return r.current, r.lookupErr
}

func TestCreateLocationReadsThePhoneInItsCountry(t *testing.T) {
	repo := &locationRepo{}
	svc := NewService(repo, newInvTxManager(t))
	in := CreateLocationInput{Slug: "berlin", Name: "Berlin", CountryCode: "de", Phone: "030 1234567"}
	if _, err := svc.CreateLocation(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	if repo.created.Phone != "+49301234567" {
		t.Errorf("stored phone = %q, want +49301234567", repo.created.Phone)
	}

	repo.created = nil
	in.CountryCode, in.Phone = "US", "030 1234567"
	if _, err := svc.CreateLocation(context.Background(), in); !errors.Is(err, phone.ErrInvalid) {
		t.Fatalf("a German national number at a US location = %v, want phone.ErrInvalid", err)
	}
	if repo.created != nil {
		t.Error("an invalid phone still reached the repository")
	}

	in.Phone = ""
	if _, err := svc.CreateLocation(context.Background(), in); err != nil || repo.created.Phone != "" {
		t.Fatalf("no phone = %q, %v, want it left empty", repo.created.Phone, err)
	}
}

// An update that doesn't move the location reads its phone in the country it already has
func TestUpdateLocationReadsThePhoneInTheCountryItHasOrGets(t *testing.T) {
	repo := &locationRepo{current: Location{CountryCode: "DE"}}
	svc := NewService(repo, newInvTxManager(t))
	ctx := context.Background()

	berlin := "030 1234567"
	if _, err := svc.UpdateLocation(ctx, UpdateLocationInput{ID: uuid.New(), Phone: &berlin}); err != nil {
		t.Fatal(err)
	}
	if got := *repo.updated.Phone; got != "+49301234567" {
		t.Errorf("phone read in the current country = %q, want +49301234567", got)
	}

	us, newYork := "us", "(212) 555-0123"
	if _, err := svc.UpdateLocation(ctx, UpdateLocationInput{ID: uuid.New(), CountryCode: &us, Phone: &newYork}); err != nil {
		t.Fatal(err)
	}
	if got := *repo.updated.Phone; got != "+12125550123" {
		t.Errorf("phone read in the country the update sets = %q, want +12125550123", got)
	}

	if _, err := svc.UpdateLocation(ctx, UpdateLocationInput{ID: uuid.New(), CountryCode: &us, Phone: &berlin}); !errors.Is(err, phone.ErrInvalid) {
		t.Fatalf("a German national number on a location moving to the US = %v, want phone.ErrInvalid", err)
	}

	repo.updated = nil
	name := "Renamed"
	if _, err := svc.UpdateLocation(ctx, UpdateLocationInput{ID: uuid.New(), Name: &name}); err != nil || repo.updated.Phone != nil {
		t.Fatalf("an update without a phone = %v, %v, want the phone left out", repo.updated.Phone, err)
	}
}

func TestUpdateLocationReturnsLookupError(t *testing.T) {
	lookupErr := errors.New("location lookup failed")
	repo := &locationRepo{lookupErr: lookupErr}
	svc := NewService(repo, newInvTxManager(t))
	number := "+12015550123"
	if _, err := svc.UpdateLocation(context.Background(), UpdateLocationInput{ID: uuid.New(), Phone: &number}); !errors.Is(err, lookupErr) {
		t.Fatalf("error = %v, want lookup failure", err)
	}
	if repo.updated != nil {
		t.Fatal("failed lookup still wrote the location")
	}
}
