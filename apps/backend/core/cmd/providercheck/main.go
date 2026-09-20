// Command providercheck exercises a remote-provider endpoint against the contract core actually enforces - the signed envelope, the response ceiling, the timeout budget and the rejection cases - so an implementer finds out here rather than in production It speaks only HTTP: point it at any language's implementation
package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/skeletg/skelet/apps/backend/core/internal/remoteprovider"
	"github.com/skeletg/skelet/apps/backend/core/internal/remoteprovider/contract"
)

// Mirrors the ceiling in remoteprovider.Invoker - a longer body is truncated by the
// LimitReader and then fails to decode, which reads as a schema bug rather than a size one
const maxResponseBodyBytes = 256 * 1024

type result struct {
	name   string
	status string // PASS | FAIL | WARN | SKIP
	detail string
}

func main() {
	var (
		target   = flag.String("url", "", "provider endpoint (required)")
		secret   = flag.String("secret", "", "signing secret, the rpsec_ value shown once at registration (required)")
		kind     = flag.String("kind", "", "provider kind, e.g. tax_provider (required)")
		slug     = flag.String("slug", "conformance", "provider slug sent in the envelope")
		payload  = flag.String("payload", "", "request payload JSON file (defaults to the built-in example for --kind)")
		config   = flag.String("config", "", "provider config JSON file, forwarded verbatim as envelope.config")
		timeout  = flag.Int("timeout-ms", 5000, "the timeout this provider is registered with")
		examples = flag.String("examples", "", "override directory of <kind>.request.json examples (default: the generated ones baked into this binary)")
	)
	flag.Parse()

	if *target == "" || *secret == "" || *kind == "" {
		fmt.Fprintln(os.Stderr, "usage: provider-check --url URL --secret SECRET --kind KIND [--payload FILE]")
		flag.PrintDefaults()
		os.Exit(2)
	}
	if !remoteprovider.Kind(*kind).Valid() {
		fmt.Fprintf(os.Stderr, "unknown kind %q\nvalid kinds: %s\n", *kind, kindList())
		os.Exit(2)
	}

	payloadJSON, err := loadPayload(*payload, *examples, *kind)
	if err != nil {
		fmt.Fprintf(os.Stderr, "payload: %v\n", err)
		os.Exit(2)
	}
	var configJSON json.RawMessage
	if *config != "" {
		b, err := os.ReadFile(*config)
		if err != nil {
			fmt.Fprintf(os.Stderr, "config: %v\n", err)
			os.Exit(2)
		}
		configJSON = json.RawMessage(b)
	}

	c := &checker{
		url: *target, secret: *secret, kind: *kind, slug: *slug,
		payload: payloadJSON, config: configJSON,
		budget: time.Duration(*timeout) * time.Millisecond,
		client: &http.Client{Timeout: 30 * time.Second},
	}

	results := []result{
		c.happyPath(),
		c.rejectsTamperedSignature(),
		c.rejectsMissingSignature(),
		c.rejectsForeignSecret(),
		c.staleTimestamp(),
		c.withinTimeoutBudget(),
		c.responseUnderCeiling(),
		c.repeatPayloadIsStable(),
	}

	fmt.Printf("\nprovider-check  %s  kind=%s  contract=v%s\n\n", c.url, c.kind, contract.Version)
	fails, warns := 0, 0
	for _, r := range results {
		switch r.status {
		case "FAIL":
			fails++
		case "WARN":
			warns++
		}
		fmt.Printf("  %-6s %-32s %s\n", r.status, r.name, r.detail)
	}
	fmt.Printf("\n%d passed, %d failed, %d warnings\n", len(results)-fails-warns, fails, warns)
	if fails > 0 {
		os.Exit(1)
	}
}

func kindList() string {
	out := make([]string, 0, len(remoteprovider.AllKinds))
	for _, k := range remoteprovider.AllKinds {
		out = append(out, string(k))
	}
	return strings.Join(out, ", ")
}

// The generated example doubles as the default payload, so a provider author needs no fixture of their own to get a
// first answer - baked in, since the checker runs inside the backend container where the repo's docs tree isn't mounted
func loadPayload(explicit, examplesDir, kind string) (json.RawMessage, error) {
	var (
		b    []byte
		err  error
		from string
	)
	switch {
	case explicit != "":
		from = explicit
		b, err = os.ReadFile(explicit)
	case examplesDir != "":
		from = filepath.Join(examplesDir, kind+".request.json")
		b, err = os.ReadFile(from)
	default:
		from = "built-in example for " + kind
		b, err = contract.ExampleRequest(kind)
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", from, err)
	}
	if !json.Valid(b) {
		return nil, fmt.Errorf("%s is not valid JSON", from)
	}
	return json.RawMessage(b), nil
}

