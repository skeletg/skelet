package wiring

import (
	"context"
	"log/slog"

	"go.uber.org/fx"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/remoteprovider"
	"github.com/skeletg/skelet/apps/backend/core/internal/webhook"
	"github.com/skeletg/skelet/apps/backend/core/lib/otelx"
	"github.com/skeletg/skelet/apps/backend/core/lib/secrets"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
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
func newWebhookService(q *db.Queries, txMgr *txrunner.Manager, allow AllowPrivateTargets) *webhook.Service {
	svc := webhook.NewService(webhook.NewRepository(q, txMgr))
	svc.AllowPrivateTargets = bool(allow)
	return svc
}

func newWebhookDispatcher(svc *webhook.Service, allow AllowPrivateTargets) *webhook.Dispatcher {
	return webhook.NewDispatcher(svc, otelx.HTTPTransport(webhook.SafeTransport(bool(allow))))
}

func newRemoteProviderService(q *db.Queries, allow AllowPrivateTargets, cipher *secrets.Cipher) *remoteprovider.Service { // provider-shaped webhooks: merchant endpoints implementing shipping/tax/address/promotion roles
	var sealer remoteprovider.Sealer // a nil *Cipher in the interface would read as a key that panics on use
	if cipher != nil {
		sealer = cipher
	}
	svc := remoteprovider.NewService(remoteprovider.NewRepository(q, sealer))
	svc.AllowPrivateTargets = bool(allow)
	return svc
}

func newRemoteProviderInvoker(svc *remoteprovider.Service, allow AllowPrivateTargets) *remoteprovider.Invoker {
	return remoteprovider.NewInvoker(svc, otelx.HTTPTransport(webhook.SafeTransport(bool(allow))))
}

// Rows written before CONFIG_ENCRYPTION_KEY was set keep plaintext secrets until edited - sealed once at start, and a failure only warns since every read still works
func sealRemoteProviderSecrets(lc fx.Lifecycle, svc *remoteprovider.Service) {
	lc.Append(fx.Hook{OnStart: func(ctx context.Context) error {
		n, err := svc.SealPlaintext(ctx)
		if err != nil {
			slog.WarnContext(ctx, "remote providers: sealing plaintext secrets failed", "err", err)
		} else if n > 0 {
			slog.InfoContext(ctx, "remote providers: sealed plaintext secrets", "providers", n)
		}
		return nil
	}})
}
