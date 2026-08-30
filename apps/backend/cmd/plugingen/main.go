package main // regenerates plugins/firstparty from plugins/plugins.json - run from apps/backend, wired into bin/codegen

import (
	"fmt"
	"os"

	"github.com/skeletg/skelet/internal/pluginset"
)

func main() {
	if err := pluginset.Generate("."); err != nil {
		fmt.Fprintln(os.Stderr, "plugingen:", err)
		os.Exit(1)
	}
	if err := pluginset.WriteGraphSchemas("."); err != nil { // before gqlgen reads them - the schema list is the enabled set, not a directory glob
		fmt.Fprintln(os.Stderr, "plugingen:", err)
		os.Exit(1)
	}
}
