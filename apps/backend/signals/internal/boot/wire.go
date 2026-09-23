package boot

import (
	"go.uber.org/fx"

	"github.com/skeletg/skelet/apps/backend/signals/internal/lens"
)

// Wiring is the glue for what only some deployments have: Lens takes a read handle where a replica
// was opened, and a blob store where a bucket is configured
var Wiring = fx.Module("wiring", fx.Invoke(wire))

func wire(rc RootContext, lensSvc *lens.Service, rq ReadQueries) error {
	if rq.Queries != nil {
		lensSvc.SetReadQueries(rq.Queries)
	}
	return wireLensBlobStore(rc.Context, lensSvc)
}
