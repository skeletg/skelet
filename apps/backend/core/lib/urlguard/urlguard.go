// Package urlguard is the SSRF guard for outbound HTTP to admin-supplied URLs - webhook targets, remote-provider endpoints, flow and destination callbacks Two layers, both required: ValidateTargetURL runs at create and update time for friendly admin errors, and SafeTransport guards at dial time against the ACTUAL resolved IP, so a hostname that passes validation and is later re-pointed at an internal address still fails at connect allowPrivate, wired to APP_ENV=development, disables the IP-range checks so local-dev callbacks against localhost keep working - scheme and shape validation always apply
package urlguard

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"slices"
	"syscall"
	"time"
)

// Rejects URLs that aren't plain http(s) endpoints, or whose host resolves to a private, loopback, link-local or otherwise non-global address
func ValidateTargetURL(raw string, allowPrivate bool) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid target url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("target url must use http or https")
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("target url must include a host")
	}
	if u.User != nil {
		return fmt.Errorf("target url must not include credentials")
	}
	if allowPrivate {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil {
		if DisallowedIP(ip) {
			return fmt.Errorf("target url must not point at a private or internal address")
		}
		return nil
	}
	// Best-effort resolution at save time - the dial-time guard is authoritative, so an NXDOMAIN here passes and an admin can register an endpoint before its DNS record exists
	ips, err := net.LookupIP(host)
	if err != nil {
		return nil
	}
	if slices.ContainsFunc(ips, DisallowedIP) {
		return fmt.Errorf("target url must not resolve to a private or internal address")
	}
	return nil
}

// An *http.Transport cloned from the default, whose dialer refuses connections to non-global addresses - checking the resolved IP at connect time closes the DNS-rebinding gap that save-time validation leaves
func SafeTransport(allowPrivate bool) *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	if allowPrivate {
		return t
	}
	dialer := &net.Dialer{
		Timeout:   5 * time.Second,
		KeepAlive: 30 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return fmt.Errorf("guarded dial: %w", err)
			}
			ip := net.ParseIP(host)
			if ip == nil || DisallowedIP(ip) {
				return fmt.Errorf("target resolves to a private or internal address")
			}
			return nil
		},
	}
	t.DialContext = dialer.DialContext
	return t
}

// Anything other than a global unicast address - loopback, RFC1918 and ULA private ranges, link-local including the 169.254.169.254 cloud metadata address, multicast, unspecified and broadcast all refuse
func DisallowedIP(ip net.IP) bool {
	return !ip.IsGlobalUnicast() || ip.IsPrivate()
}
