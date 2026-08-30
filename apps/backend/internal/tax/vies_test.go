package tax

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNormalizeVATID(t *testing.T) {
	cases := []struct {
		in           string
		country, num string
		wantErr      bool
	}{
		{"DE123456789", "DE", "123456789", false},
		{" de 123 456 789 ", "DE", "123456789", false},
		{"FR-40.303.265.045", "FR", "40303265045", false},
		{"EL123456789", "EL", "123456789", false},
		{"XI110305878", "XI", "110305878", false},
		{"GB123456789", "", "", true},       // UK proper left VIES
		{"US123456789", "", "", true},       // not EU
		{"DE1", "", "", true},               // too short
		{"DE123456789012345", "", "", true}, // too long
		{"", "", "", true},
	}
	for _, c := range cases {
		country, num, err := NormalizeVATID(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("NormalizeVATID(%q) expected error, got %s%s", c.in, country, num)
			}
			continue
		}
		if err != nil {
			t.Errorf("NormalizeVATID(%q) unexpected error: %v", c.in, err)
			continue
		}
		if country != c.country || num != c.num {
			t.Errorf("NormalizeVATID(%q) = %s/%s, want %s/%s", c.in, country, num, c.country, c.num)
		}
	}
}

func TestValidateVATIDAgainstStub(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ms/DE/vat/111":
			_, _ = w.Write([]byte(`{"isValid": true, "name": "ACME GMBH", "address": "BERLIN"}`))
		case "/ms/DE/vat/222":
			_, _ = w.Write([]byte(`{"isValid": false}`))
		case "/ms/DE/vat/333":
			_, _ = w.Write([]byte(`{"isValid": false, "userError": "MS_UNAVAILABLE"}`))
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()
	orig := viesBaseURL
	viesBaseURL = srv.URL
	defer func() { viesBaseURL = orig }()

	ctx := context.Background()
	if res, err := ValidateVATID(ctx, "DE 111"); err != nil || res.Status != VatStatusValid || res.Name != "ACME GMBH" {
		t.Errorf("valid id: got %+v err=%v", res, err)
	}
	if res, _ := ValidateVATID(ctx, "DE222"); res.Status != VatStatusInvalid {
		t.Errorf("invalid id: got %+v", res)
	}
	if res, _ := ValidateVATID(ctx, "DE333"); res.Status != VatStatusUnavailable {
		t.Errorf("MS_UNAVAILABLE should map to unavailable, got %+v", res)
	}
	if res, _ := ValidateVATID(ctx, "DE444"); res.Status != VatStatusUnavailable {
		t.Errorf("HTTP 500 should map to unavailable, got %+v", res)
	}
	if _, err := ValidateVATID(ctx, "ZZ12345"); err == nil {
		t.Error("format error must be returned to the caller")
	}
}
