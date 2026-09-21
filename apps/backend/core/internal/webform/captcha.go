package webform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/otelx"
)

// Config paths for the global form settings - section/group/field, living under the existing "cms" section and "forms" group
const (
	CfgEnabled          = "cms/forms/enabled"
	CfgCaptchaProvider  = "cms/forms/captcha_provider"   // none | turnstile | hcaptcha
	CfgCaptchaSiteKey   = "cms/forms/captcha_site_key"   // public site key
	CfgCaptchaSecretKey = "cms/forms/captcha_secret_key" // TypeSecret
)

// Maps a provider to its server-side siteverify URL - the host is fixed with no user input, so no SSRF guard is needed
var verifyEndpoints = map[string]string{
	"turnstile": "https://challenges.cloudflare.com/turnstile/v0/siteverify",
	"hcaptcha":  "https://api.hcaptcha.com/siteverify",
}

// Verifies a client CAPTCHA token against the configured provider - provider and secret come from config, so an operator can switch providers without a redeploy
type CaptchaVerifier struct {
	cfg  ConfigReader
	http *http.Client
}

func NewCaptchaVerifier(cfg ConfigReader) *CaptchaVerifier {
	return &CaptchaVerifier{
		cfg:  cfg,
		http: &http.Client{Timeout: 10 * time.Second, Transport: otelx.HTTPTransport(nil)},
	}
}

// Checks a token - `configured` is false when no provider or secret is set, leaving callers to decide whether to fail open, and when configured `ok` reflects the provider's verdict
func (v *CaptchaVerifier) Verify(ctx context.Context, token, remoteIP string) (ok, configured bool, err error) {
	provider, _ := v.cfg.GetString(ctx, CfgCaptchaProvider, config.DefaultScope)
	provider = strings.TrimSpace(strings.ToLower(provider))
	endpoint, known := verifyEndpoints[provider]
	if provider == "" || provider == "none" || !known {
		return false, false, nil
	}
	secret, _ := v.cfg.GetString(ctx, CfgCaptchaSecretKey, config.DefaultScope)
	if strings.TrimSpace(secret) == "" {
		return false, false, nil
	}
	if strings.TrimSpace(token) == "" {
		return false, true, nil
	}

	form := url.Values{"secret": {secret}, "response": {token}}
	if remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return false, true, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := v.http.Do(req)
	if err != nil {
		return false, true, err
	}
	defer resp.Body.Close()

	var body struct {
		Success bool `json:"success"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return false, true, err
	}
	return body.Success, true, nil
}
