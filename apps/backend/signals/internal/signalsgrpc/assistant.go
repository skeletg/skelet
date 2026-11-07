package signalsgrpc

import (
	"context"
	"encoding/json"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/signalsapi"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
	"github.com/skeletg/skelet/apps/backend/signals/internal/aiactivity"
	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
	"github.com/skeletg/skelet/apps/backend/signals/internal/assistantlog"
	db "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/signals/internal/signalsboot"
)

const (
	assistantPerm = "assistant.view"
	// core's clock may run a little ahead of ours, the same allowance a browser's gets
	turnClockAhead = 5 * time.Minute
)

type assistantServer struct {
	signalsv1.UnimplementedAssistantServiceServer
	log *assistantlog.Store
	rec *analytics.Recorder
	ai  *aiactivity.Store
}

func (s *assistantServer) open(ctx context.Context, channelID string) (uuid.UUID, error) {
	cid, err := channelScope(ctx, assistantPerm, channelID)
	if err != nil {
		return uuid.Nil, err
	}
	if s.log == nil {
		return uuid.Nil, status.Error(codes.Unavailable, "this Signals service keeps no assistant conversations")
	}
	return cid, nil
}

func (s *assistantServer) ListAssistantConversations(ctx context.Context, req *signalsv1.ListAssistantConversationsRequest) (*signalsv1.ListAssistantConversationsResponse, error) {
	cid, err := s.open(ctx, req.GetChannelId())
	if err != nil {
		return nil, err
	}
	if u := req.GetUnanswered(); u != "" && u != assistantlog.AnyGap && !slices.Contains(assistantlog.Gaps, u) {
		return nil, invalidArgument("unanswered is %s or one of %s, or empty for every conversation", assistantlog.AnyGap, strings.Join(assistantlog.Gaps, ", "))
	}
	q := assistantlog.Query{Limit: int(req.GetLimit()), Search: req.GetSearch(), Unanswered: req.GetUnanswered()}
	if req.CustomerId != nil && *req.CustomerId != "" {
		customer, err := parseID("customer_id", *req.CustomerId)
		if err != nil {
			return nil, err
		}
		q.Customer = &customer
	}
	if req.GetBeforeAt() != nil {
		at := req.GetBeforeAt().AsTime()
		q.BeforeAt, q.BeforeID = &at, uuidOrNil(req.GetBeforeId())
	}
	rows, more, err := s.log.List(ctx, cid, q)
	if err != nil {
		return nil, err
	}
	out := &signalsv1.ListAssistantConversationsResponse{More: more, Conversations: make([]*signalsv1.AssistantConversationSummary, len(rows))}
	for i, r := range rows {
		out.Conversations[i] = conversationOut(r)
	}
	if req.GetWithCounts() {
		c, err := s.log.GapCounts(ctx, cid, q.Customer)
		if err != nil {
			return nil, err
		}
		out.Counts = &signalsv1.AssistantGapCounts{Any: c.Any, NoAnswer: c.NoAnswer, CutShort: c.CutShort, FoundNothing: c.FoundNothing, Unsure: c.Unsure}
	}
	return out, nil
}

func conversationOut(r db.ListAssistantConversationsRow) *signalsv1.AssistantConversationSummary {
	out := &signalsv1.AssistantConversationSummary{
		Id: r.ID.String(), CustomerId: nullID(r.CustomerID), AnonId: r.AnonID, FirstQuestion: r.FirstQuestion, Turns: r.Turns,
		CartChanged: r.CartChanged, Checkout: r.Checkout, Incomplete: r.Incomplete,
		StartedAt: timestamppb.New(r.StartedAt), LastAt: timestamppb.New(r.LastAt), Unanswered: r.Unanswered,
	}
	if r.GapID != uuid.Nil {
		out.LatestUnanswered = &signalsv1.AssistantGapTurn{Id: r.GapID.String(), Question: r.GapQuestion,
			NoAnswer: r.GapNoAnswer, CutShort: r.GapCutShort, FoundNothing: r.GapFoundNothing, Unsure: r.GapUnsure}
	}
	return out
}

