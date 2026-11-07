package tracking

import (
	"errors"
	"maps"
	"testing"
)

func TestSignalsDestinationsMapOntoKindsHere(t *testing.T) {
	cases := []struct {
		kind, url, want string
		cfg             map[string]string
	}{
		{"ga4_mp", "https://www.google-analytics.com/mp/collect?measurement_id=G-ABC123", "ga4", map[string]string{"measurement_id": "G-ABC123"}},
		{"ga4_mp", "https://region1.google-analytics.com/debug/mp/collect?measurement_id=G-EU1", "ga4", map[string]string{"measurement_id": "G-EU1", "endpoint": "https://region1.google-analytics.com", "validate": "true"}},
		{"ga4_mp", "https://tags.shop.example/mp/collect?measurement_id=G-SS", "ga4", map[string]string{"measurement_id": "G-SS", "endpoint": "https://tags.shop.example"}},
		{"meta_capi", "https://graph.facebook.com/v19.0/1234567890/events", "meta_capi", map[string]string{"pixel_id": "1234567890"}},
		{"webhook", "https://hooks.shop.example/in?x=1", "webhook", map[string]string{"url": "https://hooks.shop.example/in?x=1"}},
	}
	for _, c := range cases {
		kind, cfg, err := FromLegacy(c.kind, c.url)
		if err != nil || kind != c.want || !maps.Equal(cfg, c.cfg) {
			t.Errorf("%s %s: %s %v %v", c.kind, c.url, kind, cfg, err)
		}
	}
	for _, bad := range [][2]string{
		{"ga4_mp", "https://www.google-analytics.com/mp/collect"},
		{"meta_capi", "https://graph.facebook.com/v19.0/"},
		{"webhook", "not a url"},
		{"pinterest", "https://api.pinterest.com/"},
	} {
		if _, _, err := FromLegacy(bad[0], bad[1]); !errors.Is(err, ErrLegacyUnmapped) {
			t.Errorf("%v should not map: %v", bad, err)
		}
	}
}
