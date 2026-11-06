package sdk

import (
	"context"
	"io"
	"testing"
)

func TestProductFeedRegistrationChecksAndCopiesTheRegistry(t *testing.T) {
	productFeeds.Lock()
	old := productFeeds.regs
	productFeeds.regs = nil
	productFeeds.Unlock()
	t.Cleanup(func() { productFeeds.Lock(); productFeeds.regs = old; productFeeds.Unlock() })
	p := NewPluginForTest("feed-example")
	newWriter := func(w io.Writer, _ DestinationConfig) (ProductFeedWriter, error) { return feedTestWriter{}, nil }
	valid := ProductFeed{Kind: "example", Label: "Example", Extension: "jsonl", ContentType: "application/x-ndjson", NewWriter: newWriter, Send: func(context.Context, ProductFeedClient, DestinationConfig, ProductFeedFile) error { return nil }}
	p.RegisterProductFeed(valid)
	regs := ProductFeeds()
	if len(regs) != 1 || regs[0].PluginSlug != "feed-example" || regs[0].Feed.Kind != "example" {
		t.Fatalf("registration lost: %+v", regs)
	}
	regs[0].Feed.Kind = "mutated"
	if ProductFeeds()[0].Feed.Kind != "example" {
		t.Fatal("a caller mutated the registry")
	}
	for _, f := range []ProductFeed{
		valid,
		{Kind: "../escape", NewWriter: newWriter},
		{Kind: "x", NewWriter: newWriter},
		{Kind: "x", Extension: "../escape"},
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("invalid or duplicate feed did not panic: %+v", f)
				}
			}()
			p.RegisterProductFeed(f)
		}()
	}
}

type feedTestWriter struct{}

func (feedTestWriter) WriteItem(ProductFeedItem) (bool, error) { return true, nil }
func (feedTestWriter) Close() error                            { return nil }