func (s *assistantServer) GetAssistantConversation(ctx context.Context, req *signalsv1.GetAssistantConversationRequest) (*signalsv1.GetAssistantConversationResponse, error) {
	cid, err := s.open(ctx, req.GetChannelId())
	if err != nil {
		return nil, err
	}
	conv, err := parseID("conversation_id", req.GetConversationId())
	if err != nil {
		return nil, err
	}
	summary, found, err := s.log.Conversation(ctx, cid, conv)
	if err != nil || !found {
		return &signalsv1.GetAssistantConversationResponse{}, err
	}
	rows, err := s.log.Turns(ctx, cid, conv)
	if err != nil {
		return nil, err
	}
	out := &signalsv1.GetAssistantConversationResponse{Conversation: conversationOut(summary), Turns: make([]*signalsv1.AssistantTurnRecord, len(rows))}
	for i, t := range rows {
		out.Turns[i] = &signalsv1.AssistantTurnRecord{
			Id: t.ID.String(), ConversationId: t.ConversationID.String(), ChannelId: cid.String(), CustomerId: nullID(t.CustomerID),
			AnonId: t.AnonID, FoundNothing: t.FoundNothing, Unsure: t.Unsure, Page: t.Page, Question: t.Question, Reply: t.Reply, Recap: t.Recap, FollowUps: t.FollowUps,
			Products: t.Products, CartChanged: t.CartChanged, Checkout: t.Checkout, Incomplete: t.Incomplete, Model: t.Model,
			InputTokens: t.InputTokens, OutputTokens: t.OutputTokens, CacheReadTokens: t.CacheReadTokens, CacheWriteTokens: t.CacheWriteTokens,
			At: timestamppb.New(t.At), Steps: t.Steps, ToolCalls: t.ToolCalls, Refused: t.Refused, Thought: t.Thought,
		}
	}
	return out, nil
}

func (s *assistantServer) GetAssistantTurnRounds(ctx context.Context, req *signalsv1.GetAssistantTurnRoundsRequest) (*signalsv1.GetAssistantTurnRoundsResponse, error) {
	cid, err := s.open(ctx, req.GetChannelId())
	if err != nil {
		return nil, err
	}
	turn, err := parseID("turn_id", req.GetTurnId())
	if err != nil {
		return nil, err
	}
	raw, found, err := s.log.TurnRounds(ctx, cid, turn)
	if err != nil || !found {
		return &signalsv1.GetAssistantTurnRoundsResponse{}, err
	}
	return &signalsv1.GetAssistantTurnRoundsResponse{Rounds: roundsOut(ctx, raw)}, nil
}

