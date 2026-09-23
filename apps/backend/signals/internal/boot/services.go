package boot

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/fx"

	"github.com/skeletg/skelet/apps/backend/core/lib/cache"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
	"github.com/skeletg/skelet/apps/backend/signals/internal/archivefile"
	"github.com/skeletg/skelet/apps/backend/signals/internal/behavioralsegment"
	"github.com/skeletg/skelet/apps/backend/signals/internal/destination"
	"github.com/skeletg/skelet/apps/backend/signals/internal/experiment"
	"github.com/skeletg/skelet/apps/backend/signals/internal/geo"
	"github.com/skeletg/skelet/apps/backend/signals/internal/lens"
	"github.com/skeletg/skelet/apps/backend/signals/internal/segment"
	"github.com/skeletg/skelet/apps/backend/signals/internal/sendlog"
	"github.com/skeletg/skelet/apps/backend/signals/internal/signalsboot"
	"github.com/skeletg/skelet/apps/backend/signals/internal/suppression"
)

type RootContext struct{ context.Context } // supplied by the entrypoint rather than provided, since that's where signal handling lives

// AllowPrivateTargets lets destinations point at private addresses - dev only, or set explicitly
type AllowPrivateTargets bool

// QueueMode says whether this process wraps its spine writes into the ingest queue or holds the bare engine
// Producers wrap so appends land in JetStream before the request returns; the consumer must not, or it would feed itself
type QueueMode bool

const (
	QueueProducer QueueMode = true
	QueueConsumer QueueMode = false
)

// Services is every suite service, constructed from the pools and each other - the entrypoint decides
// which of them it serves
var Services = fx.Module("services",
	fx.Provide(
		newAllowPrivateTargets,
		newSettings,
		config.NewChannelScoped,
		newStack,
		newRecorder,
		newRoster,
		newTraits,
		newLens,
		newExperiment,
		newSegment,
		newDestination,
		newChannelRoster,
		newRollup,
		newBehavioral,
		newGeo,
		newArchive,
		newSendLog,
		newSuppression,
	),
)

// The same rule core applies to its webhooks, read here from the same variable so both agree
func newAllowPrivateTargets() AllowPrivateTargets {
	if v := os.Getenv("ALLOW_PRIVATE_WEBHOOK_TARGETS"); v != "" {
		b, err := strconv.ParseBool(v)
		return AllowPrivateTargets(err == nil && b)
	}
	return AllowPrivateTargets(os.Getenv("APP_ENV") == "development")
}

// The suite's settings service reads and writes its own store alone - every path the suite reads is a
// Signals path, and core administers them over the contract rather than the other way round
func newSettings(q Queries, vk *redis.Client) *config.Service {
	repo := signalsboot.NewSettingsRepository(q.Queries)
	svc := config.NewService(repo)
	svc.SetSignalsRepo(repo)
	if vk != nil {
		svc.SetCache(cache.New(vk, "config")) // the namespace core's own settings cache uses, so an admin edit there invalidates here too
	}
	return svc
}

// Archive is where the retention sweeps export aged rows before deleting them, and only the consumer runs those sweeps
// Store is nil when there is no destination
type Archive struct {
	Store  archivefile.ObjectStore
	Prefix string
}

func newArchive(rc RootContext, mode QueueMode) Archive {
	if mode != QueueConsumer {
		return Archive{}
	}
	store, prefix := archiveDestination(rc.Context)
	return Archive{Store: store, Prefix: prefix}
}

func (a Archive) sink() archivefile.Sink {
	if a.Store == nil {
		return nil
	}
	return archivefile.NewStorageSink(a.Store, a.Prefix)
}

