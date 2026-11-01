package sdk

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
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

func TestResponseErrorSortsWhatToRetry(t *testing.T) {
	for code, want := range map[int]string{200: "ok", 204: "ok", 400: "permanent", 403: "permanent", 408: "retry", 429: "retry", 500: "retry", 503: "retry"} {
		req, _ := http.NewRequest(http.MethodPost, "https://platform.example/collect?api_secret=x", nil)
		err := ResponseError(&http.Response{StatusCode: code, Request: req, Body: http.NoBody})
		got := "ok"
		if err != nil {
			got = "retry"
			if IsPermanent(err) {
				got = "permanent"
			}
			if strings.Contains(err.Error(), "api_secret") {
				t.Errorf("%d: the error carries the query string: %v", code, err)
			}
		}
		if got != want {
			t.Errorf("%d: %s, want %s", code, got, want)
		}
	}
}
