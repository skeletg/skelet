package wiring_test

import (
	"context"
	"sync"
	"testing"

	"go.uber.org/fx"

	"github.com/skeletg/skelet/apps/backend/core/internal/wiring"
	"github.com/skeletg/skelet/apps/backend/core/lib/observability"
)

// Mirrors what cmd/server composes per role - keep in step with run(), or this stops covering what actually boots
func roleOptions(bg *sync.WaitGroup, worker, runBackground bool, out ...any) []fx.Option {
	opts := []fx.Option{
		wiring.Pools,
		wiring.Cache,
		wiring.Registries,
		wiring.Platform,
		wiring.Integrations,
		wiring.Services,
		wiring.Domain,
		wiring.DataTransfer,
		wiring.Jobs,
		wiring.Search,
		wiring.Health,
		wiring.Wiring,
	}
	if runBackground {
		opts = append(opts, wiring.BackgroundLoops)
	}
	if !worker {
		opts = append(opts, wiring.GraphQL, wiring.HTTP, wiring.ServerLoops)
	}
	return append(opts,
		fx.Supply(wiring.Config{}, wiring.RootContext{Context: context.Background()},
			observability.Telemetry{},
			wiring.BackgroundWaitGroup{WaitGroup: bg}),
		fx.Populate(out...),
		fx.NopLogger,
	)
}

// fx resolves by reflection, so a missing or duplicated provider fails at boot rather than at compile - this puts the check back in CI; ValidateApp only resolves types and never runs a constructor, so nothing here touches a database
func TestGraphResolves(t *testing.T) {
	for _, tc := range []struct {
		name          string
		worker, bgRun bool
	}{
		{"server with background", false, true},
		{"server with JOBS_EXTERNAL", false, false},
		{"worker", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var bg sync.WaitGroup
			var infra wiring.Infra
			var svcs wiring.Svcs
			out := []any{&infra, &svcs}
			if !tc.worker {
				var root wiring.RootHandler // what cmd/server actually asks the graph for
				out = append(out, &root)
			}
			if err := fx.ValidateApp(roleOptions(&bg, tc.worker, tc.bgRun, out...)...); err != nil {
				t.Fatalf("graph does not resolve: %v", err)
			}
		})
	}
}
