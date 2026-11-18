package main // rewrites the plugin-owned GraphQL resolvers and their interface declarations - run from apps/backend AFTER gqlgen, which owns the signatures this reads back

import (
	"flag"
	"fmt"
	"os"

	"github.com/skeletg/skelet/apps/backend/core/internal/pluginset"
)

func main() {
	prune := flag.Bool("prune", false, "the pass before gqlgen: clear what a departed plugin or a renamed field left behind")
	flag.Parse()
	run := pluginset.GenerateGraph
	if *prune {
		run = pluginset.PruneGraph
	}
	if err := run("."); err != nil {
		fmt.Fprintln(os.Stderr, "plugingraph:", err)
		os.Exit(1)
	}
}
