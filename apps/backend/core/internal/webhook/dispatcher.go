package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
)

const (
	maxResponseBodyBytes = 4096
	asyncHTTPTimeout     = 10 * time.Second
	syncHTTPTimeout      = 5 * time.Second
	// A hard client-level ceiling - per-attempt deadlines come from the request context, so this only backstops response-body reads that outlive the ctx deadline
	maxHTTPTimeout = time.Duration(MaxTimeoutMs)*time.Millisecond + 5*time.Second
)

// ASYNC webhooks only - sync ones never retry, since the user-facing request is waiting and retry delays would turn a transient failure into a visible hang
var retryDelays = []time.Duration{10 * time.Second, 60 * time.Second, 300 * time.Second}

type webhookPayload struct {
	APIVersion string `json:"apiVersion"`
	Event      string `json:"event"`
	Timestamp  string `json:"timestamp"`
	Data       any    `json:"data"`
}

type Dispatcher struct {
	svc    *Service
	client *http.Client
}

// transport is the base RoundTripper, nil meaning http.DefaultTransport - main.go passes otelx.HTTPTransport(nil) so outbound webhooks propagate the trace via traceparent
func NewDispatcher(svc *Service, transport http.RoundTripper) *Dispatcher {
	if transport == nil {
		transport = http.DefaultTransport
	}
	return &Dispatcher{
		svc:    svc,
		client: &http.Client{Timeout: maxHTTPTimeout, Transport: transport},
	}
}

// The webhook's own timeout_ms when set, otherwise the kind default passed in
func effectiveTimeout(wh Webhook, kindDefault time.Duration) time.Duration {
	if wh.TimeoutMs <= 0 {
		return kindDefault
	}
	d := time.Duration(wh.TimeoutMs) * time.Millisecond
	if d > time.Duration(MaxTimeoutMs)*time.Millisecond {
		return time.Duration(MaxTimeoutMs) * time.Millisecond
	}
	return d
}

// One of these is subscribed per KindAsync event type at boot - it runs detached from the request ctx, with panics recovered by the bus; a returned error makes the outbox replay the row (deliveries are at-least-once anyway)
func (d *Dispatcher) Handler() events.AsyncHandler {
	return func(ctx context.Context, e events.Event) error {
		return d.dispatchAsync(ctx, e)
	}
}

// One of these is subscribed per KindSync event type at boot - it runs on the caller's goroutine inside the emitting service's request, and a non-nil error aborts the rest of the sync chain AND the emitting operation
func (d *Dispatcher) SyncHandler() events.SyncHandler {
	return func(ctx context.Context, e events.Event) error {
		return d.dispatchSync(ctx, e)
	}
}

// --- async path ---

