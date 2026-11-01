// Package trackingwebhook sends each shop event as signed JSON to an endpoint of the merchant's own - the way to reach a
// platform there's no plugin for yet, or a warehouse. It signs the way the store's webhooks do, so one verifier serves both
package trackingwebhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/skeletg/skelet/apps/backend/sdk"
)

const (
	Slug = "tracking-webhook"
	Kind = "webhook"
)

func init() {
	p := sdk.RegisterPlugin(sdk.Manifest{Slug: Slug, Name: "Event webhook", Version: "0.1.0", SDKRange: "^0.1.2"})
	p.RegisterDestination(sdk.Destination{
		Kind:  Kind,
		Label: "Webhook (signed JSON)",
		// What the receiver does with the events is the merchant's, so it is held to the analytics choice - a row can't
		// claim a weaker purpose and get past a visitor's refusal
		Purposes: []sdk.DestinationPurpose{sdk.PurposeAnalytics},
		Fields: []sdk.DestinationField{
			{Key: "url", Label: "Endpoint URL", Help: "Receives one POST per event", Required: true},
			{Key: "signing_secret", Label: "Signing secret", Help: "Signs X-Webhook-Signature-256 as HMAC-SHA256 of <timestamp>.<body>, like the store's webhooks", Required: true, Secret: true},
		},
		Send: Send,
	})
}

func Send(ctx context.Context, client *http.Client, d sdk.DestinationConfig, ev sdk.ShopEvent) error {
	body, err := json.Marshal(ev)
	if err != nil {
		return sdk.Permanent(err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.Config["url"], bytes.NewReader(body))
	if err != nil {
		return sdk.Permanent(err)
	}
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Event", "tracking."+ev.Name)
	req.Header.Set("X-Webhook-Id", ev.ID) // stable across retries, for the receiver to dedupe on
	req.Header.Set("X-Webhook-Timestamp", ts)
	req.Header.Set("X-Webhook-Signature-256", "sha256="+Signature(d.Secret, ts, body))
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	return sdk.ResponseError(res)
}

// Signature is HMAC-SHA256 of "<timestamp>.<body>" - the timestamp gives the receiver a replay window to enforce
func Signature(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "."))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}
