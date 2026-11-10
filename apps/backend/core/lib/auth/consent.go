package auth

// What the visitor allowed in the storefront's cookie banner, and the analytics session and ad click ids that follow
// from it - read from the headers the storefront adds to its server-side calls

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
)

const ConsentHeader = "X-Consent" // the visitor's cookie consent, the storefront_consent cookie's value

// Consent is what the visitor allowed in the storefront's cookie banner, in Consent Mode v2's categories - Ads covers ad_storage and ad_user_data
type Consent struct {
	Analytics       bool `json:"analytics"`
	Ads             bool `json:"ads"`
	Personalization bool `json:"personalization"`
}

// SYNC: the same pattern as V2 in packages/storefront-core/consent/categories.ts - skelet check compares them
var consentValue = regexp.MustCompile(`^(?:v2|auto):a([01])\.d([01])\.p([01])$`)

// ParseConsent mirrors the storefront's parseConsent - ok is false for no choice and for anything unrecognised, which must never count as a grant
func ParseConsent(s string) (Consent, bool) {
	switch s {
	case "all":
		return Consent{Analytics: true, Ads: true, Personalization: true}, true
	case "essential":
		return Consent{}, true
	}
	m := consentValue.FindStringSubmatch(s)
	if m == nil {
		return Consent{}, false
	}
	return Consent{Analytics: m[1] == "1", Ads: m[2] == "1", Personalization: m[3] == "1"}, true
}

func ConsentFromContext(ctx context.Context) (Consent, bool) {
	if r, ok := RequestFromContext(ctx); ok && r != nil {
		return ParseConsent(r.Header.Get(ConsentHeader))
	}
	return Consent{}, false
}

const SessionHeader = "X-Session-Id" // the storefront's analytics session, its start time in seconds

var sessionValue = regexp.MustCompile(`^\d{9,11}$`)

// sessionID is the analytics session the storefront forwards - "" for anything that isn't one. The caller has
// already checked the visitor allowed analytics
func sessionID(r *http.Request) string {
	if v := r.Header.Get(SessionHeader); sessionValue.MatchString(v) {
		return v
	}
	return ""
}

const ClickIDsHeader = "X-Click-Ids" // the ad click ids the storefront keeps in first-party cookies, as a query string

// SYNC: the keys of CLICK_IDS in packages/storefront-core/consent/click-ids.ts - skelet check compares them
var clickIDKeys = []string{"gclid", "gbraid", "wbraid", "msclkid", "ttclid", "fbc", "fbp", "oppref", "obref"}

var clickIDValue = regexp.MustCompile(`^[\w.~-]{1,255}$`)

// ClickIDs reads the forwarded click ids, only with ads consent whatever the header says. Only known keys
// holding opaque tokens pass, so the header can't write arbitrary data into an order
func ClickIDs(r *http.Request, c Consent) map[string]string {
	if !c.Ads {
		return nil
	}
	q, err := url.ParseQuery(r.Header.Get(ClickIDsHeader))
	if err != nil {
		return nil
	}
	var out map[string]string
	for _, k := range clickIDKeys {
		if v := q.Get(k); clickIDValue.MatchString(v) {
			if out == nil {
				out = map[string]string{}
			}
			out[k] = v
		}
	}
	return out
}
