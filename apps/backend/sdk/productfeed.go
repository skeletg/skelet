package sdk

import (
	"context"
	"io"
	"mime"
	"net/http"
	"regexp"
	"sync"
)

// ProductFeedItem is a published, purchasable item at its channel's current price and stock status.
// Money is already formatted in major units with its ISO currency; no float conversion is needed.
type ProductFeedItem struct {
	SKU            string            `json:"sku"`
	Title          string            `json:"title"`
	Description    string            `json:"description"`
	URL            string            `json:"url"`
	Brand          string            `json:"brand"`
	SellerName     string            `json:"seller_name"`
	ImageURL       string            `json:"image_url"`
	Price          string            `json:"price"`
	Availability   string            `json:"availability"`
	GroupID        string            `json:"group_id,omitempty"`
	VariantOptions map[string]string `json:"variant_options,omitempty"`
}

type ProductFeedWriter interface {
	WriteItem(ProductFeedItem) (bool, error)
	Close() error
}

type ProductFeedFile struct {
	Name, ContentType string
	Size              int64
	Open              func(context.Context) (io.ReadCloser, error)
}

// Core supplies guarded network clients to catalogue plugins.
type ProductFeedClient interface {
	Do(*http.Request) (*http.Response, error)
	UploadSFTP(context.Context, SFTPTarget, io.Reader) error
}

type SFTPTarget struct {
	Address, Username, HostKeySHA256, Path string
	AuthMethod, Secret                     string // password (default) or private_key; never stored in delivery payloads
}

// A catalogue destination. Each export gets a fresh writer, so a plugin can own CSV headers,
// JSON delimiters and compression. Core owns accounts, credentials, scheduling and retries.
type ProductFeed struct {
	Kind        string
	Label       string
	Extension   string
	ContentType string
	Fields      []DestinationField
	NewWriter   func(io.Writer, DestinationConfig) (ProductFeedWriter, error)
	Send        func(context.Context, ProductFeedClient, DestinationConfig, ProductFeedFile) error
}

type RegisteredProductFeed struct {
	PluginSlug string
	Feed       ProductFeed
}

var productFeeds struct {
	sync.Mutex
	regs []RegisteredProductFeed
}

var productFeedKind = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)
var productFeedExtension = regexp.MustCompile(`^[a-z0-9]+(?:\.[a-z0-9]+)*$`)

func (pl *Plugin) RegisterProductFeed(f ProductFeed) {
	if !productFeedKind.MatchString(f.Kind) || f.Label == "" || !productFeedExtension.MatchString(f.Extension) || len(f.Extension) > 32 || f.NewWriter == nil || f.Send == nil {
		panic("sdk.RegisterProductFeed: Kind, Label, Extension, NewWriter and Send are required")
	}
	if _, _, err := mime.ParseMediaType(f.ContentType); err != nil {
		panic("sdk.RegisterProductFeed: invalid ContentType")
	}
	secrets := 0
	for _, field := range f.Fields {
		if field.Secret {
			secrets++
		}
	}
	if secrets > 1 {
		panic("sdk.RegisterProductFeed: at most one encrypted secret field")
	}
	productFeeds.Lock()
	defer productFeeds.Unlock()
	for _, r := range productFeeds.regs {
		if r.Feed.Kind == f.Kind {
			panic("sdk.RegisterProductFeed: duplicate kind " + f.Kind)
		}
	}
	productFeeds.regs = append(productFeeds.regs, RegisteredProductFeed{PluginSlug: pl.slug, Feed: f})
}

func ProductFeeds() []RegisteredProductFeed {
	productFeeds.Lock()
	defer productFeeds.Unlock()
	return append([]RegisteredProductFeed(nil), productFeeds.regs...)
}
