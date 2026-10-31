package gates

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func put(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// Every pin site the gate reads, each pin written the different ways the real files write it
func pinTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	put(t, root, map[string]string{
		"docker-compose.yml":       "services:\n  db:\n    image: postgres:18-alpine\n  ch:\n    image: clickhouse/clickhouse-server:26.7.5\n  mig:\n    image: amacneil/dbmate/v2@v2.36.0\n",
		"docker-compose.small.yml": "services:\n  db:\n    image: postgres:18\n",
		"docker-compose.prod.yml":  "services:\n  caddy:\n    image: caddy:2\n",
		".github/workflows/ci.yml": "jobs:\n  x:\n    services:\n      postgres:\n        image: postgres:18\n    steps:\n" +
			"      - run: go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0 run\n" +
			"      - uses: actions/setup-node@v4\n        with:\n          node-version:   24\n",
		"bin/lint":                          "GCL=(go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0)\n",
		"apps/backend/core/Dockerfile.prod": "# builder on alpine3.24\nFROM golang:1.27.3-alpine3.24 AS build\nRUN go install github.com/amacneil/dbmate/v2@v2.36.0\nFROM alpine:3.24\n",
		"apps/dashboard/Dockerfile":         "FROM node:24-alpine\nRUN npm install -g pnpm@12.6.0\n",
		"apps/dashboard/package.json":       `{"packageManager": "pnpm@12.6.0"}` + "\n",
		"apps/channels/shop/package.json":   `{"packageManager": "pnpm@12.6.0"}` + "\n",
		// deeper than three levels under apps/, so out of the gate's reach, as in the bash
		"apps/channels/shop/e2e/fixtures/package.json": `{"packageManager": "pnpm@1.0.0"}` + "\n",
		".nvmrc":                   " 24\n",
		"apps/backend/core/go.mod": "module x\n\ngo 1.27.3\n",
		"cmd/skelet/go.mod":        "module y\n\ngo 1.27.3\n",
	})
	return root
}

// What the bash pin-check printed for these trees before it was retired
func TestPinsHealthy(t *testing.T) {
	var out bytes.Buffer
	want := `==> version pins agree across files
  ok    postgres         18
  ok    clickhouse       26.7.5
  ok    dbmate           2.36.0
  ok    golangci         2.14.0
  ok    node             24
  ok    alpine           3.24
  ok    caddy            2
  ok    pnpm             12.6.0
==> .nvmrc matches the node pin
  ok    .nvmrc           24
==> golang images match go.mod
  ok    go               1.27.3

==> pin-check OK
`
	if code := pinCheck(pinTree(t), &out); code != 0 || out.String() != want {
		t.Errorf("exit %d\n%s\nwant:\n%s", code, out.String(), want)
	}
}

func TestPinsBroken(t *testing.T) {
	root := pinTree(t)
	put(t, root, map[string]string{
		"docker-compose.small.yml": "services:\n  db:\n    image: postgres:17\n",
		"docker-compose.prod.yml":  "services:\n  caddy:\n    image: caddy:latest\n",
		".nvmrc":                   " 22\n",
		"cmd/skelet/go.mod":        "module y\n\ngo 1.28.0\n",
	})
	want := `==> version pins agree across files
  FAIL  postgres         17 18
          docker-compose.yml:3:    image: postgres:18-alpine
          docker-compose.small.yml:3:    image: postgres:17
          .github/workflows/ci.yml:5:        image: postgres:18
  ok    clickhouse       26.7.5
  ok    dbmate           2.36.0
  ok    golangci         2.14.0
  ok    node             24
  ok    alpine           3.24
  FAIL  caddy            no pin found - did the pattern or the files move?
  ok    pnpm             12.6.0
==> .nvmrc matches the node pin
  FAIL  .nvmrc           .nvmrc says 22, the images and workflows say 24
==> golang images match go.mod
  FAIL  go               go.mod files say 1.27.3 1.28.0, golang images say 1.27.3
          apps/backend/core/go.mod:3:go 1.27.3
          cmd/skelet/go.mod:3:go 1.28.0
          apps/backend/core/Dockerfile.prod:2:FROM golang:1.27.3-alpine3.24 AS build

==> pin-check FAILED - make the listed sites agree, or update cmd/skelet/internal/gates/pins.go if a pin moved
`
	var out bytes.Buffer
	if code := pinCheck(root, &out); code != 1 || out.String() != want {
		t.Errorf("exit %d\n%s\nwant:\n%s", code, out.String(), want)
	}
}

func TestNormalize(t *testing.T) {
	for in, want := range map[string]string{
		"postgres:18-alpine": "18", "golangci-lint@v2.14.0": "2.14.0", "node-version:   24": "24",
		"alpine3.24": "3.24", "alpine:3.24": "3.24", "go 1.27.3": "1.27.3", "pnpm@12.6.0": "12.6.0",
	} {
		if got := normalize(in); got != want {
			t.Errorf("normalize(%q) = %q, want %q", in, got, want)
		}
	}
	if strings.Contains(normalize("FROM golang:1.27.3"), "golang") {
		t.Error("an image tag kept its name")
	}
}
