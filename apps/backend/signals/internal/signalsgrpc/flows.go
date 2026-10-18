package signalsgrpc

// The flow composer's reads and writes, relayed by core with the caller's grants, and Enroll - the
// moments core raises on its own token

import (
	"context"
	"encoding/json"
	"errors"
	"log"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
	"github.com/skeletg/skelet/apps/backend/signals/internal/flow"
)

type flowServer struct {
	signalsv1.UnimplementedFlowServiceServer
	svc    *flow.Service
	roster rosterChannels
}

// The channel a customer belongs to, for the profile's run list - off the synced roster
type rosterChannels interface {
	ChannelForCustomer(ctx context.Context, customerID uuid.UUID) (uuid.UUID, error)
}

func (s *flowServer) ListFlows(ctx context.Context, req *signalsv1.ListFlowsRequest) (*signalsv1.ListFlowsResponse, error) {
	cid, err := channelScopeOrSystem(ctx, "flows.view", req.GetChannelId())
	if err != nil {
		return nil, err
	}
	list, err := s.svc.List(ctx, cid, req.GetIncludeDeleted())
	if err != nil {
		return nil, err
	}
	out := make([]*signalsv1.Flow, 0, len(list))
	for _, f := range list {
		out = append(out, flowProto(f))
	}
	return &signalsv1.ListFlowsResponse{Flows: out}, nil
}

func (s *flowServer) GetFlowByID(ctx context.Context, req *signalsv1.GetFlowByIDRequest) (*signalsv1.GetFlowByIDResponse, error) {
	f, err := s.load(ctx, "flows.view", req.GetId())
	if err != nil {
		return nil, err
	}
	return &signalsv1.GetFlowByIDResponse{Flow: flowProto(f)}, nil
}

func (s *flowServer) CreateFlow(ctx context.Context, req *signalsv1.CreateFlowRequest) (*signalsv1.CreateFlowResponse, error) {
	in, err := flowInput(req.GetFlow())
	if err != nil {
		return nil, err
	}
	if _, err := channelScopeOrSystem(ctx, "flows.manage", req.GetFlow().GetChannelId()); err != nil {
		return nil, err
	}
	f, err := s.svc.Create(ctx, in)
	if err != nil {
		return nil, flowErr(err)
	}
	return &signalsv1.CreateFlowResponse{Flow: flowProto(f)}, nil
}

func (s *flowServer) UpdateFlow(ctx context.Context, req *signalsv1.UpdateFlowRequest) (*signalsv1.UpdateFlowResponse, error) {
	existing, err := s.load(ctx, "flows.manage", req.GetId())
	if err != nil {
		return nil, err
	}
	in, err := flowInput(req.GetFlow())
	if err != nil {
		return nil, err
	}
	in.ChannelID = existing.ChannelID // a flow does not move between channels
	f, err := s.svc.Update(ctx, existing.ID, in)
	if err != nil {
		return nil, flowErr(err)
	}
	return &signalsv1.UpdateFlowResponse{Flow: flowProto(f)}, nil
}

func (s *flowServer) DeleteFlow(ctx context.Context, req *signalsv1.DeleteFlowRequest) (*signalsv1.DeleteFlowResponse, error) {
	f, err := s.load(ctx, "flows.manage", req.GetId())
	if err != nil {
		return nil, err
	}
	if err := s.svc.SoftDelete(ctx, f.ID); err != nil {
		return nil, err
	}
	return &signalsv1.DeleteFlowResponse{}, nil
}

func (s *flowServer) RestoreFlow(ctx context.Context, req *signalsv1.RestoreFlowRequest) (*signalsv1.RestoreFlowResponse, error) {
	f, err := s.load(ctx, "flows.manage", req.GetId())
	if err != nil {
		return nil, err
	}
	if err := s.svc.Restore(ctx, f.ID); err != nil {
		return nil, err
	}
	return &signalsv1.RestoreFlowResponse{}, nil
}

func (s *flowServer) SetFlowStatus(ctx context.Context, req *signalsv1.SetFlowStatusRequest) (*signalsv1.SetFlowStatusResponse, error) {
	f, err := s.load(ctx, "flows.manage", req.GetId())
	if err != nil {
		return nil, err
	}
	updated, err := s.svc.SetStatus(ctx, f.ID, req.GetStatus())
	if err != nil {
		return nil, flowErr(err)
	}
	return &signalsv1.SetFlowStatusResponse{Flow: flowProto(updated)}, nil
}

