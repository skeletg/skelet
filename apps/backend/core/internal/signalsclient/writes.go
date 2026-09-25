package signalsclient

// Core's writes into the event spine - every one best-effort, since the suite being off or unreachable
// must never fail the commerce action that raised it

import (
	"context"
	"encoding/json"
	"maps"
	"slices"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
)

// A server-side commerce or account action, for the profile timeline
type ActivitySpec struct {
	EventName  string
	Source     string
	ChannelID  uuid.UUID
	CustomerID *uuid.UUID
	// For guest commerce - attributed to the anon visitor when there is no customer
	AnonID string
	At     time.Time
	Props  map[string]any
	// Keeps the write idempotent against an at-least-once bus
	DedupeKey string
}

type ConversionSpec struct {
	CustomerID   uuid.UUID
	ChannelID    uuid.UUID
	OrderID      uuid.UUID
	RevenueCents int64
	Currency     string
	At           time.Time
}

// The send log's vocabulary for who sent a message
const (
	SourceCampaign      = "campaign"
	SourceFlow          = "flow"
	SourceTransactional = "transactional"
)

// SendRef is one email send as the suite records it. Core mints the ID before the mail goes out and
// reports the whole record once; on an open or click it names the ID alone and gets the record back
type SendRef struct {
	ID         uuid.UUID
	ChannelID  uuid.UUID
	CustomerID *uuid.UUID
	// campaign | flow | transactional - a transactional send reaches the spine alone, the other two
	// are kept in the suite's send log, which is what the tracking token names
	Source string
	// The origin attribution credits, travelling with the send rather than being looked up later
	CampaignID *uuid.UUID
	FlowID     *uuid.UUID
	// What the send log keeps beyond attribution
	Email       string
	RecipientID *uuid.UUID
	Variant     string
	FlowRunID   *uuid.UUID
	FlowNodeID  string
	MessageKey  string
	OrderID     *uuid.UUID
	// Set on the answers only: when the send went out, from the log
	SentAt time.Time
}

// One pixel load or link follow - the suite classifies it against when the send went out
type Hit struct {
	UA      string
	Machine bool
	Device  string
	Client  string
	At      time.Time
}

// EmailEngagement is what an open or click resolved to: the send as the suite logged it, and whether
// the hit read as a machine - a proxy prefetch or a scanner, which is not the customer reading anything
type EmailEngagement struct {
	Send SendRef
	// false for a send the log does not hold - too old, or never a logged send - and then there is nothing to react to
	Found   bool
	Machine bool
}

func (c *Client) RecordActivity(ctx context.Context, a ActivitySpec) error {
	sys, err := c.System(ctx)
	if err != nil {
		return err
	}
	_, err = c.Ingest.RecordActivity(sys, &signalsv1.RecordActivityRequest{
		EventName: a.EventName, Source: a.Source, ChannelId: idString(a.ChannelID),
		CustomerId: optionalID(a.CustomerID), AnonId: a.AnonID, At: stamp(a.At),
		PropsJson: propsJSON(a.Props), DedupeKey: a.DedupeKey,
	})
	return err
}

func (c *Client) RecordConversion(ctx context.Context, cs ConversionSpec) error {
	sys, err := c.System(ctx)
	if err != nil {
		return err
	}
	_, err = c.Ingest.RecordConversion(sys, &signalsv1.RecordConversionRequest{
		CustomerId: cs.CustomerID.String(), ChannelId: idString(cs.ChannelID), OrderId: cs.OrderID.String(),
		RevenueCents: cs.RevenueCents, Currency: cs.Currency, At: stamp(cs.At),
	})
	return err
}

func (c *Client) RecordEmailSent(ctx context.Context, s SendRef) error {
	sys, err := c.System(ctx)
	if err != nil {
		return err
	}
	_, err = c.Ingest.RecordEmailSent(sys, &signalsv1.RecordEmailSentRequest{Send: sendProto(s)})
	return err
}

// RecordEmailOpen names the send the pixel carried; the suite stamps its log, writes the spine and
// answers with the record, which is what core reacts to - a counter bump, a flow enrolment
func (c *Client) RecordEmailOpen(ctx context.Context, sendID uuid.UUID, h Hit) (EmailEngagement, error) {
	sys, err := c.System(ctx)
	if err != nil {
		return EmailEngagement{}, err
	}
	res, err := c.Ingest.RecordEmailOpen(sys, &signalsv1.RecordEmailOpenRequest{
		Send: &signalsv1.EmailSendRef{SendId: sendID.String()}, Hit: hitProto(h),
	})
	if err != nil {
		return EmailEngagement{}, err
	}
	return engagementFrom(res.GetRecorded(), res.GetSend(), res.GetMachine()), nil
}

