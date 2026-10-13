package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/skeletg/skelet/apps/backend/core/internal/ai"
	"github.com/skeletg/skelet/apps/backend/core/internal/configstore"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/review"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/dbconn"
	"github.com/skeletg/skelet/apps/backend/core/lib/secrets"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

var ReviewsCmd = &cobra.Command{
	Use:   "reviews",
	Short: "Review maintenance",
}

var reviewsBackfillAll bool

// Same advisory verdict the storefront submission flow produces, one AI call per review - run it after the categories exist or every review sorts into nothing
var reviewsClassifyBackfillCmd = &cobra.Command{
	Use:   "classify-backfill",
	Short: "AI-classify (moderation label + category) reviews not yet classified",
	RunE:  runReviewsClassifyBackfill,
}

func init() {
	reviewsClassifyBackfillCmd.Flags().BoolVar(&reviewsBackfillAll, "all", false, "re-classify every review, including already-classified ones")
	ReviewsCmd.AddCommand(reviewsClassifyBackfillCmd)
}

func runReviewsClassifyBackfill(cmd *cobra.Command, args []string) error {
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
	txMgr := txrunner.New(conn)
	configSvc := config.NewService(configstore.New(queries))
	if rawKey := strings.TrimSpace(os.Getenv("CONFIG_ENCRYPTION_KEY")); rawKey != "" {
		cipher, cipherErr := secrets.NewCipher(rawKey)
		if cipherErr != nil {
			return fmt.Errorf("CONFIG_ENCRYPTION_KEY invalid: %w", cipherErr)
		}
		configSvc.SetCipher(cipher)
	}
	reviewSvc := review.NewService(review.NewRepository(queries, txMgr), txMgr)
	reviewSvc.SetConfigService(configSvc)
	reviewSvc.SetAIService(ai.NewService(configSvc))

	ctx := context.Background()
	const pageSize = int32(100)
	var offset int32
	processed, categorized, skipped := 0, 0, 0
	for {
		// Classifying never removes a row, so offset paging over the unfiltered list stays stable across passes
		rows, _, err := reviewSvc.ListReviewsForModeration(ctx, review.ListReviewsFilter{}, pageSize, offset)
		if err != nil {
			return fmt.Errorf("list reviews: %w", err)
		}
		if len(rows) == 0 {
			break
		}
		for _, rev := range rows {
			if !reviewsBackfillAll && rev.ModerationCheckedAt != nil {
				skipped++
				continue
			}
			updated, err := reviewSvc.ClassifyReview(ctx, rev.ID)
			if err != nil {
				if errors.Is(err, ai.ErrNotConfigured) || errors.Is(err, ai.ErrDisabled) {
					return fmt.Errorf("AI assistant is off - enable it + set a provider/key in Settings → AI (and make sure AI_ENABLED isn't false)")
				}
				fmt.Printf("  review %s: classify failed: %v\n", rev.ID, err)
				continue
			}
			processed++
			if len(updated.CategorySlugs) > 0 {
				categorized++
			}
		}
		offset += pageSize
	}
	fmt.Printf("reviews: classified %d review(s) (%d assigned a category), skipped %d already-classified\n", processed, categorized, skipped)
	return nil
}
