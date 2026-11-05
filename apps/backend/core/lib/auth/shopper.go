package auth

import "context"

// Shopper is a shop visitor as the analytics and ad platforms their events go to need them, taken while the request is
// still here - an event sent later then follows the consent that applied at the time. Each part is kept only under the
// consent it needs
type Shopper struct {
	Consent   Consent           `json:"consent"`
	AnonID    string            `json:"anon_id,omitempty"`    // analytics
	SessionID string            `json:"session_id,omitempty"` // analytics
	ClickIDs  map[string]string `json:"click_ids,omitempty"`  // ads, or an affiliate network the merchant made necessary
	IP        string            `json:"ip,omitempty"`         // ads, the platforms' match keys
	UserAgent string            `json:"ua,omitempty"`         // ads
}

// ShopperFromContext is nil for a request that states no cookie consent - admin drafts and API clients, which have no banner
func ShopperFromContext(ctx context.Context) *Shopper {
	c, ok := ConsentFromContext(ctx)
	if !ok {
		return nil
	}
	s := &Shopper{Consent: c, ClickIDs: ClickIDs(ctx)} // ClickIDs decides per id: ad platforms' need ads consent, an affiliate's may be necessary
	if c.Analytics {
		s.AnonID, s.SessionID = AnonID(ctx), SessionID(ctx)
	}
	if c.Ads {
		r, _ := RequestFromContext(ctx)
		s.IP, s.UserAgent = ClientIP(r), r.UserAgent()
	}
	return s
}

// AnalyticsRefused is a shop visitor who said no to analytics - false for nil, a caller with no banner
func (s *Shopper) AnalyticsRefused() bool { return s != nil && !s.Consent.Analytics }
