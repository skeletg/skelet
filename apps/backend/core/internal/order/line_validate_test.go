package order

import (
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
)

func TestTheCartRepricerValidatesLinesAtPlacement(t *testing.T) {
	var r CartRepricer = (*cart.Service)(nil)
	if _, ok := r.(lineValidator); !ok {
		t.Fatal("the cart service no longer offers ValidateCheckoutLines, so placement stops re-checking bundles and quantity bounds")
	}
}
