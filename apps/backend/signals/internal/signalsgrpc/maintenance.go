package signalsgrpc

// The upkeep half of the contract. Like the ingest writes these authorize on a system token: core's
// scheduler drives them, and a scheduled run has no admin to act for

import (
	"context"
	"encoding/json"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/signalsapi"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics/spinequeue"
	"github.com/skeletg/skelet/apps/backend/signals/internal/behavioralsegment"
	"github.com/skeletg/skelet/apps/backend/signals/internal/destination"
	"github.com/skeletg/skelet/apps/backend/signals/internal/lens"
	"github.com/skeletg/skelet/apps/backend/signals/internal/segment"
	"github.com/skeletg/skelet/apps/backend/signals/internal/signalsboot"
)

type maintenanceServer struct {
	roster *analytics.Roster
	traits *analytics.TraitCompute
	signalsv1.UnimplementedMaintenanceServiceServer
	rec         *analytics.Recorder
	lens        *lens.Service
	destination destinationLister
	segments    *segment.Service
	behavioral  *behavioralsegment.Service
	queue       queueStats
	settings    signalsboot.Settings
	sends       sendLogMaintainer // nil in a test server without a log
	flows       flowEngine        // nil in a test server without Pulse
	campaigns   campaignPipeline
}

// The engine ticks core's scheduler drives - one advance of the runnable runs, one pass over the due campaigns
type flowEngine interface {
	Drain(ctx context.Context, budget time.Duration) (int, error)
	// The segment_entered moments start off the refresh that found them
	Enroll(ctx context.Context, trigger string, channelID, customerID uuid.UUID, email string, payload json.RawMessage, key string) int
}

type campaignPipeline interface {
	Drain(ctx context.Context, budget time.Duration) (int, error)
}

// The slice of the send log the retention sweep drives
type sendLogMaintainer interface {
	EnsurePartitions(ctx context.Context) error
	Prune(ctx context.Context, before time.Time, archive bool) (int64, error)
}

// The slice of destination.Service this surface drives
type destinationLister interface {
	ForwardAll(ctx context.Context) error
	ListActive(ctx context.Context) ([]destination.Destination, error)
}

// The queue is optional, so this is nil when none is configured
type queueStats interface {
	Stats(ctx context.Context) (spinequeue.Stats, error)
}

func (s *maintenanceServer) PruneSpine(ctx context.Context, _ *signalsv1.PruneSpineRequest) (*signalsv1.PruneSpineResponse, error) {
	if err := requireSystem(ctx); err != nil {
		return nil, err
	}
	if err := signalsboot.SpinePruneJob(s.settings, s.rec)(ctx); err != nil {
		return nil, err
	}
	return &signalsv1.PruneSpineResponse{}, nil
}

func (s *maintenanceServer) PruneLens(ctx context.Context, _ *signalsv1.PruneLensRequest) (*signalsv1.PruneLensResponse, error) {
	if err := requireSystem(ctx); err != nil {
		return nil, err
	}
	if err := signalsboot.LensPruneJob(s.settings, s.lens)(ctx); err != nil {
		return nil, err
	}
	return &signalsv1.PruneLensResponse{}, nil
}

func (s *maintenanceServer) PruneSendLog(ctx context.Context, _ *signalsv1.PruneSendLogRequest) (*signalsv1.PruneSendLogResponse, error) {
	if err := requireSystem(ctx); err != nil {
		return nil, err
	}
	if s.sends == nil {
		return &signalsv1.PruneSendLogResponse{}, nil
	}
	if err := signalsboot.SendLogRetentionJob(s.settings, s.sends)(ctx); err != nil {
		return nil, err
	}
	return &signalsv1.PruneSendLogResponse{}, nil
}

