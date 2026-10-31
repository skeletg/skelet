// Package doctor is skelet doctor: what's wrong with this machine and this checkout, said plainly instead of found
// later as a cryptic failure. Every line is ok, warn or FAIL, and only a FAIL makes it exit non-zero
package doctor

import (
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

const About = `Checks this machine and this checkout and says what's wrong in plain words: Docker and compose,
git and Go on the host, the env files the target needs, where the frontends run and whether pnpm can
run the host ones, the profile against the env files, plugins.json against the plugin directories,
the running containers and the Signals spine, the ports and the backend's /health, and free disk.
Each line is ok, warn or FAIL, and only a FAIL fails the command. Outside a project, the installed
skelet doctor checks just the machine.`

// The network as doctor sees it - swapped in the tests, which can't own ports 8080 and up
type probes struct {
	// open reports whether something accepts a connection on localhost:port
	open func(port string) bool
	get  func(url string) (status int, body string, err error)
}

var live = probes{
	open: func(port string) bool {
		c, err := net.DialTimeout("tcp", net.JoinHostPort("localhost", port), 2*time.Second)
		if err == nil {
			_ = c.Close()
		}
		return err == nil
	},
	get: func(url string) (int, string, error) {
		resp, err := (&http.Client{Timeout: 3 * time.Second}).Get(url)
		if err != nil {
			return 0, "", err
		}
		defer func() { _ = resp.Body.Close() }()
		body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return resp.StatusCode, strings.TrimSpace(string(body)), err
	},
}

// Run checks the machine and the checkout at root
func Run(root string, _ []string) int {
	return checkout(root, os.Stdout, live)
}

// Machine is the installed binary's doctor outside a project: only what this machine needs before skelet new
func Machine(_ []string) int { return machine(os.Stdout) }

func machine(out io.Writer) int {
	r := &report{out: out}
	docker(r)
	tools(r, "")
	cwd, _ := os.Getwd()
	disk(r, cwd)
	return r.end()
}
