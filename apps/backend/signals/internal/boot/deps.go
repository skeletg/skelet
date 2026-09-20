package boot

import (
	"net/http"

	"go.uber.org/fx"
	"google.golang.org/grpc"

	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
	"github.com/skeletg/skelet/apps/backend/signals/internal/signalsboot"
)

// Deps is what cmd/signals reads out of the graph: the two surfaces it listens on, and the upkeep it
// runs from its own goroutines
type Deps struct {
	fx.In

	Mux    *http.ServeMux
	GRPC   *grpc.Server
	Stack  *signalsboot.Stack // the queue consumer drains JetStream into its spine
	Rollup *analytics.TrafficRollup
}

// IngestDeps is the public tier's set - the beacon surface and the rollup behind it, no contract
type IngestDeps struct {
	fx.In

	Mux    *http.ServeMux
	Rollup *analytics.TrafficRollup
}
