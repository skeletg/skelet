package route

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/skeletg/skelet/cmd/skelet/internal/stack"
)

var update = flag.Bool("update", false, "rewrite testdata/usage.golden")

func TestUsageGolden(t *testing.T) {
	if *update {
		if err := os.WriteFile("testdata/usage.golden", []byte(Usage()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile("testdata/usage.golden")
	if err != nil {
		t.Fatal(err)
	}
	if Usage() != string(want) {
		t.Errorf("the usage screen changed - review it, then go test ./internal/route -update\n%s", Usage())
	}
}

func TestHeader(t *testing.T) {
	cases := []struct{ script, want string }{
		{"#!/usr/bin/env bash\n# Line one\n#  indented\n#\n# after a blank\nset -e\n# not this\n", "Line one\n indented\n\nafter a blank"},
		{"#!/usr/bin/env bash\n# Top\n\nfoo() { :; }\n# still header\nset -e\n", "Top\nstill header"},
		{"#!/usr/bin/env bash\nset -e\n", ""},
		{"#!/bin/sh\n#no space\n", "no space"},
	}
	for _, c := range cases {
		if got := Header(c.script); got != c.want {
			t.Errorf("Header(%q) = %q, want %q", c.script, got, c.want)
		}
	}
}

func TestHelp(t *testing.T) {
	root := fakeRoot(t)
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "new"), []byte("#!/usr/bin/env bash\n# Forks a storefront\nset -e\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "dev"), []byte("#!/usr/bin/env bash\nset -e\ndocker compose up\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cases := []struct{ verb, want string }{
		{"add", "skelet add   channel <name> [--from <storefront>] [--tracked]\n\nruns bin/new\n\nForks a storefront\n"},
		{"new", "skelet new   channel <name> [--from <storefront>] [--tracked]\n\nruns bin/new\n\nForks a storefront\n"},
		{"up", "runs bin/dev\n\n(no description - it is 3 lines, just read bin/dev)\n"},
		{"down", stack.DownAbout + "\n"},
		{"pnpm", "skelet pnpm   <app> <pnpm args>\n\n"},
		{"db", "skelet db   " + subs["db"] + "\n\n"},
	}
	for _, c := range cases {
		if a := Help(root, c.verb); a.Stdout != c.want || a.Code != 0 {
			t.Errorf("help %s = %+v, want %q", c.verb, a, c.want)
		}
	}
	if a := Help(root, "nope"); a.Code != 2 || a.Stderr != `Unknown command "nope" - try: skelet help` {
		t.Errorf("help nope = %+v", a)
	}
}
