package order

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/internal/remoteprovider"
)

// A merchant HTTP endpoint (remoteprovider kind=returns_policy) deciding whether a return is allowed - the escape hatch for Loop, Returnly or Narvar-class platforms without a Go plugin Runs AFTER the native quantity guard and BEFORE the return is written, layering policy on top Wire contract: request {orderId, resolution, refundTarget, lines: [{orderLineId, quantity, reason}]}, response {allowed, reason} Account-global, since returns are order-scoped. fail_open ALLOWS, because the native guard already ran and a down platform mustn't strand a legitimate return; fail_closed refuses

type RemoteReturnsPolicy interface {
	// Whether the return may proceed - true with no provider or a fail_open error, and a reason accompanies a refusal
	Allowed(ctx context.Context, in RemoteReturnInput) (allowed bool, reason string)
}

type RemoteReturnInput struct {
	OrderID      uuid.UUID
	ChannelID    uuid.UUID
	Resolution   string
	RefundTarget string
	Lines        []RemoteReturnLine
}

type RemoteReturnLine struct {
	OrderLineID uuid.UUID
	Quantity    float64
	Reason      string
}

// Optional
func (s *Service) SetRemoteReturns(src RemoteReturnsPolicy) { s.remoteReturns = src }

type remoteReturnRequest struct {
	OrderID      string                `json:"orderId"`
	Resolution   string                `json:"resolution"`
	RefundTarget string                `json:"refundTarget"`
	Lines        []remoteReturnReqLine `json:"lines"`
}

type remoteReturnReqLine struct {
	OrderLineID string  `json:"orderLineId"`
	Quantity    float64 `json:"quantity"`
	Reason      string  `json:"reason"`
}

type remoteReturnResponse struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason"`
}

type remoteReturnsPolicy struct {
	svc     *remoteprovider.Service
	invoker *remoteprovider.Invoker
}

func NewRemoteReturnsPolicy(svc *remoteprovider.Service, invoker *remoteprovider.Invoker) RemoteReturnsPolicy {
	return &remoteReturnsPolicy{svc: svc, invoker: invoker}
}

func (r *remoteReturnsPolicy) Allowed(ctx context.Context, in RemoteReturnInput) (bool, string) {
	p, found, err := r.svc.ActiveFor(ctx, remoteprovider.KindReturnsPolicy, &in.ChannelID)
	if err != nil {
		// Not the same as "no provider": allowing the RMA here would open a refund the policy may forbid (window expired, final sale). This seam has no error slot, so the unknown resolves the same way the Invoke path resolves a FailClosed failure - an admin sees a retryable message instead of an irreversible refund
		return false, "returns policy check unavailable"
	}
	if !found {
		return true, "" // no provider - native quantity guard already passed
	}

	lines := make([]remoteReturnReqLine, len(in.Lines))
	for i, l := range in.Lines {
		lines[i] = remoteReturnReqLine{OrderLineID: l.OrderLineID.String(), Quantity: l.Quantity, Reason: l.Reason}
	}
	req := remoteReturnRequest{OrderID: in.OrderID.String(), Resolution: in.Resolution, RefundTarget: in.RefundTarget, Lines: lines}
	var resp remoteReturnResponse
	if err := r.invoker.Invoke(ctx, p, req, &resp); err != nil {
		if p.FailurePolicy == remoteprovider.FailClosed {
			return false, fmt.Sprintf("returns policy check unavailable (%s)", p.Slug)
		}
		return true, "" // fail_open - allow
	}
	if !resp.Allowed {
		reason := resp.Reason
		if reason == "" {
			reason = "return not permitted by policy"
		}
		return false, capPolicyReason(reason)
	}
	return true, ""
}

const maxPolicyReasonRunes = 300

// An endpoint's free text lands in the admin's error banner verbatim, so it is capped
func capPolicyReason(reason string) string {
	if r := []rune(reason); len(r) > maxPolicyReasonRunes {
		return string(r[:maxPolicyReasonRunes]) + "…"
	}
	return reason
}
