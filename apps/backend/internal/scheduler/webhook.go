package scheduler

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// WebhookConfig is the persisted payload for a kind=webhook job - Exported so
// the admin resolver can build/validate it
type WebhookConfig struct {
	URL     string            `json:"url"`
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    string            `json:"body,omitempty"`
	// Secret signs the body with HMAC-SHA256 (header X-Job-Signature-256) so the
	// receiver can verify the call came from skelet - Optional
	Secret string `json:"secret,omitempty"`
}

type webhookRunner struct {
	client       *http.Client
	allowPrivate bool
}

func newWebhookRunner(allowPrivate bool) *webhookRunner {
	r := &webhookRunner{allowPrivate: allowPrivate}
	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
		// Control runs once per RESOLVED address before connecting, so it closes the DNS-rebinding gap a create-time URL check can't - even when a hostname later resolves to an internal IP, the dial is refused here
		Control: func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				host = address
			}
			if ip := net.ParseIP(host); ip != nil && ipBlocked(ip, r.allowPrivate) {
				return fmt.Errorf("blocked address %s (private/loopback/link-local)", host)
			}
			return nil
		},
	}
	r.client = &http.Client{
		Timeout:   30 * time.Second,
		Transport: &http.Transport{DialContext: dialer.DialContext},
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return fmt.Errorf("stopped after 3 redirects")
			}
			return nil
		},
	}
	return r
}

func (r *webhookRunner) run(ctx context.Context, payload []byte) error {
	var cfg WebhookConfig
	if err := json.Unmarshal(payload, &cfg); err != nil {
		return fmt.Errorf("decode webhook config: %w", err)
	}
	if err := ValidateWebhookURL(cfg.URL, r.allowPrivate); err != nil {
		return err
	}
	method := strings.ToUpper(strings.TrimSpace(cfg.Method))
	if method == "" {
		method = http.MethodPost
	}
	req, err := http.NewRequestWithContext(ctx, method, cfg.URL, strings.NewReader(cfg.Body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Scheduler/1.0")
	for k, v := range cfg.Headers {
		req.Header.Set(k, v)
	}
	if cfg.Secret != "" {
		mac := hmac.New(sha256.New, []byte(cfg.Secret))
		_, _ = mac.Write([]byte(cfg.Body))
		req.Header.Set("X-Job-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook request: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned HTTP %d", resp.StatusCode)
	}
	return nil
}

// Enforces an http(s) scheme and, unless allowPrivate, that no resolved IP is private, loopback or link-local - used both as create-time validation and inside run()
// The connect-time Dialer.Control is the authoritative guard against rebinding; this just gives the operator early, readable feedback
func ValidateWebhookURL(raw string, allowPrivate bool) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("webhook url must be http or https")
	}
	if u.Hostname() == "" {
		return fmt.Errorf("webhook url must have a host")
	}
	host := u.Hostname()
	if ip := net.ParseIP(host); ip != nil {
		if ipBlocked(ip, allowPrivate) {
			return fmt.Errorf("webhook url host %s is a blocked address", host)
		}
		return nil
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		// Can't resolve right now, from transient DNS or an unknown host - don't reject on that, since the connect-time Dialer.Control is authoritative and refuses a blocked IP at dial time regardless
		return nil
	}
	for _, ip := range ips {
		if ipBlocked(ip, allowPrivate) {
			return fmt.Errorf("webhook url host %s resolves to a blocked address", host)
		}
	}
	return nil
}

// Whether an IP may not be a webhook target - link-local, which includes the 169.254.169.254 cloud-metadata endpoint, link-local multicast and the unspecified address are ALWAYS blocked even with allowPrivate
// allowPrivate only relaxes loopback and RFC1918 ranges, so a dev can point a job at a local test server
func ipBlocked(ip net.IP, allowPrivate bool) bool {
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return true
	}
	if allowPrivate {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate()
}
