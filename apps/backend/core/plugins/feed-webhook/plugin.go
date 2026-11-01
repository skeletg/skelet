// Package feedwebhook sends catalogue snapshots to a merchant's HTTP integration.
package feedwebhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/skeletg/skelet/apps/backend/sdk"
)

const Slug = "feed-webhook"

func init() {
	p := sdk.RegisterPlugin(sdk.Manifest{Slug: Slug, Name: "Catalogue webhook", Version: "0.1.0", SDKRange: "^0.1.0"})
	p.RegisterProductFeed(sdk.ProductFeed{Kind: "webhook", Label: "Catalogue webhook", Extension: "jsonl", ContentType: "application/x-ndjson",
		Fields: []sdk.DestinationField{
			{Key: "endpoint", Label: "HTTP endpoint", Required: true},
			{Key: "secret", Label: "Signing secret", Required: true, Secret: true},
		},
		NewWriter: func(w io.Writer, _ sdk.DestinationConfig) (sdk.ProductFeedWriter, error) {
			return &writer{json.NewEncoder(w)}, nil
		},
		Send: Send,
	})
}

type writer struct{ *json.Encoder }

func (w *writer) WriteItem(p sdk.ProductFeedItem) (bool, error) { return true, w.Encode(p) }
func (*writer) Close() error                                    { return nil }

// Sign the exact bytes without buffering the catalogue. Open is repeatable for this reason.
func Send(ctx context.Context, client sdk.ProductFeedClient, d sdk.DestinationConfig, file sdk.ProductFeedFile) error {
	stamp := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte(d.Secret))
	_, _ = io.WriteString(mac, stamp+".")
	body, err := file.Open(ctx)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(mac, body)
	closeErr := body.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	body, err = file.Open(ctx)
	if err != nil {
		return err
	}
	defer body.Close()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.Config["endpoint"], body)
	if err != nil {
		return sdk.Permanent(err)
	}
	req.ContentLength = file.Size
	req.Header.Set("Content-Type", file.ContentType)
	req.Header.Set("X-Skelet-Timestamp", stamp)
	req.Header.Set("X-Skelet-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	req.Header.Set("X-Skelet-Channel-Id", d.ChannelID.String())
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	return sdk.ResponseError(res)
}
