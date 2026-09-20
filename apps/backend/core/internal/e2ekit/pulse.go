package e2ekit

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
)

// Pulse lives in the suite, so a scenario builds its flows and emails over the contract as core itself

// SystemCtx is core's own token on the harness context, for the Pulse calls below
func (h *Harness) SystemCtx(t TB) context.Context {
	t.Helper()
	ctx, err := h.D.SignalsClient.System(h.Ctx)
	if err != nil {
		t.Fatalf("mint the system token: %v", err)
	}
	return ctx
}

// CreateLibraryEmail saves one email in the channel's library, from a block document given as Go values
func (h *Harness) CreateLibraryEmail(t TB, channelID uuid.UUID, name, subject string, blocks []map[string]any) uuid.UUID {
	t.Helper()
	raw, err := json.Marshal(blocks)
	if err != nil {
		t.Fatalf("encode blocks: %v", err)
	}
	res, err := h.D.SignalsClient.Builder.SaveLibraryEmail(h.SystemCtx(t), &signalsv1.SaveLibraryEmailRequest{Email: &signalsv1.LibraryEmailInput{
		ChannelId: channelID.String(), Name: name, Subject: subject, BlocksJson: string(raw),
	}})
	if err != nil {
		t.Fatalf("save the library email: %v", err)
	}
	id, err := uuid.Parse(res.GetEmail().GetId())
	if err != nil {
		t.Fatalf("saved email id: %v", err)
	}
	return id
}

// CreateFlow saves a flow and puts it in the given status - the graph and trigger config as Go values
func (h *Harness) CreateFlow(t TB, channelID uuid.UUID, name, trigger, status string, triggerConfig, graph map[string]any) uuid.UUID {
	t.Helper()
	g, err := json.Marshal(graph)
	if err != nil {
		t.Fatalf("encode graph: %v", err)
	}
	tc, err := json.Marshal(triggerConfig)
	if err != nil {
		t.Fatalf("encode trigger config: %v", err)
	}
	sys := h.SystemCtx(t)
	res, err := h.D.SignalsClient.Flows.CreateFlow(sys, &signalsv1.CreateFlowRequest{Flow: &signalsv1.FlowInput{
		ChannelId: channelID.String(), Name: name, TriggerType: trigger, TriggerConfigJson: string(tc), GraphJson: string(g),
	}})
	if err != nil {
		t.Fatalf("create the flow: %v", err)
	}
	if _, err := h.D.SignalsClient.Flows.SetFlowStatus(sys, &signalsv1.SetFlowStatusRequest{Id: res.GetFlow().GetId(), Status: status}); err != nil {
		t.Fatalf("set the flow %s: %v", status, err)
	}
	id, err := uuid.Parse(res.GetFlow().GetId())
	if err != nil {
		t.Fatalf("flow id: %v", err)
	}
	return id
}

// SendEmailGraph is the smallest useful flow: the trigger wired straight into a send of one saved email
func SendEmailGraph(emailID uuid.UUID) map[string]any {
	return map[string]any{
		"nodes": []map[string]any{
			{"id": "trigger", "type": "trigger", "position": map[string]any{"x": 0, "y": 0}, "data": map[string]any{"label": "Trigger"}},
			{"id": "email", "type": "send_email", "position": map[string]any{"x": 0, "y": 140}, "data": map[string]any{"label": "Email", "emailId": emailID.String()}},
		},
		"edges": []map[string]any{{"id": "trigger-email", "source": "trigger", "target": "email"}},
	}
}

// FlowRuns is every run a flow has, newest first
func (h *Harness) FlowRuns(t TB, flowID uuid.UUID) []*signalsv1.FlowRun {
	t.Helper()
	res, err := h.D.SignalsClient.Flows.ListFlowRuns(h.SystemCtx(t), &signalsv1.ListFlowRunsRequest{FlowId: flowID.String(), Limit: 500})
	if err != nil {
		t.Fatalf("list flow runs: %v", err)
	}
	return res.GetRuns()
}

// RunnableFlowRuns counts what the engine would advance for a flow - a scenario sees enrollment without draining
func (h *Harness) RunnableFlowRuns(t TB, flowID uuid.UUID) int {
	t.Helper()
	n := 0
	now := time.Now()
	for _, r := range h.FlowRuns(t, flowID) {
		if r.GetStatus() == "active" || (r.GetStatus() == "waiting" && r.ResumeAt != nil && !r.GetResumeAt().AsTime().After(now)) {
			n++
		}
	}
	return n
}

// AdvanceFlows ticks the engine until the given flows have nothing runnable left - enrollment rides the
// outbox, so the runs don't exist until it drains
func (h *Harness) AdvanceFlows(t TB, flowIDs ...uuid.UUID) {
	t.Helper()
	h.Settle(t)
	for range 20 {
		pending := 0
		for _, id := range flowIDs {
			pending += h.RunnableFlowRuns(t, id)
		}
		if pending == 0 {
			return
		}
		if _, err := h.D.SignalsClient.AdvanceFlowRuns(h.Ctx, 0); err != nil {
			t.Fatalf("advance flow runs: %v", err)
		}
		h.Settle(t)
	}
	t.Fatal("these flows' runs never drained after 20 rounds - a flow is re-enrolling in a loop")
}

// CountFlows and CountLibraryEmails are what a channel holds in Pulse, for the scenarios that assert nothing was seeded
func (h *Harness) CountFlows(t TB, channelID uuid.UUID) int {
	t.Helper()
	res, err := h.D.SignalsClient.Flows.ListFlows(h.SystemCtx(t), &signalsv1.ListFlowsRequest{ChannelId: channelID.String(), IncludeDeleted: true})
	if err != nil {
		t.Fatalf("list flows: %v", err)
	}
	return len(res.GetFlows())
}

func (h *Harness) CountLibraryEmails(t TB, channelID uuid.UUID) int {
	t.Helper()
	res, err := h.D.SignalsClient.Builder.ListLibraryEmails(h.SystemCtx(t), &signalsv1.ListLibraryEmailsRequest{ChannelId: channelID.String()})
	if err != nil {
		t.Fatalf("list library emails: %v", err)
	}
	return len(res.GetEmails())
}

// SeedStarterContent asks the suite for Pulse's starter content on a channel, as the emails hub button does
func (h *Harness) SeedStarterContent(t TB, channelID uuid.UUID) (blocks, emails, flows int) {
	t.Helper()
	res, err := h.D.SignalsClient.Flows.SeedStarterContent(h.SystemCtx(t), &signalsv1.SeedStarterContentRequest{ChannelId: channelID.String()})
	if err != nil {
		t.Fatalf("seed starter content: %v", err)
	}
	return int(res.GetBlocks()), int(res.GetEmails()), int(res.GetFlows())
}

// FlowIDByTrigger finds the channel's flow on a trigger
func (h *Harness) FlowIDByTrigger(t TB, channelID uuid.UUID, trigger string) uuid.UUID {
	t.Helper()
	res, err := h.D.SignalsClient.Flows.ListFlows(h.SystemCtx(t), &signalsv1.ListFlowsRequest{ChannelId: channelID.String()})
	if err != nil {
		t.Fatalf("list flows: %v", err)
	}
	for _, f := range res.GetFlows() {
		if f.GetTriggerType() == trigger {
			id, err := uuid.Parse(f.GetId())
			if err != nil {
				t.Fatalf("flow id %q: %v", f.GetId(), err)
			}
			return id
		}
	}
	t.Fatalf("no %s flow on channel %s", trigger, channelID)
	return uuid.Nil
}
