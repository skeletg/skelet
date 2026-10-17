package scheduler

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/skeletg/skelet/apps/backend/core/lib/secrets"
	"github.com/skeletg/skelet/apps/backend/core/lib/urlguard"
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

var webhookMethods = []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete}

type webhookRunner struct {
	client       *http.Client
	allowPrivate bool
	cipher       *secrets.Cipher
}

func newWebhookRunner(allowPrivate bool) *webhookRunner {
	return &webhookRunner{
		allowPrivate: allowPrivate,
		client: &http.Client{
			Timeout:   30 * time.Second,
			Transport: urlguard.SafeTransport(allowPrivate),
			// A followed redirect carries the job's own headers and signature to whatever host it names
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

func (r *webhookRunner) seal(c WebhookConfig) (WebhookConfig, error) {
	return r.mapSecrets(c, func(v string) (string, error) {
		if r.cipher == nil || v == "" || secrets.IsEncrypted(v) {
			return v, nil
		}
		return r.cipher.Encrypt(v)
	})
}

func (r *webhookRunner) open(c WebhookConfig) (WebhookConfig, error) {
	return r.mapSecrets(c, func(v string) (string, error) {
		if !secrets.IsEncrypted(v) {
			return v, nil
		}
		if r.cipher == nil {
			return "", fmt.Errorf("webhook config is sealed but no encryption key is configured")
		}
		return r.cipher.Decrypt(v)
	})
}

func (r *webhookRunner) mapSecrets(c WebhookConfig, f func(string) (string, error)) (WebhookConfig, error) {
	var err error
	if c.Secret, err = f(c.Secret); err != nil {
		return WebhookConfig{}, err
	}
	if len(c.Headers) > 0 {
		headers := make(map[string]string, len(c.Headers))
		for k, v := range c.Headers {
			if headers[k], err = f(v); err != nil {
				return WebhookConfig{}, err
			}
		}
		c.Headers = headers
	}
	return c, nil
}

func (r *webhookRunner) run(ctx context.Context, payload []byte) error {
	var stored WebhookConfig
	if err := json.Unmarshal(payload, &stored); err != nil {
		return fmt.Errorf("decode webhook config: %w", err)
	}
	cfg, err := r.open(stored)
	if err != nil {
		return err
	}
	if _, err := urlguard.CheckShape(cfg.URL); err != nil {
		return err
	}
	method := normalizeMethod(cfg.Method)
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
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return fmt.Errorf("webhook returned a redirect (HTTP %d) - point the job at the final URL", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned HTTP %d", resp.StatusCode)
	}
	return nil
}

// ValidateWebhookURL is the save-time check, the shared SSRF guard's - the guarded transport stays the authoritative one against rebinding
func ValidateWebhookURL(raw string, allowPrivate bool) error {
	return urlguard.ValidateTargetURL(strings.TrimSpace(raw), allowPrivate)
}

func validateMethod(m string) error {
	if !slices.Contains(webhookMethods, normalizeMethod(m)) {
		return fmt.Errorf("webhook method must be one of %s", strings.Join(webhookMethods, ", "))
	}
	return nil
}
