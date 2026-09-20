package commands

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/skeletg/skelet/apps/backend/core/internal/signalsclient"
	"github.com/skeletg/skelet/apps/backend/core/lib/backup"
)

// `db backup` targets the BACKUP_S3_* env only on purpose - a runtime-editable destination would be an exfiltration channel; local dev snapshots are a different tool (bin/db-export / bin/db-import)
var DbCmd = &cobra.Command{
	Use:   "db",
	Short: "Database operations (backups to dedicated storage)",
}

var dbBackupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Stream a pg_dump into the configured dedicated object storage",
	RunE:  runDbBackup,
}

var dbBackupsCmd = &cobra.Command{
	Use:   "backups",
	Short: "List backups in the configured bucket (oldest first)",
	RunE:  runDbBackups,
}

var (
	dbRestoreYes  bool
	dbRestoreJobs int
)

var dbRestoreCmd = &cobra.Command{
	Use:   "restore <key>",
	Short: "Stream a backup from the bucket into pg_restore (DESTRUCTIVE - drops and recreates objects)",
	Long: "Restores one dump listed by `db backups` into the matching database: skelet-signals-* keys\n" +
		"are handed to the Signals process over the contract (SIGNALS_GRPC_ADDR), everything else goes\n" +
		"to DATABASE_URL. Runs pg_restore --clean --if-exists, so existing objects are dropped first -\n" +
		"stop the app before restoring, and restore the commerce and signals dumps of the SAME\n" +
		"timestamp together.\n" +
		"The dump spools to a temp file first (set TMPDIR to a volume for very large dumps);\n" +
		"--jobs parallelizes the data/index phases and is the difference between hours and\n" +
		"minutes on a big database.",
	Args: cobra.ExactArgs(1),
	RunE: runDbRestore,
}

func init() {
	DbCmd.AddCommand(dbBackupCmd)
	DbCmd.AddCommand(dbBackupsCmd)
	dbRestoreCmd.Flags().BoolVar(&dbRestoreYes, "yes", false, "confirm overwriting the live database")
	dbRestoreCmd.Flags().IntVar(&dbRestoreJobs, "jobs", 1, "parallel pg_restore workers (data + index phases)")
	DbCmd.AddCommand(dbRestoreCmd)
}

// Env-only: the destination and creds come from BACKUP_* vars, so nothing here opens a database
// The signals database is the suite's to dump and restore, reached over the contract
func newBackupService() *backup.Service {
	return backup.New(backup.FamilyCommerce, os.Getenv("DATABASE_URL"))
}

func newSignalsClient() (*signalsclient.Client, error) {
	return signalsclient.New(os.Getenv("SIGNALS_GRPC_ADDR"), os.Getenv("SIGNALS_SERVICE_SECRET"), time.Minute, nil)
}

func runDbRestore(cmd *cobra.Command, args []string) error {
	if !dbRestoreYes {
		return fmt.Errorf("restore overwrites the live database - rerun with --yes to confirm")
	}
	ctx := context.Background()
	key := args[0]
	if backup.FamilyOf(key) == backup.FamilySignals {
		client, err := newSignalsClient()
		if err != nil {
			return err
		}
		if client == nil {
			return fmt.Errorf("%s is a signals dump, which the Signals process restores - set SIGNALS_GRPC_ADDR and SIGNALS_SERVICE_SECRET", key)
		}
		defer client.Close()
		if err := client.RestoreDatabase(ctx, key, dbRestoreJobs); err != nil {
			return err
		}
		fmt.Printf("restored %s\n", key)
		return nil
	}
	if err := newBackupService().Restore(ctx, key, dbRestoreJobs); err != nil {
		return err
	}
	fmt.Printf("restored %s\n", key)
	return nil
}

func runDbBackup(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	svc := newBackupService()
	at := time.Now()
	res, err := svc.Run(ctx, at)
	if err != nil {
		return err
	}
	fmt.Printf("uploaded %s (%d bytes)\n", res.Key, res.Bytes)
	for _, p := range res.Pruned {
		fmt.Printf("pruned %s\n", p)
	}
	client, err := newSignalsClient()
	if err != nil {
		return err
	}
	if client == nil {
		fmt.Println("no SIGNALS_GRPC_ADDR - the signals database was not dumped; run this where the Signals process is reachable")
		return nil
	}
	defer client.Close()
	sig, err := client.BackupDatabase(ctx, at)
	if err != nil {
		return fmt.Errorf("commerce dump uploaded but the signals backup failed: %w", err)
	}
	fmt.Printf("uploaded %s (%d bytes)\n", sig.Key, sig.Bytes)
	for _, p := range sig.Pruned {
		fmt.Printf("pruned %s\n", p)
	}
	if sig.SpinePrefix != "" {
		fmt.Printf("spine copied to %s\n", sig.SpinePrefix)
		for _, p := range sig.SpinePruned {
			fmt.Printf("pruned %s\n", p)
		}
	}
	svc.Heartbeat(ctx)
	return nil
}

func runDbBackups(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	svc := newBackupService()

	keys, err := svc.List(ctx)
	if err != nil {
		return err
	}
	if len(keys) == 0 {
		fmt.Println("no backups in the bucket")
		return nil
	}
	for _, k := range keys {
		fmt.Println(k)
	}
	return nil
}
