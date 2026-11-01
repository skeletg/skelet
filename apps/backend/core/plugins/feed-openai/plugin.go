// Package feedopenai exports the stable OpenAI discovery product-feed format.
package feedopenai

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/skeletg/skelet/apps/backend/sdk"
)

const (
	Slug = "feed-openai"
)

func init() {
	p := sdk.RegisterPlugin(sdk.Manifest{Slug: Slug, Name: "OpenAI product feed", Version: "0.1.0", SDKRange: "^0.1.0"})
	p.RegisterProductFeed(sdk.ProductFeed{
		Kind: "openai", Label: "OpenAI catalogue (SFTP)", Extension: "jsonl.gz", ContentType: "application/gzip",
		Fields: []sdk.DestinationField{
			{Key: "address", Label: "SFTP host:port", Required: true},
			{Key: "username", Label: "SFTP username", Required: true},
			{Key: "host_key", Label: "Verified SSH host key (SHA256:...)", Required: true},
			{Key: "credential", Label: "SFTP password or private key", Required: true, Secret: true},
			{Key: "auth_method", Label: "Authentication: password (default) or private_key"},
			{Key: "filename", Label: "Stable filename in the SFTP root (default catalog.jsonl.gz)"},
		},
		NewWriter: func(w io.Writer, _ sdk.DestinationConfig) (sdk.ProductFeedWriter, error) {
			return &feedWriter{gzip.NewWriter(w)}, nil
		},
		Send: Send,
	})
}

type feedWriter struct{ *gzip.Writer }

func (w *feedWriter) WriteItem(p sdk.ProductFeedItem) (bool, error) { return Encode(w.Writer, p) }

func Send(ctx context.Context, client sdk.ProductFeedClient, d sdk.DestinationConfig, file sdk.ProductFeedFile) error {
	name := d.Config["filename"]
	if name == "" {
		name = "catalog.jsonl.gz"
	}
	if name == "." || name == ".." || path.Base(name) != name || strings.ContainsAny(name, "\\\x00") {
		return sdk.Permanent(fmt.Errorf("OpenAI feed filename must be a file in the SFTP root"))
	}
	body, err := file.Open(ctx)
	if err != nil {
		return err
	}
	defer body.Close()
	return client.UploadSFTP(ctx, sdk.SFTPTarget{Address: d.Config["address"], Username: d.Config["username"],
		HostKeySHA256: d.Config["host_key"], AuthMethod: d.Config["auth_method"], Secret: d.Secret, Path: name}, body)
}

// Stable schema and minimal example: https://developers.openai.com/commerce/specs/file-upload/products
type item struct {
	ID               string            `json:"item_id"`
	Title            string            `json:"title"`
	Description      string            `json:"description"`
	URL              string            `json:"url"`
	Brand            string            `json:"brand"`
	SellerName       string            `json:"seller_name"`
	ImageURL         string            `json:"image_url"`
	Price            string            `json:"price"`
	Availability     string            `json:"availability"`
	GroupID          string            `json:"group_id,omitempty"`
	HasVariations    bool              `json:"listing_has_variations,omitempty"`
	Variants         map[string]string `json:"variant_dict,omitempty"`
	EligibleSearch   bool              `json:"is_eligible_search"`
	EligibleCheckout bool              `json:"is_eligible_checkout"`
	EligibleAds      bool              `json:"is_ads_eligible"`
}

func Encode(w io.Writer, p sdk.ProductFeedItem) (bool, error) {
	for _, v := range []string{p.SKU, p.Title, p.Description, p.Brand, p.SellerName, p.Price} {
		if strings.TrimSpace(v) == "" || !utf8.ValidString(v) {
			return false, nil
		}
	}
	if !publicURL(p.URL) || !publicURL(p.ImageURL) {
		return false, nil
	}
	switch p.Availability {
	case "in_stock", "backorder", "pre_order":
	default:
		return false, nil
	}
	out := item{
		ID: p.SKU, Title: truncate(p.Title, 150), Description: truncate(p.Description, 5000),
		URL: p.URL, Brand: p.Brand, SellerName: p.SellerName, ImageURL: p.ImageURL,
		Price: p.Price, Availability: p.Availability, EligibleSearch: true,
	}
	if p.GroupID != "" && p.GroupID != p.SKU && len(p.VariantOptions) > 0 {
		out.GroupID, out.HasVariations, out.Variants = p.GroupID, true, p.VariantOptions
	}
	return true, json.NewEncoder(w).Encode(out)
}

func publicURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Hostname() != "" && u.User == nil
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}
