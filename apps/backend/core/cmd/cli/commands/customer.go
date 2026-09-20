package commands

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/cobra"

	"github.com/skeletg/skelet/apps/backend/core/internal/authstore"
	"github.com/skeletg/skelet/apps/backend/core/internal/consent"
	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/signalsclient"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/dbconn"
	"github.com/skeletg/skelet/apps/backend/core/lib/email"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

var CustomerCmd = &cobra.Command{
	Use:   "customer",
	Short: "Customer management",
}

var customerGroupCmd = &cobra.Command{
	Use:   "group",
	Short: "Customer group management",
}

var customerGroupListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all customer groups",
	RunE:  runCustomerGroupList,
}

func init() {
	CustomerCmd.AddCommand(customerGroupCmd)
	customerGroupCmd.AddCommand(customerGroupListCmd)
}

func newCustomerService(ctx context.Context) (*customer.Service, func(), error) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return nil, nil, fmt.Errorf("DATABASE_URL is not set")
	}
	conn, err := dbconn.Connect(databaseURL)
	if err != nil {
		return nil, nil, fmt.Errorf("connect to database: %w", err)
	}
	queries := db.New(conn)
	txMgr := txrunner.New(conn)

	// CLI only calls read paths (ListGroups) - the email/token deps the constructor requires are never hit, so stub them with minimal real types (Valkey is contacted only when an email flow runs)
	opt, err := redis.ParseURL(os.Getenv("VALKEY_URL"))
	if err != nil {
		return nil, nil, fmt.Errorf("parse valkey url: %w", err)
	}
	valkey := redis.NewClient(opt)
	// GDPR erasure asks the suite to drop its identity mappings - nil without SIGNALS_GRPC_ADDR, which
	// the repository treats as no Signals half to erase
	signals, err := signalsclient.New(os.Getenv("SIGNALS_GRPC_ADDR"), os.Getenv("SIGNALS_SERVICE_SECRET"), time.Minute, nil)
	if err != nil {
		valkey.Close()
		conn.Close()
		return nil, nil, fmt.Errorf("signals client: %w", err)
	}
	svc := customer.NewService(
		customer.NewRepository(queries, signals, txMgr),
		auth.NewVerificationStore(authstore.NewTokenBackend(valkey, queries)),
		auth.NewPasswordResetStore(authstore.NewTokenBackend(valkey, queries)),
		auth.NewRefreshStore(authstore.NewRefreshBackend(valkey, queries, "customer"), nil, "customer"),
		email.LogSender{},
		nopStoreLookup{},
		consent.NewService(consent.NewRepository(queries)),
		nil, // no event bus - CLI read paths don't dispatch, the service nil-checks first
	)
	return svc, func() { _ = signals.Close(); valkey.Close(); conn.Close() }, nil
}

type nopStoreLookup struct{} // stub StoreLookup - CLI commands never hit email-sending paths, the only consumer

func (nopStoreLookup) GetByID(ctx context.Context, id uuid.UUID) (customer.StoreSnapshot, error) {
	return customer.StoreSnapshot{ID: id}, nil
}

func runCustomerGroupList(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	svc, close, err := newCustomerService(ctx)
	if err != nil {
		return err
	}
	defer close()

	groups, err := svc.ListGroups(ctx)
	if err != nil {
		return err
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SLUG\tNAME\tDEFAULT?\tMEMBERS\tDESCRIPTION")
	for _, g := range groups {
		desc := g.Description
		if desc == "" {
			desc = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%v\t%d\t%s\n", g.Slug, g.Name, g.IsDefault, g.MemberCount, desc)
	}
	return w.Flush()
}
