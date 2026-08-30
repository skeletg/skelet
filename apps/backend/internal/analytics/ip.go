package analytics

import (
	"net"
	"strings"
)

// Truncates an IP before it lands on the long-retention analytics spine - data minimisation, since the spine keeps events for a year by default and a customer-id to full-IP linkage is PII we don't need at rest
func AnonymizeIP(ip string) string {
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
		masked := v4.Mask(net.CIDRMask(24, 32))
		return masked.String()
	}
	return p.Mask(net.CIDRMask(48, 128)).String()
}
