package boot

import (
	"context"

	"github.com/google/uuid"
	"go.uber.org/fx"

	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
	"github.com/skeletg/skelet/apps/backend/signals/internal/assistantlog"
	"github.com/skeletg/skelet/apps/backend/signals/internal/lens"
	"github.com/skeletg/skelet/apps/backend/signals/internal/signalsgrpc"
)

// Wiring is the glue for what only some deployments have: Lens takes a read handle where a replica
// was opened, and a blob store where a bucket is configured
var Wiring = fx.Module("wiring", fx.Invoke(wire))

func wire(rc RootContext, lensSvc *lens.Service, channels *analytics.ChannelRoster, rq ReadQueries, rec *analytics.Recorder, conversations *assistantlog.Store) error {
	if rq.Queries != nil {
		lensSvc.SetReadQueries(rq.Queries)
	}
	// The erasure sweep runs once identifies buffered in any process have landed, so it catches guest chats the request itself couldn't tie to the customer
	rec.OnIdentityErasure(func(ctx context.Context, customers []uuid.UUID) error {
		return signalsgrpc.EraseConversations(ctx, rec, conversations, customers)
	})
	return wireLensBlobStore(rc.Context, lensSvc, channels)
}
