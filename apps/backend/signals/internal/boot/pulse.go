package boot

// Pulse - the email builder, the flow engine and the campaign pipeline - composed from the suite's own pools and the rosters core pushes
// Nothing here reaches core except the builder's live catalog data, which goes over core's public storefront API

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/fx"

	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/email"
	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
	"github.com/skeletg/skelet/apps/backend/signals/internal/campaign"
	"github.com/skeletg/skelet/apps/backend/signals/internal/corefront"
	signalsdb "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/signals/internal/emailtemplate"
	"github.com/skeletg/skelet/apps/backend/signals/internal/engagement"
	"github.com/skeletg/skelet/apps/backend/signals/internal/flow"
	"github.com/skeletg/skelet/apps/backend/signals/internal/segment"
	"github.com/skeletg/skelet/apps/backend/signals/internal/sendlog"
	"github.com/skeletg/skelet/apps/backend/signals/internal/signalsgrpc"
	"github.com/skeletg/skelet/apps/backend/signals/internal/sms"
	"github.com/skeletg/skelet/apps/backend/signals/internal/suppression"
)

// Pulse is the module - separate from Services so the beacon tier, which sends nothing, never builds
// a mail gateway it would not use
var Pulse = fx.Module("pulse",
	fx.Provide(
		newMailer,
		newTexter,
		newTracker,
		newRosterStores,
		newRosterContacts,
		newBuilder,
		newFlow,
		newCampaign,
	),
)

// The same SMTP_* and TWILIO_* variables core's own senders read - one relay for the whole install
func newMailer(cfg Config) email.Sender { return email.New(cfg.Env) }

func newTexter(cfg Config) sms.Sender { return sms.New(cfg.Env) }

func newTracker(rec *analytics.Recorder, sends *sendlog.Store) *engagement.Tracker {
	return engagement.New(rec, sends)
}

// RosterStores answers "what shop is this channel" off the channel roster, in each engine's own shape
type RosterStores struct{ channels *analytics.ChannelRoster }

func newRosterStores(ch *analytics.ChannelRoster) *RosterStores { return &RosterStores{channels: ch} }

func (r *RosterStores) entry(ctx context.Context, channelID uuid.UUID) (analytics.ChannelEntry, error) {
	e, ok := r.channels.Entry(ctx, channelID)
	if !ok {
		return analytics.ChannelEntry{}, fmt.Errorf("channel %s is not on the roster core pushed", channelID)
	}
	return e, nil
}

// Get is the builder's view, which renders a logo and links back to the storefront
func (r *RosterStores) Get(ctx context.Context, channelID uuid.UUID) (emailtemplate.Store, error) {
	e, err := r.entry(ctx, channelID)
	if err != nil {
		return emailtemplate.Store{}, err
	}
	return emailtemplate.Store{ID: e.ID, Name: e.Name, PublicURL: e.PublicURL, LogoURL: e.LogoURL}, nil
}

type flowStores struct{ *RosterStores }

func (r flowStores) Get(ctx context.Context, channelID uuid.UUID) (flow.Store, error) {
	e, err := r.entry(ctx, channelID)
	if err != nil {
		return flow.Store{}, err
	}
	return flow.Store{ID: e.ID, Name: e.Name, PublicURL: e.PublicURL, FromAddress: e.FromAddress, FromName: e.FromName, LogoURL: e.LogoURL}, nil
}

type campaignStores struct{ *RosterStores }

func (r campaignStores) Get(ctx context.Context, channelID uuid.UUID) (campaign.Store, error) {
	e, err := r.entry(ctx, channelID)
	if err != nil {
		return campaign.Store{}, err
	}
	return campaign.Store{ID: e.ID, Name: e.Name, PublicURL: e.PublicURL, FromAddress: e.FromAddress, FromName: e.FromName, LogoURL: e.LogoURL}, nil
}

// RosterContacts answers "who is this customer" off the customer roster - both consents included, as
// core last pushed them, which is what stops a run mailing someone who unsubscribed while it waited
type RosterContacts struct{ q *signalsdb.Queries }

func newRosterContacts(q Queries) *RosterContacts { return &RosterContacts{q: q.Queries} }

