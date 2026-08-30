// Package httpx is the outbound HTTP client for gateway plugins - request building, auth, body
// caps, error shaping and idempotency in one place
//
// It exists because six independent gateway plugins hand-rolled the same loop and all six
// hand-rolled the same two defects into it: a discarded body-read error that turned torn
// responses into phantom successful captures, and a forgotten dedupe header that let a retried
// capture move money twice. The idempotency key is a positional parameter here so forgetting
// it is a compile error, not a production incident
package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultMaxBody = 1 << 20 // gateways answer in KB - a 1MiB cap keeps a misbehaving one from ballooning memory

// Client calls one gateway's API. The zero value is not usable - set BaseURL and Name
type Client struct {
	// Gateway API root, e.g. "https://api.stripe.com" - Do prefixes it onto every path
	BaseURL string
	// Error-message prefix and metric label, e.g. "stripe"
	Name string
	// Authorize stamps credentials onto each request - Basic auth, Bearer token, custom headers
	Authorize func(*http.Request)
	// IdempotencyHeader names the gateway's dedupe header (e.g. "Idempotency-Key",
	// "PayPal-Request-Id", "Klarna-Idempotency-Key") - empty means the gateway has none and
	// idemKey arguments are dropped
	IdempotencyHeader string
	// HTTP overrides the transport - nil uses a client with a 20s ceiling; per-call deadlines
	// come from ctx, which the host caps per hook anyway
	HTTP *http.Client
	// MaxBodyBytes caps response reads - 0 means 1MiB
	MaxBodyBytes int64
}

// HTTPError is any non-2xx answer - Body is capped raw bytes so callers can decode the
// gateway's own error envelope for a friendlier message
type HTTPError struct {
	Name   string
	Method string
	Path   string
	Status int
	Body   []byte
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("%s: %s %s failed (%d): %s", e.Name, e.Method, e.Path, e.Status, Truncate(e.Body))
}

// DoJSON sends payload as a JSON body (nil for none) and decodes a 2xx response into out (nil to
// discard). idemKey rides the client's IdempotencyHeader - pass "" only for calls that are safe
// to repeat, reads and creates the caller dedupes some other way
func (c *Client) DoJSON(ctx context.Context, method, path string, payload, out any, idemKey string) ([]byte, error) {
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("%s: marshal %s body: %w", c.Name, path, err)
		}
		body = bytes.NewReader(raw)
	}
	return c.do(ctx, method, path, body, "application/json", out, idemKey)
}

// DoForm is DoJSON for form-encoded APIs (Stripe)
func (c *Client) DoForm(ctx context.Context, method, path string, form url.Values, out any, idemKey string) ([]byte, error) {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	return c.do(ctx, method, path, body, "application/x-www-form-urlencoded", out, idemKey)
}

func (c *Client) do(ctx context.Context, method, path string, body io.Reader, contentType string, out any, idemKey string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("%s: build %s %s: %w", c.Name, method, path, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("Accept", "application/json")
	if idemKey != "" && c.IdempotencyHeader != "" {
		req.Header.Set(c.IdempotencyHeader, idemKey)
	}
	if c.Authorize != nil {
		c.Authorize(req)
	}

	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	res, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %s %s: %w", c.Name, method, path, err)
	}
	defer res.Body.Close()

	maxBody := c.MaxBodyBytes
	if maxBody <= 0 {
		maxBody = defaultMaxBody
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxBody))
	if err != nil {
		// a torn body on a 2xx must never decode into a zero value the caller records as success
		return nil, fmt.Errorf("%s: %s %s: read response: %w", c.Name, method, path, err)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return raw, &HTTPError{Name: c.Name, Method: method, Path: path, Status: res.StatusCode, Body: raw}
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return raw, fmt.Errorf("%s: decode %s: %w", c.Name, path, err)
		}
	}
	return raw, nil
}

// Truncate bounds a response body for error messages - the seventh byte-identical copy of this,
// hoisted from the six plugins that each carried one
func Truncate(b []byte) string {
	s := string(b)
	if len(s) > 400 {
		return s[:400] + "…"
	}
	return s
}

// IsStatus reports whether err is an HTTPError with the given status - the common "404 means
// not-found, everything else is real" branch gateways need
func IsStatus(err error, status int) bool {
	var he *HTTPError
	return errors.As(err, &he) && he.Status == status
}

// NoRetry documents the deliberate absence: there is no automatic retry here, because retrying
// without a gateway-honored dedupe key is how money moves twice - callers that hold an idemKey
// may loop themselves, and core's retry ladders (webhook queue, scheduler) own the pacing