func (c *Client) RecordEmailClick(ctx context.Context, sendID uuid.UUID, url string, linkIndex int, h Hit) (EmailEngagement, error) {
	sys, err := c.System(ctx)
	if err != nil {
		return EmailEngagement{}, err
	}
	res, err := c.Ingest.RecordEmailClick(sys, &signalsv1.RecordEmailClickRequest{
		Send: &signalsv1.EmailSendRef{SendId: sendID.String()}, Hit: hitProto(h), Url: url, LinkIndex: int32(linkIndex),
	})
	if err != nil {
		return EmailEngagement{}, err
	}
	return engagementFrom(res.GetRecorded(), res.GetSend(), res.GetMachine()), nil
}

func engagementFrom(found bool, send *signalsv1.EmailSendRef, machine bool) EmailEngagement {
	if !found || send == nil {
		return EmailEngagement{}
	}
	return EmailEngagement{Send: sendFromProto(send), Found: true, Machine: machine}
}

func sendFromProto(p *signalsv1.EmailSendRef) SendRef {
	out := SendRef{
		ChannelID: parseOrNil(p.GetChannelId()), Source: p.GetSource(), Email: p.GetEmail(),
		Variant: p.GetVariant(), FlowNodeID: p.GetFlowNodeId(), MessageKey: p.GetMessageKey(),
		CustomerID: parseOptional(p.CustomerId), CampaignID: parseOptional(p.CampaignId), FlowID: parseOptional(p.FlowId),
		RecipientID: parseOptional(p.RecipientId), FlowRunID: parseOptional(p.FlowRunId), OrderID: parseOptional(p.OrderId),
	}
	out.ID = parseOrNil(p.GetSendId())
	if p.GetSentAt() != nil {
		out.SentAt = p.GetSentAt().AsTime()
	}
	return out
}

func parseOrNil(raw string) uuid.UUID {
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil
	}
	return id
}

func parseOptional(raw *string) *uuid.UUID {
	if raw == nil || *raw == "" {
		return nil
	}
	id, err := uuid.Parse(*raw)
	if err != nil {
		return nil
	}
	return &id
}

func sendProto(s SendRef) *signalsv1.EmailSendRef {
	return &signalsv1.EmailSendRef{
		SendId: s.ID.String(), ChannelId: idString(s.ChannelID),
		CustomerId: optionalID(s.CustomerID), Source: s.Source,
		CampaignId: optionalID(s.CampaignID), FlowId: optionalID(s.FlowID),
		Email: s.Email, RecipientId: optionalID(s.RecipientID), Variant: s.Variant,
		FlowRunId: optionalID(s.FlowRunID), FlowNodeId: s.FlowNodeID, MessageKey: s.MessageKey, OrderId: optionalID(s.OrderID),
	}
}

func hitProto(h Hit) *signalsv1.EmailHit {
	return &signalsv1.EmailHit{
		UserAgent: h.UA, Machine: h.Machine, Device: h.Device, Client: h.Client, At: stamp(h.At),
	}
}

// The nil uuid rides as an empty string, which the server reads back as absent
func idString(id uuid.UUID) string {
	if id == uuid.Nil {
		return ""
	}
	return id.String()
}

func optionalID(id *uuid.UUID) *string {
	if id == nil || *id == uuid.Nil {
		return nil
	}
	s := id.String()
	return &s
}

func stamp(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}

// Props that will not marshal are dropped rather than failing the write, the event itself being worth more than its detail
func propsJSON(p map[string]any) string {
	if len(p) == 0 {
		return ""
	}
	b, err := json.Marshal(p)
	if err != nil {
		return ""
	}
	return string(b)
}

// RosterPage is one batch of core's customers on the way to the suite
// RosterCustomer is one customer as the roster holds them - identity plus what addressing a marketing send needs
type RosterCustomer struct {
	CustomerID       uuid.UUID
	ChannelID        uuid.UUID
	Email            string
	FirstName        string
	Phone            string
	IsActive         bool
	GroupID          uuid.NullUUID
	GroupSlug        string // beside the id, since segment rules are authored against slugs
	MarketingOptedIn bool
	SMSOptedIn       bool
	Consents         map[string]bool
	ReadAt           time.Time // when core's database read the row - the suite keeps whichever read is newest
}

// ConsentsFromJSON reads the roster query's jsonb_object_agg of kind -> opted_in, which sqlc hands over untyped
func ConsentsFromJSON(v any) map[string]bool {
	out := map[string]bool{}
	var raw []byte
	switch x := v.(type) {
	case []byte:
		raw = x
	case string:
		raw = []byte(x)
	case json.RawMessage:
		raw = x
	}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out)
	}
	return out
}

type RosterPage struct {
	Customers []RosterCustomer
}

// PushCustomerRoster streams core's whole customer list across, and the suite sweeps what the stream did not carry
// A partial read abandons the stream rather than closing it, so an error partway leaves the suite's copy alone
func (c *Client) PushCustomerRoster(ctx context.Context, next func() (RosterPage, bool, error)) (synced, pruned int32, err error) {
	sys, err := c.System(ctx)
	if err != nil {
		return 0, 0, err
	}
	sys, release := context.WithCancel(sys) // an abandoned client stream is released by its context, and only this holds one
	defer release()
	stream, err := c.Ingest.SyncCustomerRoster(sys)
	if err != nil {
		return 0, 0, err
	}
	for {
		page, more, err := next()
		if err != nil {
			return 0, 0, err // abandoning the stream is what stops the suite pruning on a partial read
		}
		if !more {
			break
		}
		entries := make([]*signalsv1.RosterEntry, 0, len(page.Customers))
		for _, c := range page.Customers {
			entries = append(entries, rosterEntry(c))
		}
		if err := stream.Send(&signalsv1.SyncCustomerRosterRequest{Customers: entries}); err != nil {
			return 0, 0, err
		}
	}
	res, err := stream.CloseAndRecv()
	if err != nil {
		return 0, 0, err
	}
	return res.GetSynced(), res.GetPruned(), nil
}

