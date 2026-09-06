package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	dbconn "github.com/skeletg/skelet/apps/backend/internal/db"
	db "github.com/skeletg/skelet/apps/backend/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/internal/inventory"
	"github.com/skeletg/skelet/apps/backend/internal/tx"
)

var InventoryCmd = &cobra.Command{
	Use:   "inventory",
	Short: "Inventory maintenance",
}

var (
	migrateModeTo       string
	migrateModeLocation string
	migrateModeDryRun   bool
)

// The flag flip alone switches which column is authoritative, and the other one is usually stale -
// a store that ran multi-location through the admin UI has inventory_items.quantity at its 0
// default, so flipping to flat reads the whole catalogue as out of stock
var inventoryMigrateModeCmd = &cobra.Command{
	Use:   "migrate-mode",
	Short: "Carry stock across before flipping sales/inventory/flat_mode",
	Long: `Copies stock between the two modes' storage so a flip doesn't zero availability.

  --to flat    sums each product's salable stock across all locations into inventory_items.quantity
  --to multi   parks each product's flat quantity at --location and zeroes every other location

Run this BEFORE flipping sales/inventory/flat_mode, and --dry-run first. Both directions are
idempotent. After --to multi, run "inventory reconcile-reserved" to rebuild the reserved counters,
since flat mode writes no ledger rows.`,
	RunE: runInventoryMigrateMode,
}

var inventoryReconcileReservedCmd = &cobra.Command{ // the ledger is the source of truth and the rebuild is idempotent - safe as a periodic sweep, not just after a suspected drift
	Use:   "reconcile-reserved",
	Short: "Rebuild inventory_levels.reserved_quantity from the reservation ledger",
	RunE:  runInventoryReconcileReserved,
}

func init() {
	InventoryCmd.AddCommand(inventoryReconcileReservedCmd)
	InventoryCmd.AddCommand(inventoryMigrateModeCmd)
	inventoryMigrateModeCmd.Flags().StringVar(&migrateModeTo, "to", "", "target mode: flat or multi (required)")
	inventoryMigrateModeCmd.Flags().StringVar(&migrateModeLocation, "location", "", "location slug to park stock at, for --to multi")
	inventoryMigrateModeCmd.Flags().BoolVar(&migrateModeDryRun, "dry-run", false, "report what would change and write nothing")
}

func runInventoryReconcileReserved(cmd *cobra.Command, args []string) error {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return fmt.Errorf("DATABASE_URL is not set")
	}
	conn, err := dbconn.Connect(databaseURL)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer conn.Close()

	queries := db.New(conn)
	txMgr := tx.New(conn)
	svc := inventory.NewService(inventory.NewRepository(queries, txMgr), txMgr)

	if err := svc.ReconcileReserved(context.Background()); err != nil {
		return fmt.Errorf("reconcile reserved: %w", err)
	}
	fmt.Println("inventory: reserved_quantity reconciled from the reservation ledger")
	return nil
}

func runInventoryMigrateMode(cmd *cobra.Command, args []string) error {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return fmt.Errorf("DATABASE_URL is not set")
	}
	conn, err := dbconn.Connect(databaseURL)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer conn.Close()

	queries := db.New(conn)
	txMgr := tx.New(conn)
	svc := inventory.NewService(inventory.NewRepository(queries, txMgr), txMgr)
	ctx := context.Background()

	switch migrateModeTo {
	case "flat":
		rows, changed, err := svc.MigrateToFlatMode(ctx, migrateModeDryRun)
		if err != nil {
			return fmt.Errorf("migrate to flat: %w", err)
		}
		if migrateModeDryRun {
			for _, r := range rows {
				if r.Current != r.New {
					fmt.Printf("  %-24s %g -> %g\n", r.SKU, r.Current, r.New)
				}
			}
			fmt.Printf("dry run: %d of %d products would change\n", changed, len(rows))
			return nil
		}
		fmt.Printf("inventory: %d products' quantity set from their locations' salable stock\n", changed)
		fmt.Println("now flip sales/inventory/flat_mode to true")
		return nil

	case "multi", "multi-location":
		if migrateModeLocation == "" {
			return fmt.Errorf("--location is required for --to multi (the slug to park stock at)")
		}
		loc, err := svc.GetLocationBySlug(ctx, migrateModeLocation)
		if err != nil {
			return fmt.Errorf("location %q: %w", migrateModeLocation, err)
		}
		placed, zeroed, err := svc.MigrateToMultiLocationMode(ctx, loc.ID, migrateModeDryRun)
		if err != nil {
			return fmt.Errorf("migrate to multi-location: %w", err)
		}
		if migrateModeDryRun {
			fmt.Printf("dry run: %d products would be parked at %q, other locations zeroed\n", placed, loc.Slug)
			return nil
		}
		fmt.Printf("inventory: %d products parked at %q, %d stale levels elsewhere zeroed\n", placed, loc.Slug, zeroed)
		fmt.Println("check that location is assigned to the channels that should sell it, then flip sales/inventory/flat_mode to false")
		fmt.Println("then run: skelet inventory reconcile-reserved")
		return nil

	default:
		return fmt.Errorf("--to must be flat or multi, got %q", migrateModeTo)
	}
}
