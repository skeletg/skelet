package wiring

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/signalsclient"
	"github.com/skeletg/skelet/apps/backend/core/lib/email"
	"github.com/skeletg/skelet/apps/backend/core/signalsapi"
)

// FlowEnroller is how core's domains hand a moment to the flow engine - each becomes an Enroll over the contract
// Moments are keyed by what they happened to, so a retry, a relayed handler or a re-listed cart starts one run
type FlowEnroller struct{ client *signalsclient.Client }

func NewFlowEnroller(c *signalsclient.Client) *FlowEnroller { return &FlowEnroller{client: c} }

func (e *FlowEnroller) enroll(ctx context.Context, trigger, key string, channelID, customerID uuid.UUID, to string, payload map[string]any) (int, error) {
	return e.client.Enroll(ctx, signalsclient.Enrollment{
		Trigger: trigger, ChannelID: channelID, CustomerID: customerID, Email: to, Payload: payload, Key: key,
	})
}

// The moments that must reach the shopper now read an unknown answer as zero and send core's own mail
func (e *FlowEnroller) answered(ctx context.Context, trigger, key string, channelID, customerID uuid.UUID, to string, payload map[string]any) int {
	n, err := e.enroll(ctx, trigger, key, channelID, customerID, to, payload)
	if err != nil {
		log.Printf("[flows] %s: %v - core sends its own mail", trigger, err)
	}
	return n
}

func moment(trigger string, parts ...string) string {
	return strings.Join(append([]string{trigger}, parts...), ":")
}

// A one-time link is the moment for account mail - the same link twice is one request, hashed because it's stored beside the run
func linkDigest(link string) string {
	sum := sha256.Sum256([]byte(link))
	return hex.EncodeToString(sum[:12])
}

// --- ordermail.FlowDispatcher: guests enroll keyed by the order email, so they get their receipt too ---

func (e *FlowEnroller) OnOrderPlaced(ctx context.Context, channelID, customerID uuid.UUID, to string, orderID uuid.UUID, snap email.OrderSnapshot) int {
	return e.answered(ctx, signalsapi.TriggerOrderPlaced, moment(signalsapi.TriggerOrderPlaced, orderID.String()),
		channelID, customerID, to, signalsclient.OrderPayload(orderID, uuid.Nil, snap))
}

func (e *FlowEnroller) OnOrderShipped(ctx context.Context, channelID, customerID uuid.UUID, to string, orderID, fulfillmentID uuid.UUID, snap email.OrderSnapshot) int {
	shipment := fulfillmentID
	if shipment == uuid.Nil {
		shipment = orderID
	}
	return e.answered(ctx, signalsapi.TriggerOrderShipped, moment(signalsapi.TriggerOrderShipped, shipment.String()),
		channelID, customerID, to, signalsclient.OrderPayload(orderID, fulfillmentID, snap))
}

func (e *FlowEnroller) OnOrderCancelled(ctx context.Context, channelID, customerID uuid.UUID, to string, orderID uuid.UUID, snap email.OrderSnapshot) int {
	return e.answered(ctx, signalsapi.TriggerOrderCancelled, moment(signalsapi.TriggerOrderCancelled, orderID.String()),
		channelID, customerID, to, signalsclient.OrderPayload(orderID, uuid.Nil, snap))
}

// ref is the dedup reference (refund txn id or status) carried downstream
func (e *FlowEnroller) OnPaymentRefunded(ctx context.Context, channelID, customerID uuid.UUID, to string, orderID uuid.UUID, ref string, snap email.OrderSnapshot) int {
	p := signalsclient.OrderPayload(orderID, uuid.Nil, snap)
	p[signalsapi.PayloadRefundRef] = ref
	return e.answered(ctx, signalsapi.TriggerPaymentRefunded, moment(signalsapi.TriggerPaymentRefunded, orderID.String(), ref),
		channelID, customerID, to, p)
}

// --- customer's account mail: the suite runs these inside the call, the one-time link has to go out now ---

