package gen

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/skeletg/skelet/cmd/skelet/internal/compose"
	"github.com/skeletg/skelet/cmd/skelet/internal/container"
	"github.com/skeletg/skelet/cmd/skelet/internal/sys"
)

const (
	SqlcAbout = `sqlc for both modules with queries, each from its own sqlc.yaml under apps/backend/<module> - generate
when given nothing, or vet, diff, compile. skelet gen runs it with the rest. The image is pinned, since another
sqlc rewrites every generated file's header, and it runs as you, so what it writes is yours.`
	GqlgenAbout = `gqlgen in the app container, for one config: skelet gen gqlgen generate --config gqlgen.admin.yml. With
nothing, both surfaces, as skelet gen runs them.`
	BufAbout = `buf in the app container, run from core's module, where go tool resolves it, with the protoc plugins built
from go.mod's versions: skelet gen buf breaking /var/www/proto --against ... With nothing, what skelet gen does:
lint the contract, then generate into the backend.`
)

const sqlcImage = "sqlc/sqlc:1.31.1"

func Sqlc(root string, args []string) int {
	sys.HandleInterrupts()
	return code(sqlc(root, args...))
}

func Gqlgen(root string, args []string) int {
	sys.HandleInterrupts()
	if len(args) == 0 {
		return code(gqlgenBoth(root))
	}
	return gqlgen(root, args...).Replace()
}

func Buf(root string, args []string) int {
	sys.HandleInterrupts()
	if len(args) == 0 {
		return code(bufBoth(root))
	}
	return buf(root, args...).Replace()
}

func code(err error) int {
	if err != nil {
		return max(sys.ExitCode(err), 1)
	}
	return 0
}

// sqlc runs in its own image against the bind-mounted tree. As root, which the image defaults to, every file it wrote
// was root's
func sqlc(root string, args ...string) error {
	if len(args) == 0 {
		args = []string{"generate"}
	}
	// sqlc writes its outputs but never removes one, so a deleted query would leave its Go behind where --check can't
	// see it - its own files go first. That also clears files an older sqlc run left owned by root
	if args[0] == "generate" {
		if err := clearSqlc(root); err != nil {
			return err
		}
	}
	for _, mod := range []string{"backend/core", "backend/signals"} {
		fmt.Printf("==> sqlc %s (%s)\n", strings.Join(args, " "), mod)
		argv := append(append(append([]string{"docker", "run", "--rm"}, compose.RunAs()...), compose.UserFlags()...),
			"-v", filepath.Join(root, "apps")+":/src", "-w", "/src/"+mod, sqlcImage)
		call := sys.Call{Dir: root, Path: sys.Lookup("docker"), Argv: append(argv, args...), Env: sys.Env()}
		if err := call.Run(); err != nil {
			return err
		}
	}
	return nil
}

func appTool(root string, cmd ...string) sys.Call {
	return container.In(root, compose.Target{Service: "app", NoDeps: true, Flags: compose.UserFlags()}, cmd...)
}

func gqlgen(root string, args ...string) sys.Call {
	return appTool(root, append([]string{"go", "tool", "github.com/99designs/gqlgen"}, args...)...)
}

func gqlgenBoth(root string) error {
	if err := gqlgen(root, "generate", "--config", "gqlgen.admin.yml").Run(); err != nil {
		return err
	}
	return gqlgen(root, "generate", "--config", "gqlgen.front.yml").Run()
}

// The codegen plugins are go tool dependencies too, built into a cache directory on the PATH so go.mod pins them.
// The arguments reach buf as the script's own, never spliced into its text
const bufScript = `set -e
mkdir -p /tmp/protoc-plugins
for p in google.golang.org/protobuf/cmd/protoc-gen-go google.golang.org/grpc/cmd/protoc-gen-go-grpc; do
	out=/tmp/protoc-plugins/$(basename "$p")
	[ -x "$out" ] || go build -o "$out" "$p"
done
PATH=/tmp/protoc-plugins:$PATH exec go tool github.com/bufbuild/buf/cmd/buf "$@"`

func buf(root string, args ...string) sys.Call {
	return appTool(root, append([]string{"sh", "-c", bufScript, "buf"}, args...)...)
}

func bufBoth(root string) error {
	if err := buf(root, "lint", "/var/www/proto").Run(); err != nil {
		return err
	}
	return buf(root, "generate", "--template", "/var/www/proto/buf.gen.yaml", "/var/www/proto").Run()
}
