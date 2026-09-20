package e2e

import (
	"context"
	"sync"
	"testing"

	"go.uber.org/fx"

	"github.com/skeletg/skelet/apps/backend/core/internal/e2ekit"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/internal/wiring"
)

// The harness lives in internal/e2ekit so a plugin can drive it too. Embedded rather than aliased
// because the scenario files declare their own methods on *harness, which an alias would forbid
type harness struct {
	*e2ekit.Harness
}

type store = e2ekit.Store

func newHarness(t *testing.T) *harness {
	t.Helper()
	return &harness{e2ekit.New(t)}
}

func e2eAddress() *order.OrderAddress { return e2ekit.Address() }

// Runs everywhere, no database - proves this package's composition and populate struct resolve,
// the same guarantee wiring_test gives the real entrypoints
func TestE2EGraphResolves(t *testing.T) {
	var bg sync.WaitGroup
	var d e2ekit.Deps
	if err := fx.ValidateApp(e2ekit.Options(wiring.Config{}, context.Background(), &bg, &d)...); err != nil {
		t.Fatalf("e2e graph does not resolve: %v", err)
	}
}
