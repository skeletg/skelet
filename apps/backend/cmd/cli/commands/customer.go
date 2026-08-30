package commands

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/skeletg/skelet/internal/analytics"
	"github.com/skeletg/skelet/internal/analytics/spinech"
	"github.com/skeletg/skelet/internal/auth"
	"github.com/skeletg/skelet/internal/consent"
	"github.com/skeletg/skelet/internal/customer"
	dbconn "github.com/skeletg/skelet/internal/db"
	signalsdb "github.com/skeletg/skelet/internal/db/signals/sqlc"
	db "github.com/skeletg/skelet/internal/db/sqlc"
	"github.com/skeletg/skelet/internal/email"
	"github.com/skeletg/skelet/internal/tx"
	"github.com/spf13/cobra"
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
	txMgr := tx.New(conn)

	// CLI only calls read paths (ListGroups) - the email/token deps the constructor requires are never hit, so stub them with minimal real types (Valkey is contacted only when an email flow runs)
	opt, err := redis.ParseURL(os.Getenv("VALKEY_URL"))
	if err != nil {
		return nil, nil, fmt.Errorf("parse valkey url: %w", err)
	}
	valkey := redis.NewClient(opt)
	// GDPR erasure goes through a Recorder with the app's engine selection, so a ClickHouse deployment's erasure hits ClickHouse rather than the stale Postgres table
	sigConn, err := signalsConn()
	if err != nil {
		valkey.Close()
		conn.Close()
		return nil, nil, err
	}
	rec := analytics.NewRecorder(queries, signalsdb.New(sigConn))
	chStore, chCleanup, err := spinech.SelectFromEnv(ctx)
	if err != nil {
		valkey.Close()
		conn.Close()
		sigConn.Close()
		return nil, nil, err
	}
	if chStore != nil {
		rec.SetSpine(chStore)
	}
	svc := customer.NewService(
		customer.NewRepository(queries, rec, txMgr),
		auth.NewVerificationStore(auth.NewTokenBackend(valkey, queries)),
		auth.NewPasswordResetStore(auth.NewTokenBackend(valkey, queries)),
		auth.NewRefreshStore(auth.NewRefreshBackend(valkey, queries, "customer"), nil, "customer"),
		email.LogSender{},
		nopStoreLookup{},
		consent.NewService(consent.NewRepository(queries)),
		nil, // no event bus - CLI read paths don't dispatch, the service nil-checks first
	)
	return svc, func() { chCleanup(); valkey.Close(); conn.Close(); sigConn.Close() }, nil
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
