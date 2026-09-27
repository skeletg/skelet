package signalsclient

// Core raising the moments a flow can start on - every one runs as core itself, there is no admin behind a bus event or a job

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/skeletg/skelet/apps/backend/core/lib/email"
	"github.com/skeletg/skelet/apps/backend/core/signalsapi"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
)

// Enrollment is one moment: who it happened to, on which channel, and what the moment carries
type Enrollment struct {
	Trigger   string
	ChannelID uuid.UUID
	// uuid.Nil for a guest, who is reached by the address instead
	CustomerID uuid.UUID
	Email      string
	// Shaped by the trigger - see the signalsapi.Payload* keys
	Payload map[string]any
	// Key names the moment itself (the order, the one-time link, the cart's episode) so a repeat starts one run and answers the same count
	// Empty leaves only the already-in-flow guard
	Key string
}

// A suite that didn't answer is asked once more - the run is keyed, so the second ask can't double it
// Each ask is bounded because a shopper may be waiting on the password reset behind it
const (
	enrollAttempts       = 2
	enrollAttemptTimeout = 4 * time.Second
	enrollRetryPause     = 250 * time.Millisecond
)

// Enroll answers how many active flows took the moment - zero means core's own mail still has to go out
// An error with a zero means unknown, and with a count means those flows took it while others failed; either way a caller that can wait asks again later
func (c *Client) Enroll(ctx context.Context, e Enrollment) (int, error) {
	if c == nil {
		return 0, nil // no suite to ask is a definite no
	}
	sys, err := c.System(ctx)
	if err != nil {
		return 0, err
	}
	req := &signalsv1.EnrollRequest{TriggerType: e.Trigger, ChannelId: e.ChannelID.String(), Email: e.Email, IdempotencyKey: e.Key}
	if e.CustomerID != uuid.Nil {
		req.CustomerId = e.CustomerID.String()
	}
	if len(e.Payload) > 0 {
		raw, err := json.Marshal(e.Payload)
		if err != nil {
			return 0, fmt.Errorf("enroll %s: encode payload: %w", e.Trigger, err)
		}
		req.PayloadJson = string(raw)
	}
	var last error
	for attempt := range enrollAttempts {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return 0, ctx.Err()
			case <-time.After(enrollRetryPause):
			}
		}
		actx, cancel := context.WithTimeout(sys, enrollAttemptTimeout)
		res, err := c.Flows.Enroll(actx, req)
		cancel()
		if err == nil && res.GetIncomplete() {
			return int(res.GetEnrolled()), fmt.Errorf("enroll %s on %s: %d flows took it, some could not be asked", e.Trigger, e.ChannelID, res.GetEnrolled())
		}
		if err == nil {
			return int(res.GetEnrolled()), nil
		}
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}
		if refused(err) {
			// The suite answered and refused - a bug on this side, and a definite answer: no flow took it
			log.Printf("[signals] enroll %s on %s refused: %v", e.Trigger, e.ChannelID, err)
			return 0, nil
		}
		if !Unreachable(err) {
			return 0, fmt.Errorf("enroll %s on %s: %w", e.Trigger, e.ChannelID, err) // the suite failed partway, so the count is unknown
		}
		last = err
	}
	return 0, fmt.Errorf("enroll %s on %s: %w", e.Trigger, e.ChannelID, last)
}

func refused(err error) bool {
	switch status.Code(err) {
	case codes.InvalidArgument, codes.PermissionDenied, codes.Unauthenticated, codes.NotFound, codes.Unimplemented, codes.FailedPrecondition:
		return true
	default:
		return false
	}
}

// OrderPayload is what an order moment carries - the ids the send-once record keys on, and the snapshot the
// emails render from, since the order isn't readable where the run advances
func OrderPayload(orderID, fulfillmentID uuid.UUID, snap email.OrderSnapshot) map[string]any {
	p := map[string]any{
		signalsapi.PayloadOrderID: orderID.String(), signalsapi.PayloadOrderNumber: snap.Number, signalsapi.PayloadOrder: snap,
	}
	if fulfillmentID != uuid.Nil {
		p[signalsapi.PayloadFulfillmentID] = fulfillmentID.String()
	}
	return p
}
