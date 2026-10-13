package channel

import (
	"net"
	"net/url"
	"strings"
)

const (
	maxHostnameLength = 253
	maxHostPortLength = maxHostnameLength + len(":65535")
)

// The routing key for a public URL - the same normalization the Host header goes through, so the two compare as strings
func storefrontHost(publicURL string) string {
	raw := strings.TrimSpace(publicURL)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		if u, err = url.Parse("//" + raw); err != nil || u.Host == "" {
			return ""
		}
	}
	return normalizeHost(strings.ToLower(u.Scheme), u.Host)
}

// Lowercases a host[:port] and drops the scheme's default port - an unknown scheme drops both 80 and 443
func normalizeHost(scheme, hostport string) string {
	hostport = strings.ToLower(strings.TrimSpace(hostport))
	if hostport == "" || len(hostport) > maxHostPortLength {
		return ""
	}
	host, port, err := net.SplitHostPort(hostport)
	if err != nil {
		host, port = strings.TrimSuffix(strings.TrimPrefix(hostport, "["), "]"), ""
	}
	if host == "" {
		return ""
	}
	if (port == "80" && scheme != "https") || (port == "443" && scheme != "http") {
		port = ""
	}
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if port != "" {
		return host + ":" + port
	}
	return host
}

// Empty is allowed; anything else must be an absolute http(s) URL with a host and nothing a link builder would trip over
func normalizePublicURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if strings.ContainsAny(raw, "?#") {
		return "", ErrInvalidPublicURL
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Opaque != "" || u.User != nil {
		return "", ErrInvalidPublicURL
	}
	if h := u.Hostname(); h == "" || len(h) > maxHostnameLength {
		return "", ErrInvalidPublicURL
	}
	return raw, nil
}
