package tracking

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

var ErrLegacyUnmapped = errors.New("this Signals destination can't be imported")

// FromLegacy maps an old Signals event destination - a kind and the one URL it posted to - onto a kind here and its
// settings. The Signals kinds kept their ids in the URL: GA4's measurement id in the query, Meta's pixel in the path.
// Secrets don't cross over; the admin enters them again
func FromLegacy(kind, targetURL string) (string, map[string]string, error) {
	u, err := url.Parse(strings.TrimSpace(targetURL))
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return "", nil, fmt.Errorf("%w: %q is not a URL", ErrLegacyUnmapped, targetURL)
	}
	switch kind {
	case "ga4_mp":
		id := u.Query().Get("measurement_id")
		if id == "" {
			return "", nil, fmt.Errorf("%w: no measurement_id in %s", ErrLegacyUnmapped, u.Redacted())
		}
		cfg := map[string]string{"measurement_id": id}
		if u.Host != "www.google-analytics.com" { // Google's own is the default; the EU host or a server-side GTM is kept
			cfg["endpoint"] = u.Scheme + "://" + u.Host
		}
		if strings.HasPrefix(u.Path, "/debug/") {
			cfg["validate"] = "true"
		}
		return "ga4", cfg, nil
	case "meta_capi":
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		for i := 1; i < len(parts); i++ {
			if parts[i] == "events" && parts[i-1] != "" {
				return "meta_capi", map[string]string{"pixel_id": parts[i-1]}, nil
			}
		}
		return "", nil, fmt.Errorf("%w: no pixel id in %s", ErrLegacyUnmapped, u.Path)
	case "webhook":
		return "webhook", map[string]string{"url": u.String()}, nil
	}
	return "", nil, fmt.Errorf("%w: kind %q", ErrLegacyUnmapped, kind)
}
