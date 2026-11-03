// Package profile reads a clone's setup from its root .env, and parses (never sources) the answers file setup --profile replays
package profile

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/skeletg/skelet/cmd/skelet/internal/envfile"
)

const (
	AppsKey   = "SKELET_APPS_SELECTED"
	DockerKey = "SKELET_DOCKER_APPS"
	ToolsKey  = "SKELET_TOOLS_SELECTED"
	TargetKey = "SKELET_TARGET"
)

const (
	EnvTarget   = "SKELET_TARGET"
	EnvApps     = "SKELET_APPS"
	EnvProfiles = "COMPOSE_PROFILES"
)

var DevTools = []string{"mailpit", "webhook-tester", "grafana", "garage-webui"}

var infra = []string{"bundled", "edge", "ingest", "media", "queue", "search", "signals", "worker"}

var signalsOff = regexp.MustCompile(`(?im)^[ \t]*SIGNALS_ENABLED[ \t]*=[ \t]*(false|0|f|no|off)[ \t]*$`)

func EnvPath(root string) string { return filepath.Join(root, ".env") }

func SignalsOn(root string) bool {
	goEnv, _ := os.ReadFile(filepath.Join(root, "env", "go.env"))
	return !signalsOff.Match(goEnv)
}

// Profile maps a key to its value. A key that is present but empty differs from one that is absent -
// SKELET_APPS_SELECTED= means backend only, a missing one means setup hasn't asked yet
type Profile map[string]string

func Current(root string) Profile {
	env := EnvPath(root)
	p := Profile{}
	if v, ok, _ := envfile.Get(env, EnvTarget); ok {
		p[TargetKey] = v
	}
	if v, ok, _ := envfile.Get(env, EnvApps); ok {
		p[AppsKey] = v
	}
	if v, ok, _ := envfile.Get(env, EnvProfiles); ok {
		var docker, tools []string
		for _, name := range split(v) {
			switch {
			case slices.Contains(DevTools, name):
				tools = append(tools, name)
			case !slices.Contains(infra, name):
				docker = append(docker, name)
			}
		}
		p[DockerKey], p[ToolsKey] = strings.Join(docker, ","), strings.Join(tools, ",")
	}
	return p
}

// DevProfiles takes in the sidecars skelet up adds itself, so a bare docker compose up starts the same stack
func DevProfiles(docker, tools []string, signals bool) string {
	p := append(append([]string{"search"}, docker...), tools...)
	if signals {
		p = append(p, "signals")
	}
	return strings.Join(p, ",")
}

// Load reads path; a missing file is an empty profile
func Load(path string) (Profile, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Profile{}, nil
	}
	if err != nil {
		return nil, err
	}
	p := Profile{}
	for line := range strings.Lines(string(raw)) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(strings.TrimPrefix(line, "export "), "=")
		if !ok {
			continue
		}
		if len(val) >= 2 && (val[0] == '"' || val[0] == '\'') && val[len(val)-1] == val[0] {
			val = val[1 : len(val)-1]
		}
		p[strings.TrimSpace(key)] = val
	}
	return p, nil
}

func (p Profile) Get(key string) (string, bool) {
	v, ok := p[key]
	return v, ok
}

func (p Profile) List(key string) ([]string, bool) {
	v, ok := p[key]
	return split(v), ok
}

func (p Profile) Apps() ([]string, bool) { return p.List(AppsKey) }

func split(v string) []string {
	var items []string
	for item := range strings.SplitSeq(v, ",") {
		if item = strings.TrimSpace(item); item != "" {
			items = append(items, item)
		}
	}
	return items
}
