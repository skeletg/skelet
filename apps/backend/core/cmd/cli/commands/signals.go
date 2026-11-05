package commands

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/skeletg/skelet/apps/backend/core/internal/configstore"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/internal/signalsclient"
	"github.com/skeletg/skelet/apps/backend/core/internal/signalsclient/activity"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/dbconn"
)

var SignalsCmd = &cobra.Command{
	Use:   "signals",
	Short: "Signals suite maintenance",
}

var signalsBackfillOrdersCmd = &cobra.Command{
	Use:   "backfill-orders",
	Short: "Replay every placed order into Signals - safe to re-run, orders it already holds are skipped by their dedupe key",
	Long: "For orders that never reached Signals: placed while the suite was off or unreachable for longer than the outbox retries, " +
		"or guest orders from before guests without a visitor id were recorded. Each lands at its original time. " +
		"Orders placed without analytics consent are skipped - they already count, and a replay would count them twice.",
	Args: cobra.NoArgs,
	RunE: runSignalsBackfillOrders,
}

func init() {
	SignalsCmd.AddCommand(signalsBackfillOrdersCmd)
}

func runSignalsBackfillOrders(cmd *cobra.Command, _ []string) error {
	addr := strings.TrimSpace(os.Getenv("SIGNALS_GRPC_ADDR"))
	if addr == "" {
		return fmt.Errorf("SIGNALS_GRPC_ADDR is not set - there is no Signals to backfill")
	}
	conn, err := dbconn.Connect(os.Getenv("DATABASE_URL"))
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer conn.Close()
	tr, err := signalsclient.TransportFromEnv(os.Getenv)
	if err != nil {
		return fmt.Errorf("signals transport: %w", err)
	}
	signals, err := signalsclient.New(addr, tr, auth.ServiceKeysFromEnv(os.Getenv), time.Minute, nil)
	if err != nil {
		return fmt.Errorf("signals client: %w", err)
	}
	defer func() { _ = signals.Close() }()

	repo := order.NewRepository(db.New(conn))
	// A backfill hands each order over, so the tracker never looks one up
	tracker := activity.NewTracker(signals, nil, config.NewService(configstore.New(db.New(conn))))
	ctx := cmd.Context()
	const page = 500
	processed, skipped := 0, 0
	for off := int32(0); ; off += page {
		orders, _, err := repo.ListOrdersPage(ctx, order.ListPageFilter{Limit: page, Offset: off, SkipCount: true})
		if err != nil {
			return fmt.Errorf("list orders: %w", err)
		}
		for _, o := range orders {
			if o.Status == order.OrderStatusDraft { // not placed yet - it gets its event when it is
				skipped++
				continue
			}
			// A draft's created_at is when it was opened - the live event carries the placement, stamped in the transaction that wrote this row
			placedAt := o.CreatedAt
			history, err := repo.ListOrderEventsByOrder(ctx, o.ID)
			if err != nil {
				return fmt.Errorf("order %s history: %w", o.Number, err)
			}
			for _, e := range history {
				if e.Type == "order_placed" {
					placedAt = e.CreatedAt
					break
				}
			}
			if err := tracker.BackfillOrder(ctx, o, placedAt); err != nil {
				return fmt.Errorf("order %s: %w (processed %d before it; re-run to continue)", o.Number, err, processed)
			}
			processed++
		}
		if len(orders) < page {
			break
		}
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Processed %d orders for Signals (%d drafts skipped)\n", processed, skipped)
	return nil
}
