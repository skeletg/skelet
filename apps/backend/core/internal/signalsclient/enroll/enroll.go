// Package enroll is how core's domains hand a moment to the flow engine - each becomes an Enroll over the contract
package enroll

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/signalsclient"
	"github.com/skeletg/skelet/apps/backend/core/lib/email"
	"github.com/skeletg/skelet/apps/backend/core/signalsapi"
)

// Moments are keyed by what they happened to, so a retry, a relayed handler or a re-listed cart starts one run
type Enroller struct{ client *signalsclient.Client }

func New(c *signalsclient.Client) *Enroller { return &Enroller{client: c} }

func (e *Enroller) enroll(ctx context.Context, trigger, key string, channelID, customerID uuid.UUID, to string, payload map[string]any) (int, error) {
	return e.client.Enroll(ctx, signalsclient.Enrollment{
		Trigger: trigger, ChannelID: channelID, CustomerID: customerID, Email: to, Payload: payload, Key: key,
	})
}

// Moments that must reach the shopper now send core's own mail on an unknown answer, unless some flow took them
func (e *Enroller) answered(ctx context.Context, trigger, key string, channelID, customerID uuid.UUID, to string, payload map[string]any) int {
	n, err := e.enroll(ctx, trigger, key, channelID, customerID, to, payload)
	switch {
	case err != nil && n == 0:
		log.Printf("[flows] %s: %v - core sends its own mail", trigger, err)
	case err != nil:
		log.Printf("[flows] %s: %v", trigger, err)
	}
	return n
}

