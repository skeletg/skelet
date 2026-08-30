package commands

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var RoleCmd = &cobra.Command{
	Use:   "role",
	Short: "Admin role management",
}

var roleListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all admin roles",
	RunE:  runRoleList,
}

func init() {
	RoleCmd.AddCommand(roleListCmd)
}

func runRoleList(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	svc, close, err := newService(ctx)
	if err != nil {
		return err
	}
	defer close()

	roles, err := svc.ListRoles(ctx)
	if err != nil {
		return err
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SLUG\tNAME\tMEMBERS\tPERMISSIONS")
	for _, r := range roles {
		seen := make(map[string]struct{}, len(r.PermissionScopes)) // dedupe across scopes - `catalog.manage` on three channels is still one entry in this list
		slugs := make([]string, 0, len(r.PermissionScopes))
		for _, s := range r.PermissionScopes {
			if _, ok := seen[s.Permission.Slug]; ok {
				continue
			}
			seen[s.Permission.Slug] = struct{}{}
			slugs = append(slugs, s.Permission.Slug)
		}
		permsStr := strings.Join(slugs, ", ")
		if permsStr == "" {
			permsStr = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\n", r.Slug, r.Name, r.MemberCount, permsStr)
	}
	return w.Flush()
}
