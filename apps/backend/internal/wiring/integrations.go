package wiring

import (
	"go.uber.org/fx"

	db "github.com/skeletg/skelet/internal/db/sqlc"
	"github.com/skeletg/skelet/internal/otelx"
	"github.com/skeletg/skelet/internal/remoteprovider"
	"github.com/skeletg/skelet/internal/tx"
	"github.com/skeletg/skelet/internal/webhook"
)

type AllowPrivateTargets bool // its own type so the graph can't confuse it with another bool - cmd/signals reads the same env so both agree

var Integrations = fx.Module("integrations", // the two outbound-HTTP surfaces: store webhooks, and remote providers fulfilling pluggable roles
	fx.Provide(
		newAllowPrivateTargets,
		newWebhookService,
		newWebhookDispatcher,
		newRemoteProviderService,
		newRemoteProviderInvoker,
	),
)

func newAllowPrivateTargets() AllowPrivateTargets {
	return AllowPrivateTargets(webhook.AllowPrivateTargetsFromEnv())
}

// SSRF guard both halves: validated at save time AND the transport refuses private dials (DNS-rebinding-safe)
func newWebhookService(q *db.Queries, txMgr *tx.Manager, allow AllowPrivateTargets) *webhook.Service {
	svc := webhook.NewService(webhook.NewRepository(q, txMgr))
	svc.AllowPrivateTargets = bool(allow)
	return svc
}

func newWebhookDispatcher(svc *webhook.Service, allow AllowPrivateTargets) *webhook.Dispatcher {
	return webhook.NewDispatcher(svc, otelx.HTTPTransport(webhook.SafeTransport(bool(allow))))
}

func newRemoteProviderService(q *db.Queries, allow AllowPrivateTargets) *remoteprovider.Service { // provider-shaped webhooks: merchant endpoints implementing shipping/tax/address/promotion roles
	svc := remoteprovider.NewService(remoteprovider.NewRepository(q))
	svc.AllowPrivateTargets = bool(allow)
	return svc
}

func newRemoteProviderInvoker(svc *remoteprovider.Service, allow AllowPrivateTargets) *remoteprovider.Invoker {
	return remoteprovider.NewInvoker(svc, otelx.HTTPTransport(webhook.SafeTransport(bool(allow))))
}
