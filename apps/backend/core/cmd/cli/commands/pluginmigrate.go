package commands

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"text/tabwriter"

	"github.com/spf13/cobra"

	pluginhost "github.com/skeletg/skelet/apps/backend/core/internal/plugin"
	"github.com/skeletg/skelet/apps/backend/core/internal/pluginmigrate"
	"github.com/skeletg/skelet/apps/backend/core/lib/dbconn"
)

var pluginMigrateCmd = &cobra.Command{ // lives under PluginCmd rather than at the top level, so every plugin meta-operation shares one umbrella
	Use:   "migrate",
	Short: "Apply or roll back Tier 1 plugin SQL migrations",
	Long: `Manages SQL migrations registered by Tier 1 plugins via
p.RegisterMigrations. Each plugin has its own namespace and version
sequence; applied versions live in the plugin_migrations table.

Core migrations are unrelated - run 'skelet db migrate up' for those.`,
}

var pluginMigrateUpCmd = &cobra.Command{
	Use:   "up [namespace]",
	Short: "Apply pending migrations for all plugins (or one named plugin)",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runPluginMigrateUp,
}

var pluginMigrateDownCmd = &cobra.Command{
	Use:   "down <namespace> [steps]",
	Short: "Roll back the most recent N migrations for one plugin (default 1)",
	Args:  cobra.RangeArgs(1, 2),
	RunE:  runPluginMigrateDown,
}

var pluginMigrateStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show applied/pending migrations per plugin",
	RunE:  runPluginMigrateStatus,
}

func init() {
	pluginMigrateCmd.AddCommand(pluginMigrateUpCmd)
	pluginMigrateCmd.AddCommand(pluginMigrateDownCmd)
	pluginMigrateCmd.AddCommand(pluginMigrateStatusCmd)
}

func newPluginMigrateRunner() (*pluginmigrate.Runner, func(), error) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return nil, nil, fmt.Errorf("DATABASE_URL is not set")
	}
	conn, err := dbconn.Connect(databaseURL)
	if err != nil {
		return nil, nil, fmt.Errorf("connect to database: %w", err)
	}
	return pluginmigrate.New(conn), func() { conn.Close() }, nil
}

func runPluginMigrateUp(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	runner, close, err := newPluginMigrateRunner()
	if err != nil {
		return err
	}
	defer close()

	if len(args) == 1 {
		ns := args[0]
		if err := runner.UpNamespace(ctx, ns); err != nil {
			return err
		}
		fmt.Printf("Applied pending migrations for plugin %q\n", ns)
		return nil
	}
	// Validating the whole graph up front stops a missing dep / cycle / SDK-incompat plugin from writing half its tables before the next one fails
	if err := pluginhost.ValidateManifests(); err != nil {
		return fmt.Errorf("plugin manifest validation: %w", err)
	}
	order := pluginhost.OrderedManifestSlugs() // deps first, so a plugin's schema lands on top of the schemas it builds on
	if err := runner.UpInOrder(ctx, order); err != nil {
		return err
	}
	fmt.Println("Applied pending migrations for all registered plugins")
	return nil
}

func runPluginMigrateDown(cmd *cobra.Command, args []string) error {
	ns := args[0]
	steps := 1
	if len(args) == 2 {
		n, err := strconv.Atoi(args[1])
		if err != nil || n <= 0 {
			return fmt.Errorf("steps must be a positive integer (got %q)", args[1])
		}
		steps = n
	}

	ctx := context.Background()
	runner, close, err := newPluginMigrateRunner()
	if err != nil {
		return err
	}
	defer close()

	if err := runner.Down(ctx, ns, steps); err != nil {
		return err
	}
	fmt.Printf("Rolled back %d migration(s) for plugin %q\n", steps, ns)
	return nil
}

func runPluginMigrateStatus(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	runner, close, err := newPluginMigrateRunner()
	if err != nil {
		return err
	}
	defer close()

	statuses, err := runner.Status(ctx)
	if err != nil {
		return err
	}
	if len(statuses) == 0 {
		fmt.Println("No plugins have registered migrations.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAMESPACE\tAPPLIED\tHIGHEST\tPENDING\tAVAILABLE")
	for _, s := range statuses {
		applied := fmt.Sprintf("%d", len(s.AppliedVersions))
		pending := fmt.Sprintf("%d", len(s.PendingVersions))
		highest := "-"
		if s.HighestApplied > 0 {
			highest = fmt.Sprintf("%04d", s.HighestApplied)
		}
		avail := "-"
		if s.HighestAvailable > 0 {
			avail = fmt.Sprintf("%04d", s.HighestAvailable)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", s.Namespace, applied, highest, pending, avail)
	}
	return w.Flush()
}
