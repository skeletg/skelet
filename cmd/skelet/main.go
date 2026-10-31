// Command skelet is the front door for a Skelet checkout, and the installer that creates one
package main

import (
	"os"

	"github.com/skeletg/skelet/cmd/skelet/internal/app"
	"github.com/skeletg/skelet/cmd/skelet/internal/sys"
)

// Set with -ldflags -X. The launcher builds a project's own CLI with mode=checkout; anything else is the released binary
var (
	version = "dev"
	mode    = "global"
)

func main() {
	sys.Exit(app.Main(app.Config{Version: version, Mode: mode}, os.Args[1:]))
}