// The lifecycle events that start a run - core's events, so subscribed here and not by the engine
// customer.registered rides the outbox, so a suite that was down is asked again on the relay's next pass
func (e *Enroller) Subscribe(bus *events.Bus) {
	bus.Subscribe(customer.EventRegistered, func(ctx context.Context, ev events.Event) error {
		evt := ev.(*customer.CustomerRegistered)
		return e.OnCustomerRegistered(ctx, evt.ChannelID, evt.CustomerID)
	})
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

func (e *Enroller) OnOrderPlaced(ctx context.Context, channelID, customerID uuid.UUID, to string, orderID uuid.UUID, snap email.OrderSnapshot) int {
	return e.answered(ctx, signalsapi.TriggerOrderPlaced, moment(signalsapi.TriggerOrderPlaced, orderID.String()),
		channelID, customerID, to, signalsclient.OrderPayload(orderID, uuid.Nil, snap))
}

func (e *Enroller) OnOrderShipped(ctx context.Context, channelID, customerID uuid.UUID, to string, orderID, fulfillmentID uuid.UUID, snap email.OrderSnapshot) int {
	shipment := fulfillmentID
	if shipment == uuid.Nil {
		shipment = orderID
	}
	return e.answered(ctx, signalsapi.TriggerOrderShipped, moment(signalsapi.TriggerOrderShipped, shipment.String()),
		channelID, customerID, to, signalsclient.OrderPayload(orderID, fulfillmentID, snap))
}

func (e *Enroller) OnOrderCancelled(ctx context.Context, channelID, customerID uuid.UUID, to string, orderID uuid.UUID, snap email.OrderSnapshot) int {
	return e.answered(ctx, signalsapi.TriggerOrderCancelled, moment(signalsapi.TriggerOrderCancelled, orderID.String()),
		channelID, customerID, to, signalsclient.OrderPayload(orderID, uuid.Nil, snap))
}

// ref is the dedup reference (refund txn id or status) carried downstream
func (e *Enroller) OnPaymentRefunded(ctx context.Context, channelID, customerID uuid.UUID, to string, orderID uuid.UUID, ref string, snap email.OrderSnapshot) int {
	p := signalsclient.OrderPayload(orderID, uuid.Nil, snap)
	p[signalsapi.PayloadRefundRef] = ref
	return e.answered(ctx, signalsapi.TriggerPaymentRefunded, moment(signalsapi.TriggerPaymentRefunded, orderID.String(), ref),
		channelID, customerID, to, p)
}

// --- customer's account mail: the suite runs these inside the call, the one-time link has to go out now ---

func (e *Enroller) OnEmailVerification(ctx context.Context, channelID, customerID uuid.UUID, verifyLink, storeName, firstName string) int {
	return e.answered(ctx, signalsapi.TriggerEmailVerification, moment(signalsapi.TriggerEmailVerification, customerID.String(), linkDigest(verifyLink)),
		channelID, customerID, "", map[string]any{
			signalsapi.PayloadVerifyLink: verifyLink, signalsapi.PayloadStoreName: storeName, signalsapi.PayloadFirstName: firstName,
		})
}

func (e *Enroller) OnPasswordReset(ctx context.Context, channelID, customerID uuid.UUID, resetLink, expiresIn, storeName, firstName string) int {
	return e.answered(ctx, signalsapi.TriggerPasswordReset, moment(signalsapi.TriggerPasswordReset, customerID.String(), linkDigest(resetLink)),
		channelID, customerID, "", map[string]any{
			signalsapi.PayloadResetLink: resetLink, signalsapi.PayloadResetExpiresIn: expiresIn,
			signalsapi.PayloadStoreName: storeName, signalsapi.PayloadFirstName: firstName,
		})
}

// --- the marketing moments core's domains and jobs raise ---
// No mail of core's to fall back on, so an unknown answer comes back as the error and the relay or the next tick asks again

// One registration per account, ever
func (e *Enroller) OnCustomerRegistered(ctx context.Context, channelID, customerID uuid.UUID) error {
	_, err := e.enroll(ctx, signalsapi.TriggerCustomerRegistered, moment(signalsapi.TriggerCustomerRegistered, customerID.String()), channelID, customerID, "", nil)
	return err
}

// A guest cart enrolls keyed by the email captured at checkout entry, so guest carts recover too
// lastActivity names the episode - a cart touched and left again is a new moment
func (e *Enroller) OnCartAbandoned(ctx context.Context, channelID, customerID uuid.UUID, to string, cartID uuid.UUID, lastActivity time.Time) error {
	_, err := e.enroll(ctx, signalsapi.TriggerCartAbandoned, moment(signalsapi.TriggerCartAbandoned, cartID.String(), strconv.FormatInt(lastActivity.Unix(), 10)),
		channelID, customerID, to, map[string]any{signalsapi.PayloadCartID: cartID.String()})
	return err
}

// The browsed product rides the run so the recovery email can show it through {{ product.* }}
func (e *Enroller) OnBrowseAbandoned(ctx context.Context, channelID, customerID uuid.UUID, pageKey, sku, name, image string, lastBrowseAt time.Time) error {
	_, err := e.enroll(ctx, signalsapi.TriggerBrowseAbandoned, moment(signalsapi.TriggerBrowseAbandoned, customerID.String(), strconv.FormatInt(lastBrowseAt.Unix(), 10)),
		channelID, customerID, "", map[string]any{
			signalsapi.PayloadPageKey: pageKey, signalsapi.PayloadSKU: sku, signalsapi.PayloadProductName: name, signalsapi.PayloadProductImage: image,
		})
	return err
}

// The notifier treats a zero as its cue to send the built-in notice - it keeps its own send-once record, so no key here
func (e *Enroller) OnBackInStock(ctx context.Context, channelID, customerID uuid.UUID, sku, name, image, url string) int {
	return e.answered(ctx, signalsapi.TriggerBackInStock, "", channelID, customerID, "", map[string]any{
		signalsapi.PayloadSKU: sku, signalsapi.PayloadProductName: name, signalsapi.PayloadProductImage: image, signalsapi.PayloadProductURL: url,
	})
}

// Two submissions of the same form are two moments
func (e *Enroller) OnFormSubmitted(ctx context.Context, channelID, customerID uuid.UUID, formSlug string) int {
	return e.answered(ctx, signalsapi.TriggerFormSubmitted, "", channelID, customerID, "", map[string]any{signalsapi.PayloadFormSlug: formSlug})
}