// Get is the flow engine's view of one person
func (r *RosterContacts) Get(ctx context.Context, customerID uuid.UUID) (flow.Contact, bool, error) {
	row, err := r.q.RosterContact(ctx, customerID)
	if err != nil {
		return flow.Contact{}, false, ignoreNoRows(err) // a run whose customer is gone simply stops
	}
	return flow.Contact{
		ID: row.CustomerID, ChannelID: row.ChannelID, Email: row.Email, FirstName: row.FirstName,
		Phone: strings.TrimSpace(row.Phone), EmailOptedIn: row.MarketingOptedIn, SMSOptedIn: row.SmsOptedIn,
	}, true, nil
}

// PreviewContacts is the builder's preview seam over the same row - a preview rendered as a real customer
type PreviewContacts struct{ *RosterContacts }

func (r PreviewContacts) Get(ctx context.Context, customerID uuid.UUID) (signalsgrpc.PreviewContact, bool, error) {
	c, ok, err := r.RosterContacts.Get(ctx, customerID)
	return signalsgrpc.PreviewContact{Email: c.Email, FirstName: c.FirstName}, ok, err
}

// The audience a campaign draws from, consent included, and consent re-read for a page at send time
type campaignAudience struct{ *RosterContacts }

func (r campaignAudience) ListForChannel(ctx context.Context, channelID uuid.UUID) ([]campaign.Contact, error) {
	rows, err := r.q.ChannelRosterContacts(ctx, channelID)
	if err != nil {
		return nil, err
	}
	out := make([]campaign.Contact, 0, len(rows))
	for _, row := range rows {
		c := campaign.Contact{
			ID: row.CustomerID, ChannelID: row.ChannelID, Email: row.Email, FirstName: row.FirstName,
			Active: row.IsActive, OptedIn: row.MarketingOptedIn,
		}
		if row.CustomerGroupID.Valid {
			c.GroupID = row.CustomerGroupID.UUID
		}
		out = append(out, c)
	}
	return out, nil
}

func (r campaignAudience) Get(ctx context.Context, customerID uuid.UUID) (campaign.Contact, bool, error) {
	c, ok, err := r.RosterContacts.Get(ctx, customerID)
	if err != nil || !ok {
		return campaign.Contact{}, ok, err
	}
	return campaign.Contact{ID: c.ID, ChannelID: c.ChannelID, Email: c.Email, FirstName: c.FirstName, Active: true, OptedIn: c.EmailOptedIn}, true, nil
}

func (r campaignAudience) OptedInByIDs(ctx context.Context, customerIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
	out := make(map[uuid.UUID]bool, len(customerIDs))
	if len(customerIDs) == 0 {
		return out, nil
	}
	rows, err := r.q.RosterMarketingConsent(ctx, customerIDs)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.CustomerID] = row.MarketingOptedIn
	}
	return out, nil
}

// The builder, with the products block reading core's storefront API when CORE_FRONT_API_URL names it
func newBuilder(cfg Config, q Queries, stores *RosterStores) *emailtemplate.Service {
	svc := emailtemplate.NewService(emailtemplate.NewRepository(q.Queries), stores)
	if cat := corefront.New(os.Getenv("CORE_FRONT_API_URL"), stores, cfg.ServiceSecret); cat != nil {
		svc.SetCatalogReader(cat)
	}
	return svc
}

// The engine, with the conditions and tags the suite's own data answers registered right here
func newFlow(
	cfg Config, allow AllowPrivateTargets, q Queries, tmpl *emailtemplate.Service,
	contacts *RosterContacts, stores *RosterStores, mailer email.Sender, texter sms.Sender,
	track *engagement.Tracker, rec *analytics.Recorder, segs *segment.Service, suppress *suppression.Service,
) *flow.Service {
	svc := flow.NewService(flow.NewRepository(q.Queries), tmpl, contacts, flowStores{stores}, mailer, texter, track, bool(allow), []byte(cfg.JWTSecret))
	svc.SetSuppression(suppress)
	members := segmentMembers{segs}
	traits := traitReader{rec}
	svc.RegisterCondition(flow.NewEmailActivityCondition(track))
	svc.RegisterCondition(flow.NewSegmentCondition(members))
	svc.RegisterCondition(flow.NewSegmentAliasCondition(members)) // saved flows still carry the older spelling
	svc.RegisterCondition(flow.NewLifecycleCondition(traits))
	svc.RegisterCondition(flow.NewRFMCondition(traits))
	svc.RegisterNode(flow.NewAddSegmentNode(segs))
	svc.RegisterMergeVars(synapseVars{rec: rec, segs: segs})
	return svc
}

