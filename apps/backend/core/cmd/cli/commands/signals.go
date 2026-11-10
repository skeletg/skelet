package commands

import (
	"fmt"
	"github.com/skeletg/skelet/apps/backend/core/internal/configstore"
	"github.com/skeletg/skelet/apps/backend/core/internal/currency"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
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
	Short: "Replay every placed order and its refunds into Signals - safe to re-run, what it already holds is skipped by its dedupe key",
	Long: "For orders that never reached Signals: placed while the suite was off or unreachable for longer than the outbox retries, " +
		"or guest orders from before guests without a visitor id were recorded - and for the order lines and refunded lines that " +
		"product reports read, on orders from before they were recorded. Each lands at its original time. Edits made before then " +
		"can't be replayed: the lines as they stood before an edit weren't kept. " +
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
	q := db.New(conn)
	tracker := activity.NewTracker(signals, nil, config.NewService(configstore.New(q)), q)
	tracker.SetSales(nil, activity.BaseConverter(config.NewService(configstore.New(q)), currency.NewService(currency.NewRepository(q))))
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
			full, err := repo.GetOrderByID(ctx, o.ID) // the page carries no lines, and the order lines come from them
			if err != nil {
				return fmt.Errorf("order %s: %w", o.Number, err)
			}
			if err := tracker.BackfillOrder(ctx, full, placedAt); err != nil {
				return fmt.Errorf("order %s: %w (processed %d before it; re-run to continue)", o.Number, err, processed)
			}
			memos, err := repo.ListCreditMemos(ctx, o.ID)
			if err != nil {
				return fmt.Errorf("order %s credit memos: %w", o.Number, err)
			}
			for _, m := range memos {
				if err := tracker.BackfillRefund(ctx, full, m); err != nil {
					return fmt.Errorf("order %s credit memo %s: %w", o.Number, m.Number, err)
				}
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
