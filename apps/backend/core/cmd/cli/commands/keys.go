package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

// KeysCmd mints the customer session key pair - core keeps the private half, the Signals tiers get only the public one
var KeysCmd = &cobra.Command{
	Use:   "keys",
	Short: "Generate or inspect session keys",
}

var keysCustomerCmd = &cobra.Command{
	Use:   "customer",
	Short: "Print a fresh CUSTOMER_JWT_PRIVATE_KEY / CUSTOMER_JWT_PUBLIC_KEY pair",
	RunE: func(cmd *cobra.Command, _ []string) error {
		seed, pub, err := auth.GenerateCustomerKey()
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s=%s\n%s=%s\n", auth.EnvCustomerPrivateKey, seed, auth.EnvCustomerPublicKey, pub)
		return nil
	},
}

var keysCustomerPublicCmd = &cobra.Command{
	Use:   "customer-public <private-key>",
	Short: "Print the CUSTOMER_JWT_PUBLIC_KEY a CUSTOMER_JWT_PRIVATE_KEY pairs with",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := auth.NewCustomerSigner(args[0])
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), s.PublicKey())
		return nil
	},
}

func init() {
	KeysCmd.AddCommand(keysCustomerCmd, keysCustomerPublicCmd)
}