func (e *FlowEnroller) OnEmailVerification(ctx context.Context, channelID, customerID uuid.UUID, verifyLink, storeName, firstName string) int {
	return e.answered(ctx, signalsapi.TriggerEmailVerification, moment(signalsapi.TriggerEmailVerification, customerID.String(), linkDigest(verifyLink)),
		channelID, customerID, "", map[string]any{
			signalsapi.PayloadVerifyLink: verifyLink, signalsapi.PayloadStoreName: storeName, signalsapi.PayloadFirstName: firstName,
		})
}

func (e *FlowEnroller) OnPasswordReset(ctx context.Context, channelID, customerID uuid.UUID, resetLink, expiresIn, storeName, firstName string) int {
	return e.answered(ctx, signalsapi.TriggerPasswordReset, moment(signalsapi.TriggerPasswordReset, customerID.String(), linkDigest(resetLink)),
		channelID, customerID, "", map[string]any{
			signalsapi.PayloadResetLink: resetLink, signalsapi.PayloadResetExpiresIn: expiresIn,
			signalsapi.PayloadStoreName: storeName, signalsapi.PayloadFirstName: firstName,
		})
}

// --- the marketing moments core's domains and jobs raise ---
// No mail of core's to fall back on, so an unknown answer comes back as the error and the relay or the next tick asks again

// One registration per account, ever
func (e *FlowEnroller) OnCustomerRegistered(ctx context.Context, channelID, customerID uuid.UUID) error {
	_, err := e.enroll(ctx, signalsapi.TriggerCustomerRegistered, moment(signalsapi.TriggerCustomerRegistered, customerID.String()), channelID, customerID, "", nil)
	return err
}

// A guest cart enrolls keyed by the email captured at checkout entry, so guest carts recover too
// lastActivity names the episode - a cart touched and left again is a new moment
func (e *FlowEnroller) OnCartAbandoned(ctx context.Context, channelID, customerID uuid.UUID, to string, cartID uuid.UUID, lastActivity time.Time) error {
	_, err := e.enroll(ctx, signalsapi.TriggerCartAbandoned, moment(signalsapi.TriggerCartAbandoned, cartID.String(), strconv.FormatInt(lastActivity.Unix(), 10)),
		channelID, customerID, to, map[string]any{signalsapi.PayloadCartID: cartID.String()})
	return err
}

// The browsed product rides the run so the recovery email can show it through {{ product.* }}
func (e *FlowEnroller) OnBrowseAbandoned(ctx context.Context, channelID, customerID uuid.UUID, pageKey, sku, name, image string, lastBrowseAt time.Time) error {
	_, err := e.enroll(ctx, signalsapi.TriggerBrowseAbandoned, moment(signalsapi.TriggerBrowseAbandoned, customerID.String(), strconv.FormatInt(lastBrowseAt.Unix(), 10)),
		channelID, customerID, "", map[string]any{
			signalsapi.PayloadPageKey: pageKey, signalsapi.PayloadSKU: sku, signalsapi.PayloadProductName: name, signalsapi.PayloadProductImage: image,
		})
	return err
}

// The notifier treats a zero as its cue to send the built-in notice - it keeps its own send-once record, so no key here
func (e *FlowEnroller) OnBackInStock(ctx context.Context, channelID, customerID uuid.UUID, sku, name, image, url string) int {
	return e.answered(ctx, signalsapi.TriggerBackInStock, "", channelID, customerID, "", map[string]any{
		signalsapi.PayloadSKU: sku, signalsapi.PayloadProductName: name, signalsapi.PayloadProductImage: image, signalsapi.PayloadProductURL: url,
	})
}

// Two submissions of the same form are two moments
func (e *FlowEnroller) OnFormSubmitted(ctx context.Context, channelID, customerID uuid.UUID, formSlug string) int {
	return e.answered(ctx, signalsapi.TriggerFormSubmitted, "", channelID, customerID, "", map[string]any{signalsapi.PayloadFormSlug: formSlug})
}

// The consent kind a marketing mail's unsubscribe link opts out of, as consent names it
const marketingConsentKind = "marketing_email"