// tickBudget is how long a tick may keep draining - what the caller asked for, held inside the call's deadline with room to answer
func tickBudget(ctx context.Context, asked *durationpb.Duration) time.Duration {
	budget := asked.AsDuration()
	if dl, ok := ctx.Deadline(); ok {
		if room := time.Until(dl) - 5*time.Second; room < budget {
			budget = max(room, 0)
		}
	}
	return budget
}

func (s *maintenanceServer) AdvanceFlowRuns(ctx context.Context, req *signalsv1.AdvanceFlowRunsRequest) (*signalsv1.AdvanceFlowRunsResponse, error) {
	if err := requireSystem(ctx); err != nil {
		return nil, err
	}
	if s.flows == nil {
		return &signalsv1.AdvanceFlowRunsResponse{}, nil
	}
	n, err := s.flows.Drain(ctx, tickBudget(ctx, req.GetBudget()))
	if err != nil {
		return nil, err
	}
	return &signalsv1.AdvanceFlowRunsResponse{RunsAdvanced: int32(n)}, nil
}

func (s *maintenanceServer) ProcessDueCampaigns(ctx context.Context, req *signalsv1.ProcessDueCampaignsRequest) (*signalsv1.ProcessDueCampaignsResponse, error) {
	if err := requireSystem(ctx); err != nil {
		return nil, err
	}
	if s.campaigns == nil {
		return &signalsv1.ProcessDueCampaignsResponse{}, nil
	}
	n, err := s.campaigns.Drain(ctx, tickBudget(ctx, req.GetBudget()))
	if err != nil {
		return nil, err
	}
	return &signalsv1.ProcessDueCampaignsResponse{RecipientsProcessed: int32(n)}, nil
}

func (s *maintenanceServer) ComputeTraits(ctx context.Context, _ *signalsv1.ComputeTraitsRequest) (*signalsv1.ComputeTraitsResponse, error) {
	if err := requireSystem(ctx); err != nil {
		return nil, err
	}
	if err := signalsboot.TraitComputeJob(s.roster, s.traits, s.settings)(ctx); err != nil {
		return nil, err
	}
	return &signalsv1.ComputeTraitsResponse{}, nil
}

func (s *maintenanceServer) ForwardDestinations(ctx context.Context, _ *signalsv1.ForwardDestinationsRequest) (*signalsv1.ForwardDestinationsResponse, error) {
	if err := requireSystem(ctx); err != nil {
		return nil, err
	}
	if err := s.destination.ForwardAll(ctx); err != nil {
		return nil, err
	}
	return &signalsv1.ForwardDestinationsResponse{}, nil
}

func (s *maintenanceServer) GetRetentionAction(ctx context.Context, _ *signalsv1.GetRetentionActionRequest) (*signalsv1.GetRetentionActionResponse, error) {
	if err := requireSystem(ctx); err != nil {
		return nil, err
	}
	archive, err := signalsboot.ArchiveAction(ctx, s.settings)
	if err != nil {
		return nil, err
	}
	return &signalsv1.GetRetentionActionResponse{Archive: archive}, nil
}

// GetHealth gathers every probe core's health page shows for the suite. Each carries its own error
// text rather than failing the call, so one unreachable dependency does not blank the whole panel
func (s *maintenanceServer) GetHealth(ctx context.Context, _ *signalsv1.GetHealthRequest) (*signalsv1.GetHealthResponse, error) {
	if err := requireSystem(ctx); err != nil {
		return nil, err
	}
	out := &signalsv1.GetHealthResponse{SpineEngine: "clickhouse"} // no enabled flag - a deployed suite is on, there is no switch here to report
	if stats, err := s.rec.SpineStats(ctx); err != nil {
		out.SpineError = err.Error()
	} else {
		out.SpineOldest, out.SpineNewest = timestampOrNil(stats.Oldest), timestampOrNil(stats.Newest)
	}
	if n, err := s.rec.PendingErasures(ctx); err != nil {
		out.ErasuresError = err.Error()
	} else {
		out.PendingErasures = int32(n)
	}
	out.Queue = s.queueHealth(ctx)
	if dests, err := s.destination.ListActive(ctx); err != nil {
		out.DestinationsError = err.Error()
	} else {
		for _, d := range dests {
			out.Destinations = append(out.Destinations, &signalsv1.DestinationHealth{
				Name: d.Name, LastError: d.LastError, LastForwardedAt: timestampOrNil(d.LastForwardedAt),
			})
		}
	}
	return out, nil
}

