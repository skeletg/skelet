package boot

import (
	"context"
	"testing"

	"go.uber.org/fx"
)

// The two processes' graphs as cmd/signals and cmd/ingest assemble them - a provider left out of one only surfaces at boot otherwise
func TestBothSignalsProcessesAssemble(t *testing.T) {
	root := RootContext{Context: context.Background()}
	for name, opts := range map[string][]fx.Option{
		"signals": {fx.Provide(Load), Pools, Services, Pulse, Wiring, Beacon, Contract, fx.Supply(root, QueueConsumer), fx.Populate(new(Deps))},
		"ingest":  {fx.Provide(LoadIngest), IngestPools, Services, Pulse, Wiring, Beacon, IngestFallback, fx.Supply(root, QueueProducer), fx.Populate(new(IngestDeps))},
	} {
		if err := fx.ValidateApp(opts...); err != nil {
			t.Errorf("%s does not assemble: %v", name, err)
		}
	}
}
