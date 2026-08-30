package wiring

import (
	"database/sql"

	"go.uber.org/fx"

	"github.com/skeletg/skelet/internal/channel"
	"github.com/skeletg/skelet/internal/config"
	"github.com/skeletg/skelet/internal/destination"
	"github.com/skeletg/skelet/internal/lens"
	"github.com/skeletg/skelet/internal/secrets"
	"github.com/skeletg/skelet/internal/signalsboot"
)

// Excludes Registries and the Platform wiring invoke on purpose - a worker must not replay plugin registrations, sync permissions, or install the admin seams
var SignalsCore = fx.Module("signalscore",
	fx.Provide(
		newSignalsStack,
		newRecorder,
		newSignalsEnabled,
		newLensService,
		newAllowPrivateTargets,
		newDestinationService,
		newSecretsCipher, // this process is env-checked for CONFIG_ENCRYPTION_KEY, so it has to be able to read a secret path
	),
)

func LoadSignalsConfig() (Config, error) { // skips the JWT rules (a worker signs nothing) and sizes the primary for what a Signals worker actually does with it - read config
	c, err := loadBase()
	if err != nil {
		return Config{}, err
	}
	c.MaxConns = EnvInt("SIGNALS_PRIMARY_MAX_CONNS", 4)
	return c, nil
}

type SignalsDeps struct { // not wiring.Infra on purpose - that one carries a Valkey client, which a cache-less entrypoint has no provider for
	fx.In

	Config             Config
	DB                 *sql.DB
	SignalsPool        SignalsPool
	SignalsReadQueries SignalsReadQueries
	ConfigSvc          *config.Service
	ChannelSvc         *channel.Service
	Stack              *signalsboot.Stack
	Lens               *lens.Service
	Destination        *destination.Service
}

var SignalsWiring = fx.Module("signalswiring", fx.Invoke(wireSignalsWorker)) // the signals worker's cross-domain wiring - it reads on its own pool, and takes the lenient blob store because a worker should not die for want of a bucket

func wireSignalsWorker(rc RootContext, lensSvc *lens.Service, sq SignalsReadQueries, cfgSvc *config.Service, cipher *secrets.Cipher) error {
	lensSvc.SetReadQueries(sq.Queries)
	if cipher != nil {
		cfgSvc.SetCipher(cipher)
	}
	return WireLensBlobStore(rc.Context, lensSvc)
}
