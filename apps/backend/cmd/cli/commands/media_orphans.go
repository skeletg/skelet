package commands

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"os"
	"sort"
	"strings"

	dbconn "github.com/skeletg/skelet/apps/backend/internal/db"
	"github.com/skeletg/skelet/apps/backend/internal/media"
	"github.com/spf13/cobra"
)

var MediaOrphansCmd = &cobra.Command{
	Use:   "media-orphans",
	Short: "Find object-storage files with no DB record (and optionally delete them)",
	RunE:  runMediaOrphans,
}

func runMediaOrphans(cmd *cobra.Command, args []string) error {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return fmt.Errorf("DATABASE_URL is not set")
	}
	conn, err := dbconn.Connect(databaseURL)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer conn.Close()

	ctx := context.Background()
	storage, err := media.New(ctx, media.Config{
		Bucket:           os.Getenv("S3_BUCKET_NAME"),
		Region:           os.Getenv("AWS_REGION"),
		InternalEndpoint: os.Getenv("S3_ENDPOINT_URL"),
		PresignEndpoint:  os.Getenv("S3_PRESIGN_ENDPOINT_URL"),
		PublicURLBase:    os.Getenv("S3_PUBLIC_URL_BASE"),
		UsePathStyle:     os.Getenv("S3_USE_PATH_STYLE") == "true",
	})
	if err != nil {
		return fmt.Errorf("init storage: %w", err)
	}

	referenced, err := loadReferencedKeys(ctx, conn)
	if err != nil {
		return fmt.Errorf("load referenced keys: %w", err)
	}

	bucketKeys, err := storage.ListPrefix(ctx, "")
	if err != nil {
		return fmt.Errorf("list bucket: %w", err)
	}

	var orphans []string
	for _, k := range bucketKeys {
		if _, ok := referenced[k]; !ok {
			orphans = append(orphans, k)
		}
	}
	sort.Strings(orphans)

	fmt.Printf("Bucket:     %s\n", storage.Bucket())
	fmt.Printf("In bucket:  %d objects\n", len(bucketKeys))
	fmt.Printf("Referenced: %d keys (across media_assets, catalog_product_media, catalog_collections, catalog_categories)\n", len(referenced))
	fmt.Printf("Orphans:    %d\n\n", len(orphans))

	if len(orphans) == 0 {
		fmt.Println("Nothing to do.")
		return nil
	}

	const previewLimit = 50
	fmt.Println("Orphan keys:")
	for i, k := range orphans {
		if i >= previewLimit {
			fmt.Printf("  ... and %d more\n", len(orphans)-previewLimit)
			break
		}
		fmt.Printf("  - %s\n", k)
	}
	fmt.Println()

	stdin := bufio.NewReader(os.Stdin)
	answer, err := promptLine(stdin, fmt.Sprintf("Delete these %d orphan files? Type 'yes' to confirm: ", len(orphans)))
	if err != nil {
		return err
	}
	if strings.ToLower(answer) != "yes" {
		fmt.Println("Aborted. No files deleted.")
		return nil
	}

	deleted, failed := 0, 0
	for _, k := range orphans {
		if err := storage.Delete(ctx, k); err != nil {
			failed++
			fmt.Fprintf(os.Stderr, "  delete %s: %v\n", k, err)
			continue
		}
		deleted++
	}
	fmt.Printf("\nDone. Deleted %d, failed %d.\n", deleted, failed)
	return nil
}

func loadReferencedKeys(ctx context.Context, conn *sql.DB) (map[string]struct{}, error) {
	out := make(map[string]struct{})
	queries := []string{ // a media-referencing column missing from this list reads as an orphan and gets deleted
		`SELECT object_key FROM media_assets WHERE object_key IS NOT NULL AND object_key <> ''`,
		`SELECT s3_key FROM catalog_product_media WHERE s3_key IS NOT NULL AND s3_key <> ''`,
		`SELECT cover_s3_key FROM catalog_collections WHERE cover_s3_key IS NOT NULL AND cover_s3_key <> ''`,
		`SELECT cover_s3_key FROM catalog_categories WHERE cover_s3_key IS NOT NULL AND cover_s3_key <> ''`,
	}
	// Its own function so Close can be deferred - inside the loop a defer would pile up until the
	// sweep finished, and closing by hand in every branch leaks as soon as an early return appears
	collect := func(q string) error {
		rows, err := conn.QueryContext(ctx, q)
		if err != nil {
			return fmt.Errorf("query %q: %w", q, err)
		}
		defer rows.Close()
		for rows.Next() {
			var k string
			if err := rows.Scan(&k); err != nil {
				return err
			}
			out[k] = struct{}{}
		}
		return rows.Err()
	}
	for _, q := range queries {
		if err := collect(q); err != nil {
			return nil, err
		}
	}
	return out, nil
}
