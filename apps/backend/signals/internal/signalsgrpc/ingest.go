package signalsgrpc

// The write half of the contract. Unlike every read here, these authorize on a system token: core
// writes as itself, the event bus that raises them carrying no admin to act for

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
	"github.com/skeletg/skelet/apps/backend/signals/internal/engagement"
	"github.com/skeletg/skelet/apps/backend/signals/internal/sendlog"
)

type ingestServer struct {
	roster *analytics.Roster
	signalsv1.UnimplementedIngestServiceServer
	rec      *analytics.Recorder
	channels *analytics.ChannelRoster
	sends    *sendlog.Store // nil in a test server without a log, and then the ref is taken as sent
	track    *engagement.Tracker
}

func (s *ingestServer) RecordActivity(ctx context.Context, req *signalsv1.RecordActivityRequest) (*signalsv1.RecordActivityResponse, error) {
	if err := requireSystem(ctx); err != nil {
		return nil, err
	}
	if req.GetEventName() == "" {
		return nil, invalidArgument("event_name is required")
	}
	var channel, customer = uuidOrNil(req.GetChannelId()), optionalUUID(req.CustomerId)
	spec := analytics.ActivitySpec{
		EventName: req.GetEventName(), Source: req.GetSource(), ChannelID: channel,
		CustomerID: customer, AnonID: req.GetAnonId(), At: req.GetAt().AsTime(),
		Props: propsFromJSON(req.GetPropsJson()), DedupeKey: req.GetDedupeKey(),
	}
	ok, err := s.rec.RecordActivity(ctx, spec)
	if err != nil {
		return nil, err
	}
	return &signalsv1.RecordActivityResponse{Recorded: ok}, nil
}

func (s *ingestServer) RecordConversion(ctx context.Context, req *signalsv1.RecordConversionRequest) (*signalsv1.RecordConversionResponse, error) {
	if err := requireSystem(ctx); err != nil {
		return nil, err
	}
	cust, err := parseID("customer_id", req.GetCustomerId())
	if err != nil {
		return nil, err
	}
	order, err := parseID("order_id", req.GetOrderId())
	if err != nil {
		return nil, err
	}
	ok, err := s.rec.RecordConversion(ctx, analytics.ConversionSpec{
		CustomerID: cust, ChannelID: uuidOrNil(req.GetChannelId()), OrderID: order,
		RevenueCents: req.GetRevenueCents(), Currency: req.GetCurrency(), At: req.GetAt().AsTime(),
	})
	if err != nil {
		return nil, err
	}
	return &signalsv1.RecordConversionResponse{Recorded: ok}, nil
}

// A campaign or flow send is kept in the log under the id core minted, then reaches the spine; a
// transactional one reaches the spine alone
func (s *ingestServer) RecordEmailSent(ctx context.Context, req *signalsv1.RecordEmailSentRequest) (*signalsv1.RecordEmailSentResponse, error) {
	if err := requireSystem(ctx); err != nil {
		return nil, err
	}
	spec, err := sendSpecFromProto(req.GetSend())
	if err != nil {
		return nil, err
	}
	if err := s.track.Sent(ctx, spec); err != nil {
		return nil, err
	}
	return &signalsv1.RecordEmailSentResponse{Recorded: true}, nil
}

// The pixel names the send; the log says the rest, and how long after the send the hit landed
// decides whether it was a reader or a proxy - the first-open stamp is human hits only
func (s *ingestServer) RecordEmailOpen(ctx context.Context, req *signalsv1.RecordEmailOpenRequest) (*signalsv1.RecordEmailOpenResponse, error) {
	if err := requireSystem(ctx); err != nil {
		return nil, err
	}
	send, ok, err := s.resolveSend(ctx, req.GetSend())
	if err != nil || !ok {
		return &signalsv1.RecordEmailOpenResponse{Recorded: false}, err
	}
	hit, err := s.track.OpenOf(ctx, send, hitFromProto(req.GetHit()))
	if err != nil {
		return nil, err
	}
	return &signalsv1.RecordEmailOpenResponse{Recorded: true, Send: sendProto(send), Machine: hit.Machine}, nil
}