func newStack(lc fx.Lifecycle, rc RootContext, mode QueueMode, q Queries, rq ReadQueries, a Archive) (*signalsboot.Stack, error) {
	opts := signalsboot.Options{
		Queries: q.Queries, WrapQueue: bool(mode),
		ArchiveStore: a.Store, ArchivePrefix: a.Prefix,
	}
	stack, err := signalsboot.Build(rc.Context, opts)
	if err != nil {
		return nil, err
	}
	lc.Append(fx.Hook{OnStop: func(context.Context) error { stack.Close(); return nil }})
	return stack, nil
}

func newRecorder(s *signalsboot.Stack) *analytics.Recorder { return s.Recorder }

func newRoster(s *signalsboot.Stack) *analytics.Roster { return s.Roster }

func newTraits(s *signalsboot.Stack) *analytics.TraitCompute { return s.Traits }

func newLens(q Queries, rec *analytics.Recorder, cfg *config.ChannelScoped, rollup *analytics.TrafficRollup) *lens.Service {
	return lens.NewService(q.Queries, rec, cfg, rollup)
}

func newExperiment(q Queries, rec *analytics.Recorder) *experiment.Service { // after the spine engine is chosen - its repo reads exposures through the Recorder
	return experiment.NewService(experiment.NewRepository(q.Queries, rec))
}

// The rules query runs on the signals pool, and a windowed order condition asks the spine - the engine itself, since a read needs no queue in front of it
func newSegment(q Queries, p SignalsPool, txMgr *txrunner.Manager, stack *signalsboot.Stack) *segment.Service {
	return segment.NewService(segment.NewRepository(q.Queries), txMgr, p.DB, spineOrders{stack.Spine})
}

// The spine answering "who ordered what since when" for a segment rule - the same read trait compute takes its lifetime aggregates from, narrowed
type spineOrders struct{ spine analytics.SpineStore }

func (o spineOrders) CustomerOrdersSince(ctx context.Context, channelID uuid.UUID, since time.Time) ([]segment.CustomerOrderStat, error) {
	rows, err := o.spine.ChannelOrderAggregates(ctx, channelID, since)
	if err != nil {
		return nil, err
	}
	out := make([]segment.CustomerOrderStat, len(rows))
	for i, r := range rows {
		out[i] = segment.CustomerOrderStat{CustomerID: r.CustomerID, OrderCount: int64(r.OrderCount), MonetaryCents: r.MonetaryCents}
	}
	return out, nil
}

func newDestination(q Queries, rec *analytics.Recorder, allow AllowPrivateTargets) *destination.Service {
	return destination.NewService(q.Queries, rec, urlGuard{}, bool(allow))
}

// The beacon surface's copy of core's active channels, which core's push job keeps current
func newChannelRoster(q Queries) *analytics.ChannelRoster {
	return analytics.NewChannelRoster(q.Queries)
}

// Coalesces in memory and flushes aggregates, so busy sites never store a row per hit nor scan raw events
func newRollup(q Queries) *analytics.TrafficRollup { return analytics.NewTrafficRollup(q.Queries) }

func newBehavioral(rec *analytics.Recorder, segs *segment.Service) *behavioralsegment.Service {
	return behavioralsegment.New(rec, segs)
}

// The send log core's campaigns and flows write over the contract, with the same archive destination
// the spine sweep uses - unset, archive mode fails closed rather than deleting what it could not export
func newSendLog(q Queries, rq ReadQueries, a Archive) *sendlog.Store {
	store := sendlog.NewStore(q.Queries)
	if rq.Queries != nil {
		store.SetReadQueries(rq.Queries)
	}
	if sink := a.sink(); sink != nil {
		store.SetArchiveSink(sink)
	}
	return store
}

func newSuppression(q Queries) *suppression.Service { return suppression.New(q.Queries) }

func newGeo() *geo.Resolver { // a bad GEOIP_DB_PATH is logged rather than fatal - geo just stops appearing on profiles
	resolver, err := geo.Open(os.Getenv("GEOIP_DB_PATH"))
	if err != nil {
		log.Printf("[geo] disabled - could not open GEOIP_DB_PATH: %v", err)
		resolver, _ = geo.Open("")
	}
	return resolver
}
