package commands

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/skeletg/skelet/apps/backend/core/lib/dbconn"
)

// Wipes all business data but keeps superuser admins and seeded reference/config data - refuses to run unless APP_ENV=development
var DevResetCmd = &cobra.Command{
	Use:   "dev-reset",
	Short: "Wipe all business data, keeping superuser accounts (DEV ONLY)",
	RunE:  runDevReset,
}

// Everything in schema `public` NOT listed here gets truncated - a keep-list (not a truncate-list) so newly-added business tables are wiped by default; add tables that should survive a reset
var devResetKeepTables = map[string]struct{}{
	// Migration bookkeeping
	"schema_migrations": {},

	// Auth / RBAC seed data
	"admin_users":               {}, // non-superusers deleted separately below
	"admin_roles":               {},
	"admin_role_permissions":    {},
	"admin_role_channel_scopes": {},
	"admin_user_roles":          {}, // role assignments for superusers
	"permissions":               {},
	"permission_groups":         {},

	// Operational/reference data
	"locales":         {},
	"channels":        {},
	"channel_locales": {},
	"countries":       {},
	"currencies":      {},
	"config_values":   {},
}

func runDevReset(cmd *cobra.Command, args []string) error {
	if env := strings.ToLower(os.Getenv("APP_ENV")); env != "development" && env != "dev" {
		return fmt.Errorf("dev-reset refuses to run unless APP_ENV=development (got %q)", os.Getenv("APP_ENV"))
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return fmt.Errorf("DATABASE_URL is not set")
	}
	target := describeDatabaseURL(databaseURL)

	conn, err := dbconn.Connect(databaseURL)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer conn.Close()

	ctx := context.Background()
	tables, err := listPublicTables(ctx, conn)
	if err != nil {
		return fmt.Errorf("list tables: %w", err)
	}

	wipe := make([]string, 0, len(tables))
	for _, t := range tables {
		if _, keep := devResetKeepTables[t]; !keep {
			wipe = append(wipe, t)
		}
	}
	if len(wipe) == 0 {
		return fmt.Errorf("no tables to wipe (unexpected - schema empty?)")
	}
	sort.Strings(wipe)

	fmt.Printf("\n!! DESTRUCTIVE OPERATION !!\n")
	fmt.Printf("Target: %s\n", target)
	fmt.Printf("\nWill TRUNCATE %d tables and DELETE every non-superuser admin user.\n", len(wipe))
	fmt.Printf("Preserved: superuser admin_users, admin_roles, permissions, locales,\n")
	fmt.Printf("currencies, countries, channels, config_values, schema_migrations.\n\n")
	fmt.Printf("Tables to truncate:\n")
	for _, t := range wipe {
		fmt.Printf("  - %s\n", t)
	}
	fmt.Println()

	stdin := bufio.NewReader(os.Stdin)
	first, err := promptLine(stdin, "Type 'WIPE' to continue: ")
	if err != nil {
		return err
	}
	if first != "WIPE" {
		return fmt.Errorf("aborted")
	}
	second, err := promptLine(stdin, "Type 'YES I AM SURE' to proceed: ")
	if err != nil {
		return err
	}
	if second != "YES I AM SURE" {
		return fmt.Errorf("aborted")
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	quoted := make([]string, len(wipe))
	for i, t := range wipe {
		quoted[i] = `"` + t + `"`
	}
	truncateSQL := fmt.Sprintf(
		"TRUNCATE %s RESTART IDENTITY CASCADE",
		strings.Join(quoted, ", "),
	)
	if _, err := tx.ExecContext(ctx, truncateSQL); err != nil {
		return fmt.Errorf("truncate: %w", err)
	}

	res, err := tx.ExecContext(ctx, "DELETE FROM admin_users WHERE is_superuser = FALSE")
	if err != nil {
		return fmt.Errorf("delete non-superusers: %w", err)
	}
	deleted, _ := res.RowsAffected()

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	fmt.Printf("\nDone. Truncated %d tables; deleted %d non-superuser admin user(s).\n",
		len(wipe), deleted)
	return nil
}

func listPublicTables(ctx context.Context, conn *sql.DB) ([]string, error) {
	rows, err := conn.QueryContext(ctx,
		`SELECT tablename FROM pg_tables WHERE schemaname = 'public' ORDER BY tablename`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func describeDatabaseURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "(unparseable DATABASE_URL)"
	}
	dbname := strings.TrimPrefix(u.Path, "/")
	if dbname == "" {
		dbname = "(no database)"
	}
	return fmt.Sprintf("%s/%s", u.Host, dbname)
}

func promptLine(r *bufio.Reader, label string) (string, error) {
	fmt.Print(label)
	line, err := r.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("read input: %w", err)
	}
	return strings.TrimRight(line, "\r\n"), nil
}