func (s *ingestServer) RecordEmailClick(ctx context.Context, req *signalsv1.RecordEmailClickRequest) (*signalsv1.RecordEmailClickResponse, error) {
	if err := requireSystem(ctx); err != nil {
		return nil, err
	}
	send, ok, err := s.resolveSend(ctx, req.GetSend())
	if err != nil || !ok {
		return &signalsv1.RecordEmailClickResponse{Recorded: false}, err
	}
	hit, err := s.track.ClickOf(ctx, send, req.GetUrl(), int(req.GetLinkIndex()), hitFromProto(req.GetHit()))
	if err != nil {
		return nil, err
	}
	return &signalsv1.RecordEmailClickResponse{Recorded: true, Send: sendProto(send), Machine: hit.Machine}, nil
}

// resolveSend is the logged record when there is a log, else the ref as the caller sent it - a test
// server without a log still answers, and so does a caller carrying the whole record itself
func (s *ingestServer) resolveSend(ctx context.Context, ref *signalsv1.EmailSendRef) (sendlog.Send, bool, error) {
	id, err := parseID("send_id", ref.GetSendId())
	if err != nil {
		return sendlog.Send{}, false, err
	}
	if s.sends != nil {
		return s.sends.Get(ctx, id)
	}
	spec, err := sendSpecFromProto(ref)
	if err != nil {
		return sendlog.Send{}, false, err
	}
	return sendlog.Send{
		ID: spec.ID, ChannelID: spec.ChannelID, Source: spec.Source, Email: spec.Email, CustomerID: spec.CustomerID,
		CampaignID: spec.CampaignID, FlowID: spec.FlowID, RecipientID: spec.RecipientID, SentAt: ref.GetSentAt().AsTime(),
	}, true, nil
}

func sendSpecFromProto(ref *signalsv1.EmailSendRef) (sendlog.Spec, error) {
	id, err := parseID("send_id", ref.GetSendId())
	if err != nil {
		return sendlog.Spec{}, err
	}
	return sendlog.Spec{
		ID: id, ChannelID: uuidOrNil(ref.GetChannelId()), Source: ref.GetSource(), Email: ref.GetEmail(),
		CustomerID: optionalUUID(ref.CustomerId), CampaignID: optionalUUID(ref.CampaignId), RecipientID: optionalUUID(ref.RecipientId),
		Variant: ref.GetVariant(), FlowID: optionalUUID(ref.FlowId), FlowRunID: optionalUUID(ref.FlowRunId),
		FlowNodeID: ref.GetFlowNodeId(), MessageKey: ref.GetMessageKey(), OrderID: optionalUUID(ref.OrderId),
	}, nil
}

func sendProto(send sendlog.Send) *signalsv1.EmailSendRef {
	out := &signalsv1.EmailSendRef{
		SendId: send.ID.String(), ChannelId: send.ChannelID.String(), Source: send.Source, Email: send.Email,
		Variant: send.Variant, FlowNodeId: send.FlowNodeID, MessageKey: send.MessageKey,
		SentAt: timestamppb.New(send.SentAt),
	}
	out.CustomerId = optionalString(send.CustomerID)
	out.CampaignId = optionalString(send.CampaignID)
	out.FlowId = optionalString(send.FlowID)
	out.RecipientId = optionalString(send.RecipientID)
	out.FlowRunId = optionalString(send.FlowRunID)
	out.OrderId = optionalString(send.OrderID)
	return out
}

func optionalString(id *uuid.UUID) *string {
	if id == nil {
		return nil
	}
	s := id.String()
	return &s
}

func hitFromProto(h *signalsv1.EmailHit) analytics.Hit {
	return analytics.Hit{
		UA: h.GetUserAgent(), Machine: h.GetMachine(),
		Device: h.GetDevice(), Client: h.GetClient(), At: h.GetAt().AsTime(),
	}
}