// Enqueues one durable webhook_queue row per matching webhook - the QueueWorker owns delivery, the retry ladder and dead-lettering
// From here the delivery survives restarts, leaving only a crash between the emitting commit and this enqueue as a loss window - closed for outbox-routed events, best-effort for the rest
func (d *Dispatcher) dispatchAsync(ctx context.Context, e events.Event) error {
	webhooks, err := d.svc.ListActiveByEvent(ctx, e.Type(), KindAsync, eventStoreID(e))
	if err != nil {
		log.Printf("[webhook] list active async webhooks for %s: %v", e.Type(), err)
		return err
	}
	if len(webhooks) == 0 {
		return nil
	}

	bodies := newPayloadCache(e)
	bodies.at = events.EmittedAtOr(ctx, time.Now())
	dedupeKey := replayDedupeKey(ctx)
	var errs []error
	for _, wh := range webhooks {
		body, err := bodies.forVersion(wh.PayloadVersion)
		if err != nil {
			log.Printf("[webhook] marshal payload for %s: %v", e.Type(), err) // this version can't serialize, but the other webhooks' versions still might
			continue
		}
		if _, err := d.svc.EnqueueDelivery(ctx, wh.ID, e.Type(), string(body), dedupeKey); err != nil {
			log.Printf("[webhook] enqueue %s for %s: %v", e.Type(), wh.ID, err)
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// The outbox row the event came from, shared by its replays - empty on the fire-and-forget path, which never replays
func replayDedupeKey(ctx context.Context) string {
	id, ok := events.OutboxID(ctx)
	if !ok {
		return ""
	}
	return "outbox:" + id.String()
}

// --- sync path ---

func (d *Dispatcher) dispatchSync(ctx context.Context, e events.Event) error {
	webhooks, err := d.svc.ListActiveByEvent(ctx, e.Type(), KindSync, eventStoreID(e))
	if err != nil {
		return fmt.Errorf("list sync webhooks: %w", err)
	}
	if len(webhooks) == 0 {
		return nil
	}

	bodies := newPayloadCache(e)
	applier, eventImplementsApplier := e.(SyncResponseApplier)

	for _, wh := range webhooks {
		if err := d.deliverSyncOne(ctx, wh, e, bodies, applier, eventImplementsApplier); err != nil {
			return err
		}
	}
	return nil
}

// Runs a single sync webhook, honoring its failure policy - fail_closed turns any delivery failure into an abort of the emitting operation, fail_open logs and moves on
// An explicit {"reject": true} aborts under both, since fail_open forgives broken endpoints, not deliberate vetoes
func (d *Dispatcher) deliverSyncOne(ctx context.Context, wh Webhook, e events.Event, bodies *payloadCache, applier SyncResponseApplier, hasApplier bool) error {
	failOpen := wh.FailurePolicy == FailOpen

	body, err := bodies.forVersion(wh.PayloadVersion)
	if err != nil {
		// A marshal failure is a programming error in the event struct, not an endpoint problem - no policy leniency
		return fmt.Errorf("marshal payload for %s: %w", e.Type(), err)
	}
	secret, err := d.svc.GetSigningSecret(ctx, wh.ID)
	if err != nil {
		if failOpen {
			log.Printf("[webhook] get secret for %s: %v (fail_open, skipping)", wh.ID, err)
			return nil
		}
		return fmt.Errorf("get secret for webhook %s: %w", wh.Name, err)
	}

	// The circuit check comes after the cheap local failures, so a blocked probe can never get stuck half-open without a recorded outcome
	ticket, ok := circuits.Allow(wh.ID)
	if !ok {
		if failOpen {
			log.Printf("[webhook] circuit open, skipping sync webhook %q for %s", wh.Name, e.Type())
			return nil
		}
		return fmt.Errorf("webhook %q rejected: endpoint circuit is open after repeated failures - fix the endpoint or switch the webhook to fail_open", wh.Name)
	}

	delivery, respBody := d.attemptDelivery(ctx, wh, secret, body, e.Type(), 1, effectiveTimeout(wh, syncHTTPTimeout))
	// Record the delivery regardless of outcome so admins can debug
	if _, err := d.svc.RecordDelivery(ctx, delivery); err != nil {
		log.Printf("[webhook] record sync delivery for %s: %v", wh.ID, err)
	}
	ticket.Done(ctx, delivery.Success) // a caller walking away isn't the endpoint failing

	if !delivery.Success {
		reason := "delivery failed"
		if delivery.ErrorMessage != nil {
			reason = *delivery.ErrorMessage
		}
		if failOpen {
			log.Printf("[webhook] sync webhook %q failed for %s: %s (fail_open, continuing)", wh.Name, e.Type(), reason)
			return nil
		}
		return fmt.Errorf("webhook %q rejected: %s", wh.Name, reason)
	}

	// 2xx - hand the body to the event - Only events implementing the interface can be mutated or rejected; the rest treat sync delivery as a blocking notification
	if hasApplier && len(respBody) > 0 {
		reject, reason, err := applier.ApplyWebhookResponse(respBody)
		if err != nil {
			if failOpen {
				log.Printf("[webhook] sync webhook %q invalid response for %s: %v (fail_open, ignoring)", wh.Name, e.Type(), err)
				return nil
			}
			return fmt.Errorf("webhook %q invalid response: %w", wh.Name, err)
		}
		if reject {
			if reason == "" {
				reason = "no reason given"
			}
			return fmt.Errorf("webhook %q rejected: %s", wh.Name, reason)
		}
	}
	return nil
}

// --- shared HTTP path ---

// Runs one HTTP request against the webhook with the given per-attempt timeout, returning the Delivery record AND the raw response body, empty on a transport failure or non-2xx
// Callers decide the retry and apply-response semantics
func (d *Dispatcher) attemptDelivery(ctx context.Context, wh Webhook, secret string, body []byte, eventType string, attempt int32, timeout time.Duration) (Delivery, []byte) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// The timestamp rides inside the signed string, Stripe-style "<ts>.<body>", so receivers can reject replays outside their tolerance window - signing the body alone lets a captured delivery be replayed forever
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	signature := computeSignature(secret, timestamp, body)

	requestHeaders := map[string]string{
		"Content-Type":            "application/json",
		"User-Agent":              "Webhook/1.0",
		"X-Webhook-Event":         eventType,
		"X-Webhook-Timestamp":     timestamp,
		"X-Webhook-Signature-256": "sha256=" + signature,
		"X-Webhook-Api-Version":   wh.PayloadVersion,
	}
	// Merchant headers ride along but never shadow the dispatcher's own - write-time validation already rejects reserved names, so this just defends against rows written before that check existed
	for k, v := range wh.CustomHeaders {
		if _, exists := requestHeaders[http.CanonicalHeaderKey(k)]; exists {
			continue
		}
		requestHeaders[http.CanonicalHeaderKey(k)] = v
	}

	// The request carries ctx so shutdown and upstream deadlines can cancel an in-flight delivery, and so the OTel transport can read the span off it and emit traceparent - with a bare http.NewRequest the documented trace propagation never happened
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, wh.TargetURL, bytes.NewReader(body))
	if err != nil {
		errMsg := fmt.Sprintf("build request: %v", err)
		return Delivery{
			WebhookID:      wh.ID,
			EventType:      eventType,
			URL:            wh.TargetURL,
			RequestBody:    string(body),
			RequestHeaders: requestHeaders,
			Success:        false,
			ErrorMessage:   &errMsg,
			Attempt:        attempt,
		}, nil
	}
	for k, v := range requestHeaders {
		req.Header.Set(k, v)
	}

	start := time.Now()
	resp, err := d.client.Do(req)
	durationMs := int32(time.Since(start).Milliseconds())

	if err != nil {
		errMsg := classifyTransportError(err)
		return Delivery{
			WebhookID:      wh.ID,
			EventType:      eventType,
			URL:            wh.TargetURL,
			RequestBody:    string(body),
			RequestHeaders: requestHeaders,
			DurationMs:     durationMs,
			Success:        false,
			ErrorMessage:   &errMsg,
			Attempt:        attempt,
		}, nil
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes))
	respBodyStr := string(respBody)
	statusCode := int32(resp.StatusCode)
	success := resp.StatusCode >= 200 && resp.StatusCode < 300

	delivery := Delivery{
		WebhookID:       wh.ID,
		EventType:       eventType,
		URL:             wh.TargetURL,
		RequestBody:     string(body),
		RequestHeaders:  requestHeaders,
		ResponseStatus:  &statusCode,
		ResponseBody:    &respBodyStr,
		ResponseHeaders: flattenHeaders(resp.Header),
		DurationMs:      durationMs,
		Success:         success,
		Attempt:         attempt,
	}
	if !success {
		errMsg := fmt.Sprintf("HTTP %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
		delivery.ErrorMessage = &errMsg
	}
	return delivery, respBody
}

// Marshals the envelope once per payload version per event - today exactly once, since only v1 exists, but this is the seam a v2 envelope plugs into without touching the dispatch loops
type payloadCache struct {
	event  events.Event
	at     time.Time
	bodies map[string][]byte
}

func newPayloadCache(e events.Event) *payloadCache {
	return &payloadCache{event: e, at: time.Now(), bodies: map[string][]byte{}}
}

func (c *payloadCache) forVersion(version string) ([]byte, error) {
	if version == "" {
		version = PayloadVersionV1
	}
	if body, ok := c.bodies[version]; ok {
		return body, nil
	}
	body, err := json.Marshal(webhookPayload{
		APIVersion: version,
		Event:      c.event.Type(),
		Timestamp:  c.at.UTC().Format(time.RFC3339),
		Data:       c.event,
	})
	if err != nil {
		return nil, err
	}
	c.bodies[version] = body
	return body, nil
}

// Joins multi-value headers with ", " since a map[string]string is friendlier for the admin UI than Go's canonical http.Header shape
func flattenHeaders(h http.Header) map[string]string {
	if len(h) == 0 {
		return nil
	}
	out := make(map[string]string, len(h))
	for k, v := range h {
		if len(v) == 1 {
			out[k] = v[0]
		} else {
			out[k] = strings.Join(v, ", ")
		}
	}
	return out
}

// Turns net/http's wrapped errors into something an admin can skim - "connection refused", "TLS handshake failed" - rather than the raw "Post ...: dial tcp ..." chain
func classifyTransportError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "context deadline exceeded"), strings.Contains(lower, "timeout"):
		return fmt.Sprintf("timeout: %s", msg)
	case strings.Contains(lower, "no such host"), strings.Contains(lower, "dns"):
		return fmt.Sprintf("DNS lookup failed: %s", msg)
	case strings.Contains(lower, "connection refused"):
		return fmt.Sprintf("connection refused: %s", msg)
	case strings.Contains(lower, "tls"), strings.Contains(lower, "certificate"):
		return fmt.Sprintf("TLS error: %s", msg)
	case strings.Contains(lower, "eof"), strings.Contains(lower, "connection reset"):
		return fmt.Sprintf("connection closed: %s", msg)
	}
	return fmt.Sprintf("transport error: %s", msg)
}

// HMACs "<unix-ts>.<body>" - including the timestamp is what gives receivers a replay window to enforce, with 5 minutes the recommended tolerance
func computeSignature(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// The store affiliation of the event payload, or nil when the event type isn't store-scoped - type-asserts on ChannelScopedEvent, which events carrying a store implement and the rest don't
func eventStoreID(e events.Event) *uuid.UUID {
	scoped, ok := e.(ChannelScopedEvent)
	if !ok {
		return nil
	}
	id := scoped.EventChannelID()
	if id == (uuid.UUID{}) {
		return nil
	}
	return &id
}
