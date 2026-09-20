package signalsclient

import (
	"context"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
)

// The suite's upkeep, driven from core's scheduler - it owns the cadence and the run history, the
// suite owns the work. Every one runs as core itself, a scheduled run having no admin behind it

func (c *Client) PruneSpine(ctx context.Context) error {
	sys, err := c.System(ctx)
	if err != nil {
		return err
	}
	_, err = c.Maintenance.PruneSpine(sys, &signalsv1.PruneSpineRequest{})
	return err
}

func (c *Client) PruneLens(ctx context.Context) error {
	sys, err := c.System(ctx)
	if err != nil {
		return err
	}
	_, err = c.Maintenance.PruneLens(sys, &signalsv1.PruneLensRequest{})
	return err
}

func (c *Client) ComputeTraits(ctx context.Context) error {
	sys, err := c.System(ctx)
	if err != nil {
		return err
	}
	_, err = c.Maintenance.ComputeTraits(sys, &signalsv1.ComputeTraitsRequest{})
	return err
}

func (c *Client) ForwardDestinations(ctx context.Context) error {
	sys, err := c.System(ctx)
	if err != nil {
		return err
	}
	_, err = c.Maintenance.ForwardDestinations(sys, &signalsv1.ForwardDestinationsRequest{})
	return err
}

// AdvanceFlowRuns is one tick of the flow engine - the suite keeps claiming batches while the budget allows, zero runs one batch
// Answers how many runs the tick advanced
func (c *Client) AdvanceFlowRuns(ctx context.Context, budget time.Duration) (int, error) {
	sys, err := c.System(ctx)
	if err != nil {
		return 0, err
	}
	res, err := c.Maintenance.AdvanceFlowRuns(sys, &signalsv1.AdvanceFlowRunsRequest{Budget: durationpb.New(budget)})
	if err != nil {
		return 0, err
	}
	return int(res.GetRunsAdvanced()), nil
}

// ProcessDueCampaigns is one tick of the campaign pipeline - send passes go on while the budget allows
// Answers how many recipients it handled
func (c *Client) ProcessDueCampaigns(ctx context.Context, budget time.Duration) (int, error) {
	sys, err := c.System(ctx)
	if err != nil {
		return 0, err
	}
	res, err := c.Maintenance.ProcessDueCampaigns(sys, &signalsv1.ProcessDueCampaignsRequest{Budget: durationpb.New(budget)})
	if err != nil {
		return 0, err
	}
	return int(res.GetRecipientsProcessed()), nil
}

// What a night's Signals backup produced: the database dump and, when a bucket is configured, the spine's copy beside it
type SignalsBackup struct {
	Key         string
	Bytes       int64
	Pruned      []string
	SpinePrefix string
	SpinePruned []string
}

// BackupDatabase asks the suite to back itself up into the shared BACKUP_* destination under the night's stamp
func (c *Client) BackupDatabase(ctx context.Context, at time.Time) (SignalsBackup, error) {
	sys, err := c.System(ctx)
	if err != nil {
		return SignalsBackup{}, err
	}
	res, err := c.Maintenance.BackupDatabase(sys, &signalsv1.BackupDatabaseRequest{At: timestamppb.New(at)})
	if err != nil {
		return SignalsBackup{}, err
	}
	return SignalsBackup{
		Key: res.GetKey(), Bytes: res.GetBytes(), Pruned: res.GetPruned(),
		SpinePrefix: res.GetSpinePrefix(), SpinePruned: res.GetSpinePruned(),
	}, nil
}

// RestoreDatabase is the destructive half, for `skelet db restore` handing a signals dump to the process that holds that database
func (c *Client) RestoreDatabase(ctx context.Context, key string, jobs int) error {
	sys, err := c.System(ctx)
	if err != nil {
		return err
	}
	_, err = c.Maintenance.RestoreDatabase(sys, &signalsv1.RestoreDatabaseRequest{Key: key, Jobs: int32(jobs)})
	return err
}

// PruneSendLog is the send-log sweep - partitions ahead, whole months behind the window dropped, archived first under retention_action=archive
func (c *Client) PruneSendLog(ctx context.Context) error {
	sys, err := c.System(ctx)
	if err != nil {
		return err
	}
	_, err = c.Maintenance.PruneSendLog(sys, &signalsv1.PruneSendLogRequest{})
	return err
}

// SuiteHealth is what core's health page reports about the suite, gathered in one round trip
type SuiteHealth struct {
	SpineEngine       string
	SpineError        string
	SpineOldest       *time.Time
	SpineNewest       *time.Time
	PendingErasures   int
	ErasuresError     string
	Queue             QueueHealth
	Destinations      []DestinationHealth
	DestinationsError string
}

type QueueHealth struct {
	Configured     bool
	Error          string
	HasConsumer    bool
	StreamMessages int64
	Pending        int64
	Redelivering   int64
}

type DestinationHealth struct {
	Name            string
	LastError       string
	LastForwardedAt *time.Time
}

func (c *Client) Health(ctx context.Context) (SuiteHealth, error) {
	sys, err := c.System(ctx)
	if err != nil {
		return SuiteHealth{}, err
	}
	res, err := c.Maintenance.GetHealth(sys, &signalsv1.GetHealthRequest{})
	if err != nil {
		return SuiteHealth{}, err
	}
	h := SuiteHealth{
		SpineEngine: res.GetSpineEngine(), SpineError: res.GetSpineError(),
		SpineOldest: timeOrNil(res.SpineOldest), SpineNewest: timeOrNil(res.SpineNewest),
		PendingErasures: int(res.GetPendingErasures()), ErasuresError: res.GetErasuresError(),
		DestinationsError: res.GetDestinationsError(),
	}
	if q := res.GetQueue(); q != nil {
		h.Queue = QueueHealth{
			Configured: q.GetConfigured(), Error: q.GetError(), HasConsumer: q.GetHasConsumer(),
			StreamMessages: q.GetStreamMessages(), Pending: q.GetPending(), Redelivering: q.GetRedelivering(),
		}
	}
	for _, d := range res.GetDestinations() {
		h.Destinations = append(h.Destinations, DestinationHealth{
			Name: d.GetName(), LastError: d.GetLastError(), LastForwardedAt: timeOrNil(d.LastForwardedAt),
		})
	}
	return h, nil
}

