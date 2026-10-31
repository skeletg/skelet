package stack

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/skeletg/skelet/cmd/skelet/internal/compose"
	"github.com/skeletg/skelet/cmd/skelet/internal/envfile"
)

// A port the next up would publish
type binding struct {
	service string
	target  int
	port    int
}

// The .env knob that moves a published host port, for the loopback-only datastores - nothing on the host dials those,
// so any port works. The rest are baked into origins and CSP and stay where they are
var portVars = map[string]string{
	"db:5432": "DB_HOST_PORT", "signals-db:5432": "SIGNALS_DB_HOST_PORT", "valkey:6379": "VALKEY_HOST_PORT",
	"clickhouse:8123": "CLICKHOUSE_HTTP_HOST_PORT", "clickhouse:9000": "CLICKHOUSE_HOST_PORT",
	"opensearch:9200": "OPENSEARCH_HOST_PORT", "nats:4222": "NATS_HOST_PORT", "signals:8090": "SIGNALS_HOST_PORT",
	"garage:3903": "GARAGE_ADMIN_HOST_PORT", "grafana:3000": "GRAFANA_HOST_PORT",
}

func (b binding) knob() string { return portVars[b.service+":"+strconv.Itoa(b.target)] }

// What the next up would publish - compose resolves the profiles and the .env overrides, so this is what will try to
// bind. Nothing when compose can't say, and up then reports why
func published(root string, lib compose.Lib) []binding {
	out, err := compose.Command(root, lib.Env(), "config", "--format", "json").Output()
	if err != nil {
		return nil
	}
	var cfg struct {
		Services map[string]struct {
			Ports []struct {
				Target    int
				Published string
			}
		}
	}
	if json.Unmarshal(out, &cfg) != nil {
		return nil
	}
	var bs []binding
	for name, svc := range cfg.Services {
		for _, p := range svc.Ports {
			if n, err := strconv.Atoi(p.Published); err == nil && n > 0 {
				bs = append(bs, binding{name, p.Target, n})
			}
		}
	}
	slices.SortFunc(bs, func(a, b binding) int {
		if c := strings.Compare(a.service, b.service); c != 0 {
			return c
		}
		return a.target - b.target
	})
	return bs
}

var (
	hostPort = regexp.MustCompile(`:([0-9]+)->`)
	digits   = regexp.MustCompile(`^[0-9]+$`)
)

// ours is the host ports this project's running containers hold, so re-running on a live stack passes
func ours(root string, lib compose.Lib) map[int]bool {
	out, _ := compose.Command(root, lib.Env(), "ps", "--format", "{{.Ports}}").Output()
	held := map[int]bool{}
	for _, m := range hostPort.FindAllStringSubmatch(string(out), -1) {
		n, _ := strconv.Atoi(m[1])
		held[n] = true
	}
	return held
}

// inUse says something on this machine answers on the port
var inUse = func(port int) bool {
	c, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 500*time.Millisecond)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

// holder is who holds a port, as far as can be told: a container by name, else a pointer at the tool that knows
func holder(port int) string {
	out, _ := exec.Command("docker", "ps", "--filter", "publish="+strconv.Itoa(port), "--format", "{{.Names}}").Output()
	if names := strings.Fields(string(out)); len(names) > 0 {
		return fmt.Sprintf("container %s (docker stop %s)", names[0], names[0])
	}
	return fmt.Sprintf("a program on this machine (find it: lsof -nP -iTCP:%d -sTCP:LISTEN)", port)
}

// prompter reads answers from a terminal; off, preflight only lists what's in the way
type prompter struct {
	in io.Reader
	on bool
	r  *bufio.Reader
}

// line is the next answer, trimmed - ok is false once input ends, which means stop asking
func (p *prompter) line(prompt string) (string, bool) {
	if p.r == nil {
		p.r = bufio.NewReader(p.in)
	}
	fmt.Print(prompt)
	s, err := p.r.ReadString('\n')
	if err != nil && s == "" {
		fmt.Println()
		return "", false
	}
	return strings.TrimSpace(s), true
}

// preflight refuses to start into a port something else holds - compose only finds out halfway through bringing the
// stack up, and a local Postgres on 5432 is the usual one. On a terminal it asks until every port is free; otherwise
// it lists them and fails
func preflight(root string, lib compose.Lib, ask prompter) bool {
	for {
		held := ours(root, lib)
		taken := map[int]bool{}
		var busy []binding
		for _, b := range published(root, lib) {
			taken[b.port] = true
			if !held[b.port] && inUse(b.port) {
				busy = append(busy, b)
			}
		}
		if len(busy) == 0 {
			return true
		}
		fmt.Fprintln(os.Stderr, "==> Ports the stack needs are already taken:")
		for _, b := range busy {
			fmt.Fprintf(os.Stderr, "    %-6d %-14s held by %s\n", b.port, b.service, holder(b.port))
		}
		if !ask.on {
			for _, b := range busy {
				if v := b.knob(); v != "" {
					fmt.Fprintf(os.Stderr, "    free :%d, or move %s with %s=<port> in .env\n", b.port, b.service, v)
				} else {
					fmt.Fprintf(os.Stderr, "    free :%d - %s has to have it\n", b.port, b.service)
				}
			}
			return false
		}
		for _, b := range busy {
			if !inUse(b.port) {
				continue
			}
			moved, ok := askAbout(root, b, taken, &ask)
			if !ok {
				return false
			}
			if moved != 0 {
				taken[moved] = true
			}
		}
		// Round again - everything is re-probed, so "I stopped it" when nothing stopped asks again
	}
}

// askAbout offers to wait for a held port to be freed or to move the service off it. moved is the new port, or 0;
// ok is false to quit
func askAbout(root string, b binding, taken map[int]bool, ask *prompter) (moved int, ok bool) {
	knob := b.knob()
	fmt.Println()
	fmt.Printf("    :%d (%s) is in use.\n", b.port, b.service)
	fmt.Println("      1) I stopped it - check again")
	if knob != "" {
		fmt.Printf("      2) Use a different port for %s\n", b.service)
		fmt.Println("      3) Quit")
	} else {
		fmt.Printf("      2) Quit   (%s can't move - its port is part of the app's URLs)\n", b.service)
	}
	reply, ok := ask.line("    pick: ")
	switch {
	case !ok:
		return 0, false
	case knob == "" && reply == "2", knob != "" && reply == "3":
		return 0, false
	case knob == "" || reply != "2":
		return 0, true
	}
	// A suggestion that's free now and that nothing else in the stack publishes - this round's moves included
	next := b.port + 10
	for inUse(next) || taken[next] {
		next++
	}
	for {
		reply, ok := ask.line(fmt.Sprintf("    port for %s [%d]: ", b.service, next))
		if !ok {
			return 0, false
		}
		if reply == "" {
			reply = strconv.Itoa(next)
		}
		n, err := strconv.Atoi(reply)
		switch {
		case !digits.MatchString(reply) || err != nil:
			fmt.Println("    a number, please")
		case n < 1024 || n > 65535:
			fmt.Println("    pick one between 1024 and 65535")
		case inUse(n) || taken[n]:
			fmt.Printf("    :%d is taken too\n", n)
		default:
			if err := envfile.Set(filepath.Join(root, ".env"), knob, reply); err != nil {
				fmt.Fprintln(os.Stderr, "   ", err)
				return 0, false
			}
			// The environment beats .env in compose, so a stale export would undo the write
			_ = os.Setenv(knob, reply)
			fmt.Printf("    %s=%s written to .env - %s will publish on 127.0.0.1:%s\n", knob, reply, b.service, reply)
			return n, true
		}
	}
}
