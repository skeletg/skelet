package commands

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	dbconn "github.com/skeletg/skelet/internal/db"
	db "github.com/skeletg/skelet/internal/db/sqlc"
	"github.com/skeletg/skelet/internal/integration"
	"github.com/skeletg/skelet/internal/tx"
	"github.com/spf13/cobra"
)

// IntegrationCmd is the parent command for API integration management.
var IntegrationCmd = &cobra.Command{
	Use:   "integration",
	Short: "API integration management",
}

var integrationListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all API integrations",
	RunE:  runIntegrationList,
}

func init() {
	IntegrationCmd.AddCommand(integrationListCmd)
}

func newIntegrationService(ctx context.Context) (*integration.Service, func(), error) {
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
	return integration.NewService(integration.NewRepository(queries, txMgr)), func() { conn.Close() }, nil
}

func runIntegrationList(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	svc, close, err := newIntegrationService(ctx)
	if err != nil {
		return err
	}
	defer close()

	list, err := svc.List(ctx)
	if err != nil {
		return err
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tACTIVE\tEXPIRES\tPERMISSIONS")
	for _, i := range list {
		slugs := make([]string, len(i.Permissions))
		for j, p := range i.Permissions {
			slugs[j] = p.Slug
		}
		permsStr := strings.Join(slugs, ", ")
		if permsStr == "" {
			permsStr = "-"
		}
		expiresStr := "never"
		if i.ExpiresAt.Valid {
			expiresStr = i.ExpiresAt.Time.Format("2006-01-02 15:04")
		}
		fmt.Fprintf(w, "%s\t%s\t%v\t%s\t%s\n", i.ID, i.Name, i.IsActive, expiresStr, permsStr)
	}
	return w.Flush()
}
