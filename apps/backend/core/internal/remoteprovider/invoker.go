package remoteprovider

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/internal/circuit"
	"github.com/skeletg/skelet/apps/backend/core/internal/remoteprovider/contract"
)

const (
	defaultTimeout       = 5 * time.Second
	maxResponseBodyBytes = 256 * 1024
	// hard client ceiling; per-invoke deadlines come from the request ctx
	maxHTTPTimeout = time.Duration(MaxTimeoutMs)*time.Millisecond + 5*time.Second
)

// Shared across all invokers in the process - one circuit per provider row, with the same semantics as the sync-webhook breaker
var breaker = circuit.New(circuit.DefaultThreshold, circuit.DefaultCooldown)

// CircuitInfoFor exposes breaker state to the admin API
func CircuitInfoFor(id uuid.UUID) circuit.Info {
	return breaker.Info(id)
}

// What every remote provider receives - Config is the provider row's JSON blob of API keys and options, and payload is the kind-specific request the consumer built
type invocationEnvelope struct {
	Kind     string `json:"kind"`
	Provider string `json:"provider"`
	// The wire-contract version this payload was built under - also sent as X-Provider-Contract-Version, and inside the signed body so nothing between core and the provider can strip it
	ContractVersion string          `json:"contractVersion"`
	Timestamp       string          `json:"timestamp"`
	Config          json.RawMessage `json:"config,omitempty"`
	Payload         any             `json:"payload"`
}

// Performs the HTTP round-trip for provider invocations - HMAC signing on the same Stripe-style scheme as webhooks, per-provider timeout, response caching and the circuit breaker
// Consumers own the payload and response schemas; Invoke just moves signed JSON
type Invoker struct {
	svc    *Service
	client *http.Client
}

// transport should be the SSRF-safe, OTel-wrapped RoundTripper from main.go - nil falls back to http.DefaultTransport, for tests
func NewInvoker(svc *Service, transport http.RoundTripper) *Invoker {
	if transport == nil {
		transport = http.DefaultTransport
	}
	return &Invoker{
		svc:    svc,
		client: &http.Client{Timeout: maxHTTPTimeout, Transport: transport},
	}
}

// POSTs the payload to the provider and decodes the JSON response into out Transport errors, timeouts, non-2xx, undecodable bodies and an open circuit all come back as errors, and the CALLER decides what the provider's failure policy means - fall back or abort Successful responses cache for the provider's CacheTTLs, keyed by the exact request payload
func (i *Invoker) InvokeUncached(ctx context.Context, p Provider, payload any, out any) error {
	p.CacheTTLs = 0
	return i.Invoke(ctx, p, payload, out)
}

func (i *Invoker) Invoke(ctx context.Context, p Provider, payload any, out any) error {
	body, err := json.Marshal(invocationEnvelope{
		Kind:            string(p.Kind),
		Provider:        p.Slug,
		ContractVersion: contract.Version,
		Timestamp:       time.Now().UTC().Format(time.RFC3339),
		Config:          p.Config,
		Payload:         payload,
	})
	if err != nil {
		return fmt.Errorf("marshal invocation: %w", err)
	}

	var cacheKey string
	if p.CacheTTLs > 0 {
		// cache key ignores the envelope timestamp - hash the payload alone
		payloadJSON, err := json.Marshal(payload)
		if err == nil {
			cacheKey = cacheKeyFor(p.ID, payloadJSON)
			if cached, ok := invocationCache.get(cacheKey); ok {
				return json.Unmarshal(cached, out)
			}
		}
	}

	ticket, ok := breaker.Allow(p.ID)
	if !ok {
		return fmt.Errorf("%w: %s/%s", ErrCircuitOpen, p.Kind, p.Slug)
	}

	respBody, err := i.roundTrip(ctx, p, body)
	ticket.Done(ctx, err == nil) // the per-call timeout is a child of ctx, so a slow provider still counts while a caller walking away doesn't
	if err != nil {
		return err
	}

	if out != nil {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("remote provider %s/%s: undecodable response: %w", p.Kind, p.Slug, err)
		}
	}
	if cacheKey != "" {
		invocationCache.put(cacheKey, p.ID, respBody, time.Duration(p.CacheTTLs)*time.Second)
	}
	return nil
}

func (i *Invoker) roundTrip(ctx context.Context, p Provider, body []byte) ([]byte, error) {
	timeout := defaultTimeout
	if p.TimeoutMs > 0 {
		timeout = time.Duration(p.TimeoutMs) * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	secret, err := i.svc.repo.GetSecret(ctx, p.ID)
	if err != nil {
		return nil, fmt.Errorf("remote provider %s/%s: secret unavailable: %w", p.Kind, p.Slug, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.TargetURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("remote provider %s/%s: build request: %w", p.Kind, p.Slug, err)
	}
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "RemoteProvider/1.0")
	req.Header.Set("X-Provider-Kind", string(p.Kind))
	req.Header.Set("X-Provider-Contract-Version", contract.Version)
	req.Header.Set("X-Provider-Slug", p.Slug)
	req.Header.Set("X-Provider-Timestamp", timestamp)
	req.Header.Set("X-Provider-Signature-256", "sha256="+computeSignature(secret, timestamp, body))

	resp, err := i.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("remote provider %s/%s: %w", p.Kind, p.Slug, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("remote provider %s/%s: HTTP %d", p.Kind, p.Slug, resp.StatusCode)
	}
	return respBody, nil
}

// HMACs "<unix-ts>.<body>" - the same scheme as webhook deliveries, so receivers verify both with one helper
func computeSignature(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// --- response cache ---

type cacheEntry struct {
	providerID uuid.UUID
	body       []byte
	expiresAt  time.Time
}

type responseCache struct {
	mu sync.Mutex
	m  map[string]cacheEntry
}

var invocationCache = &responseCache{m: map[string]cacheEntry{}}

func cacheKeyFor(id uuid.UUID, payload []byte) string {
	sum := sha256.Sum256(payload)
	return id.String() + ":" + hex.EncodeToString(sum[:])
}

func (c *responseCache) get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[key]
	if !ok || time.Now().After(e.expiresAt) {
		delete(c.m, key)
		return nil, false
	}
	return e.body, true
}

func (c *responseCache) put(key string, providerID uuid.UUID, body []byte, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// opportunistic sweep keeps the map from accreting dead entries on
	// low-traffic providers - cheap at these sizes
	if len(c.m) > 4096 {
		now := time.Now()
		for k, e := range c.m {
			if now.After(e.expiresAt) {
				delete(c.m, k)
			}
		}
	}
	c.m[key] = cacheEntry{providerID: providerID, body: body, expiresAt: time.Now().Add(ttl)}
}

// invalidate drops every cached response for one provider - config or URL
// edits must not serve stale answers for up to an hour
func (c *responseCache) invalidate(providerID uuid.UUID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, e := range c.m {
		if e.providerID == providerID {
			delete(c.m, k)
		}
	}
}
