package setup

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/skeletg/skelet/cmd/skelet/internal/envfile"
	"github.com/skeletg/skelet/cmd/skelet/internal/plugins"
	"github.com/skeletg/skelet/cmd/skelet/internal/profile"
)

var switchedOff = regexp.MustCompile(`(?i)^\s*(false|0|f|no|off)\s*$`)

// current is setup's defaults read back off the files the answers became, so there's no second copy to drift
func current(root string, c catalog) profile.Profile {
	p := profile.Current(root)
	backend := "env/go.env"
	if t := p[kTarget]; t != "" && t != "dev" {
		backend = "env/" + t + ".env"
	}
	for key, answer := range map[string]string{"SIGNALS_ENABLED": kSignals, "AI_ENABLED": kAI} {
		if v, ok, _ := envfile.Get(filepath.Join(root, backend), key); ok {
			p[answer] = strconv.FormatBool(!switchedOff.MatchString(v))
		}
	}
	env := profile.EnvPath(root)
	if _, err := os.Stat(env); err == nil {
		p[kSearch] = "opensearch"
		if v, ok, _ := envfile.Get(env, "OPENSEARCH_URL"); ok && v == "" {
			p[kSearch] = "postgres"
		}
		slug, _, _ := envfile.Get(env, "ADMIN_API_SLUG")
		p[kSlug] = map[bool]string{true: "1", false: "0"}[slug != ""]
	}
	if set, before, err := plugins.Load(root); err == nil && before != nil {
		var on []string
		for _, g := range c.gateways {
			if set.Plugins[g].Enabled {
				on = append(on, g)
			}
		}
		p[kGates] = join(on)
	}
	return p
}
