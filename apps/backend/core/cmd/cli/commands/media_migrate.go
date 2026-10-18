package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/skeletg/skelet/apps/backend/core/lib/objstore"
)

// The source is whatever the server's own storage env points at - moving between S3, Garage and a local directory is copy, verify, flip the env, restart; --dest-dir picks a directory as the destination Destination creds come from DEST_AWS_ACCESS_KEY_ID / DEST_AWS_SECRET_ACCESS_KEY, or the --dest-access-key / --dest-secret-key flags which win - with neither set the destination reuses the SOURCE credential chain, right for same-account S3→S3 and wrong for cross-provider Only objects move - db rows store object KEYS so they survive the switch untouched, and the public URLs those keys render to follow the store
var MediaMigrateCmd = &cobra.Command{
	Use:   "media-migrate",
	Short: "Copy all object-storage files to another store (S3, Garage or a local directory)",
	RunE:  runMediaMigrate,
}

var mediaMigrateFlags struct {
	destDir       string
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
	f.StringVar(&mediaMigrateFlags.destDir, "dest-dir", "", "destination directory for the filesystem driver, instead of a bucket")
	f.StringVar(&mediaMigrateFlags.destEndpoint, "dest-endpoint", "", "destination S3 endpoint URL (empty = real AWS S3)")
	f.StringVar(&mediaMigrateFlags.destBucket, "dest-bucket", "", "destination bucket name")
	f.StringVar(&mediaMigrateFlags.destRegion, "dest-region", "", "destination region (default: source AWS_REGION)")
	f.BoolVar(&mediaMigrateFlags.destPathStyle, "dest-path-style", false, "use path-style addressing for the destination (Garage/MinIO-style endpoints)")
	f.StringVar(&mediaMigrateFlags.destAccessKey, "dest-access-key", "", "destination access key (prefer DEST_AWS_ACCESS_KEY_ID env)")
	f.StringVar(&mediaMigrateFlags.destSecretKey, "dest-secret-key", "", "destination secret key (prefer DEST_AWS_SECRET_ACCESS_KEY env)")
	f.StringVar(&mediaMigrateFlags.prefix, "prefix", "", "only copy keys under this prefix")
	f.BoolVar(&mediaMigrateFlags.dryRun, "dry-run", false, "list what would be copied without writing")
	f.BoolVar(&mediaMigrateFlags.overwrite, "overwrite", false, "copy even when the destination already has the key with the same size")
	f.IntVar(&mediaMigrateFlags.workers, "workers", 8, "concurrent copy workers")
	MediaMigrateCmd.MarkFlagsOneRequired("dest-bucket", "dest-dir")
	MediaMigrateCmd.MarkFlagsMutuallyExclusive("dest-bucket", "dest-dir")
}

func runMediaMigrate(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	flags := mediaMigrateFlags

	src, err := objstore.New(ctx, objstore.FromEnv())
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
	dstCfg := objstore.Config{
		Bucket:           flags.destBucket,
		Region:           destRegion,
		InternalEndpoint: flags.destEndpoint,
		UsePathStyle:     flags.destPathStyle,
		AccessKeyID:      destAccessKey,
		SecretAccessKey:  destSecretKey,
	}
	if flags.destDir != "" {
		dstCfg = objstore.Config{Driver: objstore.DriverFilesystem, Dir: flags.destDir}
	}
	dst, err := objstore.New(ctx, dstCfg)
	if err != nil {
		return fmt.Errorf("init destination storage: %w", err)
	}

	mode := "copying"
	if flags.dryRun {
		mode = "dry-run: would copy"
	}
	fmt.Printf("%s %s → %s\n", mode, storeLabel(src, os.Getenv("S3_ENDPOINT_URL")), storeLabel(dst, flags.destEndpoint))

	stats, err := objstore.Migrate(ctx, src, dst, objstore.MigrateOptions{
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
		fmt.Println("done - point the storage env at the destination and restart to switch over")
	}
	return nil
}

func storeLabel(s objstore.Storage, endpoint string) string {
	if _, ok := s.(*objstore.FileStore); ok {
		return fmt.Sprintf("directory %q", s.Bucket())
	}
	if endpoint == "" {
		endpoint = "AWS S3"
	}
	return fmt.Sprintf("%q (%s)", s.Bucket(), endpoint)
}
