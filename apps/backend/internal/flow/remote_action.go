package flow

import (
	"context"
	"encoding/json"
	"log"
	"maps"

	"github.com/skeletg/skelet/apps/backend/internal/remoteprovider"
)

// Unlike the fire-and-forget webhook node, this one calls a remote provider (kind=flow_action) and WAITS for the response, folding it back into the run - the escape hatch for a custom step like CRM enrichment, ML scoring or an external decision, without a Go plugin
// Wire contract: request {flowId, customerId, context}, response {context?, branch?}
// context shallow-merges into the run context so later nodes see the new keys, and branch steers the run down a matching labeled edge, otherwise it continues on the default path
// A failure is best-effort - logged, with the run continuing on the default path, since a down enrichment service must not strand a customer mid-flow

func (s *Service) SetRemoteActions(svc *remoteprovider.Service, invoker *remoteprovider.Invoker) {
	s.remoteProviders = svc
	s.remoteInvoker = invoker
}

type remoteActionRequest struct {
	FlowID     string          `json:"flowId"`
	CustomerID string          `json:"customerId"`
	Context    json.RawMessage `json:"context,omitempty"`
}

type remoteActionResponse struct {
	Context json.RawMessage `json:"context"`
	Branch  string          `json:"branch"`
}

// Invokes the node's provider and returns the branch handle it chose, "" meaning the default path
func (s *Service) execRemoteAction(ctx context.Context, node Node, run *FlowRun) string {
	if s.remoteProviders == nil || s.remoteInvoker == nil {
		return ""
	}
	slug := node.Data.Provider
	if slug == "" {
		return ""
	}
	p, err := s.remoteProviders.GetBySlug(ctx, remoteprovider.KindFlowAction, slug)
	if err != nil || !p.IsActive {
		log.Printf("[flow] remote action provider %q unavailable (skipped)", slug)
		return ""
	}

	req := remoteActionRequest{
		FlowID:     run.FlowID.String(),
		CustomerID: run.CustomerID.String(),
		Context:    run.Context,
	}
	var resp remoteActionResponse
	if err := s.remoteInvoker.Invoke(ctx, p, req, &resp); err != nil {
		log.Printf("[flow] remote action %q failed (continuing on default path): %v", slug, err)
		return ""
	}
	if len(resp.Context) > 0 {
		run.Context = mergeContext(run.Context, resp.Context)
	}
	return resp.Branch
}

// Shallow-merges patch's top-level keys over base - both are JSON objects, and a malformed side is left untouched so a bad response can't corrupt the run context
func mergeContext(base, patch json.RawMessage) json.RawMessage {
	baseMap := map[string]any{}
	if len(base) > 0 {
		if err := json.Unmarshal(base, &baseMap); err != nil {
			return base // base isn't an object - don't touch it
		}
	}
	patchMap := map[string]any{}
	if err := json.Unmarshal(patch, &patchMap); err != nil {
		return base // patch isn't an object - ignore it
	}
	maps.Copy(baseMap, patchMap)
	merged, err := json.Marshal(baseMap)
	if err != nil {
		return base
	}
	return merged
}
