package commands

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/internal/signalsclient"
	"github.com/skeletg/skelet/apps/backend/core/internal/signalsclient/activity"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
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
		"or guest orders from before guests without a visitor id were recorded. Each lands at its original time.",
	Args: cobra.NoArgs,
	RunE: runSignalsBackfillOrders,
}

func init() {
	SignalsCmd.AddCommand(signalsBackfillOrdersCmd)
}

// The tracker loads an order by id for its gift-card follow-up - the repository names the same read GetOrderByID
type orderRepoLookup struct{ order.Repository }

func (r orderRepoLookup) GetByID(ctx context.Context, id uuid.UUID) (order.Order, error) {
	return r.GetOrderByID(ctx, id)
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
	signals, err := signalsclient.New(addr, auth.ServiceKeysFromEnv(os.Getenv), time.Minute, nil)
	if err != nil {
		return fmt.Errorf("signals client: %w", err)
	}
	defer func() { _ = signals.Close() }()

	repo := order.NewRepository(db.New(conn))
	tracker := activity.NewTracker(signals, orderRepoLookup{repo})
	ctx := cmd.Context()
	const page = 500
	sent, skipped := 0, 0
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
			if err := tracker.BackfillOrder(ctx, o); err != nil {
				return fmt.Errorf("order %s: %w (sent %d before it; re-run to continue)", o.Number, err, sent)
			}
			sent++
		}
		if len(orders) < page {
			break
		}
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Replayed %d orders into Signals (%d drafts skipped)\n", sent, skipped)
	return nil
}
