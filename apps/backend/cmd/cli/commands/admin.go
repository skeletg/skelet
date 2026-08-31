package commands

import (
	"context"
	"fmt"
	"os"
	"strings"
	"syscall"
	"text/tabwriter"

	"github.com/skeletg/skelet/apps/backend/internal/admin"
	dbconn "github.com/skeletg/skelet/apps/backend/internal/db"
	db "github.com/skeletg/skelet/apps/backend/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/internal/tx"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// AdminCmd is the parent command for admin user management.
var AdminCmd = &cobra.Command{
	Use:   "admin",
	Short: "Admin user management",
}

var createSuperuserCmd = &cobra.Command{
	Use:   "create-superuser",
	Short: "Create a superuser with full system access",
	RunE:  runCreateSuperuser,
}

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create an admin user assigned to one or more roles",
	RunE:  runCreate,
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all admin users",
	RunE:  runList,
}

// Non-interactive superuser creation, for provisioning and for CI standing a stack up - the
// prompts still run for whatever is left unset, so a human typing the bare command sees no change
var superuserFlags struct {
	email     string
	password  string
	firstName string
	lastName  string
}

func init() {
	f := createSuperuserCmd.Flags()
	f.StringVar(&superuserFlags.email, "email", "", "skip the email prompt")
	f.StringVar(&superuserFlags.password, "password", "", "skip the password prompts - visible in shell history, so prefer the prompt outside automation")
	f.StringVar(&superuserFlags.firstName, "first-name", "", "skip the first-name prompt")
	f.StringVar(&superuserFlags.lastName, "last-name", "", "skip the last-name prompt")

	AdminCmd.AddCommand(createSuperuserCmd)
	AdminCmd.AddCommand(createCmd)
	AdminCmd.AddCommand(listCmd)
}

func newService(ctx context.Context) (*admin.Service, func(), error) {
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
	return admin.NewService(admin.NewRepository(queries, txMgr)), func() { conn.Close() }, nil
}

func runCreateSuperuser(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	svc, close, err := newService(ctx)
	if err != nil {
		return err
	}
	defer close()

	email, err := valueOrPrompt(superuserFlags.email, "Email: ")
	if err != nil {
		return err
	}
	firstName, err := valueOrPrompt(superuserFlags.firstName, "First name: ")
	if err != nil {
		return err
	}
	lastName, err := valueOrPrompt(superuserFlags.lastName, "Last name: ")
	if err != nil {
		return err
	}
	password := superuserFlags.password
	// Confirming a password the caller passed on the command line would only ask them to type
	// their own flag back - the second prompt exists to catch a typo nobody can see
	if password == "" {
		if password, err = promptPassword("Password: "); err != nil {
			return err
		}
		confirm, err := promptPassword("Confirm password: ")
		if err != nil {
			return err
		}
		if password != confirm {
			return fmt.Errorf("passwords do not match")
		}
	}

	if err := svc.CreateSuperuser(ctx, email, password, firstName, lastName); err != nil {
		return err
	}
	fmt.Printf("\nSuperuser %s %s <%s> created successfully.\n", firstName, lastName, email)
	return nil
}

func runCreate(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	svc, close, err := newService(ctx)
	if err != nil {
		return err
	}
	defer close()

	email, err := prompt("Email: ")
	if err != nil {
		return err
	}
	firstName, err := prompt("First name: ")
	if err != nil {
		return err
	}
	lastName, err := prompt("Last name: ")
	if err != nil {
		return err
	}
	password, err := promptPassword("Password: ")
	if err != nil {
		return err
	}
	confirm, err := promptPassword("Confirm password: ")
	if err != nil {
		return err
	}
	if password != confirm {
		return fmt.Errorf("passwords do not match")
	}

	roles, err := svc.ListRoles(ctx)
	if err != nil {
		return fmt.Errorf("load roles: %w", err)
	}
	fmt.Println("\nAvailable roles:")
	for _, r := range roles {
		fmt.Printf("  %-20s %s\n", r.Slug, r.Name)
		if r.Description != "" {
			fmt.Printf("  %-20s   %s\n", "", r.Description)
		}
	}

	rawRoles, err := prompt("\nRoles (comma-separated slugs, or leave blank for none): ")
	if err != nil {
		return err
	}
	var roleSlugs []string
	for s := range strings.SplitSeq(rawRoles, ",") {
		s = strings.TrimSpace(s)
		if s != "" {
			roleSlugs = append(roleSlugs, s)
		}
	}

	user, err := svc.CreateAdmin(ctx, email, password, firstName, lastName, roleSlugs)
	if err != nil {
		return err
	}
	fmt.Printf("\nAdmin %s %s <%s> created successfully.\n", user.FirstName, user.LastName, user.Email)
	return nil
}

func runList(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	svc, close, err := newService(ctx)
	if err != nil {
		return err
	}
	defer close()

	users, err := svc.List(ctx)
	if err != nil {
		return err
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "EMAIL\tNAME\tACTIVE\tSUPERUSER\tROLES")
	for _, u := range users {
		slugs := make([]string, len(u.Roles))
		for i, r := range u.Roles {
			slugs[i] = r.Slug
		}
		rolesStr := strings.Join(slugs, ", ")
		if u.IsSuperuser {
			rolesStr = "(bypass)"
		}
		if rolesStr == "" {
			rolesStr = "-"
		}
		fmt.Fprintf(w, "%s\t%s %s\t%v\t%v\t%s\n",
			u.Email, u.FirstName, u.LastName, u.IsActive, u.IsSuperuser, rolesStr)
	}
	return w.Flush()
}

func prompt(label string) (string, error) {
	fmt.Print(label)
	var value string
	if _, err := fmt.Scanln(&value); err != nil && err.Error() != "unexpected newline" {
		return "", fmt.Errorf("read input: %w", err)
	}
	return strings.TrimSpace(value), nil
}

func valueOrPrompt(value, label string) (string, error) {
	if value != "" {
		return value, nil
	}
	return prompt(label)
}

func promptPassword(label string) (string, error) {
	fmt.Print(label)
	b, err := term.ReadPassword(syscall.Stdin)
	fmt.Println()
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	return string(b), nil
}
