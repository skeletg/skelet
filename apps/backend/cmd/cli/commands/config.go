package commands

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/skeletg/skelet/internal/config"
	dbconn "github.com/skeletg/skelet/internal/db"
	db "github.com/skeletg/skelet/internal/db/sqlc"
	"github.com/skeletg/skelet/internal/tx"
	"github.com/spf13/cobra"
)

// ConfigCmd is the parent command for runtime configuration management. All subcommands operate at config.DefaultScope
// for now; --scope flags can be added when Skelet grows multi-store support
var ConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Runtime configuration management",
}

var configListCmd = &cobra.Command{
	Use:   "list",
	Short: "List every registered config value at default scope",
	RunE:  runConfigList,
}

var configGetCmd = &cobra.Command{
	Use:   "get <path>",
	Short: "Show a single config value at default scope",
	Args:  cobra.ExactArgs(1),
	RunE:  runConfigGet,
}

var configSetCmd = &cobra.Command{
	Use:   "set <path> <value>",
	Short: "Set a config value at default scope",
	Args:  cobra.ExactArgs(2),
	RunE:  runConfigSet,
}

var configResetCmd = &cobra.Command{
	Use:   "reset <path>",
	Short: "Delete the stored value at default scope; registry default resumes",
	Args:  cobra.ExactArgs(1),
	RunE:  runConfigReset,
}

var configOrphansCmd = &cobra.Command{
	Use:   "orphans",
	Short: "List DB rows whose paths are no longer in the registry",
	RunE:  runConfigOrphans,
}

var configPruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Hard-delete every orphaned config row (prompts for confirmation)",
	RunE:  runConfigPrune,
}

func init() {
	ConfigCmd.AddCommand(configListCmd)
	ConfigCmd.AddCommand(configGetCmd)
	ConfigCmd.AddCommand(configSetCmd)
	ConfigCmd.AddCommand(configResetCmd)
	ConfigCmd.AddCommand(configOrphansCmd)
	ConfigCmd.AddCommand(configPruneCmd)
}

func newConfigService(ctx context.Context) (*config.Service, func(), error) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return nil, nil, fmt.Errorf("DATABASE_URL is not set")
	}
	conn, err := dbconn.Connect(databaseURL)
	if err != nil {
		return nil, nil, fmt.Errorf("connect to database: %w", err)
	}
	queries := db.New(conn)
	txMgr := tx.New(conn)
	return config.NewService(config.NewRepository(queries, txMgr)), func() { conn.Close() }, nil
}

func runConfigList(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	svc, close, err := newConfigService(ctx)
	if err != nil {
		return err
	}
	defer close()

	entries, err := svc.ListAll(ctx, config.DefaultScope)
	if err != nil {
		return err
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SECTION\tGROUP\tFIELD\tTYPE\tPUBLIC\tDEFAULT?\tVALUE")
	for _, e := range entries {
		section, group, field, _ := config.ParsePath(e.Definition.Path)
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%v\t%v\t%s\n",
			section, group, field, e.Definition.Type,
			e.Definition.Public, e.IsDefault, e.Value)
	}
	return w.Flush()
}

func runConfigGet(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	svc, close, err := newConfigService(ctx)
	if err != nil {
		return err
	}
	defer close()

	entry, err := svc.Get(ctx, args[0], config.DefaultScope)
	if err != nil {
		return err
	}
	section, group, field, _ := config.ParsePath(entry.Definition.Path)
	fmt.Printf("Path:        %s\n", entry.Definition.Path)
	fmt.Printf("Section:     %s\n", section)
	fmt.Printf("Group:       %s\n", group)
	fmt.Printf("Field:       %s\n", field)
	fmt.Printf("Label:       %s\n", entry.Definition.Label)
	fmt.Printf("Type:        %s\n", entry.Definition.Type)
	fmt.Printf("Public:      %v\n", entry.Definition.Public)
	fmt.Printf("Is default:  %v\n", entry.IsDefault)
	fmt.Printf("Value:       %s\n", entry.Value)
	if !entry.IsDefault && !entry.UpdatedAt.IsZero() {
		fmt.Printf("Updated at:  %s\n", entry.UpdatedAt.Format("2006-01-02 15:04:05"))
	}
	return nil
}

func runConfigSet(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	svc, close, err := newConfigService(ctx)
	if err != nil {
		return err
	}
	defer close()

	entry, err := svc.Set(ctx, args[0], args[1], config.DefaultScope)
	if err != nil {
		return err
	}
	fmt.Printf("Set %s = %s\n", entry.Definition.Path, entry.Value)
	return nil
}

func runConfigReset(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	svc, close, err := newConfigService(ctx)
	if err != nil {
		return err
	}
	defer close()

	entry, err := svc.Reset(ctx, args[0], config.DefaultScope)
	if err != nil {
		return err
	}
	fmt.Printf("Reset %s; effective value is now %q (default: %v)\n",
		entry.Definition.Path, entry.Value, entry.IsDefault)
	return nil
}

func runConfigOrphans(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	svc, close, err := newConfigService(ctx)
	if err != nil {
		return err
	}
	defer close()

	orphans, err := svc.ListOrphans(ctx)
	if err != nil {
		return err
	}
	if len(orphans) == 0 {
		fmt.Println("No orphaned config rows.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SCOPE\tPATH\tVALUE")
	for _, o := range orphans {
		fmt.Fprintf(w, "%s\t%s\t%s\n", config.FormatScope(o.Scope), o.Path, o.Value)
	}
	return w.Flush()
}

func runConfigPrune(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	svc, close, err := newConfigService(ctx)
	if err != nil {
		return err
	}
	defer close()

	orphans, err := svc.ListOrphans(ctx)
	if err != nil {
		return err
	}
	if len(orphans) == 0 {
		fmt.Println("No orphaned config rows.")
		return nil
	}

	fmt.Printf("%d orphan row(s) will be deleted:\n", len(orphans))
	for _, o := range orphans {
		fmt.Printf("  %s/%s\n", config.FormatScope(o.Scope), o.Path)
	}
	fmt.Print("Proceed? [y/N]: ")

	reader := bufio.NewReader(os.Stdin)
	resp, _ := reader.ReadString('\n')
	if strings.ToLower(strings.TrimSpace(resp)) != "y" {
		fmt.Println("Aborted.")
		return nil
	}

	n, err := svc.Prune(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("Deleted %d orphan row(s).\n", n)
	return nil
}