// BrowseAbandoner is one idle product browse, for the flow that chases it
type BrowseAbandoner struct {
	CustomerID   uuid.UUID
	ChannelID    uuid.UUID
	PageKey      string
	Sku          string
	ProductName  string
	ProductImage string
	LastBrowseAt time.Time
}

func (c *Client) BrowseAbandoners(ctx context.Context, idleSince, maxAge time.Time, limit int32) ([]BrowseAbandoner, error) {
	sys, err := c.System(ctx)
	if err != nil {
		return nil, err
	}
	res, err := c.Maintenance.ListBrowseAbandoners(sys, &signalsv1.ListBrowseAbandonersRequest{
		IdleSince: timestamppb.New(idleSince), MaxAge: timestamppb.New(maxAge), Limit: limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]BrowseAbandoner, 0, len(res.GetAbandoners()))
	for _, a := range res.GetAbandoners() {
		cust, cerr := uuid.Parse(a.GetCustomerId())
		ch, cherr := uuid.Parse(a.GetChannelId())
		if cerr != nil || cherr != nil {
			continue
		}
		out = append(out, BrowseAbandoner{
			CustomerID: cust, ChannelID: ch, PageKey: a.GetPageKey(), Sku: a.GetSku(),
			ProductName: a.GetProductName(), ProductImage: a.GetProductImage(),
			LastBrowseAt: a.GetLastBrowseAt().AsTime(),
		})
	}
	return out, nil
}

func (c *Client) MarkBrowseNotified(ctx context.Context, channelID, customerID uuid.UUID, lastBrowseAt time.Time) error {
	sys, err := c.System(ctx)
	if err != nil {
		return err
	}
	_, err = c.Maintenance.MarkBrowseNotified(sys, &signalsv1.MarkBrowseNotifiedRequest{
		ChannelId: channelID.String(), CustomerId: customerID.String(), LastBrowseAt: timestamppb.New(lastBrowseAt),
	})
	return err
}

func timeOrNil(ts *timestamppb.Timestamp) *time.Time {
	if ts == nil {
		return nil
	}
	t := ts.AsTime()
	return &t
}

// RefreshDynamicSegments re-materializes the rule-based audiences, reporting a bad one rather than failing
func (c *Client) RefreshDynamicSegments(ctx context.Context) (segments, members int, reported string, err error) {
	sys, err := c.System(ctx)
	if err != nil {
		return 0, 0, "", err
	}
	res, err := c.Maintenance.RefreshDynamicSegments(sys, &signalsv1.RefreshDynamicSegmentsRequest{})
	if err != nil {
		return 0, 0, "", err
	}
	return int(res.GetSegments()), int(res.GetMembers()), res.GetError(), nil
}

// AffinityAttribute is one catalog attribute an operator flagged for auto-segmenting
type AffinityAttribute struct{ Slug, Name string }

// RefreshBehavioralSegments recomputes the suite's audiences - whoever newly joined one starts their segment_entered runs there
func (c *Client) RefreshBehavioralSegments(ctx context.Context, autoSegment []AffinityAttribute) error {
	sys, err := c.System(ctx)
	if err != nil {
		return err
	}
	attrs := make([]*signalsv1.AffinityAttribute, 0, len(autoSegment))
	for _, a := range autoSegment {
		attrs = append(attrs, &signalsv1.AffinityAttribute{Slug: a.Slug, Name: a.Name})
	}
	_, err = c.Maintenance.RefreshBehavioralSegments(sys, &signalsv1.RefreshBehavioralSegmentsRequest{
		AutoSegmentAttributes: attrs,
	})
	return err
}

// CustomerTraits is what Synapse knows about one customer - Computed false means the recompute has
// not reached them, which a flow condition must tell apart from every score being zero
type CustomerTraits struct {
	Computed        bool
	RFMRecency      int
	RFMFrequency    int
	RFMMonetary     int
	Lifecycle       string
	EngagementScore int
	Frequency       int
	MonetaryCents   int64
	Affinities      map[string]string
}

func (c *Client) CustomerTraits(ctx context.Context, customerID uuid.UUID, affinityWindowDays int) (CustomerTraits, error) {
	sys, err := c.System(ctx)
	if err != nil {
		return CustomerTraits{}, err
	}
	res, err := c.Maintenance.GetCustomerTraits(sys, &signalsv1.GetCustomerTraitsRequest{
		CustomerId: customerID.String(), AffinityWindowDays: int32(affinityWindowDays),
	})
	if err != nil {
		return CustomerTraits{}, err
	}
	return CustomerTraits{
		Computed:   res.GetComputed(),
		RFMRecency: int(res.GetRfmRecency()), RFMFrequency: int(res.GetRfmFrequency()), RFMMonetary: int(res.GetRfmMonetary()),
		Lifecycle: res.GetLifecycle(), EngagementScore: int(res.GetEngagementScore()),
		Frequency: int(res.GetFrequency()), MonetaryCents: res.GetMonetaryCents(),
		Affinities: res.GetAffinities(),
	}, nil
}
