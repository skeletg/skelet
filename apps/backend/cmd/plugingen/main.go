package main // regenerates plugins/firstparty from plugins/plugins.json - run from apps/backend, wired into bin/codegen

import (
	"fmt"
	"os"

	"github.com/skeletg/skelet/apps/backend/internal/pluginset"
)

func main() {
	// Staging first: a module plugin's SDL has to be on disk at a stable path before WriteGraphSchemas
	// can point gqlgen at it
	if err := pluginset.Stage("."); err != nil {
		fmt.Fprintln(os.Stderr, "plugingen:", err)
		os.Exit(1)
	}
	if err := pluginset.Generate("."); err != nil {
		fmt.Fprintln(os.Stderr, "plugingen:", err)
		os.Exit(1)
	}
	if err := pluginset.WriteGraphSchemas("."); err != nil { // before gqlgen reads them - the schema list is the enabled set, not a directory glob
		fmt.Fprintln(os.Stderr, "plugingen:", err)
		os.Exit(1)
	}
}
