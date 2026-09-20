package wiring

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/signalsapi"
)

// A beacon that lost its own route wouldn't 404 - the /t/ catch-all would answer it 204, silently
func TestTheCommerceBeaconsOutrankTheBeaconSurface(t *testing.T) {
	mux := http.NewServeMux()
	registerCommerceBeacons(mux, commerceBeaconDeps{})
	mux.Handle("/t/", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	for _, p := range signalsapi.CommerceBeaconPaths {
		if _, pattern := mux.Handler(httptest.NewRequest(http.MethodPost, p, nil)); pattern == "/t/" {
			t.Errorf("%s falls through to the beacon surface, which would answer it 204", p)
		}
	}
}

// A path served here but missing from the shared list gets answered as a beacon on the ingest tier,
// and a listed path nothing serves is forwarded to a 404
func TestEveryCommerceBeaconIsOnTheListTheBeaconTierForwards(t *testing.T) {
	forwarded := map[string]bool{}
	for _, p := range signalsapi.CommerceBeaconPaths {
		forwarded[p] = true
	}

	mux := http.NewServeMux()
	registerCommerceBeacons(mux, commerceBeaconDeps{})
	for _, b := range commerceBeacons {
		if !forwarded[b.path] {
			t.Errorf("%s is served here but not forwarded by the beacon tier", b.path)
		}
	}
	if len(forwarded) != len(commerceBeacons) {
		t.Errorf("%d paths forwarded, %d served", len(forwarded), len(commerceBeacons))
	}
}

// Own bucket each, so a shopper signing up isn't silenced by somebody else's restock traffic
func TestTheCommerceBeaconsRateLimitSeparately(t *testing.T) {
	seen := map[string]string{}
	for _, b := range commerceBeacons {
		if prev, dup := seen[b.limitKey]; dup {
			t.Errorf("%s shares the %q bucket with %s", b.path, b.limitKey, prev)
		}
		seen[b.limitKey] = b.path
	}
}