type checker struct {
	url, secret, kind, slug string
	payload, config         json.RawMessage
	budget                  time.Duration
	client                  *http.Client
}

// The envelope core sends - field order and names must match remoteprovider.invocationEnvelope
type envelope struct {
	Kind            string          `json:"kind"`
	Provider        string          `json:"provider"`
	ContractVersion string          `json:"contractVersion"`
	Timestamp       string          `json:"timestamp"`
	Config          json.RawMessage `json:"config,omitempty"`
	Payload         json.RawMessage `json:"payload"`
}

func (c *checker) body() ([]byte, error) {
	return json.Marshal(envelope{
		Kind:            c.kind,
		Provider:        c.slug,
		ContractVersion: contract.Version,
		Timestamp:       time.Now().UTC().Format(time.RFC3339),
		Config:          c.config,
		Payload:         c.payload,
	})
}

func sign(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

type sendOpts struct {
	signWith  string // secret used for the signature; "" leaves the header off entirely
	timestamp string // header timestamp; "" means now
	corrupt   bool   // flip the signature after computing it
	timeout   time.Duration
}

type sendResult struct {
	status  int
	body    []byte
	elapsed time.Duration
	err     error
}

func (c *checker) send(o sendOpts) sendResult {
	body, err := c.body()
	if err != nil {
		return sendResult{err: err}
	}
	ts := o.timestamp
	if ts == "" {
		ts = strconv.FormatInt(time.Now().Unix(), 10)
	}
	to := o.timeout
	if to == 0 {
		to = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), to)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return sendResult{err: err}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "RemoteProvider/1.0")
	req.Header.Set("X-Provider-Kind", c.kind)
	req.Header.Set("X-Provider-Contract-Version", contract.Version)
	req.Header.Set("X-Provider-Slug", c.slug)
	req.Header.Set("X-Provider-Timestamp", ts)
	if o.signWith != "" {
		sig := sign(o.signWith, ts, body)
		if o.corrupt {
			sig = flipLastHexDigit(sig)
		}
		req.Header.Set("X-Provider-Signature-256", "sha256="+sig)
	}

	start := time.Now()
	resp, err := c.client.Do(req)
	elapsed := time.Since(start)
	if err != nil {
		return sendResult{elapsed: elapsed, err: err}
	}
	defer resp.Body.Close()
	// Deliberately reads past the ceiling so responseUnderCeiling can see the real size
	rb, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes*4))
	return sendResult{status: resp.StatusCode, body: rb, elapsed: elapsed}
}

func flipLastHexDigit(s string) string {
	if s == "" {
		return s
	}
	last := s[len(s)-1]
	if last == '0' {
		return s[:len(s)-1] + "1"
	}
	return s[:len(s)-1] + "0"
}

func (c *checker) happyPath() result {
	r := c.send(sendOpts{signWith: c.secret})
	switch {
	case r.err != nil:
		return result{"valid request", "FAIL", "request failed: " + r.err.Error()}
	case r.status < 200 || r.status >= 300:
		return result{"valid request", "FAIL", fmt.Sprintf("HTTP %d - core treats any non-2xx as an invocation failure", r.status)}
	case len(bytes.TrimSpace(r.body)) == 0:
		return result{"valid request", "FAIL", "empty body - core decodes the response as JSON"}
	case !json.Valid(r.body):
		return result{"valid request", "FAIL", "body is not valid JSON"}
	}
	return result{"valid request", "PASS", fmt.Sprintf("HTTP %d, %d bytes, %s", r.status, len(r.body), r.elapsed.Round(time.Millisecond))}
}

// Every rejection case below is about the provider's own verification - core signs
// correctly, so the only way these pass is if the receiver actually checks
func (c *checker) rejectsTamperedSignature() result {
	r := c.send(sendOpts{signWith: c.secret, corrupt: true})
	if r.err != nil {
		return result{"rejects bad signature", "WARN", "request failed: " + r.err.Error()}
	}
	if r.status >= 200 && r.status < 300 {
		return result{"rejects bad signature", "FAIL", fmt.Sprintf("HTTP %d - a forged signature was accepted, anyone who learns the URL can drive this provider", r.status)}
	}
	return result{"rejects bad signature", "PASS", fmt.Sprintf("HTTP %d", r.status)}
}

