package wiring

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"go.uber.org/fx"

	"github.com/skeletg/skelet/apps/backend/internal/analytics"
	"github.com/skeletg/skelet/apps/backend/internal/cache"
	"github.com/skeletg/skelet/apps/backend/internal/channel"
	"github.com/skeletg/skelet/apps/backend/internal/config"
	db "github.com/skeletg/skelet/apps/backend/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/internal/experiment"
	"github.com/skeletg/skelet/apps/backend/internal/geo"
	"github.com/skeletg/skelet/apps/backend/internal/lens"
	"github.com/skeletg/skelet/apps/backend/internal/media"
	"github.com/skeletg/skelet/apps/backend/internal/remoteprovider"
	"github.com/skeletg/skelet/apps/backend/internal/secrets"
	"github.com/skeletg/skelet/apps/backend/internal/segment"
	"github.com/skeletg/skelet/apps/backend/internal/signalsboot"
)

// Two pools only - commerce for channels/config/experiments/segments, signals for the write volume - deliberately not the read pools Pools opens: this tier absorbs beacon spikes, so two more per replica buys nothing
var IngestPools = fx.Module("ingestpools",
	fx.Provide(
		newQueryTracer,
		newPrimaryPool,
		newSignalsPool,
		newQueries,
		newSignalsQueries,
		newTxManager,
		newSignalsTxManager,
	),
)

// Its own stack provider rather than SignalsCore's, because that one takes the read handles this module never opens
var IngestSignals = fx.Module("ingestsignals",
	fx.Provide(
		newIngestSignalsStack,
		newRecorder,
		newSignalsEnabled,
		newLensService,
		newExperimentService,
		newSegmentService,
		newTrafficRollup,
		newSecretsCipher, // this tier composes no Platform, but it reads config like the app does
		newAllowPrivateTargets,
		newRemoteProviderService,
		newRemoteProviderInvoker,
	),
)

// nil read handles, as the hand-wired version had - load-bearing, not lazy: SetSpineReadQueries rebuilds the spine wholesale via newPGSpine when the store is pgSpine or nil, and this is a queue PRODUCER whose WrapSpine goes last
func newIngestSignalsStack(lc fx.Lifecycle, rc RootContext, q *db.Queries, sq SignalsQueries) (*signalsboot.Stack, error) {
	stack, err := signalsboot.Build(rc.Context, signalsboot.Options{
		Commerce:  q,
		Spine:     sq.Queries,
		WrapQueue: true, // beacons land in JetStream before the 204, and the cmd/signals consumer drains it
	})
	if err != nil {
		return nil, err
	}
	lc.Append(fx.Hook{OnStop: func(context.Context) error { stack.Close(); return nil }})
	return stack, nil
}

type IngestDeps struct { // not wiring.Infra - that carries read pools this module never opens
	fx.In

	Config      Config
	DB          *sql.DB
	Cache       CacheRedis // ingest holds no durable Valkey state - its limiter and read-through caches all fail open so they ride the evictable client
	SignalsPool SignalsPool

	Stack    *signalsboot.Stack
	Recorder *analytics.Recorder
	Rollup   *analytics.TrafficRollup

	ConfigSvc  *config.Service
	ChannelSvc *channel.Service
	Lens       *lens.Service
	Experiment *experiment.Service
	Segment    *segment.Service
}

// Skips the JWT rule (cmd/ingest enforces its own, /t needs it) and sizes both pools for the beacon tier
func LoadIngestConfig() (Config, error) {
	c, err := loadBase()
	if err != nil {
		return Config{}, err
	}
	c.MaxConns = EnvInt("INGEST_PRIMARY_MAX_CONNS", 4)
	c.SignalsMaxConns = EnvInt("INGEST_SIGNALS_MAX_CONNS", 16) // beacons write the spine inside the handler, so this bounds concurrent /t writes - 16 fits signals-db's 100-connection budget beside app/worker/signals-worker and leaves room for a second replica; past it the queue is the lever, landing a batch as one statement where more connections only move the wait inside a 2-core Postgres
	return c, nil
}

// The beacon tier's cross-domain wiring - it sets no read queries on purpose, since those are optional and reads fall back to the primary
var IngestWiring = fx.Module("ingestwiring", fx.Invoke(wireIngestWorker))

type ingestWiringDeps struct {
	fx.In

	Root     RootContext
	Cache    CacheRedis
	Cipher   *secrets.Cipher
	Config   *config.Service
	Channel  *channel.Service
	Lens     *lens.Service
	Rollup   *analytics.TrafficRollup
	Segment  *segment.Service
	Provider *remoteprovider.Service
	Invoker  *remoteprovider.Invoker
}

func wireIngestWorker(d ingestWiringDeps) error {
	d.Lens.SetRollup(d.Rollup)

	d.Config.SetCache(cache.New(d.Cache.Client, "config")) // the caches wirePlatform installs on the app tier, same namespaces so an admin edit invalidates both - lens.IngestConfig is six config reads per /t/config hit and channel resolution runs on every beacon, against four connections
	d.Channel.SetCache(cache.New(d.Cache.Client, "channel"))
	if d.Cipher != nil {
		d.Config.SetCipher(d.Cipher) // CONFIG_ENCRYPTION_KEY is required of this tier, so a secret path had better resolve here
	}
	d.Segment.SetRemoteSegments(segment.NewRemoteSegmentSource(d.Provider, d.Invoker)) // the one remote-provider seam /t reaches - without it a segment-gated beacon sees native memberships only

	return WireLensBlobStore(d.Root.Context, d.Lens)
}

func NewGeoResolver() *geo.Resolver { return newGeoResolver() } // same resolver cmd/server builds - a bad GEOIP_DB_PATH is logged rather than fatal, geo just stops appearing

func WireLensBlobStore(ctx context.Context, lensSvc *lens.Service) error {
	cfg := mediaConfigFromEnv()
	if cfg.Bucket == "" {
		slog.Warn("S3 not configured - Lens replay blobs assumed in-DB, object-storage GC skipped")
		return nil
	}
	store, err := media.New(ctx, cfg)
	if err != nil {
		return fmt.Errorf("media storage: %w", err)
	}
	lensSvc.SetBlobStore(store)
	slog.Info("lens blob store wired for replay-blob GC")
	return nil
}
