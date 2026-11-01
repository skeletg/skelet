package setup

import (
	"slices"
	"strings"

	"github.com/skeletg/skelet/cmd/skelet/internal/gitx"
	"github.com/skeletg/skelet/cmd/skelet/internal/plugins"
	"github.com/skeletg/skelet/cmd/skelet/internal/storefronts"
)

// What this checkout can be set up with
type catalog struct {
	root string
	// on disk, then any only in HEAD - one sparse-checkout removed stays pickable
	storefronts []storefronts.Storefront
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
	c := catalog{root: root, storefronts: storefronts.List(root)}
	if gitx.IsWorkTree(root) {
		c.storefronts = storefronts.Known(root)
	}
	var err error
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

func (c catalog) apps() []string {
	out := []string{"dashboard"}
	for _, s := range c.storefronts {
		out = append(out, s.Name)
	}
	return out
}

// templateNames is the templates a shop can be copied from
func (c catalog) templateNames() []string {
	var out []string
	for _, s := range c.storefronts {
		if s.Template() {
			out = append(out, s.Name)
		}
	}
	return out
}

// ownStorefront says the project has a storefront of its own already
func (c catalog) ownStorefront() bool {
	return slices.ContainsFunc(c.storefronts, func(s storefronts.Storefront) bool { return !s.Template() })
}

// defaultApps is what a clone runs unless told otherwise: the dashboard and the project's own storefronts. A template is
// what a shop is copied from - running it is for working on the template itself, so it's left out until asked for
func (c catalog) defaultApps() []string {
	out := []string{"dashboard"}
	for _, s := range c.storefronts {
		if !s.Template() {
			out = append(out, s.Name)
		}
	}
	return out
}

// appDir is where an app lives: on disk, or where HEAD has it when sparse-checkout took it out of the working tree
func appDir(root, app string) string {
	if app == "dashboard" {
		return "apps/dashboard"
	}
	if s, ok := storefronts.Find(root, app); ok {
		return s.Dir
	}
	for _, home := range storefronts.Homes() {
		if slices.Contains(gitx.TreeDirs(root, home), app) {
			return home + "/" + app
		}
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
