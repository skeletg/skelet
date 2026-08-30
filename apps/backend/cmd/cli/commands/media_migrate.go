package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/skeletg/skelet/internal/media"
	"github.com/spf13/cobra"
)

// The source is whatever the server's own S3_* / AWS_* env points at - moving between the managed (S3) and self-hosted (Garage) profiles is copy, verify, flip the env, restart
// Destination creds come from DEST_AWS_ACCESS_KEY_ID / DEST_AWS_SECRET_ACCESS_KEY, or the --dest-access-key / --dest-secret-key flags which win - with neither set the destination reuses the SOURCE credential chain, right for same-account S3→S3 and wrong for cross-provider
// Only objects move - db rows store object KEYS so they survive the switch untouched, and the public URLs those keys render to follow S3_PUBLIC_URL_BASE
var MediaMigrateCmd = &cobra.Command{
	Use:   "media-migrate",
	Short: "Copy all object-storage files to another S3-compatible store (S3 ↔ Garage)",
	RunE:  runMediaMigrate,
}

var mediaMigrateFlags struct {
	destEndpoint  string
	destBucket    string
	destRegion    string
	destPathStyle bool
	destAccessKey string
	destSecretKey string
	prefix        string
	dryRun        bool
	overwrite     bool
	workers       int
}

func init() {
	f := MediaMigrateCmd.Flags()
	f.StringVar(&mediaMigrateFlags.destEndpoint, "dest-endpoint", "", "destination S3 endpoint URL (empty = real AWS S3)")
	f.StringVar(&mediaMigrateFlags.destBucket, "dest-bucket", "", "destination bucket name (required)")
	f.StringVar(&mediaMigrateFlags.destRegion, "dest-region", "", "destination region (default: source AWS_REGION)")
	f.BoolVar(&mediaMigrateFlags.destPathStyle, "dest-path-style", false, "use path-style addressing for the destination (Garage/MinIO-style endpoints)")
	f.StringVar(&mediaMigrateFlags.destAccessKey, "dest-access-key", "", "destination access key (prefer DEST_AWS_ACCESS_KEY_ID env)")
	f.StringVar(&mediaMigrateFlags.destSecretKey, "dest-secret-key", "", "destination secret key (prefer DEST_AWS_SECRET_ACCESS_KEY env)")
	f.StringVar(&mediaMigrateFlags.prefix, "prefix", "", "only copy keys under this prefix")
	f.BoolVar(&mediaMigrateFlags.dryRun, "dry-run", false, "list what would be copied without writing")
	f.BoolVar(&mediaMigrateFlags.overwrite, "overwrite", false, "copy even when the destination already has the key with the same size")
	f.IntVar(&mediaMigrateFlags.workers, "workers", 8, "concurrent copy workers")
	_ = MediaMigrateCmd.MarkFlagRequired("dest-bucket")
}

func runMediaMigrate(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	flags := mediaMigrateFlags

	srcBucket := os.Getenv("S3_BUCKET_NAME")
	if srcBucket == "" {
		return fmt.Errorf("S3_BUCKET_NAME is not set - the source is the currently configured store")
	}
	src, err := media.New(ctx, media.Config{
		Bucket:           srcBucket,
		Region:           os.Getenv("AWS_REGION"),
		InternalEndpoint: os.Getenv("S3_ENDPOINT_URL"),
		UsePathStyle:     os.Getenv("S3_USE_PATH_STYLE") == "true",
	})
	if err != nil {
		return fmt.Errorf("init source storage: %w", err)
	}

	destAccessKey := flags.destAccessKey
	if destAccessKey == "" {
		destAccessKey = os.Getenv("DEST_AWS_ACCESS_KEY_ID")
	}
	destSecretKey := flags.destSecretKey
	if destSecretKey == "" {
		destSecretKey = os.Getenv("DEST_AWS_SECRET_ACCESS_KEY")
	}
	destRegion := flags.destRegion
	if destRegion == "" {
		destRegion = os.Getenv("AWS_REGION")
	}
	dst, err := media.New(ctx, media.Config{
		Bucket:           flags.destBucket,
		Region:           destRegion,
		InternalEndpoint: flags.destEndpoint,
		UsePathStyle:     flags.destPathStyle,
		AccessKeyID:      destAccessKey,
		SecretAccessKey:  destSecretKey,
	})
	if err != nil {
		return fmt.Errorf("init destination storage: %w", err)
	}

	mode := "copying"
	if flags.dryRun {
		mode = "dry-run: would copy"
	}
	fmt.Printf("%s %q (%s) → %q (%s)\n",
		mode, srcBucket, endpointLabel(os.Getenv("S3_ENDPOINT_URL")),
		flags.destBucket, endpointLabel(flags.destEndpoint))

	stats, err := media.Migrate(ctx, src, dst, media.MigrateOptions{
		Prefix:    flags.prefix,
		DryRun:    flags.dryRun,
		Overwrite: flags.overwrite,
		Workers:   flags.workers,
		Log: func(format string, args ...any) {
			fmt.Printf(format+"\n", args...)
		},
	})
	fmt.Printf("scanned %d · copied %d · skipped %d · %d bytes\n",
		stats.Scanned, stats.Copied, stats.Skipped, stats.Bytes)
	if err != nil {
		return err
	}
	if !flags.dryRun {
		fmt.Println("done - flip the S3_*/AWS_* env vars to the destination and restart to switch over")
	}
	return nil
}

func endpointLabel(endpoint string) string {
	if endpoint == "" {
		return "AWS S3"
	}
	return endpoint
}