func (s *maintenanceServer) queueHealth(ctx context.Context) *signalsv1.QueueHealth {
	if s.queue == nil {
		return &signalsv1.QueueHealth{}
	}
	stats, err := s.queue.Stats(ctx)
	if err != nil {
		return &signalsv1.QueueHealth{Configured: true, Error: err.Error()}
	}
	return &signalsv1.QueueHealth{
		Configured: true, HasConsumer: stats.HasConsumer,
		StreamMessages: int64(stats.StreamMessages), Pending: int64(stats.Pending),
		Redelivering: int64(stats.Redelivering),
	}
}

func (s *maintenanceServer) ListBrowseAbandoners(ctx context.Context, req *signalsv1.ListBrowseAbandonersRequest) (*signalsv1.ListBrowseAbandonersResponse, error) {
	if err := requireSystem(ctx); err != nil {
		return nil, err
	}
	var channelID uuid.UUID
	if raw := req.GetChannelId(); raw != "" {
		id, err := parseID("channel_id", raw)
		if err != nil {
			return nil, err
		}
		channelID = id
	}
	cands, err := s.lens.ListBrowseAbandoners(ctx, channelID, req.GetIdleSince().AsTime(), req.GetMaxAge().AsTime(), req.GetLimit())
	if err != nil {
		return nil, err
	}
	out := make([]*signalsv1.BrowseAbandoner, 0, len(cands))
	for _, c := range cands {
		out = append(out, &signalsv1.BrowseAbandoner{
			CustomerId: c.CustomerID.String(), ChannelId: c.ChannelID.String(),
			PageKey: c.PageKey, Sku: c.Sku, ProductName: c.ProductName, ProductImage: c.ProductImage,
			LastBrowseAt: timestamppb.New(c.LastBrowseAt),
		})
	}
	return &signalsv1.ListBrowseAbandonersResponse{Abandoners: out}, nil
}

func (s *maintenanceServer) MarkBrowseNotified(ctx context.Context, req *signalsv1.MarkBrowseNotifiedRequest) (*signalsv1.MarkBrowseNotifiedResponse, error) {
	if err := requireSystem(ctx); err != nil {
		return nil, err
	}
	channelID, err := parseID("channel_id", req.GetChannelId())
	if err != nil {
		return nil, err
	}
	customerID, err := parseID("customer_id", req.GetCustomerId())
	if err != nil {
		return nil, err
	}
	if err := s.lens.MarkBrowseNotified(ctx, channelID, customerID, req.GetLastBrowseAt().AsTime()); err != nil {
		return nil, err
	}
	return &signalsv1.MarkBrowseNotifiedResponse{}, nil
}

func timestampOrNil(t *time.Time) *timestamppb.Timestamp {
	if t == nil {
		return nil
	}
	return timestamppb.New(*t)
}

func (s *maintenanceServer) RefreshDynamicSegments(ctx context.Context, _ *signalsv1.RefreshDynamicSegmentsRequest) (*signalsv1.RefreshDynamicSegmentsResponse, error) {
	if err := requireSystem(ctx); err != nil {
		return nil, err
	}
	segs, members, err := s.segments.RefreshAllDynamic(ctx)
	out := &signalsv1.RefreshDynamicSegmentsResponse{Segments: int32(segs), Members: int32(members)}
	if err != nil {
		out.Error = err.Error() // one bad segment is reported, not raised - failing would only retry the whole sweep
	}
	return out, nil
}

