// Command secheadgen composes the Content-Security-Policy and Permissions-Policy from core's
// baseline plus every compiled-in plugin's Manifest.SecurityOrigins, then rewrites the constants
// in internal/httpx/securityheaders.go, the dashboard's nginx template and the Caddy edge config -
// so installing a gateway plugin updates the headers everywhere through bin/codegen instead of a
// hand edit in core
// The blank firstparty import is the data source: it runs every enabled plugin's init(), whose
// manifests sdk.RegisteredManifests() then reports - no host is stood up
// A target file that is absent is skipped (forks drop the edge profile or the dashboard),
// but a present file where a header line can't be found fails loudly - a rewrite that
// quietly matches nothing is how a generator rots into a no-op
package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/skeletg/skelet/apps/backend/internal/httpx"
	"github.com/skeletg/skelet/apps/backend/sdk"

	_ "github.com/skeletg/skelet/apps/backend/plugins/firstparty"
)

type sub struct {
	// two capture groups: everything before the value and everything after it
	re    *regexp.Regexp
	value string
}

type target struct {
	path string
	subs []sub
}

func main() {
	root := "../.."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	var origins []httpx.OriginSet
	for _, m := range sdk.RegisteredManifests() {
		if m.SecurityOrigins == nil {
			continue
		}
		o := m.SecurityOrigins
		origins = append(origins, httpx.OriginSet{
			Script:       o.Script,
			Connect:      o.Connect,
			Frame:        o.Frame,
			FormAction:   o.FormAction,
			PaymentAllow: o.PaymentAllow,
		})
	}
	rawCSP, rawPP := httpx.ComposeSecurityHeaders(origins)

	// both edge formats escape a literal double quote the same way; the Go const targets get raw values
	csp := strings.ReplaceAll(rawCSP, `"`, `\"`)
	pp := strings.ReplaceAll(rawPP, `"`, `\"`)

	targets := []target{
		{root + "/apps/backend/internal/httpx/securityheaders.go", []sub{
			{regexp.MustCompile(`^(const ContentSecurityPolicy = ").*(")$`), strings.ReplaceAll(rawCSP, `"`, `\"`)},
			{regexp.MustCompile("^(const PermissionsPolicy = `).*(`)$"), rawPP},
		}},
		{root + "/apps/dashboard/nginx.conf.template", []sub{
			{regexp.MustCompile(`^(\s*add_header Content-Security-Policy ").*(" always;)$`), csp},
			{regexp.MustCompile(`^(\s*add_header Permissions-Policy ").*(" always;)$`), pp},
		}},
		{root + "/deploy/caddy/Caddyfile", []sub{
			{regexp.MustCompile(`^(\s*Content-Security-Policy ").*(")\s*$`), csp},
			{regexp.MustCompile(`^(\s*Permissions-Policy ").*(")\s*$`), pp},
		}},
	}
	failed := false
	for _, t := range targets {
		if err := rewrite(t); err != nil {
			fmt.Fprintf(os.Stderr, "secheadgen: %s: %v\n", t.path, err)
			failed = true
		}
	}
	if failed {
		os.Exit(1)
	}
}

func rewrite(t target) error {
	src, err := os.ReadFile(t.path)
	if os.IsNotExist(err) {
		fmt.Printf("    (skipped - %s not in this checkout)\n", t.path)
		return nil
	}
	if err != nil {
		return err
	}
	lines := strings.Split(string(src), "\n")
	for _, s := range t.subs {
		hits := 0
		for i, ln := range lines {
			m := s.re.FindStringSubmatch(ln)
			if m == nil {
				continue
			}
			lines[i] = m[1] + s.value + m[2]
			hits++
		}
		if hits != 1 {
			return fmt.Errorf("found %d lines matching %s, want exactly 1", hits, s.re)
		}
	}
	out := strings.Join(lines, "\n")
	if out == string(src) {
		return nil
	}
	return os.WriteFile(t.path, []byte(out), 0o644)
}
