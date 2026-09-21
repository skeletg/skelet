package boot

import (
	"go.uber.org/fx"

	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
	"github.com/skeletg/skelet/apps/backend/signals/internal/lens"
)

// Wiring is the cross-service glue the constructors cannot express: Lens takes its read handle where
// one was opened, coalesces page views through the rollup, and GCs replay blobs in the bucket
var Wiring = fx.Module("wiring", fx.Invoke(wire))

func wire(rc RootContext, lensSvc *lens.Service, rq ReadQueries, rollup *analytics.TrafficRollup) error {
	if rq.Queries != nil {
		lensSvc.SetReadQueries(rq.Queries)
	}
	lensSvc.SetRollup(rollup)
	return wireLensBlobStore(rc.Context, lensSvc)
}
