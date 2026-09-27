package wiring

// Core's dialler for the Signals service - everything core reads out of the suite goes through it

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	"go.uber.org/fx"

	"github.com/skeletg/skelet/apps/backend/core/internal/admin"
	"github.com/skeletg/skelet/apps/backend/core/internal/signalsclient"
	"github.com/skeletg/skelet/apps/backend/core/lib/envcheck"
)

// Long enough to cover one request's fan-out, short enough that a leaked token is worthless
const signalsTokenTTL = 2 * time.Minute

// Nil when the suite is switched off or unaddressed, which every caller reads as out of reach
func newSignalsClient(lc fx.Lifecycle, cfg Config, enabled SignalsEnabled, adm *admin.Service) (*signalsclient.Client, error) {
	addr := strings.TrimSpace(os.Getenv("SIGNALS_GRPC_ADDR"))
	if !bool(enabled) {
		addr = "" // the switch is the same thing as having nowhere to dial
	} else if addr == "" {
		return nil, errors.New(envcheck.ErrSignalsUnaddressed)
	}
	// A missing secret would otherwise read as "no suite", and every call would quietly go nowhere
	if addr != "" && cfg.SignalsServiceSecret == "" {
		return nil, errors.New("SIGNALS_GRPC_ADDR is set but SIGNALS_SERVICE_SECRET is not - core signs every call on the contract with it, and the suite verifies with the same value")
	}
	c, err := signalsclient.New(addr, cfg.SignalsServiceSecret, signalsTokenTTL, adm)
	if err != nil {
		return nil, err
	}
	c.SetSegmentReadTimeout(time.Duration(EnvInt("SIGNALS_SEGMENT_READ_TIMEOUT_MS", 0)) * time.Millisecond)
	lc.Append(fx.Hook{OnStop: func(context.Context) error { return c.Close() }})
	return c, nil
}
