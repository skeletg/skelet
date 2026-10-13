package customerio

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	"github.com/skeletg/skelet/apps/backend/core/internal/dataio"
)

type fakeContacts struct {
	lookupErr error
	existing  bool
	sources   []string
}

func (f *fakeContacts) CustomerIDByEmail(context.Context, uuid.UUID, string) (uuid.UUID, bool, error) {
	return uuid.New(), f.existing, f.lookupErr
}

func (f *fakeContacts) EnsureContactFrom(_ context.Context, _ uuid.UUID, email, first string, _ bool, source string) (customer.Customer, bool, error) {
	f.sources = append(f.sources, source)
	return customer.Customer{ID: uuid.New(), Email: email, FirstName: first}, !f.existing, nil
}

func (f *fakeContacts) NormalizeProfilePhone(_ context.Context, _ uuid.UUID, raw *string) (*string, error) {
	return raw, nil
}
func (f *fakeContacts) UpdateProfileFrom(_ context.Context, id uuid.UUID, _ customer.UpdateCustomerInput, source string) (customer.Customer, error) {
	f.sources = append(f.sources, source)
	return customer.Customer{ID: id}, nil
}

func importRow(t *testing.T, f *fakeContacts, dry bool, cells map[string]string) (dataio.Outcome, error) {
	t.Helper()
	o := optsFor(uuid.New())
	o.DryRun = dry
	im := &customerImporter{customers: f, opts: o}
	r := dataio.NewRow()
	for k, v := range cells {
		r.Set(k, v)
	}
	return im.Row(context.Background(), r)
}

// A dry run that counted a failed lookup as a create would promise rows the real run then fails
func TestDryRunFailsTheRowOnALookupError(t *testing.T) {
	f := &fakeContacts{lookupErr: errors.New("connection reset")}
	if out, err := importRow(t, f, true, map[string]string{colEmail: "ada@example.com"}); err == nil || out != dataio.OutcomeSkipped {
		t.Fatalf("dry run = %v, %v; want the row failed with the lookup error", out, err)
	}
	f = &fakeContacts{existing: true}
	if out, err := importRow(t, f, true, map[string]string{colEmail: "ada@example.com"}); err != nil || out != dataio.OutcomeUpdated {
		t.Fatalf("dry run over an existing contact = %v, %v; want updated", out, err)
	}
}

// Both runs must refuse what account creation refuses, or the preview counts rows the write rejects
func TestDryAndRealRunsRejectTheSameAddresses(t *testing.T) {
	for _, bad := range []string{"a@b@c", "Ada <ada@example.com>", "ada@", "@example.com"} {
		for _, dry := range []bool{true, false} {
			if _, err := importRow(t, &fakeContacts{}, dry, map[string]string{colEmail: bad}); !errors.Is(err, errInvalidEmail) {
				t.Errorf("dry=%v email %q: err = %v, want errInvalidEmail", dry, bad, err)
			}
		}
	}
	long := make([]byte, customer.MaxNameRunes+1)
	for i := range long {
		long[i] = 'a'
	}
	if _, err := importRow(t, &fakeContacts{}, true, map[string]string{colEmail: "ada@example.com", colLastName: string(long)}); err == nil {
		t.Error("dry run accepted a last name the real update refuses")
	}
}

func TestTheImportStampsItsWritesAsAnImport(t *testing.T) {
	f := &fakeContacts{}
	if _, err := importRow(t, f, false, map[string]string{colEmail: "ada@example.com", colPhone: "555"}); err != nil {
		t.Fatal(err)
	}
	if len(f.sources) != 2 {
		t.Fatalf("writes = %v, want the contact and the profile update", f.sources)
	}
	for _, s := range f.sources {
		if s != customer.SourceImport {
			t.Fatalf("a write was stamped %q, want %q so it never reads as the shopper's own edit", s, customer.SourceImport)
		}
	}
}
