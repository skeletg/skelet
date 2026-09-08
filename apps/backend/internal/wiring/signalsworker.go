package wiring

import (
	"database/sql"
	"errors"
	"github.com/redis/go-redis/v9"

	"go.uber.org/fx"

	"github.com/skeletg/skelet/apps/backend/internal/channel"
	"github.com/skeletg/skelet/apps/backend/internal/config"
	"github.com/skeletg/skelet/apps/backend/internal/destination"
	"github.com/skeletg/skelet/apps/backend/internal/experiment"
	"github.com/skeletg/skelet/apps/backend/internal/lens"
	"github.com/skeletg/skelet/apps/backend/internal/secrets"
	"github.com/skeletg/skelet/apps/backend/internal/signalsboot"
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
		newExperimentService, // Prism's admin API is served here, not by cmd/server
		newValkey,            // optional - without it a killed session keeps working here until its token expires
		newSecretsCipher,     // this process is env-checked for CONFIG_ENCRYPTION_KEY, so it has to be able to read a secret path
	),
)

func LoadSignalsConfig() (Config, error) { // sizes the primary for what this process does with it - read config
	c, err := loadBase()
	if err != nil {
		return Config{}, err
	}
	// It signs nothing, but it verifies: the Signals API authenticates service tokens core minted with this secret
	if c.JWTSecret == "" {
		return Config{}, errors.New("JWT_SECRET is not set - cmd/signals verifies the service tokens core mints with it")
	}
	c.MaxConns = EnvInt("SIGNALS_PRIMARY_MAX_CONNS", 4)
	return c, nil
}

type SignalsDeps struct { // not wiring.Infra on purpose - this process needs a narrower set than a full server does
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
	Experiment         *experiment.Service
	Valkey             *redis.Client `optional:"true"`
}

var SignalsWiring = fx.Module("signalswiring", fx.Invoke(wireSignalsWorker)) // the signals worker's cross-domain wiring - it reads on its own pool, and takes the lenient blob store because a worker should not die for want of a bucket

func wireSignalsWorker(rc RootContext, lensSvc *lens.Service, sq SignalsReadQueries, cfgSvc *config.Service, cipher *secrets.Cipher) error {
	lensSvc.SetReadQueries(sq.Queries)
	if cipher != nil {
		cfgSvc.SetCipher(cipher)
	}
	return WireLensBlobStore(rc.Context, lensSvc)
}
