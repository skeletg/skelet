package setup

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/skeletg/skelet/cmd/skelet/internal/gitx"
	"github.com/skeletg/skelet/cmd/skelet/internal/plugins"
)

// What this checkout can be set up with
type catalog struct {
	// on disk, then any only in HEAD - one sparse-checkout removed stays pickable
	storefronts []string
	// the payment plugins the tree carries, so the picker never offers one that can't compile
	gateways []string
	// every plugin directory that registers, for plugins.json's decisions
	plugins []string
}

// Optional dev tools, each a compose profile of its own name - the stack runs without any of them
var tools = []string{"mailpit", "webhook-tester", "grafana", "garage-webui"}

var toolHints = map[string]string{
	"mailpit":        "Mailpit - catches outgoing email, UI on :8025 (without it, mail prints to the app log)",
	"webhook-tester": "Webhook tester - catches outbound webhooks, UI on :8081",
	"grafana":        "Grafana - traces, metrics and container logs, UI on :3030",
	"garage-webui":   "Garage web UI - object storage console on :9901",
}

func scan(root string) (catalog, error) {
	var c catalog
	entries, err := os.ReadDir(filepath.Join(root, "apps", "channels"))
	if err != nil && !os.IsNotExist(err) {
		return c, err
	}
	for _, e := range entries {
		if e.IsDir() {
			c.storefronts = append(c.storefronts, e.Name())
		}
	}
	if gitx.IsWorkTree(root) {
		for _, n := range gitx.TreeDirs(root, "apps/channels") {
			if !slices.Contains(c.storefronts, n) {
				c.storefronts = append(c.storefronts, n)
			}
		}
	}
	if c.plugins, err = plugins.Discover(root); err != nil {
		return c, err
	}
	for _, p := range c.plugins {
		if plugins.IsGateway(p) {
			c.gateways = append(c.gateways, p)
		}
	}
	return c, nil
}

func (c catalog) apps() []string { return append([]string{"dashboard"}, c.storefronts...) }

func appDir(app string) string {
	if app == "dashboard" {
		return "apps/dashboard"
	}
	return "apps/channels/" + app
}

// split reads a comma list the way the bash setup did - empty items dropped, duplicates kept once
func split(s string) []string {
	var out []string
	for v := range strings.SplitSeq(s, ",") {
		if v = strings.TrimSpace(v); v != "" && !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}

func join(list []string) string { return strings.Join(list, ",") }