// The affinity sync is best-effort: a failure there must not cost the membership refresh below it
func (s *maintenanceServer) RefreshBehavioralSegments(ctx context.Context, req *signalsv1.RefreshBehavioralSegmentsRequest) (*signalsv1.RefreshBehavioralSegmentsResponse, error) {
	if err := requireSystem(ctx); err != nil {
		return nil, err
	}
	now := time.Now()
	if attrs := req.GetAutoSegmentAttributes(); len(attrs) > 0 {
		auto := make([]analytics.AffinityAttr, 0, len(attrs))
		for _, a := range attrs {
			auto = append(auto, analytics.AffinityAttr{Slug: a.GetSlug(), Name: a.GetName()})
		}
		channels, err := s.roster.RosterChannels(ctx) // core's channels, already copied across for the trait recompute
		if err != nil {
			return nil, err
		}
		_, _, _ = s.behavioral.SyncAffinity(ctx, behavioralsegment.AffinitySyncOptions{
			Channels: channels, Attributes: auto,
			MinActors:  s.settingInt(ctx, "signals/synapse/affinity_segment_min_actors", 5),
			MaxValues:  s.settingInt(ctx, "signals/synapse/affinity_segment_max_values", 20),
			WindowDays: s.settingInt(ctx, "signals/synapse/affinity_segment_window_days", 90),
		}, now)
	}
	// Collected first and enrolled after, so the membership swap holds no transaction open while runs are written
	type entry struct {
		channel, customer uuid.UUID
		slug              string
	}
	var entered []entry
	n, err := s.behavioral.RefreshAll(ctx, now, func(ch, cust uuid.UUID, slug string) {
		entered = append(entered, entry{channel: ch, customer: cust, slug: slug})
	})
	if err != nil {
		return nil, err
	}
	if s.flows != nil {
		for _, e := range entered {
			payload, _ := json.Marshal(map[string]string{signalsapi.PayloadSegmentSlug: e.slug})
			s.flows.Enroll(ctx, signalsapi.TriggerSegmentEntered, e.channel, e.customer, "", payload, "")
		}
	}
	return &signalsv1.RefreshBehavioralSegmentsResponse{Refreshed: int32(n)}, nil
}

func (s *maintenanceServer) settingInt(ctx context.Context, path string, def int) int {
	if v, err := s.settings.Int(ctx, path, nil); err == nil && v > 0 {
		return int(v)
	}
	return def
}

// GetCustomerTraits answers both halves a flow run needs about one customer - the trait projection its
// conditions split on, and the attribute affinities its merge tags render
func (s *maintenanceServer) GetCustomerTraits(ctx context.Context, req *signalsv1.GetCustomerTraitsRequest) (*signalsv1.GetCustomerTraitsResponse, error) {
	if err := requireSystem(ctx); err != nil {
		return nil, err
	}
	id, err := parseID("customer_id", req.GetCustomerId())
	if err != nil {
		return nil, err
	}
	out := &signalsv1.GetCustomerTraitsResponse{}
	t, ok, err := s.rec.CustomerTraits(ctx, id)
	if err != nil {
		return nil, err
	}
	if ok {
		out.Computed = true
		out.RfmRecency, out.RfmFrequency, out.RfmMonetary = int32(t.RFMRecency), int32(t.RFMFrequency), int32(t.RFMMonetary)
		out.Lifecycle, out.EngagementScore = t.Lifecycle, int32(t.EngagementScore)
		out.Frequency, out.MonetaryCents = int32(t.Frequency), t.MonetaryCents
	}
	days := req.GetAffinityWindowDays()
	if days > 0 {
		affs, aerr := s.rec.CustomerTopAffinities(ctx, id, time.Now().AddDate(0, 0, -int(days)))
		if aerr != nil {
			return nil, aerr
		}
		out.Affinities = affs
	}
	return out, nil
}

// The operator maintenance an admin drives from the CLI - it reaches the suite over the contract, so
// the tooling needs neither the signals database nor the object-store credentials
