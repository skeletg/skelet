package review

import "testing"

// The storefront mints these and core verifies them - the same vector is asserted in vetrina-demo's voter-token.test.ts
func TestAStorefrontVoterTokenVerifies(t *testing.T) {
	id, err := ParseVoterToken([]byte("review-voter-secret-0123456789ab"), "7f1c2b9e-3d4a-4c5b-8e6f-0a1b2c3d4e5f.T_cUvNzohQcHKVs78HBkxHrxriRElUTXhFJpF381kRc")
	if err != nil || id.String() != "7f1c2b9e-3d4a-4c5b-8e6f-0a1b2c3d4e5f" {
		t.Fatalf("ParseVoterToken = %v, %v", id, err)
	}
}
