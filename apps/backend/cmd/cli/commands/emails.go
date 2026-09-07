package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	dbconn "github.com/skeletg/skelet/apps/backend/internal/db"
	db "github.com/skeletg/skelet/apps/backend/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/internal/emailtemplate"
	"github.com/skeletg/skelet/apps/backend/internal/flow"
)

var EmailsCmd = &cobra.Command{
	Use:   "emails",
	Short: "Email maintenance",
}

var seedTxChannel string

// Per channel, idempotently: (1) an editable library email per transactional slot from the built-in scaffolds, then (2) an active default Pulse flow per lifecycle event delivering it - nothing is hardcoded, every email + flow stays editable/deletable in the dashboard and existing ones are left untouched
var emailsSeedTransactionalCmd = &cobra.Command{
	Use:   "seed-transactional",
	Short: "Create editable transactional emails + delivery flows per channel (idempotent)",
	RunE:  runEmailsSeedTransactional,
}

var seedBlocksChannel string

// Starter reusable blocks (header, hero, CTA, social, footer) - idempotent by slug, existing slugs are left untouched
var emailsSeedBlocksCmd = &cobra.Command{
	Use:   "seed-blocks",
	Short: "Create starter reusable email blocks per channel (idempotent)",
	RunE:  runEmailsSeedBlocks,
}

func init() {
	emailsSeedTransactionalCmd.Flags().StringVar(&seedTxChannel, "channel", "all", "channel id to seed, or \"all\"")
	EmailsCmd.AddCommand(emailsSeedTransactionalCmd)
	emailsSeedBlocksCmd.Flags().StringVar(&seedBlocksChannel, "channel", "all", "channel id to seed, or \"all\"")
	EmailsCmd.AddCommand(emailsSeedBlocksCmd)
}

func runEmailsSeedBlocks(cmd *cobra.Command, args []string) error {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return fmt.Errorf("DATABASE_URL is not set")
	}
	conn, err := dbconn.Connect(databaseURL)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer conn.Close()

	queries := db.New(conn)
	svc := emailtemplate.NewService(emailtemplate.NewRepository(queries), nil, nil)
	ctx := context.Background()

	channels, err := resolveSeedChannels(ctx, queries, seedBlocksChannel)
	if err != nil {
		return err
	}
	total := 0
	for _, ch := range channels {
		n, err := svc.SeedStandardBlocks(ctx, ch.ID)
		if err != nil {
			return fmt.Errorf("seed blocks for channel %s (%s): %w", ch.Slug, ch.ID, err)
		}
		total += n
		fmt.Printf("  %-8s %s: blocks +%d\n", ch.Slug, ch.ID, n)
	}
	fmt.Printf("Done. Seeded %d block(s) across %d channel(s).\n", total, len(channels))
	return nil
}

func runEmailsSeedTransactional(cmd *cobra.Command, args []string) error {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return fmt.Errorf("DATABASE_URL is not set")
	}
	conn, err := dbconn.Connect(databaseURL)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer conn.Close()

	queries := db.New(conn)
	svc := emailtemplate.NewService(emailtemplate.NewRepository(queries), nil, nil)
	flowSvc := flow.NewService(flow.NewRepository(queries), svc, nil, nil, nil, nil, nil, nil, nil, nil, nil, false, nil) // seeding needs only the repo + template service (slot lookups) - the other deps go unused
	ctx := context.Background()

	channels, err := resolveSeedChannels(ctx, queries, seedTxChannel)
	if err != nil {
		return err
	}

	totalEmails, totalFlows := 0, 0
	for _, ch := range channels {
		n, err := svc.SeedTransactional(ctx, ch.ID)
		if err != nil {
			return fmt.Errorf("seed emails for channel %s (%s): %w", ch.Slug, ch.ID, err)
		}
		fn, err := flowSvc.SeedTransactionalFlows(ctx, ch.ID)
		if err != nil {
			return fmt.Errorf("seed flows for channel %s (%s): %w", ch.Slug, ch.ID, err)
		}
		totalEmails += n
		totalFlows += fn
		fmt.Printf("  %-8s %s: emails +%d (skipped %d), flows +%d\n", ch.Slug, ch.ID, n, len(emailtemplate.TransactionalSlots)-n, fn)
	}
	fmt.Printf("Done. Seeded %d email(s) + %d flow(s) across %d channel(s).\n", totalEmails, totalFlows, len(channels))
	return nil
}

// resolveSeedChannels returns the channel(s) a seed command targets: all
// channels when flag == "all", otherwise the single channel by id.
func resolveSeedChannels(ctx context.Context, queries *db.Queries, flag string) ([]db.Channel, error) {
	if flag == "all" {
		channels, err := queries.ListChannels(ctx)
		if err != nil {
			return nil, fmt.Errorf("list channels: %w", err)
		}
		return channels, nil
	}
	id, err := uuid.Parse(flag)
	if err != nil {
		return nil, fmt.Errorf("invalid --channel: %w", err)
	}
	ch, err := queries.GetChannelByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("channel %s: %w", id, err)
	}
	return []db.Channel{ch}, nil
}
