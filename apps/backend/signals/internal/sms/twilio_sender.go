package sms

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// TwilioSender posts to Twilio's Messages API - Key-ready: the operator supplies the account SID, auth token, and From
// number; until then the factory falls back to LogSender (dev) or errSender (prod)
type TwilioSender struct {
	AccountSID string
	AuthToken  string
	From       string
	http       *http.Client
}

func (t TwilioSender) Send(ctx context.Context, to, body string) error {
	if strings.TrimSpace(to) == "" {
		return fmt.Errorf("sms: empty recipient")
	}
	form := url.Values{"To": {to}, "From": {t.From}, "Body": {body}}
	endpoint := "https://api.twilio.com/2010-04-01/Accounts/" + t.AccountSID + "/Messages.json"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.SetBasicAuth(t.AccountSID, t.AuthToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	cl := t.http
	if cl == nil {
		cl = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := cl.Do(req)
	if err != nil {
		return fmt.Errorf("sms: twilio request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("sms: twilio status %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}
