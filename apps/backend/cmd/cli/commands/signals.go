package commands

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/skeletg/skelet/apps/backend/internal/analytics"
	dbconn "github.com/skeletg/skelet/apps/backend/internal/db"
	signalsdb "github.com/skeletg/skelet/apps/backend/internal/db/signals/sqlc"
	db "github.com/skeletg/skelet/apps/backend/internal/db/sqlc"
)

var SignalsCmd = &cobra.Command{
	Use:   "signals",
	Short: "Signals maintenance",
}

// Trait compute derives RFM from order_placed events now, and the spine only goes back to when tracking started - without this every customer's history is truncated at that cutover
var signalsBackfillOrdersCmd = &cobra.Command{
	Use:   "backfill-orders",
	Short: "Replay historical orders onto the spine, so RFM sees history from before tracking (one-time, off-peak)",
	RunE:  runSignalsBackfillOrders,
}

func init() {
	SignalsCmd.AddCommand(signalsBackfillOrdersCmd)
}

const backfillPage = 1000

func runSignalsBackfillOrders(cmd *cobra.Command, args []string) error {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return fmt.Errorf("DATABASE_URL is not set")
	}
	conn, err := dbconn.Connect(databaseURL)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer conn.Close()
	sigConn, err := signalsConn()
	if err != nil {
		return err
	}
	defer sigConn.Close()

	ctx := context.Background()
	queries := db.New(conn)
	rec := analytics.NewRecorder(queries, signalsdb.New(sigConn))
	spine := rec.SpineStore()

	// Bounded at start time so a long run doesn't chase orders the live tracker is already recording
	before := time.Now()
	var placed, cancelled int
	for off := int32(0); ; off += backfillPage {
		rows, err := queries.OrdersForSpineBackfill(ctx, db.OrdersForSpineBackfillParams{Before: before, Lim: backfillPage, Off: off})
		if err != nil {
			return fmt.Errorf("read orders: %w", err)
		}
		if len(rows) == 0 {
			break
		}
		batch := make([]analytics.SpineEvent, 0, len(rows))
		for _, o := range rows {
			cust, ch := o.CustomerID.UUID, o.ChannelID
			props := map[string]any{"order_id": o.ID.String(), "total_cents": o.TotalAmount, "backfilled": true}
			// The tracker's own dedupe keys, so an order it already recorded collapses instead of double-counting
			batch = append(batch, analytics.SpineEvent{
				EventName: "order_placed", OccurredAt: o.CreatedAt, ChannelID: &ch, CustomerID: &cust,
				Source: analytics.SourceCommerce, Props: props, DedupeKey: "act:order_placed:" + o.ID.String(),
			})
			placed++
			if o.Status == "cancelled" {
				batch = append(batch, analytics.SpineEvent{
					EventName: "order_cancelled", OccurredAt: o.CreatedAt, ChannelID: &ch, CustomerID: &cust,
					Source: analytics.SourceCommerce,
					Props:  map[string]any{"order_id": o.ID.String(), "status": "cancelled", "backfilled": true},
					// The live event keys on the transition, and this stands in for whenever it happened
					DedupeKey: "act:order_status:cancelled:" + o.ID.String(),
				})
				cancelled++
			}
		}
		if err := spine.AppendEvents(ctx, batch); err != nil {
			return fmt.Errorf("append events: %w", err)
		}
		fmt.Printf("signals: backfilled %d orders (%d cancelled)\n", placed, cancelled)
		if len(rows) < backfillPage {
			break
		}
	}
	// Arms the refresh: until a channel has a watermark, trait compute leaves the migration-seeded projection alone
	chans, err := queries.ListChannels(ctx)
	if err != nil {
		return fmt.Errorf("list channels: %w", err)
	}
	for _, ch := range chans {
		if err := queries.SetOrderAggWatermark(ctx, db.SetOrderAggWatermarkParams{ChannelID: ch.ID, RanAt: before}); err != nil {
			return fmt.Errorf("arm channel %s: %w", ch.Slug, err)
		}
	}
	fmt.Printf("signals: done - %d order_placed, %d order_cancelled written, %d channel(s) armed\n", placed, cancelled, len(chans))
	fmt.Println("signals: trait compute now rebuilds customer_order_agg from the spine on its next run")
	return nil
}