func (s *flowServer) ListFlowConditionKinds(ctx context.Context, _ *signalsv1.ListFlowConditionKindsRequest) (*signalsv1.ListFlowConditionKindsResponse, error) {
	if err := requirePermissionOrSystem(ctx, "flows.view"); err != nil {
		return nil, err
	}
	return &signalsv1.ListFlowConditionKindsResponse{Kinds: s.svc.ConditionKinds()}, nil
}

func (s *flowServer) ListFlowRuns(ctx context.Context, req *signalsv1.ListFlowRunsRequest) (*signalsv1.ListFlowRunsResponse, error) {
	f, err := s.load(ctx, "flows.view", req.GetFlowId())
	if err != nil {
		return nil, err
	}
	limit := req.GetLimit()
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	runs, err := s.svc.ListRuns(ctx, f.ID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]*signalsv1.FlowRun, 0, len(runs))
	for _, r := range runs {
		out = append(out, runProto(r))
	}
	return &signalsv1.ListFlowRunsResponse{Runs: out}, nil
}

// customers.view on the customer's own channel - the profile that shows these is a customer surface
func (s *flowServer) ListCustomerFlowRuns(ctx context.Context, req *signalsv1.ListCustomerFlowRunsRequest) (*signalsv1.ListCustomerFlowRunsResponse, error) {
	if err := requirePermissionOrSystem(ctx, "customers.view"); err != nil {
		return nil, err
	}
	id, err := parseID("customer_id", req.GetCustomerId())
	if err != nil {
		return nil, err
	}
	if s.roster == nil {
		return nil, status.Error(codes.FailedPrecondition, "the customer roster is not configured")
	}
	channelID, err := s.roster.ChannelForCustomer(ctx, id)
	if err != nil {
		return nil, status.Error(codes.NotFound, "customer not found") // no channel resolved is no channel to authorize against
	}
	if err := requireChannelOrSystem(ctx, "customers.view", channelID); err != nil {
		return nil, err
	}
	runs, err := s.svc.RunsForCustomer(ctx, id)
	if err != nil {
		return nil, err
	}
	out := make([]*signalsv1.CustomerFlowRun, 0, len(runs))
	for _, r := range runs {
		out = append(out, &signalsv1.CustomerFlowRun{
			RunId: r.RunID.String(), FlowId: r.FlowID.String(), FlowName: r.FlowName, TriggerType: r.Trigger,
			Status: r.Status, CreatedAt: ts(r.CreatedAt), UpdatedAt: ts(r.UpdatedAt),
		})
	}
	return &signalsv1.ListCustomerFlowRunsResponse{Runs: out}, nil
}

// Both halves of the starter content are the operator's to manage, so both grants are asked for
func (s *flowServer) SeedStarterContent(ctx context.Context, req *signalsv1.SeedStarterContentRequest) (*signalsv1.SeedStarterContentResponse, error) {
	channelID, err := channelScopeOrSystem(ctx, "flows.manage", req.GetChannelId())
	if err != nil {
		return nil, err
	}
	if err := requireChannelOrSystem(ctx, "emails.manage", channelID); err != nil {
		return nil, err
	}
	res, err := s.svc.SeedStarterContent(ctx, channelID)
	if err != nil {
		return nil, flowErr(err)
	}
	return &signalsv1.SeedStarterContentResponse{Blocks: int32(res.Blocks), Emails: int32(res.Emails), Flows: int32(res.Flows)}, nil
}

// One of core's moments - the payload becomes the run context as it stands
func (s *flowServer) Enroll(ctx context.Context, req *signalsv1.EnrollRequest) (*signalsv1.EnrollResponse, error) {
	if err := requireSystem(ctx); err != nil {
		return nil, err
	}
	channelID, err := parseID("channel_id", req.GetChannelId())
	if err != nil {
		return nil, err
	}
	customerID := uuidOrNil(req.GetCustomerId())
	var payload json.RawMessage
	if raw := req.GetPayloadJson(); raw != "" {
		if !json.Valid([]byte(raw)) {
			return nil, invalidArgument("payload_json is not valid JSON")
		}
		payload = json.RawMessage(raw)
	}
	n, err := s.svc.EnrollMoment(ctx, req.GetTriggerType(), channelID, customerID, req.GetEmail(), payload, req.GetIdempotencyKey())
	return enrollAnswer(req.GetTriggerType(), n, err)
}

