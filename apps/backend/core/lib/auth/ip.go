package auth

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync/atomic"

	"github.com/google/uuid"
)

const requestKey contextKey = "http_request"

func WithRequest(ctx context.Context, r *http.Request) context.Context {
	return context.WithValue(ctx, requestKey, r)
}

func RequestFromContext(ctx context.Context) (*http.Request, bool) {
	r, ok := ctx.Value(requestKey).(*http.Request)
	return r, ok
}

const AnonHeader = "X-Anon-Id" // what the storefront forwards the visitor's first-party anonymous id in, the Lens lens_aid cookie

func AnonID(ctx context.Context) string {
	if r, ok := RequestFromContext(ctx); ok && r != nil {
		return SanitizeAnonID(r.Header.Get(AnonHeader))
	}
	return ""
}

// SanitizeAnonID is the gate every client-supplied visitor id passes on its way in - "" when it doesn't look like one
// The SDKs mint crypto.randomUUID(), but any short token passes so a custom integration's id scheme survives; the cap is what matters, since anon_id lands in spine rows, rollup map keys and the browse_seen PK, where unbounded input is a junk-data amplifier
func SanitizeAnonID(s string) string {
	if len(s) == 0 || len(s) > 64 {
		return ""
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '-' && c != '_' {
			return ""
		}
	}
	return s
}

var trustedProxies atomic.Pointer[[]*net.IPNet] // the CIDR set whose forwarded headers are believed - atomic because it's written once at boot and read on every request

// Takes CIDRs and bare IPs, called once at boot from TRUSTED_PROXIES - with the default empty set forwarded headers are ignored entirely and the TCP peer wins, since trusting them unconditionally would let any client spoof a fresh IP per request and sidestep every per-IP rate limit
func SetTrustedProxies(specs []string) error {
	nets := make([]*net.IPNet, 0, len(specs))
	for _, s := range specs {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if l := strings.ToLower(s); l == "none" || l == "direct" { // the boot check requires TRUSTED_PROXIES in production, so an operator whose backend is directly internet-facing needs a way to say so without listing a CIDR
			continue
		}
		if !strings.Contains(s, "/") {
			if ip := net.ParseIP(s); ip != nil {
				bits := 32
				if ip.To4() == nil {
					bits = 128
				}
				s = fmt.Sprintf("%s/%d", ip, bits)
			}
		}
		_, n, err := net.ParseCIDR(s)
		if err != nil {
			return fmt.Errorf("trusted proxy %q: %w", s, err)
		}
		nets = append(nets, n)
	}
	trustedProxies.Store(&nets)
	return nil
}

func isTrustedProxy(ip net.IP) bool {
	nets := trustedProxies.Load()
	if nets == nil || ip == nil {
		return false
	}
	for _, n := range *nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// Feeds rate limiting and session metadata, so forwarded headers count only when the TCP peer is a trusted proxy - X-Forwarded-For is walked right-to-left because our own proxies appended the rightmost entries and the first address NOT in the trusted set is the real client, while the leftmost entry is the one part of the header the client itself controls
func ClientIP(r *http.Request) string {
	remote := remoteIP(r)
	rip := net.ParseIP(remote)
	if rip == nil || !isTrustedProxy(rip) {
		return remote
	}
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		entries := strings.Split(fwd, ",")
		leftmost := ""
		for _, v := range slices.Backward(entries) {
			ipStr := hostOnly(strings.TrimSpace(v))
			ip := net.ParseIP(ipStr)
			if ip == nil {
				return remote // malformed entry - don't trust anything to its left
			}
			if !isTrustedProxy(ip) {
				return ipStr
			}
			leftmost = ipStr
		}
		if leftmost != "" { // every hop trusted, so the originator is one of our own proxies
			return leftmost
		}
	}
	if real := strings.TrimSpace(r.Header.Get("X-Real-IP")); real != "" {
		if ip := net.ParseIP(hostOnly(real)); ip != nil {
			return hostOnly(real)
		}
	}
	return remote
}

func remoteIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

func hostOnly(s string) string { // some proxies append a :port suffix
	if host, _, err := net.SplitHostPort(s); err == nil {
		return host
	}
	return s
}

// StorefrontSessionCookie is where the storefront keeps the customer access token a same-origin request carries
const StorefrontSessionCookie = "storefront_access_token"

// VerifiedStorefrontCustomer is the only customer identity a public endpoint may trust - header and body ids are forgeable there
// Bearer token first, else the httpOnly session cookie, uuid.Nil when there is neither or the session belongs to another channel
func VerifiedStorefrontCustomer(r *http.Request, v CustomerVerifier, channelID uuid.UUID) uuid.UUID {
	tokenStr := ""
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		tokenStr = strings.TrimPrefix(h, "Bearer ")
	} else if c, err := r.Cookie(StorefrontSessionCookie); err == nil {
		tokenStr = c.Value
	}
	if tokenStr == "" {
		return uuid.Nil
	}
	cc, err := ValidateCustomerToken(tokenStr, v)
	if err != nil || cc.ChannelID != channelID {
		return uuid.Nil
	}
	return cc.CustomerID
}
