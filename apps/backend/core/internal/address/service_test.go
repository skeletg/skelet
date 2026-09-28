package address

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/remoteprovider"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

type fixedSettings struct{ validator string }

func (f fixedSettings) GetString(context.Context, string, config.Scope) (string, error) {
	return f.validator, nil
}
func (fixedSettings) GetBool(context.Context, string, config.Scope) (bool, error) { return false, nil }

type alwaysInvalid struct{}

func (alwaysInvalid) Validate(context.Context, uuid.UUID, Input) (Result, error) {
	return Result{Valid: false, Messages: []string{"nope"}}, nil
}

func TestServiceRunsTheConfiguredValidatorAndPassesEverythingElse(t *testing.T) {
	Register("test.invalid", "Always invalid", alwaysInvalid{})
	for name, tc := range map[string]struct {
		slug      string
		wantValid bool
	}{
		"registered slug runs": {"test.invalid", false},
		"padded slug runs":     {"  test.invalid ", false},
		"none skips":           {"none", true},
		"empty skips":          {"", true},
		"unknown slug skips":   {"uninstalled", true},
	} {
		t.Run(name, func(t *testing.T) {
			res, err := (&Service{cfg: fixedSettings{tc.slug}}).Validate(context.Background(), uuid.New(), Input{})
			if err != nil || res.Valid != tc.wantValid {
				t.Fatalf("Validate = %+v, %v, want valid=%v", res, err, tc.wantValid)
			}
		})
	}
}

type fakeLookup struct {
	provider remoteprovider.Provider
	found    bool
	err      error
	asked    *uuid.UUID
}

func (f *fakeLookup) ActiveFor(_ context.Context, kind remoteprovider.Kind, channelID *uuid.UUID) (remoteprovider.Provider, bool, error) {
	if kind != remoteprovider.KindAddressValidator {
		return remoteprovider.Provider{}, false, errors.New("wrong kind")
	}
	f.asked = channelID
	return f.provider, f.found, f.err
}

type fakeInvoker struct {
	resp remoteAddressResponse
	err  error
}

func (f fakeInvoker) Invoke(_ context.Context, _ remoteprovider.Provider, _ any, out any) error {
	if f.err != nil {
		return f.err
	}
	*out.(*remoteAddressResponse) = f.resp
	return nil
}

func TestRemoteValidatorHonoursTheProvidersFailurePolicy(t *testing.T) {
	down := errors.New("connection refused")
	for name, tc := range map[string]struct {
		lookup    fakeLookup
		invoker   fakeInvoker
		wantValid bool
		wantErr   error
	}{
		"lookup failure passes":   {lookup: fakeLookup{err: errors.New("db down")}, wantValid: true},
		"no provider passes":      {lookup: fakeLookup{}, wantValid: true},
		"fail_open outage passes": {lookup: fakeLookup{found: true, provider: remoteprovider.Provider{Slug: "p", FailurePolicy: remoteprovider.FailOpen}}, invoker: fakeInvoker{err: down}, wantValid: true},
		"fail_closed outage is unavailable": {
			lookup: fakeLookup{found: true, provider: remoteprovider.Provider{Slug: "p", FailurePolicy: remoteprovider.FailClosed}}, invoker: fakeInvoker{err: down}, wantErr: ErrUnavailable,
		},
		"provider verdict is returned": {
			lookup: fakeLookup{found: true, provider: remoteprovider.Provider{Slug: "p", FailurePolicy: remoteprovider.FailClosed}}, invoker: fakeInvoker{resp: remoteAddressResponse{Valid: false, Messages: []string{"no such street"}}}, wantValid: false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			channelID := uuid.New()
			lookup := tc.lookup
			res, err := (&remoteValidator{providers: &lookup, invoker: tc.invoker}).Validate(context.Background(), channelID, Input{Street1: "1 Main St"})
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil || res.Valid != tc.wantValid {
				t.Fatalf("Validate = %+v, %v, want valid=%v", res, err, tc.wantValid)
			}
			if lookup.asked == nil || *lookup.asked != channelID {
				t.Errorf("provider looked up for %v, want the order's channel %v", lookup.asked, channelID)
			}
		})
	}
}

func TestRemoteValidatorMapsTheNormalizedAddressBack(t *testing.T) {
	lookup := &fakeLookup{found: true, provider: remoteprovider.Provider{Slug: "p"}}
	norm := remoteAddressInput{Street1: "1 MAIN ST", City: "AUSTIN", PostalCode: "78701-1234", CountryCode: "US"}
	res, err := (&remoteValidator{providers: lookup, invoker: fakeInvoker{resp: remoteAddressResponse{Valid: true, Normalized: &norm}}}).Validate(context.Background(), uuid.New(), Input{})
	if err != nil || res.Normalized == nil || *res.Normalized != Input(norm) {
		t.Fatalf("Validate = %+v, %v, want the provider's normalized address", res, err)
	}
}

func TestValidatorSelectLabelsEveryRegisteredValidator(t *testing.T) {
	RegisterConfigSources()
	opts, err := config.Definition{Path: "sales/checkout/address_validator", Type: config.TypeSelect, Source: "address_validators"}.GetOptions(context.Background())
	if err != nil {
		t.Fatalf("resolve source: %v", err)
	}
	if len(opts) == 0 || opts[0].Value != "none" {
		t.Fatalf("options = %+v, want the none sentinel first", opts)
	}
	for _, o := range opts {
		if o.Label == "" || o.Label == o.Value {
			t.Errorf("option %q is labelled %q, want a readable label", o.Value, o.Label)
		}
	}
}
