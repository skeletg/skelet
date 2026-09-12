package signalsgrpc

// The write half of the contract. Unlike every read here, these authorize on a system token: core
// writes as itself, the event bus that raises them carrying no admin to act for

import (
	"context"
	"encoding/json"
	"errors"
	"io"

	"github.com/google/uuid"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/skeletg/skelet/apps/backend/internal/analytics"
	"github.com/skeletg/skelet/apps/backend/internal/auth"
	signalsv1 "github.com/skeletg/skelet/apps/backend/internal/signalspb/skelet/signals/v1"
)

type ingestServer struct {
	signalsv1.UnimplementedIngestServiceServer
	rec *analytics.Recorder
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

func (s *ingestServer) RecordEmailSent(ctx context.Context, req *signalsv1.RecordEmailSentRequest) (*signalsv1.RecordEmailSentResponse, error) {
	ref, err := s.scopedSend(ctx, req.GetSend())
	if err != nil {
		return nil, err
	}
	if err := s.rec.RecordSent(ctx, ref); err != nil {
		return nil, err
	}
	return &signalsv1.RecordEmailSentResponse{Recorded: true}, nil
}

func (s *ingestServer) RecordEmailOpen(ctx context.Context, req *signalsv1.RecordEmailOpenRequest) (*signalsv1.RecordEmailOpenResponse, error) {
	ref, err := s.scopedSend(ctx, req.GetSend())
	if err != nil {
		return nil, err
	}
	ok, err := s.rec.RecordOpen(ctx, ref, hitFromProto(req.GetHit()))
	if err != nil {
		return nil, err
	}
	return &signalsv1.RecordEmailOpenResponse{Recorded: ok}, nil
}

func (s *ingestServer) RecordEmailClick(ctx context.Context, req *signalsv1.RecordEmailClickRequest) (*signalsv1.RecordEmailClickResponse, error) {
	ref, err := s.scopedSend(ctx, req.GetSend())
	if err != nil {
		return nil, err
	}
	ok, err := s.rec.RecordClick(ctx, ref, req.GetUrl(), int(req.GetLinkIndex()), hitFromProto(req.GetHit()))
	if err != nil {
		return nil, err
	}
	return &signalsv1.RecordEmailClickResponse{Recorded: ok}, nil
}

func (s *ingestServer) scopedSend(ctx context.Context, send *signalsv1.EmailSendRef) (analytics.SendRef, error) {
	if err := requireSystem(ctx); err != nil {
		return analytics.SendRef{}, err
	}
	id, err := parseID("send_id", send.GetSendId())
	if err != nil {
		return analytics.SendRef{}, err
	}
	return analytics.SendRef{
		ID: id, ChannelID: uuidOrNil(send.GetChannelId()),
		CustomerID: optionalUUID(send.CustomerId), Source: send.GetSource(),
		CampaignID: optionalUUID(send.CampaignId), FlowID: optionalUUID(send.FlowId),
	}, nil
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

// SyncCustomerRoster takes core's whole customer list as a stream. The sweep runs only once the
// stream has ended cleanly, so a push that dies partway leaves the roster as it was rather than
// emptying it and taking every trait and membership with it
func (s *ingestServer) SyncCustomerRoster(stream signalsv1.IngestService_SyncCustomerRosterServer) error {
	if err := requireSystem(stream.Context()); err != nil {
		return err
	}
	sync := s.rec.BeginRosterSync()
	for {
		page, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err // the client gave up, so nothing is pruned
		}
		customers := make([]uuid.UUID, 0, len(page.GetCustomers()))
		channels := make([]uuid.UUID, 0, len(page.GetCustomers()))
		for _, e := range page.GetCustomers() {
			cust, cerr := uuid.Parse(e.GetCustomerId())
			ch, cherr := uuid.Parse(e.GetChannelId())
			if cerr != nil || cherr != nil {
				return invalidArgument("roster entry carries an unparseable id")
			}
			customers = append(customers, cust)
			channels = append(channels, ch)
		}
		if err := sync.Page(stream.Context(), customers, channels); err != nil {
			return err
		}
	}
	synced, pruned, err := sync.Commit(stream.Context())
	if err != nil {
		return err
	}
	return stream.SendAndClose(&signalsv1.SyncCustomerRosterResponse{Synced: int32(synced), Pruned: int32(pruned)})
}