// Props ride as JSON, so the contract carries whatever an event names without a schema per event
func propsFromJSON(raw string) map[string]any {
	if raw == "" {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

// A system token authorizes nothing else here, and an admin token does not authorize this
func requireSystem(ctx context.Context) error {
	if !auth.IsSystemCaller(ctx) {
		return status.Error(codes.PermissionDenied, "this method takes a system token")
	}
	return nil
}

// SyncCustomerRoster takes core's whole customer list as a stream
// The sweep runs only once the stream ends cleanly, so a push that dies partway leaves the roster as it was rather than emptying it
func (s *ingestServer) SyncCustomerRoster(stream signalsv1.IngestService_SyncCustomerRosterServer) error {
	if err := requireSystem(stream.Context()); err != nil {
		return err
	}
	sync := s.roster.BeginRosterSync()
	for {
		page, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err // the client gave up, so nothing is pruned
		}
		entries := make([]analytics.RosterEntry, 0, len(page.GetCustomers()))
		for _, e := range page.GetCustomers() {
			entry, err := rosterEntry(e)
			if err != nil {
				return err
			}
			entries = append(entries, entry)
		}
		if err := sync.Page(stream.Context(), entries); err != nil {
			return err
		}
	}
	synced, pruned, err := sync.Commit(stream.Context())
	if err != nil {
		return err
	}
	return stream.SendAndClose(&signalsv1.SyncCustomerRosterResponse{Synced: int32(synced), Pruned: int32(pruned), RosterVersion: analytics.RosterVersion})
}

// Translates one wire entry, refusing the whole page on a bad id rather than landing a half-identified customer
func rosterEntry(e *signalsv1.RosterEntry) (analytics.RosterEntry, error) {
	cust, cerr := uuid.Parse(e.GetCustomerId())
	ch, cherr := uuid.Parse(e.GetChannelId())
	if cerr != nil || cherr != nil {
		return analytics.RosterEntry{}, invalidArgument("roster entry carries an unparseable id")
	}
	out := analytics.RosterEntry{
		CustomerID: cust, ChannelID: ch, Email: e.GetEmail(), FirstName: e.GetFirstName(),
		Phone: e.GetPhone(), IsActive: e.GetIsActive(),
		MarketingOptedIn: e.GetMarketingOptedIn(), SMSOptedIn: e.GetSmsOptedIn(),
	}
	if e.GetReadAt() != nil {
		out.ReadAt = e.GetReadAt().AsTime()
	}
	if cs := e.GetConsents(); len(cs) > 0 {
		out.Consents = make(map[string]bool, len(cs))
		for _, c := range cs {
			out.Consents[c.GetKind()] = c.GetOptedIn()
		}
	}
	if g := e.GetCustomerGroupId(); g != "" {
		id, err := uuid.Parse(g)
		if err != nil {
			return analytics.RosterEntry{}, invalidArgument("roster entry carries an unparseable group id")
		}
		out.GroupID = uuid.NullUUID{UUID: id, Valid: true}
		out.GroupSlug = e.GetCustomerGroupSlug()
	}
	return out, nil
}

func (s *ingestServer) UpsertRosterEntry(ctx context.Context, req *signalsv1.UpsertRosterEntryRequest) (*signalsv1.UpsertRosterEntryResponse, error) {
	if err := requireSystem(ctx); err != nil {
		return nil, err
	}
	entry, err := rosterEntry(req.GetCustomer())
	if err != nil {
		return nil, err
	}
	if err := s.roster.UpsertRosterEntry(ctx, entry); err != nil {
		return nil, err
	}
	st, err := s.roster.RosterStatus(ctx)
	if err != nil {
		return nil, err
	}
	return &signalsv1.UpsertRosterEntryResponse{RosterVersion: st.Version, NeedsFullSync: st.NeedsFullSync}, nil
}

func (s *ingestServer) RosterStatus(ctx context.Context, _ *signalsv1.RosterStatusRequest) (*signalsv1.RosterStatusResponse, error) {
	if err := requireSystem(ctx); err != nil {
		return nil, err
	}
	st, err := s.roster.RosterStatus(ctx)
	if err != nil {
		return nil, err
	}
	out := &signalsv1.RosterStatusResponse{RosterVersion: st.Version, FilledVersion: st.FilledVersion, NeedsFullSync: st.NeedsFullSync}
	if st.FilledAt != nil {
		out.FilledAt = timestamppb.New(*st.FilledAt)
	}
	return out, nil
}

func (s *ingestServer) DeleteRosterEntry(ctx context.Context, req *signalsv1.DeleteRosterEntryRequest) (*signalsv1.DeleteRosterEntryResponse, error) {
	if err := requireSystem(ctx); err != nil {
		return nil, err
	}
	id, err := uuid.Parse(req.GetCustomerId())
	if err != nil {
		return nil, invalidArgument("customer id is not a uuid")
	}
	if err := s.roster.DeleteRosterEntry(ctx, id); err != nil {
		return nil, err
	}
	return &signalsv1.DeleteRosterEntryResponse{}, nil
}

// SyncChannels replaces the set the beacon surface resolves against. It arrives whole rather than
// as a stream, so a push that fails never lands half a set and starts dropping live channels' beacons
func (s *ingestServer) SyncChannels(ctx context.Context, req *signalsv1.SyncChannelsRequest) (*signalsv1.SyncChannelsResponse, error) {
	if err := requireSystem(ctx); err != nil {
		return nil, err
	}
	if len(req.GetChannels()) == 0 {
		return nil, invalidArgument("an empty channel set would prune every channel") // core always has its default channel
	}
	channels := make([]analytics.ChannelEntry, 0, len(req.GetChannels()))
	complete := true
	for _, e := range req.GetChannels() {
		complete = complete && e.IsActive != nil
		id, err := uuid.Parse(e.GetChannelId())
		if err != nil {
			return nil, invalidArgument("channel entry carries an unparseable id")
		}
		channels = append(channels, analytics.ChannelEntry{
			ID: id, IsDefault: e.GetIsDefault(), IsActive: e.IsActive == nil || e.GetIsActive(), Name: e.GetName(), PublicURL: e.GetPublicUrl(),
			FromAddress: e.GetFromAddress(), FromName: e.GetFromName(), LogoURL: e.GetLogoUrl(),
		})
	}
	if !complete {
		slog.Warn("channel push without is_active - applied without pruning, since the deactivated channels it leaves out are not deleted ones", "channels", len(channels))
		synced, err := s.channels.UpsertChannels(ctx, channels)
		if err != nil {
			return nil, err
		}
		return &signalsv1.SyncChannelsResponse{Synced: int32(synced)}, nil
	}
	synced, pruned, err := s.channels.SyncChannels(ctx, channels)
	if err != nil {
		return nil, err
	}
	return &signalsv1.SyncChannelsResponse{Synced: int32(synced), Pruned: int32(pruned)}, nil
}

// Core surfaces a failure here rather than reporting an erasure it could not complete
func (s *ingestServer) EraseCustomerIdentities(ctx context.Context, req *signalsv1.EraseCustomerIdentitiesRequest) (*signalsv1.EraseCustomerIdentitiesResponse, error) {
	if err := requireSystem(ctx); err != nil {
		return nil, err
	}
	id, err := uuid.Parse(req.GetCustomerId())
	if err != nil {
		return nil, invalidArgument("customer_id is not a uuid")
	}
	if err := s.rec.DeleteCustomerIdentities(ctx, id); err != nil {
		return nil, err
	}
	// The send log carries the address too, and a scrub that only cleared the spine would leave it there
	if s.sends != nil {
		if err := s.sends.ScrubIdentity(ctx, id, req.GetEmail()); err != nil {
			return nil, err
		}
	}
	var erasedAt time.Time
	if req.GetErasedAt() != nil {
		erasedAt = req.GetErasedAt().AsTime()
	}
	if err := s.roster.EraseCustomer(ctx, id, req.GetEmail(), erasedAt); err != nil {
		return nil, err
	}
	return &signalsv1.EraseCustomerIdentitiesResponse{}, nil
}
