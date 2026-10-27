package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/skeletg/skelet/apps/backend/core/internal/dbmigrate"
	pluginhost "github.com/skeletg/skelet/apps/backend/core/internal/plugin"
)

var MigrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Bring the commerce database up to date - schema, plugin migrations, data patches",
	Long: `The step the server runs as it boots, for a deploy that wants the database
migrated before any process starts. Running it beside a booting server is
safe: each migration takes a lock, so whichever gets there first applies it.

The Signals databases migrate as the signals and ingest processes boot.`,
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		databaseURL := os.Getenv("DATABASE_URL")
		if databaseURL == "" {
			return fmt.Errorf("DATABASE_URL is not set")
		}
		if err := pluginhost.ValidateManifests(); err != nil {
			return fmt.Errorf("plugin manifest validation: %w", err)
		}
		if err := dbmigrate.Up(context.Background(), databaseURL); err != nil {
			return err
		}
		fmt.Println("The commerce database is up to date.")
		return nil
	},
}
