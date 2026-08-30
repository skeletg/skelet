package httpx

import (
	"sort"
	"strings"
)

// OriginSet is one plugin's declared browser-side origins, mirrored from sdk.SecurityOrigins so this
// package never imports the SDK
type OriginSet struct {
	Script       []string
	Connect      []string
	Frame        []string
	FormAction   []string
	PaymentAllow []string
}

// ComposeSecurityHeaders renders the CSP and Permissions-Policy from core's baseline plus every
// installed plugin's declared origins - cmd/secheadgen calls this with the compiled-in plugin set and
// rewrites the two constants in securityheaders.go (plus the nginx/caddy copies), so the values the
// middleware serves, the edge serves and contract-check compares all come from one composition
func ComposeSecurityHeaders(plugins []OriginSet) (csp, permissionsPolicy string) {
	var script, connect, frame, formAction, payment []string
	for _, p := range plugins {
		script = append(script, p.Script...)
		connect = append(connect, p.Connect...)
		frame = append(frame, p.Frame...)
		formAction = append(formAction, p.FormAction...)
		payment = append(payment, p.PaymentAllow...)
	}
	script = dedupeSorted(script)
	connect = dedupeSorted(connect)
	frame = dedupeSorted(frame)
	formAction = dedupeSorted(formAction)
	payment = dedupeSorted(payment)

	csp = "default-src 'self'; " +
		"script-src 'self' 'unsafe-inline' 'unsafe-eval'" + joined(script) + "; " +
		"style-src 'self' 'unsafe-inline'; " +
		"img-src 'self' data: blob: https:; " +
		"font-src 'self' data:; " +
		"connect-src 'self'" + joined(connect) + "; " +
		// frame-src with no origins must stay a valid directive - 'none' is CSP's explicit empty set
		"frame-src " + orNone(frame) + "; " +
		"frame-ancestors 'self'; base-uri 'self'; form-action 'self'" + joined(formAction)

	var pp strings.Builder
	pp.WriteString(`camera=(), microphone=(), geolocation=(), payment=(self`)
	for _, o := range payment {
		pp.WriteString(` "` + o + `"`)
	}
	pp.WriteString(`)`)
	return csp, pp.String()
}

func joined(origins []string) string {
	if len(origins) == 0 {
		return ""
	}
	return " " + strings.Join(origins, " ")
}

func orNone(origins []string) string {
	if len(origins) == 0 {
		return "'none'"
	}
	return strings.Join(origins, " ")
}

func dedupeSorted(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
