package main // rewrites the plugin-owned GraphQL resolvers and their interface declarations - run from apps/backend AFTER gqlgen, which owns the signatures this reads back

import (
	"fmt"
	"os"

	"github.com/skeletg/skelet/apps/backend/core/internal/pluginset"
)

func main() {
	if err := pluginset.GenerateGraph("."); err != nil {
		fmt.Fprintln(os.Stderr, "plugingraph:", err)
		os.Exit(1)
	}
}
