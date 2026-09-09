package main

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
	"github.com/skeletg/skelet/apps/backend/cmd/cli/commands"
	pluginhost "github.com/skeletg/skelet/apps/backend/internal/plugin"
	"github.com/spf13/cobra"

	_ "github.com/skeletg/skelet/apps/backend/plugins/firstparty" // the same set cmd/server ships, so `bin/cli config list|set` sees every plugin's config paths and permissions
)

var rootCmd = &cobra.Command{
	Use:   "skelet",
	Short: "Skelet CLI",
}

func main() {
	_ = godotenv.Load()

	pluginhost.Install()

	rootCmd.AddCommand(commands.AdminCmd)
	rootCmd.AddCommand(commands.RoleCmd)
	rootCmd.AddCommand(commands.IntegrationCmd)
	rootCmd.AddCommand(commands.ConfigCmd)
	rootCmd.AddCommand(commands.PluginCmd)
	rootCmd.AddCommand(commands.CustomerCmd)
	rootCmd.AddCommand(commands.DataCmd)
	rootCmd.AddCommand(commands.DevResetCmd)
	rootCmd.AddCommand(commands.MediaOrphansCmd)
	rootCmd.AddCommand(commands.MediaMigrateCmd)
	rootCmd.AddCommand(commands.SignalsCmd)
	rootCmd.AddCommand(commands.DbCmd)
	rootCmd.AddCommand(commands.DataPatchCmd)
	rootCmd.AddCommand(commands.SearchCmd)
	rootCmd.AddCommand(commands.URLRewriteCmd)
	rootCmd.AddCommand(commands.InventoryCmd)
	rootCmd.AddCommand(commands.LensCmd)
	rootCmd.AddCommand(commands.ReviewsCmd)
	rootCmd.AddCommand(commands.EmailsCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
