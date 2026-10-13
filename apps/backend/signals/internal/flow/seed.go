package flow

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/signals/internal/emailtemplate"
)

// The lifecycle flows seeded for every channel - one per transactional trigger, each wired to send the matching slot email
var transactionalFlowSpecs = []struct {
	Trigger string
	Slot    string
	Name    string
}{
	{TriggerOrderPlaced, emailtemplate.KeyOrderConfirmation, "Order confirmation"},
	{TriggerOrderShipped, emailtemplate.KeyOrderShipment, "Shipment notification"},
	{TriggerOrderCancelled, emailtemplate.KeyOrderCancellation, "Order cancellation"},
	{TriggerPaymentRefunded, emailtemplate.KeyOrderRefund, "Refund notification"},
	{TriggerEmailVerification, emailtemplate.KeyVerification, "Email verification"},
	{TriggerPasswordReset, emailtemplate.KeyPasswordReset, "Password reset"},
	{TriggerSignupConfirmation, emailtemplate.KeySignupConfirmation, "Signup confirmation"},
	{TriggerAccountExists, emailtemplate.KeyAccountExists, "Account already exists"},
}

// StarterContent is what one seeding run created - zeros mean the channel had it all already
type StarterContent struct {
	Blocks int
	Emails int
	Flows  int
}

// SeedStarterContent gives a channel Pulse's starter content - the blocks, an email per transactional slot and an active flow that sends it
// Idempotent, and only on request - an active order_placed flow moves the receipt into the builder, the operator's call
func (s *Service) SeedStarterContent(ctx context.Context, channelID uuid.UUID) (StarterContent, error) {
	var out StarterContent
	var err error
	if out.Blocks, err = s.tmpl.SeedStandardBlocks(ctx, channelID); err != nil {
		return out, err
	}
	if out.Emails, err = s.tmpl.SeedTransactional(ctx, channelID); err != nil {
		return out, err
	}
	if out.Flows, err = s.SeedTransactionalFlows(ctx, channelID); err != nil {
		return out, err
	}
	return out, nil
}

// Ensures each transactional trigger has an active, editable default flow for the channel, designated transactional so it bypasses the marketing-consent gate
// This is the works-out-of-the-box default with nothing hardcoded, since every flow stays fully editable and deletable - Idempotent, leaving a channel that already has a flow for a trigger untouched, and it needs the slot emails to exist first
func (s *Service) SeedTransactionalFlows(ctx context.Context, channelID uuid.UUID) (int, error) {
	// Archived flows count as already seeded - otherwise archiving a seeded flow would make the next run recreate it
	existing, err := s.repo.List(ctx, channelID, true)
	if err != nil {
		return 0, err
	}
	seen := make(map[string]bool, len(existing))
	for _, f := range existing {
		seen[f.TriggerType] = true
	}

	tc, _ := json.Marshal(TriggerConfig{Transactional: true})
	created := 0
	for _, spec := range transactionalFlowSpecs {
		if seen[spec.Trigger] {
			continue // channel already has a flow for this trigger
		}
		slotEmail, ok, err := s.tmpl.SlotEmail(ctx, channelID, spec.Slot)
		if err != nil {
			return created, err
		}
		if !ok {
			continue // slot email not seeded yet - skip (run the email seed first)
		}
		f, err := s.Create(ctx, Input{
			ChannelID:     channelID,
			Name:          spec.Name,
			TriggerType:   spec.Trigger,
			TriggerConfig: tc,
			Graph:         defaultTransactionalGraph(spec.Name, slotEmail.ID.String()),
		})
		if err != nil {
			return created, err
		}
		if _, err := s.SetStatus(ctx, f.ID, StatusActive); err != nil {
			return created, err
		}
		created++
	}
	return created, nil
}

// Builds the minimal graph, trigger to send_email against the seeded slot library email - operators can edit and extend it freely
func defaultTransactionalGraph(name, emailID string) Graph {
	return Graph{
		Nodes: []Node{
			{ID: "trigger", Type: NodeTrigger, Position: Position{X: 240, Y: 0}, Data: NodeData{Label: "Trigger"}},
			{ID: "email", Type: NodeSendEmail, Position: Position{X: 240, Y: 140}, Data: NodeData{Label: name, EmailID: emailID}},
		},
		Edges: []Edge{{ID: "trigger-email", Source: "trigger", Target: "email"}},
	}
}
