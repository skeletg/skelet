package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/skeletg/skelet/apps/backend/internal/analytics"
	"github.com/skeletg/skelet/apps/backend/internal/analytics/spinech"
	dbconn "github.com/skeletg/skelet/apps/backend/internal/db"
	signalsdb "github.com/skeletg/skelet/apps/backend/internal/db/signals/sqlc"
	"github.com/skeletg/skelet/apps/backend/internal/lens"
	"github.com/skeletg/skelet/apps/backend/internal/media"
)

var LensCmd = &cobra.Command{
	Use:   "lens",
	Short: "Lens web-analytics maintenance",
}

var lensBackfillRollupsCmd = &cobra.Command{ // fills lens_traffic_daily + lens_visitor_seen from the raw spine - re-runnable, view dims re-aggregate and visitor last-seen merges by GREATEST
	Use:   "backfill-rollups",
	Short: "Backfill Lens hub rollups from existing raw web events (one-time, off-peak)",
	RunE:  runLensBackfillRollups,
}

var lensMigrateBlobsTo string

var lensMigrateBlobsCmd = &cobra.Command{ // batched + resumable, so a re-run after an interrupt picks up where it stopped
	Use:   "migrate-blobs",
	Short: "Move Lens replay chunks + DOM snapshots between db and object storage",
	RunE:  runLensMigrateBlobs,
}

var lensBlobStatusCmd = &cobra.Command{
	Use:   "blob-status",
	Short: "Show how many Lens blobs are stored in-db vs object storage",
	RunE:  runLensBlobStatus,
}

func init() {
	LensCmd.AddCommand(lensBackfillRollupsCmd)
	lensMigrateBlobsCmd.Flags().StringVar(&lensMigrateBlobsTo, "to", "", "destination: 'object' or 'db'") // explicit, not read from pulse/lens/blob_storage - that config only picks where NEW blobs land
	LensCmd.AddCommand(lensMigrateBlobsCmd)
	LensCmd.AddCommand(lensBlobStatusCmd)
}

func lensServiceWithBlobs(ctx context.Context) (*lens.Service, func(), error) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return nil, nil, fmt.Errorf("DATABASE_URL is not set")
	}
	conn, err := dbconn.Connect(databaseURL)
	if err != nil {
		return nil, nil, fmt.Errorf("connect to database: %w", err)
	}
	sigConn, err := signalsConn()
	if err != nil {
		conn.Close()
		return nil, nil, err
	}
	storage, err := media.New(ctx, media.Config{
		Bucket:           os.Getenv("S3_BUCKET_NAME"),
		Region:           os.Getenv("AWS_REGION"),
		InternalEndpoint: os.Getenv("S3_ENDPOINT_URL"),
		PresignEndpoint:  os.Getenv("S3_PRESIGN_ENDPOINT_URL"),
		PublicURLBase:    os.Getenv("S3_PUBLIC_URL_BASE"),
		UsePathStyle:     os.Getenv("S3_USE_PATH_STYLE") == "true",
	})
	if err != nil {
		conn.Close()
		sigConn.Close()
		return nil, nil, fmt.Errorf("init object storage: %w", err)
	}
	// Lens tables live on the signals handle - the Recorder still needs its commerce handle for traits/email_sends plus the spine handle for the moved tables
	svc := lens.NewService(signalsdb.New(sigConn), analytics.NewRecorder(signalsdb.New(sigConn)), nil)
	svc.SetBlobStore(storage)
	return svc, func() { conn.Close(); sigConn.Close() }, nil
}

func runLensMigrateBlobs(cmd *cobra.Command, args []string) error {
	if lensMigrateBlobsTo != "object" && lensMigrateBlobsTo != "db" {
		return fmt.Errorf("--to must be 'object' or 'db'")
	}
	ctx := context.Background()
	svc, closeFn, err := lensServiceWithBlobs(ctx)
	if err != nil {
		return err
	}
	defer closeFn()

	fmt.Printf("lens: migrating blobs → %s (batched, resumable)…\n", lensMigrateBlobsTo)
	moved, err := svc.MigrateBlobs(ctx, lensMigrateBlobsTo)
	if err != nil {
		return fmt.Errorf("migrate blobs (moved %d before error): %w", moved, err)
	}
	fmt.Printf("lens: moved %d blob(s) to %s\n", moved, lensMigrateBlobsTo)
	return nil
}

func runLensBlobStatus(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	svc, closeFn, err := lensServiceWithBlobs(ctx)
	if err != nil {
		return err
	}
	defer closeFn()

	st, err := svc.BlobStats(ctx)
	if err != nil {
		return fmt.Errorf("blob status: %w", err)
	}
	fmt.Printf("Lens blob storage:\n")
	fmt.Printf("  replay chunks:  %d in-db, %d in object storage\n", st.ChunksInDB, st.ChunksInObject)
	fmt.Printf("  DOM snapshots:  %d in-db, %d in object storage\n", st.SnapshotsInDB, st.SnapshotsInObject)
	return nil
}

func runLensBackfillRollups(cmd *cobra.Command, args []string) error {
	if spinech.EngineIsClickHouse() {
		return fmt.Errorf("lens backfill-rollups reads raw web events from the Postgres spine, but SIGNALS_ENGINE=clickhouse - the events live in ClickHouse, so this backfill would produce empty rollups")
	}
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

	svc := lens.NewService(signalsdb.New(sigConn), analytics.NewRecorder(signalsdb.New(sigConn)), nil)

	fmt.Println("lens: backfilling hub rollups from raw web events (this can take a while)…")
	if err := svc.BackfillRollups(context.Background()); err != nil {
		return fmt.Errorf("backfill rollups: %w", err)
	}
	fmt.Println("lens: rollups backfilled")
	return nil
}
