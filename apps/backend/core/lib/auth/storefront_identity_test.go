package auth

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestAStorefrontSessionOnlyIdentifiesOnItsOwnChannel(t *testing.T) {
	cust, home, other := uuid.New(), uuid.New(), uuid.New()
	signer := testCustomerSigner(t)
	tok, err := GenerateCustomerToken(cust, home, "a@b.test", signer, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/t/collect", nil)
	r.Header.Set("Authorization", "Bearer "+tok)
	if got := VerifiedStorefrontCustomer(r, signer.Verifier(), home); got != cust {
		t.Fatalf("own channel = %v, want the customer", got)
	}
	if got := VerifiedStorefrontCustomer(r, signer.Verifier(), other); got != uuid.Nil {
		t.Fatal("a session from another channel identified the visitor here, stitching them into this channel's segments")
	}
}