func (c *checker) rejectsMissingSignature() result {
	r := c.send(sendOpts{})
	if r.err != nil {
		return result{"rejects unsigned request", "WARN", "request failed: " + r.err.Error()}
	}
	if r.status >= 200 && r.status < 300 {
		return result{"rejects unsigned request", "FAIL", fmt.Sprintf("HTTP %d - an unsigned request was accepted", r.status)}
	}
	return result{"rejects unsigned request", "PASS", fmt.Sprintf("HTTP %d", r.status)}
}

func (c *checker) rejectsForeignSecret() result {
	r := c.send(sendOpts{signWith: c.secret + "-wrong"})
	if r.err != nil {
		return result{"rejects wrong secret", "WARN", "request failed: " + r.err.Error()}
	}
	if r.status >= 200 && r.status < 300 {
		return result{"rejects wrong secret", "FAIL", fmt.Sprintf("HTTP %d - a well-formed signature from the wrong key was accepted", r.status)}
	}
	return result{"rejects wrong secret", "PASS", fmt.Sprintf("HTTP %d", r.status)}
}

// Core does not cap the age itself, so this is advisory - a receiver that accepts an
// hour-old timestamp is replayable for an hour by anyone who captured one request
func (c *checker) staleTimestamp() result {
	old := strconv.FormatInt(time.Now().Add(-1*time.Hour).Unix(), 10)
	r := c.send(sendOpts{signWith: c.secret, timestamp: old})
	if r.err != nil {
		return result{"stale timestamp", "WARN", "request failed: " + r.err.Error()}
	}
	if r.status >= 200 && r.status < 300 {
		return result{"stale timestamp", "WARN", "a 1-hour-old signed request was accepted - consider rejecting beyond a few minutes to bound replay"}
	}
	return result{"stale timestamp", "PASS", fmt.Sprintf("HTTP %d, replay window bounded", r.status)}
}

func (c *checker) withinTimeoutBudget() result {
	r := c.send(sendOpts{signWith: c.secret, timeout: c.budget})
	if r.err != nil {
		if errors.Is(r.err, context.DeadlineExceeded) {
			return result{"within timeout budget", "FAIL", fmt.Sprintf("exceeded %s - core abandons the call and applies the failure policy", c.budget)}
		}
		return result{"within timeout budget", "WARN", "request failed: " + r.err.Error()}
	}
	if r.elapsed > c.budget*8/10 {
		return result{"within timeout budget", "WARN", fmt.Sprintf("%s of a %s budget - little headroom under load", r.elapsed.Round(time.Millisecond), c.budget)}
	}
	return result{"within timeout budget", "PASS", fmt.Sprintf("%s of %s", r.elapsed.Round(time.Millisecond), c.budget)}
}

func (c *checker) responseUnderCeiling() result {
	r := c.send(sendOpts{signWith: c.secret})
	if r.err != nil {
		return result{"response under 256 KiB", "WARN", "request failed: " + r.err.Error()}
	}
	if len(r.body) > maxResponseBodyBytes {
		return result{"response under 256 KiB", "FAIL", fmt.Sprintf("%d bytes - core truncates at %d and the decode then fails", len(r.body), maxResponseBodyBytes)}
	}
	return result{"response under 256 KiB", "PASS", fmt.Sprintf("%d bytes", len(r.body))}
}

// Core caches on a hash of the payload alone, so the same payload may not reach the provider again for the whole TTL -
// an answer that changes between identical calls is one the merchant will see go stale
func (c *checker) repeatPayloadIsStable() result {
	a := c.send(sendOpts{signWith: c.secret})
	b := c.send(sendOpts{signWith: c.secret})
	if a.err != nil || b.err != nil {
		return result{"identical payload stable", "WARN", "one of the two calls failed"}
	}
	if !bytes.Equal(a.body, b.body) {
		return result{"identical payload stable", "WARN", "same payload returned different bodies - with a cache TTL set, core serves the first answer for the whole window"}
	}
	return result{"identical payload stable", "PASS", "same payload, same answer"}
}
