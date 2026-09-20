// Package content holds the small pieces of logic shared across the content domains - blog and cms, plus catalog's slug rules - so rules that must stay identical can't drift between parallel copies
package content

import (
	"encoding/json"
	"fmt"
)

// Enforces URL-slug rules - non-empty, and only lowercase ASCII letters, digits and the separators '-', '_' and '.'
func ValidateSlug(slug string, invalid error) error {
	if slug == "" {
		return fmt.Errorf("%w: empty", invalid)
	}
	for _, r := range slug {
		isLower := r >= 'a' && r <= 'z'
		isDigit := r >= '0' && r <= '9'
		isSep := r == '-' || r == '_' || r == '.'
		if !(isLower || isDigit || isSep) {
			return fmt.Errorf("%w: %q contains %q", invalid, slug, r)
		}
	}
	return nil
}

// Interprets a publish-webhook subscriber's response as a publish veto - an empty body is "no opinion" and allows, while {"reject": true, "reason": "..."} vetoes
func DecodePublishVeto(body []byte) (rejected bool, reason string, err error) {
	if len(body) == 0 {
		return false, "", nil
	}
	var resp struct {
		Reject bool   `json:"reject"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return false, "", err
	}
	if resp.Reject {
		return true, resp.Reason, nil
	}
	return false, "", nil
}
