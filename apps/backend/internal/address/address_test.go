package address

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestBasicValidator(t *testing.T) {
	b := Basic{}
	cases := []struct {
		name      string
		in        Input
		wantValid bool
		wantZip   string // expected normalized postal code ("" = no normalization expected)
	}{
		{"valid US", Input{Street1: "1 Main St", City: "Austin", PostalCode: "78701", CountryCode: "US"}, true, ""},
		{"US zip+4 missing dash normalizes", Input{Street1: "1 Main St", City: "Austin", PostalCode: "787011234", CountryCode: "US"}, true, "78701-1234"},
		{"bad US zip", Input{Street1: "1 Main St", City: "Austin", PostalCode: "ABC", CountryCode: "US"}, false, ""},
		{"missing street", Input{City: "Austin", PostalCode: "78701", CountryCode: "US"}, false, ""},
		{"lowercase country normalizes", Input{Street1: "1 Main St", City: "Berlin", PostalCode: "10115", CountryCode: "de"}, true, ""},
		{"unknown country only needs non-empty zip", Input{Street1: "5 Rua A", City: "Lisboa", PostalCode: "anything", CountryCode: "PT"}, true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := b.Validate(context.Background(), uuid.UUID{}, tc.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res.Valid != tc.wantValid {
				t.Fatalf("valid = %v, want %v (messages: %v)", res.Valid, tc.wantValid, res.Messages)
			}
			if tc.wantZip != "" {
				if res.Normalized == nil || res.Normalized.PostalCode != tc.wantZip {
					t.Fatalf("normalized zip = %+v, want %s", res.Normalized, tc.wantZip)
				}
			}
		})
	}
}
