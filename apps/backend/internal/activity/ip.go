package activity

import (
	"net"
	"strings"
)

// Truncates an IP before it goes on the spine - the spine keeps events for a year by default, and a
// customer-id to full-IP linkage is PII core has no reason to put there
func anonymizeIP(ip string) string {
	s := strings.TrimSpace(ip)
	if s == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(s); err == nil {
		s = host
	}
	p := net.ParseIP(s)
	if p == nil {
		return ""
	}
	if v4 := p.To4(); v4 != nil {
		return v4.Mask(net.CIDRMask(24, 32)).String()
	}
	return p.Mask(net.CIDRMask(48, 128)).String()
}
