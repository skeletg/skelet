package sdk

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
)

func TestADestinationIsKeptWithThePluginThatRegisteredIt(t *testing.T) {
	destinationsMu.Lock()
	saved := destinations
	destinations = nil
	destinationsMu.Unlock()
	t.Cleanup(func() { destinationsMu.Lock(); destinations = saved; destinationsMu.Unlock() })

	send := func(context.Context, *http.Client, DestinationConfig, ShopEvent) error { return nil }
	(&Plugin{slug: "ga4"}).RegisterDestination(Destination{Kind: "ga4", Purposes: []DestinationPurpose{PurposeAnalytics}, Send: send})
	got := Destinations()
	if len(got) != 1 || got[0].PluginSlug != "ga4" || got[0].Destination.Kind != "ga4" {
		t.Fatalf("Destinations() = %+v", got)
	}

	for name, d := range map[string]Destination{
		"no kind":     {Send: send},
		"no send":     {Kind: "x"},
		"two secrets": {Kind: "x", Send: send, Fields: []DestinationField{{Key: "a", Secret: true}, {Key: "b", Secret: true}}},
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: registered without a panic", name)
				}
			}()
			(&Plugin{slug: "bad"}).RegisterDestination(d)
		}()
	}
}

func TestPermanentSurvivesWrapping(t *testing.T) {
	err := fmt.Errorf("send: %w", Permanent(errors.New("400 bad request")))
	if !IsPermanent(err) || IsPermanent(errors.New("502")) {
		t.Error("IsPermanent must see through wrapping and only there")
	}
}