func (s *ingestServer) RecordAssistantTurn(ctx context.Context, req *signalsv1.RecordAssistantTurnRequest) (*signalsv1.RecordAssistantTurnResponse, error) {
	if err := requireSystem(ctx); err != nil {
		return nil, err
	}
	if s.assistant == nil {
		return &signalsv1.RecordAssistantTurnResponse{}, nil
	}
	t := req.GetTurn()
	id, err := parseID("id", t.GetId())
	if err != nil {
		return nil, err
	}
	conv, err := parseID("conversation_id", t.GetConversationId())
	if err != nil {
		return nil, err
	}
	ch, err := parseID("channel_id", t.GetChannelId())
	if err != nil {
		return nil, err
	}
	var customer *uuid.UUID
	if t.GetCustomerId() != "" {
		c, err := parseID("customer_id", t.GetCustomerId())
		if err != nil {
			return nil, err
		}
		customer = &c
	}
	if t.GetAnonId() != "" && auth.SanitizeAnonID(t.GetAnonId()) == "" {
		return nil, invalidArgument("anon_id is up to 64 letters, digits, - and _")
	}
	if customer == nil && t.GetAnonId() == "" {
		return nil, invalidArgument("a turn needs a customer_id or an anon_id")
	}
	if !signalsapi.AssistantTurnFits(t) {
		return nil, invalidArgument("the turn is larger than a turn can be")
	}
	rounds, err := roundsIn(t.GetRounds())
	if err != nil {
		return nil, err
	}
	if customer != nil {
		erased, err := s.roster.Erased(ctx, *customer)
		if err != nil {
			return nil, err
		}
		if erased {
			return &signalsv1.RecordAssistantTurnResponse{}, nil
		}
	}
	at := turnTime(t.GetAt(), time.Now())
	followUps := make([]string, len(t.GetFollowUps()))
	for i, f := range t.GetFollowUps() {
		followUps[i] = noNUL(f)
	}
	p := db.RecordAssistantTurnParams{
		ID: id, ConversationID: conv, ChannelID: ch, AnonID: t.GetAnonId(), Page: noNUL(t.GetPage()), Question: noNUL(t.GetQuestion()),
		Reply: noNUL(t.GetReply()), Recap: noNUL(t.GetRecap()), FollowUps: followUps, Rounds: rounds, Products: t.GetProducts(),
		CartChanged: t.GetCartChanged(), Checkout: t.GetCheckout(), Incomplete: t.GetIncomplete(), FoundNothing: t.GetFoundNothing(), Unsure: t.GetUnsure(), Model: noNUL(t.GetModel()),
		InputTokens: t.GetInputTokens(), OutputTokens: t.GetOutputTokens(), CacheReadTokens: t.GetCacheReadTokens(), CacheWriteTokens: t.GetCacheWriteTokens(), At: at,
	}
	if customer != nil {
		p.CustomerID = uuid.NullUUID{UUID: *customer, Valid: true}
	}
	ok, err := s.assistant.Record(ctx, p)
	if err != nil {
		return nil, err
	}
	return &signalsv1.RecordAssistantTurnResponse{Recorded: ok}, nil
}

func (s *maintenanceServer) PruneAssistantConversations(ctx context.Context, _ *signalsv1.PruneAssistantConversationsRequest) (*signalsv1.PruneAssistantConversationsResponse, error) {
	if err := requireSystem(ctx); err != nil {
		return nil, err
	}
	if s.assistant == nil {
		return &signalsv1.PruneAssistantConversationsResponse{}, nil
	}
	conversations, turns, err := signalsboot.AssistantPruneJob(s.settings, s.assistant)(ctx)
	if err != nil {
		return nil, err
	}
	return &signalsv1.PruneAssistantConversationsResponse{Pruned: conversations, Turns: turns}, nil
}

type visitorIDs interface {
	AnonIDsForCustomers(ctx context.Context, customerIDs []uuid.UUID) ([]string, error)
}

// EraseConversations drops the customers' conversations, guest chats under their stitched visitor ids included
func EraseConversations(ctx context.Context, ids visitorIDs, log *assistantlog.Store, customers []uuid.UUID) error {
	if len(customers) == 0 {
		return nil
	}
	anon, err := ids.AnonIDsForCustomers(ctx, customers)
	if err != nil {
		return err
	}
	return log.Erase(ctx, customers, anon)
}

// A replay keeps the time it happened, so retention prunes it on the same schedule as everything else
func turnTime(stamp *timestamppb.Timestamp, now time.Time) time.Time {
	if stamp.CheckValid() != nil {
		return now
	}
	if at := stamp.AsTime(); !at.After(now.Add(turnClockAhead)) {
		return at
	}
	return now
}

// Postgres text and jsonb both refuse a NUL, and one in a shopper's message must not cost the whole turn
func noNUL(s string) string { return strings.ReplaceAll(s, "\x00", "") }