// RosterNeedsFullSync is the suite saying its roster schema grew past the last full sync - the caller runs the sweep now
func (c *Client) RosterNeedsFullSync(ctx context.Context) (bool, error) {
	sys, err := c.System(ctx)
	if err != nil {
		return false, err
	}
	res, err := c.Ingest.RosterStatus(sys, &signalsv1.RosterStatusRequest{})
	if err != nil {
		return false, err
	}
	return res.GetNeedsFullSync(), nil
}

type ChannelEntry struct {
	ID        uuid.UUID
	IsDefault bool
	IsActive  bool
	// The identity a message for this channel is sent under, resolved from core's config registry
	Name        string
	PublicURL   string
	FromAddress string
	FromName    string
	LogoURL     string
}

// PushChannels replaces the suite's copy of core's channels, which is what its beacon surface
// resolves against - the whole set goes in one call, so a failure leaves the old copy serving
func (c *Client) PushChannels(ctx context.Context, channels []ChannelEntry) (synced, pruned int32, err error) {
	sys, err := c.System(ctx)
	if err != nil {
		return 0, 0, err
	}
	entries := make([]*signalsv1.ChannelEntry, 0, len(channels))
	for _, ch := range channels {
		entries = append(entries, &signalsv1.ChannelEntry{
			ChannelId: ch.ID.String(), IsDefault: ch.IsDefault, IsActive: &ch.IsActive, Name: ch.Name, PublicUrl: ch.PublicURL,
			FromAddress: ch.FromAddress, FromName: ch.FromName, LogoUrl: ch.LogoURL,
		})
	}
	res, err := c.Ingest.SyncChannels(sys, &signalsv1.SyncChannelsRequest{Channels: entries})
	if err != nil {
		return 0, 0, err
	}
	return res.GetSynced(), res.GetPruned(), nil
}

// DeleteCustomerIdentities is the Signals half of a GDPR erasure - the mappings that would re-link an anonymized customer to everything they did
// email is the one core is about to redact - sends from before they had an account carry only that
func (c *Client) DeleteCustomerIdentities(ctx context.Context, customerID uuid.UUID, email string, erasedAt time.Time) error {
	sys, err := c.System(ctx)
	if err != nil {
		return err
	}
	_, err = c.Ingest.EraseCustomerIdentities(sys, &signalsv1.EraseCustomerIdentitiesRequest{
		CustomerId: customerID.String(), Email: email, ErasedAt: stamp(erasedAt),
	})
	return err
}

func rosterEntry(c RosterCustomer) *signalsv1.RosterEntry {
	e := &signalsv1.RosterEntry{
		CustomerId: c.CustomerID.String(), ChannelId: c.ChannelID.String(),
		Email: c.Email, FirstName: c.FirstName, Phone: c.Phone, IsActive: c.IsActive,
		MarketingOptedIn: c.MarketingOptedIn, SmsOptedIn: c.SMSOptedIn, ReadAt: stamp(c.ReadAt),
	}
	if c.GroupID.Valid {
		e.CustomerGroupId = c.GroupID.UUID.String()
		e.CustomerGroupSlug = c.GroupSlug
	}
	for _, kind := range slices.Sorted(maps.Keys(c.Consents)) {
		e.Consents = append(e.Consents, &signalsv1.Consent{Kind: kind, OptedIn: c.Consents[kind]})
	}
	return e
}

// UpsertRosterCustomer pushes one customer on a change core just committed, so an audience built
// before the next sweep still sees it - the answer says whether the suite wants a full sweep too
func (c *Client) UpsertRosterCustomer(ctx context.Context, cust RosterCustomer) (needsFullSync bool, err error) {
	sys, err := c.System(ctx)
	if err != nil {
		return false, err
	}
	res, err := c.Ingest.UpsertRosterEntry(sys, &signalsv1.UpsertRosterEntryRequest{Customer: rosterEntry(cust)})
	if err != nil {
		return false, err
	}
	return res.GetNeedsFullSync(), nil
}

// DeleteRosterCustomer drops one customer ahead of the sweep that would eventually have caught them
func (c *Client) DeleteRosterCustomer(ctx context.Context, customerID uuid.UUID) error {
	sys, err := c.System(ctx)
	if err != nil {
		return err
	}
	_, err = c.Ingest.DeleteRosterEntry(sys, &signalsv1.DeleteRosterEntryRequest{CustomerId: customerID.String()})
	return err
}
