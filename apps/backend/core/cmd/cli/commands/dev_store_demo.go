package commands

import (
	"fmt"
	"os"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/configstore"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/dbconn"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
	"github.com/spf13/cobra"
)

// Explicit opt-in fixture command; never runs during deployment or migration.
var DevStoreDemoCmd = &cobra.Command{
	Use:   "dev-store-demo <channel-id>",
	Short: "Add repeatable store availability demo products and locations (DEV ONLY)",
	Args:  cobra.ExactArgs(1),
	RunE:  runDevStoreDemo,
}

func runDevStoreDemo(cmd *cobra.Command, args []string) error {
	if err := requireDevEnv("dev-store-demo"); err != nil {
		return err
	}
	id, err := uuid.Parse(args[0])
	if err != nil {
		return fmt.Errorf("invalid channel id: %w", err)
	}
	conn, err := dbconn.Connect(os.Getenv("DATABASE_URL"))
	if err != nil {
		return err
	}
	defer conn.Close()
	ctx := cmd.Context()
	q := db.New(conn)
	channels := channel.NewService(channel.NewRepository(q, txrunner.New(conn)))
	ch, err := channels.GetByID(ctx, id)
	if err != nil {
		return err
	}
	cfg := config.NewService(configstore.New(q))
	flat, err := cfg.GetBool(ctx, "sales/inventory/flat_mode", config.DefaultScope)
	if err != nil {
		return err
	}
	if flat {
		return fmt.Errorf("demo stock requires multi-location inventory; migrate inventory before disabling flat mode")
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	tq := q.WithTx(tx)
	if err = tq.SeedStoreAvailabilityDemo(ctx, id); err != nil {
		return err
	}
	if _, err = config.NewService(configstore.New(tq)).Set(ctx, "sales/store_availability/enabled", "true", config.Scope{Kind: config.ScopeChannel, ChannelID: &id}); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Store availability demo ready on channel %s (%s). Existing demo stock is preserved.\n", ch.Slug, id)
	for _, path := range []string{"stores", "store-demo-backpack", "store-demo-scarf", "store-demo-mug", "store-demo-fabric", "store-demo-hoodie"} {
		fmt.Fprintf(cmd.OutOrStdout(), "  /%s\n", path)
	}
	return nil
}
