package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/skeletg/skelet/apps/backend/core/internal/datapatch"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/dbconn"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"

	_ "github.com/skeletg/skelet/apps/backend/core/internal/datapatch/patches" // core patches register via init() - plugin patches come through the SDK when linked in
)

// The server runs the patches as it boots, after core and plugin schema, so this is for inspecting
// what ran and for applying by hand - the Long text below covers the rest
var DataPatchCmd = &cobra.Command{
	Use:   "data-patch",
	Short: "Apply or inspect run-once code-based data patches",
	Long: `Code-based data patches transform data on deploy when the change needs Go
that SQL can't express - walking a block tree to build a search vector,
encrypting with a key from the environment, anything reading a code-defined
registry. Each runs at most once (tracked in data_patches).

Reference data does NOT belong here - it goes in the reference-data migration,
which is why core ships no patches. Plugins register theirs through the SDK.

The server applies pending patches as it boots, after core's and the
plugins' schema, so a normal deploy needs nothing from you. Run it by hand
after 'skelet cli plugin migrate up' if you have applied plugin schema separately.`,
}

var dataPatchUpCmd = &cobra.Command{
	Use:          "up",
	Short:        "Apply all pending data patches",
	SilenceUsage: true,
	RunE:         runDataPatchUp,
}

var dataPatchStatusCmd = &cobra.Command{
	Use:          "status",
	Short:        "Show applied and pending data patches",
	SilenceUsage: true,
	RunE:         runDataPatchStatus,
}

func init() {
	DataPatchCmd.AddCommand(dataPatchUpCmd)
	DataPatchCmd.AddCommand(dataPatchStatusCmd)
}

func dataPatchDeps() (*db.Queries, *txrunner.Manager, func(), error) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return nil, nil, nil, fmt.Errorf("DATABASE_URL is not set")
	}
	conn, err := dbconn.Connect(databaseURL)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("connect to database: %w", err)
	}
	return db.New(conn), txrunner.New(conn), func() { conn.Close() }, nil
}

func runDataPatchUp(cmd *cobra.Command, args []string) error {
	queries, txMgr, closeFn, err := dataPatchDeps()
	if err != nil {
		return err
	}
	defer closeFn()

	res, err := datapatch.Run(context.Background(), queries, txMgr)
	if err != nil {
		return err
	}
	if len(res.Applied) == 0 {
		fmt.Printf("No pending data patches (%d already applied).\n", res.Skipped)
		return nil
	}
	fmt.Printf("Applied %d data patch(es):\n", len(res.Applied))
	for _, id := range res.Applied {
		fmt.Printf("  ✓ %s\n", id)
	}
	return nil
}

func runDataPatchStatus(cmd *cobra.Command, args []string) error {
	queries, _, closeFn, err := dataPatchDeps()
	if err != nil {
		return err
	}
	defer closeFn()

	applied, pending, err := datapatch.Status(context.Background(), queries)
	if err != nil {
		return err
	}
	fmt.Printf("Applied (%d):\n", len(applied))
	for _, id := range applied {
		fmt.Printf("  ✓ %s\n", id)
	}
	fmt.Printf("Pending (%d):\n", len(pending))
	for _, id := range pending {
		fmt.Printf("  • %s\n", id)
	}
	return nil
}