func newCampaign(
	cfg Config, q Queries, tmpl *emailtemplate.Service, contacts *RosterContacts, stores *RosterStores,
	settings *config.Service, mailer email.Sender, track *engagement.Tracker, segs *segment.Service, suppress *suppression.Service,
) *campaign.Service {
	svc := campaign.NewService(campaign.NewRepository(q.Queries), tmpl, campaignAudience{contacts}, segmentAudience{segs}, campaignStores{stores}, settings, mailer, track, []byte(cfg.JWTSecret))
	svc.SetSuppression(suppress)
	return svc
}

// --- the suite's answers to the engine's questions ---

type segmentMembers struct{ segs *segment.Service }

// InSegment answers a saved flow's condition, which stored the segment by id
func (m segmentMembers) InSegment(ctx context.Context, customerID, segmentID uuid.UUID) bool {
	refs, err := m.segs.SegmentsForCustomer(ctx, customerID)
	if err != nil {
		return false
	}
	for _, r := range refs {
		if r.ID == segmentID {
			return true
		}
	}
	return false
}

type segmentAudience struct{ segs *segment.Service }

func (a segmentAudience) SegmentMemberIDs(ctx context.Context, segmentID uuid.UUID) ([]uuid.UUID, error) {
	return a.segs.MemberIDs(ctx, segmentID)
}

// Computed is load-bearing: a customer the recompute has not reached must not match an rfm condition whose minimums are all zero
type traitReader struct{ rec *analytics.Recorder }

func (t traitReader) CustomerTraits(ctx context.Context, customerID uuid.UUID) (flow.CustomerTraits, bool, error) {
	src, ok, err := t.rec.CustomerTraits(ctx, customerID)
	if err != nil || !ok {
		return flow.CustomerTraits{}, false, err
	}
	return flow.CustomerTraits{
		RFMRecency: src.RFMRecency, RFMFrequency: src.RFMFrequency, RFMMonetary: src.RFMMonetary,
		Lifecycle: src.Lifecycle, EngagementScore: src.EngagementScore,
	}, true, nil
}

// How far back the {{customer.affinity.*}} tags look - a quarter, long enough to survive a seasonal gap
const affinityWindowDays = 90

// The personalization tags Synapse computes - a customer the recompute has not reached renders them empty
type synapseVars struct {
	rec  *analytics.Recorder
	segs *segment.Service
}

func (v synapseVars) MergeVars(ctx context.Context, _ uuid.UUID, customerID uuid.UUID) map[string]string {
	m := map[string]string{}
	if t, ok, err := v.rec.CustomerTraits(ctx, customerID); err == nil && ok {
		m["customer.lifecycle"] = t.Lifecycle
		m["customer.orders"] = fmt.Sprintf("%d", t.Frequency)
		m["customer.ltv"] = fmt.Sprintf("%.2f", float64(t.MonetaryCents)/100)
	}
	// Attribute affinities become {{customer.affinity.<slug>}}, e.g. "your size, 205/55R16, is back in stock"
	if affs, err := v.rec.CustomerTopAffinities(ctx, customerID, daysAgo(affinityWindowDays)); err == nil {
		for slug, val := range affs {
			m["customer.affinity."+slug] = val
		}
	}
	if slugs, err := v.segs.SlugsForCustomer(ctx, customerID); err == nil {
		m["customer.segments"] = strings.Join(slugs, ", ")
	}
	return m
}

func daysAgo(n int) time.Time { return time.Now().AddDate(0, 0, -n) }

// A missing roster row is an absent customer, not a failure
func ignoreNoRows(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	return err
}