// A flow that took the moment has already sent, so a partial failure still answers the count
func enrollAnswer(trigger string, n int, err error) (*signalsv1.EnrollResponse, error) {
	if err == nil {
		return &signalsv1.EnrollResponse{Enrolled: int32(n)}, nil
	}
	if n == 0 {
		return nil, status.Errorf(codes.Internal, "enroll %s: %v", trigger, err)
	}
	log.Printf("[flow] enroll %s taken by %d flows, the rest failed: %v", trigger, n, err)
	return &signalsv1.EnrollResponse{Enrolled: int32(n), Incomplete: true}, nil
}

// load is the entity gate: the flow has to exist, and the grant has to cover its channel
func (s *flowServer) load(ctx context.Context, perm, rawID string) (flow.Flow, error) {
	if err := requirePermissionOrSystem(ctx, perm); err != nil {
		return flow.Flow{}, err
	}
	id, err := parseID("id", rawID)
	if err != nil {
		return flow.Flow{}, err
	}
	f, ok, err := s.svc.Get(ctx, id)
	if err != nil {
		return flow.Flow{}, err
	}
	if !ok {
		return flow.Flow{}, notFound("flow")
	}
	if err := requireChannelOrSystem(ctx, perm, f.ChannelID); err != nil {
		return flow.Flow{}, err
	}
	return f, nil
}

func flowInput(in *signalsv1.FlowInput) (flow.Input, error) {
	cid, err := parseID("channel_id", in.GetChannelId())
	if err != nil {
		return flow.Input{}, err
	}
	var g flow.Graph
	if raw := in.GetGraphJson(); raw != "" {
		if err := json.Unmarshal([]byte(raw), &g); err != nil {
			return flow.Input{}, invalidArgument("graph_json: %v", err)
		}
	}
	var tc json.RawMessage
	if raw := in.GetTriggerConfigJson(); raw != "" {
		if !json.Valid([]byte(raw)) {
			return flow.Input{}, invalidArgument("trigger_config_json is not valid JSON")
		}
		tc = json.RawMessage(raw)
	}
	return flow.Input{ChannelID: cid, Name: in.GetName(), TriggerType: in.GetTriggerType(), TriggerConfig: tc, Graph: g}, nil
}

func flowProto(f flow.Flow) *signalsv1.Flow {
	if f.Graph.Nodes == nil {
		f.Graph.Nodes = []flow.Node{}
	}
	if f.Graph.Edges == nil {
		f.Graph.Edges = []flow.Edge{}
	}
	graph, _ := json.Marshal(f.Graph)
	tc := "{}"
	if len(f.TriggerConfig) > 0 {
		tc = string(f.TriggerConfig)
	}
	return &signalsv1.Flow{
		Id: f.ID.String(), ChannelId: f.ChannelID.String(), Name: f.Name, TriggerType: f.TriggerType,
		TriggerConfigJson: tc, GraphJson: string(graph), Status: f.Status,
		CreatedAt: ts(f.CreatedAt), UpdatedAt: ts(f.UpdatedAt), DeletedAt: tsPtr(f.DeletedAt),
	}
}

func runProto(r flow.FlowRun) *signalsv1.FlowRun {
	out := &signalsv1.FlowRun{
		Id: r.ID.String(), FlowId: r.FlowID.String(), Email: r.Email, CurrentNodeId: r.CurrentNodeID,
		Status: r.Status, ResumeAt: tsPtr(r.ResumeAt), Error: r.Error,
	}
	if !r.IsGuest() {
		out.CustomerId = r.CustomerID.String()
	}
	if len(r.Context) > 0 {
		out.ContextJson = string(r.Context)
	}
	return out
}

// The flow service's sentinels as the codes core's resolvers render - anything else is the suite's own failure
func flowErr(err error) error {
	switch {
	case errors.Is(err, flow.ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, flow.ErrInvalid):
		return status.Error(codes.InvalidArgument, err.Error())
	}
	return err
}
