package wiring

// Core's dialler for the Signals service - everything core reads out of the suite goes through it

import (
	"context"
	"os"
	"time"

	"go.uber.org/fx"

	"github.com/skeletg/skelet/apps/backend/internal/admin"
	"github.com/skeletg/skelet/apps/backend/internal/signalsclient"
)

// Long enough to cover one request's fan-out, short enough that a leaked token is worthless
const signalsTokenTTL = 2 * time.Minute

// Nil when the suite is switched off or unaddressed, which every caller reads as out of reach
func newSignalsClient(lc fx.Lifecycle, cfg Config, enabled SignalsEnabled, adm *admin.Service) (*signalsclient.Client, error) {
	addr := os.Getenv("SIGNALS_GRPC_ADDR")
	if !bool(enabled) {
		addr = "" // the switch is the same thing as having nowhere to dial
	}
	c, err := signalsclient.New(addr, cfg.JWTSecret, signalsTokenTTL, adm)
	if err != nil {
		return nil, err
	}
	lc.Append(fx.Hook{OnStop: func(context.Context) error { return c.Close() }})
	return c, nil
}