// Rounds are kept as the contract's own JSON, measured the way core trims them, so what's stored is exactly what comes back
func roundsIn(in []*signalsv1.AssistantRound) (json.RawMessage, error) {
	clean := make([]*signalsv1.AssistantRound, len(in))
	for i, r := range in {
		clean[i] = &signalsv1.AssistantRound{Thinking: noNUL(r.GetThinking()), Text: noNUL(r.GetText()), Shown: r.GetShown()}
		for _, c := range r.GetCalls() {
			clean[i].Calls = append(clean[i].Calls, &signalsv1.AssistantToolCall{Tool: noNUL(c.GetTool()), InputJson: noNUL(c.GetInputJson()),
				Output: noNUL(c.GetOutput()), IsError: c.GetIsError(), Skipped: c.GetSkipped()})
		}
	}
	out, err := signalsapi.AssistantRoundsJSON(clean)
	if err != nil {
		return nil, invalidArgument("a round can't be kept: %v", err)
	}
	if len(out) > signalsapi.MaxAssistantRoundsBytes {
		return nil, invalidArgument("the turn's rounds are larger than a turn can be")
	}
	return out, nil
}

var roundsReader = protojson.UnmarshalOptions{DiscardUnknown: true}

// A row that won't decode answers without its rounds rather than failing the whole turn
func roundsOut(ctx context.Context, raw json.RawMessage) []*signalsv1.AssistantRound {
	var rounds []json.RawMessage
	if err := json.Unmarshal(raw, &rounds); err != nil {
		slog.WarnContext(ctx, "signals: an assistant turn's rounds don't decode", "err", err)
		return nil
	}
	out := make([]*signalsv1.AssistantRound, 0, len(rounds))
	for _, r := range rounds {
		round := &signalsv1.AssistantRound{}
		if err := roundsReader.Unmarshal(r, round); err != nil {
			slog.WarnContext(ctx, "signals: an assistant round doesn't decode", "err", err)
			return nil
		}
		out = append(out, round)
	}
	return out
}

func nullID(id uuid.NullUUID) *string {
	if !id.Valid {
		return nil
	}
	s := id.UUID.String()
	return &s
}

const (
	aiInsightsDays   = signalsapi.DefaultReportDays
	aiProductsListed = 20
	aiAgentsListed   = 20
)

func (s *assistantServer) GetAIInsights(ctx context.Context, req *signalsv1.GetAIInsightsRequest) (*signalsv1.GetAIInsightsResponse, error) {
	cid, err := channelScope(ctx, assistantPerm, req.GetChannelId())
	if err != nil {
		return nil, err
	}
	if s.rec == nil {
		return nil, status.Error(codes.Unavailable, "this Signals service has no event log to read")
	}
	from := since(req.GetSinceDays(), aiInsightsDays)
	impact, err := s.rec.AIImpact(ctx, cid, from, strings.ToUpper(strings.TrimSpace(req.GetCurrency())))
	if err != nil {
		return nil, err
	}
	products, err := s.rec.AIProducts(ctx, cid, from, aiProductsListed)
	if err != nil {
		return nil, err
	}
	var agents []aiactivity.Agent
	if s.ai != nil {
		if agents, err = s.ai.Agents(ctx, cid, from, aiAgentsListed); err != nil {
			return nil, err
		}
	}
	out := &signalsv1.GetAIInsightsResponse{
		Chatted: aiCohort(impact.Chatted), DidNotChat: aiCohort(impact.DidNotChat),
		AgentOrders: impact.AgentOrders, AgentRevenueCents: impact.AgentRevenueCents,
	}
	for _, p := range products {
		out.Products = append(out.Products, &signalsv1.AIProduct{ProductId: p.ProductID, Shown: p.Shown, People: p.People, Carted: p.Carted, Ordered: p.Ordered})
	}
	for _, a := range agents {
		agent := &signalsv1.AIAgent{Agent: a.Name, Calls: a.Calls, Searches: a.Searches, EmptySearches: a.Empty}
		for _, t := range a.Tools {
			agent.Tools = append(agent.Tools, &signalsv1.AIToolCount{Tool: t.Tool, Calls: t.Calls})
		}
		out.Agents = append(out.Agents, agent)
	}
	return out, nil
}

func aiCohort(c analytics.AICohort) *signalsv1.AICohort {
	return &signalsv1.AICohort{People: c.People, Buyers: c.Buyers, Orders: c.Orders, RevenueCents: c.RevenueCents,
		RevenueOrders: c.RevenueOrders, ReturnedOrders: c.ReturnedOrders}
}
