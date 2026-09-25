package order

import (
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
)

func TestTheCartRepricerValidatesBundleLinesAtPlacement(t *testing.T) {
	var r CartRepricer = (*cart.Service)(nil)
	if _, ok := r.(bundleValidator); !ok {
		t.Fatal("the cart service no longer offers ValidateBundleLines, so placement stops re-checking bundle lines")
	}
}
