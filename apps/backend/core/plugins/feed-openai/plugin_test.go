package feedopenai

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/skeletg/skelet/apps/backend/sdk"
)

func example(t *testing.T) sdk.ProductFeedItem {
	t.Helper()
	raw, err := os.ReadFile("testdata/minimal.json")
	if err != nil {
		t.Fatal(err)
	}
	var v item
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	return sdk.ProductFeedItem{SKU: v.ID, Title: v.Title, Description: v.Description, URL: v.URL,
		Brand: v.Brand, SellerName: v.SellerName, ImageURL: v.ImageURL, Price: v.Price, Availability: v.Availability}
}

func TestTheFeedMatchesOpenAIsStableMinimalExample(t *testing.T) {
	p := example(t)
	var buf bytes.Buffer
	ok, err := Encode(&buf, p)
	if err != nil || !ok {
		t.Fatalf("encode: %v, %v", ok, err)
	}
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile("testdata/minimal.json")
	var want map[string]any
	_ = json.Unmarshal(raw, &want)
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %v, want %v", k, got[k], v)
		}
	}
	if got["is_eligible_search"] != true || got["is_eligible_checkout"] != false || got["is_ads_eligible"] != false {
		t.Error("a discovery feed must not opt into Ads or checkout")
	}
}

func TestIncompleteAndUnavailableItemsAreSkipped(t *testing.T) {
	for name, mutate := range map[string]func(*sdk.ProductFeedItem){
		"missing brand":        func(p *sdk.ProductFeedItem) { p.Brand = "" },
		"missing description":  func(p *sdk.ProductFeedItem) { p.Description = " " },
		"missing image":        func(p *sdk.ProductFeedItem) { p.ImageURL = "" },
		"relative product URL": func(p *sdk.ProductFeedItem) { p.URL = "/mug" },
		"URL credentials":      func(p *sdk.ProductFeedItem) { p.URL = "https://secret@example.com/mug" },
		"out of stock":         func(p *sdk.ProductFeedItem) { p.Availability = "out_of_stock" },
		"unknown stock":        func(p *sdk.ProductFeedItem) { p.Availability = "unknown" },
	} {
		t.Run(name, func(t *testing.T) {
			p := example(t)
			mutate(&p)
			var buf bytes.Buffer
			if ok, err := Encode(&buf, p); ok || err != nil || buf.Len() != 0 {
				t.Fatalf("invalid item produced data: %v, %v, %s", ok, err, &buf)
			}
		})
	}
}

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestVariantsUnicodeAndWriteFailures(t *testing.T) {
	p := example(t)
	p.Title = strings.Repeat("ž", 151)
	p.Description = strings.Repeat("ž", 5001)
	p.GroupID, p.VariantOptions = "MUG-350", map[string]string{"color": "Blue"}
	var buf bytes.Buffer
	if _, err := Encode(&buf, p); err != nil {
		t.Fatal(err)
	}
	var got item
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.HasVariations || got.GroupID != p.GroupID || got.Variants["color"] != "Blue" || len([]rune(got.Title)) != 150 || len([]rune(got.Description)) != 5000 {
		t.Fatalf("variant or Unicode content lost: %+v", got)
	}
	if _, err := Encode(brokenWriter{}, p); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("write failure swallowed: %v", err)
	}
}

type captureClient struct {
	target sdk.SFTPTarget
	body   []byte
	calls  int
}

func (*captureClient) Do(*http.Request) (*http.Response, error) { panic("SFTP only") }
func (c *captureClient) UploadSFTP(_ context.Context, target sdk.SFTPTarget, body io.Reader) error {
	c.calls++
	c.target = target
	var err error
	c.body, err = io.ReadAll(body)
	return err
}

func TestRegisteredAdapterFinishesGzipAndUsesTheConfiguredSFTPAccount(t *testing.T) {
	var feed sdk.ProductFeed
	for _, r := range sdk.ProductFeeds() {
		if r.PluginSlug == Slug {
			feed = r.Feed
		}
	}
	if feed.NewWriter == nil {
		t.Fatal("adapter not registered")
	}
	var buf bytes.Buffer
	d := sdk.DestinationConfig{Secret: "private key bytes", Config: map[string]string{
		"address": "sftp.example.com:22", "username": "merchant", "host_key": "SHA256:verified", "auth_method": "private_key",
	}}
	w, err := feed.NewWriter(&buf, d)
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := w.WriteItem(example(t)); err != nil || !ok {
		t.Fatalf("write: %v, %v", ok, err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	client := &captureClient{}
	file := sdk.ProductFeedFile{Open: func(context.Context) (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(buf.Bytes())), nil }}
	if err := feed.Send(context.Background(), client, d, file); err != nil {
		t.Fatal(err)
	}
	want := sdk.SFTPTarget{Address: "sftp.example.com:22", Username: "merchant", HostKeySHA256: "SHA256:verified", AuthMethod: "private_key", Secret: d.Secret, Path: "catalog.jsonl.gz"}
	if client.target != want {
		t.Fatalf("wrong SFTP account: %+v", client.target)
	}
	z, err := gzip.NewReader(bytes.NewReader(client.body))
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	raw, err := io.ReadAll(z)
	if err != nil {
		t.Fatalf("gzip trailer: %v", err)
	}
	var got item
	if err := json.Unmarshal(raw, &got); err != nil || got.ID != example(t).SKU {
		t.Fatalf("wrong snapshot: %s, %v", raw, err)
	}
	for _, name := range []string{"../escape.gz", "dir/catalog.gz", "..", ".", "bad\\name", "bad\x00name"} {
		d.Config["filename"] = name
		if err := feed.Send(context.Background(), client, d, file); !sdk.IsPermanent(err) {
			t.Errorf("unsafe filename %q: %v", name, err)
		}
	}
	if client.calls != 1 {
		t.Fatal("invalid filenames reached the server")
	}
	d.Config["filename"] = "other.jsonl.gz"
	if err := feed.Send(context.Background(), client, d, file); err != nil || client.target.Path != "other.jsonl.gz" {
		t.Fatalf("custom filename: %v", err)
	}
}
